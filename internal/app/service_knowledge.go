package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/knowledgeops"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/markdownnote"
)

func (s *Service) ExportKnowledgeProjection(_ context.Context, req KnowledgeExportProjectionRequest) (domain.Projection, error) {
	const command = "knowledge.export-projection"
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	outputPath := strings.TrimSpace(req.Output)
	if outputPath == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "knowledge export-projection requires --output", Hint: "pinax knowledge export-projection --output ./projection.json --vault <vault> --json"}
		return domain.NewErrorProjection(command, err), err
	}
	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if pathInside(root, absOutput) {
		err := &domain.CommandError{Code: "unsafe_path", Message: "knowledge projection output must not write into the vault", Hint: "Pass --output outside the vault"}
		return domain.NewErrorProjection(command, err), err
	}

	allowlist, err := resolveKnowledgeAllowlist(req)
	if err != nil {
		return errorProjection(command, err), err
	}

	var prior *knowledgeops.Package
	if strings.TrimSpace(req.FromPackage) != "" {
		loaded, loadErr := loadKnowledgePackage(req.FromPackage)
		if loadErr != nil {
			return errorProjection(command, loadErr), loadErr
		}
		prior = loaded
	}

	candidates, scanStats, err := scanKnowledgeCandidates(root, allowlist)
	if err != nil {
		return errorProjection(command, err), err
	}
	current, omitted := knowledgeops.SelectCurrent(candidates)
	pkg := knowledgeops.AssemblePackage(s.currentTimeUTC().Format(time.RFC3339), allowlist, scanStats.pathAllowlisted, scanStats.readBodies, current, omitted, prior)
	if err := writeJSONAsset(absOutput, pkg); err != nil {
		return errorProjection(command, err), err
	}

	projection := domain.NewProjection(command, knowledgeExportSummary(pkg))
	projection.Facts["schema_version"] = pkg.SchemaVersion
	projection.Facts["output"] = absOutput
	projection.Facts["empty_reason"] = pkg.EmptyReason
	projection.Facts["entries"] = fmt.Sprint(len(pkg.Entries))
	projection.Facts["included"] = fmt.Sprint(pkg.Audit.Included)
	projection.Facts["tombstones"] = fmt.Sprint(pkg.Audit.Tombstones)
	projection.Facts["unchanged"] = fmt.Sprint(pkg.Audit.Unchanged)
	projection.Facts["allowlist_paths"] = fmt.Sprint(pkg.Audit.AllowlistPaths)
	projection.Facts["path_allowlisted"] = fmt.Sprint(pkg.Audit.PathAllowlisted)
	projection.Facts["omitted_missing_marker"] = fmt.Sprint(pkg.Audit.OmittedMissingMarker)
	projection.Facts["read_note_bodies"] = fmt.Sprint(pkg.Audit.ReadNoteBodies)
	projection.Facts["vault_write"] = "false"
	projection.Facts["index_write"] = "false"
	projection.Evidence = []string{absOutput}
	projection.Data = map[string]any{
		"package": pkg,
		"output":  absOutput,
		"audit":   pkg.Audit,
	}
	projection.Actions = []domain.Action{{Name: "inspect", Command: "pinax knowledge export-projection --output " + absOutput + " --vault <vault> --json"}}
	return projection, nil
}

func knowledgeExportSummary(pkg knowledgeops.Package) string {
	if pkg.EmptyReason == knowledgeops.EmptyReasonAllowlistEmpty {
		return "Knowledge projection exported empty because the allowlist is empty."
	}
	if pkg.EmptyReason == knowledgeops.EmptyReasonNoMatch {
		return "Knowledge projection exported empty because no note met both allowlist conditions."
	}
	if pkg.Audit.Tombstones > 0 {
		return "Knowledge projection exported with digest-diff entries and tombstones."
	}
	return "Knowledge projection exported."
}

func resolveKnowledgeAllowlist(req KnowledgeExportProjectionRequest) ([]string, error) {
	paths := append([]string{}, req.Allowlist...)
	file := strings.TrimSpace(req.AllowlistFile)
	if file == "" {
		return knowledgeops.NormalizeAllowlist(paths), nil
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(abs)
	if err != nil {
		return nil, &domain.CommandError{Code: "allowlist_unreadable", Message: "allowlist file could not be read", Hint: "Pass an existing --allowlist-file or omit it"}
	}
	paths = append(paths, knowledgeops.ParseAllowlistFile(string(content))...)
	return knowledgeops.NormalizeAllowlist(paths), nil
}

type knowledgeScanStats struct {
	pathAllowlisted int
	readBodies      int
}

func scanKnowledgeCandidates(root string, allowlist []string) ([]knowledgeops.Candidate, knowledgeScanStats, error) {
	stats := knowledgeScanStats{}
	if len(allowlist) == 0 {
		return nil, stats, nil
	}
	candidates := make([]knowledgeops.Candidate, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if shouldSkipVaultWalkDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !knowledgeops.PathAllowed(rel, allowlist) {
			return nil
		}
		stats.pathAllowlisted++
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		stats.readBodies++
		meta, _, _ := markdownnote.ParseFrontmatter(content)
		if !isPinaxNoteFrontmatter(meta) {
			return nil
		}
		note := parseNote(rel, string(content))
		if isSystemIndexNote(note) {
			return nil
		}
		info, err := entry.Info()
		changedAt := ""
		if err == nil {
			changedAt = info.ModTime().UTC().Format(time.RFC3339)
		}
		if note.UpdatedAt != "" {
			changedAt = note.UpdatedAt
		}
		candidates = append(candidates, knowledgeops.Candidate{
			Path:        rel,
			NoteID:      note.ID,
			Title:       note.Title,
			Frontmatter: note.Frontmatter,
			Digest:      knowledgeContentDigest(content),
			ChangedAt:   changedAt,
		})
		return nil
	})
	if err != nil {
		return nil, stats, err
	}
	return candidates, stats, nil
}

func knowledgeContentDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func loadKnowledgePackage(path string) (*knowledgeops.Package, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		return nil, &domain.CommandError{Code: "prior_package_unreadable", Message: "prior projection package could not be read", Hint: "Pass an existing --from-package path"}
	}
	var pkg knowledgeops.Package
	if err := json.Unmarshal(body, &pkg); err != nil {
		return nil, &domain.CommandError{Code: "prior_package_invalid", Message: "prior projection package is not valid JSON", Hint: "Export a package first, then pass it to --from-package"}
	}
	if pkg.SchemaVersion != "" && pkg.SchemaVersion != knowledgeops.SchemaVersion {
		return nil, &domain.CommandError{Code: "prior_package_schema_unsupported", Message: "prior projection package schema is unsupported", Hint: "Re-export a current package before incremental export"}
	}
	return &pkg, nil
}

func pathInside(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	if candidate == root {
		return true
	}
	return strings.HasPrefix(candidate, root+string(os.PathSeparator))
}
