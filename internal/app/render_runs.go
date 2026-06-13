package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

const renderRunSchemaVersion = "pinax.render_run.v1"

type RenderRunRequest struct {
	VaultPath string
	Template  string
	NoteRef   string
	Keep      int
	DryRun    bool
	Yes       bool
}

type renderRunReceipt struct {
	SchemaVersion string            `json:"schema_version"`
	RunID         string            `json:"run_id"`
	Name          string            `json:"name,omitempty"`
	CreatedAt     string            `json:"created_at"`
	Command       string            `json:"command"`
	Template      string            `json:"template,omitempty"`
	TargetNote    string            `json:"target_note,omitempty"`
	Args          map[string]string `json:"args,omitempty"`
	RenderedHash  string            `json:"rendered_hash"`
	RenderedPath  string            `json:"rendered_markdown"`
	RowCount      string            `json:"row_count,omitempty"`
}

type renderRunIndex struct {
	SchemaVersion string             `json:"schema_version"`
	Latest        string             `json:"latest,omitempty"`
	Aliases       map[string]string  `json:"aliases,omitempty"`
	Runs          []renderRunReceipt `json:"runs,omitempty"`
}

func (s *Service) PruneTemplateRuns(_ context.Context, req RenderRunRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("template.runs.prune", err), err
	}
	templateName, err := cleanTemplateName(req.Template)
	if err != nil {
		return errorProjection("template.runs.prune", err), err
	}
	scope, err := templateRenderRunScope(root, templateName)
	if err != nil {
		return errorProjection("template.runs.prune", err), err
	}
	idx, _ := loadRenderRunIndex(scope)
	runs := append([]renderRunReceipt(nil), idx.Runs...)
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].CreatedAt > runs[j].CreatedAt })
	keep := req.Keep
	if keep < 0 {
		keep = 0
	}
	deleteRuns := []renderRunReceipt{}
	if len(runs) > keep {
		deleteRuns = runs[keep:]
	}
	projection := domain.NewProjection("template.runs.prune", "Render run prune plan generated.")
	projection.Facts["template"] = templateName
	projection.Facts["keep"] = fmt.Sprint(keep)
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun || !req.Yes)
	projection.Facts["delete_candidates"] = fmt.Sprint(len(deleteRuns))
	projection.Data = map[string]any{"delete_candidates": deleteRuns}
	if req.DryRun || !req.Yes {
		return projection, nil
	}
	for _, run := range deleteRuns {
		_ = os.RemoveAll(filepath.Join(scope, run.RunID))
	}
	remaining := runs[:minInt(keep, len(runs))]
	idx.Runs = remaining
	idx.Aliases = aliasesForExistingRuns(idx.Aliases, remaining)
	if len(remaining) > 0 {
		idx.Latest = remaining[0].RunID
	} else {
		idx.Latest = ""
	}
	if err := saveRenderRunIndex(scope, idx); err != nil {
		return errorProjection("template.runs.prune", err), err
	}
	projection.Summary = "Render run pruned."
	projection.Facts["dry_run"] = "false"
	return projection, nil
}

func (s *Service) RepairTemplateRuns(_ context.Context, req RenderRunRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("template.runs.repair", err), err
	}
	base := filepath.Join(root, ".pinax", "renders", "templates")
	repaired := 0
	_ = filepath.WalkDir(base, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || !d.IsDir() || path == base {
			return nil
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return filepath.SkipDir
		}
		runs := []renderRunReceipt{}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			receipt, err := readRenderRunReceipt(filepath.Join(path, entry.Name(), "receipt.json"))
			if err == nil {
				runs = append(runs, receipt)
			}
		}
		if len(runs) > 0 {
			idx := renderRunIndex{SchemaVersion: renderRunSchemaVersion, Aliases: map[string]string{}, Runs: runs}
			sort.SliceStable(idx.Runs, func(i, j int) bool { return idx.Runs[i].CreatedAt > idx.Runs[j].CreatedAt })
			idx.Latest = idx.Runs[0].RunID
			for _, run := range idx.Runs {
				if run.Name != "" {
					idx.Aliases[run.Name] = run.RunID
				}
			}
			if err := saveRenderRunIndex(path, idx); err == nil {
				repaired++
			}
		}
		return filepath.SkipDir
	})
	projection := domain.NewProjection("template.runs.repair", "Render run index repaired.")
	projection.Facts["scopes"] = fmt.Sprint(repaired)
	return projection, nil
}

func saveTemplateRenderRun(root string, req TemplateRequest, body string) (renderRunReceipt, error) {
	templateName, err := cleanTemplateName(req.Name)
	if err != nil {
		return renderRunReceipt{}, err
	}
	scope, err := templateRenderRunScope(root, templateName)
	if err != nil {
		return renderRunReceipt{}, err
	}
	return saveRenderRun(scope, renderRunReceipt{Command: "template.render", Template: templateName, Name: req.SaveRun, Args: templateRunArgs(req)}, body)
}

func saveNoteRenderRun(root string, notePath, name, body string) (renderRunReceipt, error) {
	scope, err := noteRenderRunScope(root, notePath)
	if err != nil {
		return renderRunReceipt{}, err
	}
	return saveRenderRun(scope, renderRunReceipt{Command: "note.refresh", TargetNote: notePath, Name: name}, body)
}

func loadTemplateRunArgs(root, templateName, ref string) (map[string]string, renderRunReceipt, error) {
	scope, err := templateRenderRunScope(root, templateName)
	if err != nil {
		return nil, renderRunReceipt{}, err
	}
	run, err := resolveRenderRun(scope, ref)
	if err != nil {
		return nil, renderRunReceipt{}, err
	}
	return run.Args, run, nil
}

func loadNoteRenderedSnapshot(root, notePath, ref string) (string, renderRunReceipt, error) {
	scope, err := noteRenderRunScope(root, notePath)
	if err != nil {
		return "", renderRunReceipt{}, err
	}
	run, err := resolveRenderRun(scope, ref)
	if err != nil {
		return "", renderRunReceipt{}, err
	}
	b, err := os.ReadFile(filepath.Join(scope, run.RunID, "rendered.md"))
	if err != nil {
		return "", renderRunReceipt{}, err
	}
	if hashString(string(b)) != run.RenderedHash {
		return "", renderRunReceipt{}, &domain.CommandError{Code: "render_snapshot_corrupt", Message: "Rendered snapshot hash does not match", Hint: "Run template runs repair to check the render run index"}
	}
	return string(b), run, nil
}

func listTemplateRenderRuns(root, templateName string) ([]renderRunReceipt, error) {
	scope, err := templateRenderRunScope(root, templateName)
	if err != nil {
		return nil, err
	}
	idx, err := loadRenderRunIndex(scope)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []renderRunReceipt{}, nil
		}
		return nil, err
	}
	return idx.Runs, nil
}

func listNoteRenderRuns(root, notePath string) ([]renderRunReceipt, error) {
	scope, err := noteRenderRunScope(root, notePath)
	if err != nil {
		return nil, err
	}
	idx, err := loadRenderRunIndex(scope)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []renderRunReceipt{}, nil
		}
		return nil, err
	}
	return idx.Runs, nil
}

func saveRenderRun(scope string, receipt renderRunReceipt, body string) (renderRunReceipt, error) {
	if err := os.MkdirAll(scope, 0o755); err != nil {
		return renderRunReceipt{}, err
	}
	now := time.Now().UTC()
	receipt.SchemaVersion = renderRunSchemaVersion
	receipt.CreatedAt = now.Format(time.RFC3339)
	receipt.RunID = newRenderRunID(now, receipt.Name+receipt.Command+body)
	receipt.RenderedPath = "rendered.md"
	receipt.RenderedHash = hashString(body)
	receipt.Args = redactRunArgs(receipt.Args)
	dir := filepath.Join(scope, receipt.RunID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return renderRunReceipt{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "rendered.md"), []byte(body), 0o644); err != nil {
		return renderRunReceipt{}, err
	}
	b, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return renderRunReceipt{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "receipt.json"), append(b, '\n'), 0o644); err != nil {
		return renderRunReceipt{}, err
	}
	idx, _ := loadRenderRunIndex(scope)
	if idx.SchemaVersion == "" {
		idx.SchemaVersion = renderRunSchemaVersion
	}
	if idx.Aliases == nil {
		idx.Aliases = map[string]string{}
	}
	idx.Latest = receipt.RunID
	if receipt.Name != "" {
		idx.Aliases[receipt.Name] = receipt.RunID
	}
	idx.Runs = append([]renderRunReceipt{receipt}, idx.Runs...)
	if err := saveRenderRunIndex(scope, idx); err != nil {
		return renderRunReceipt{}, err
	}
	return receipt, nil
}

func resolveRenderRun(scope, ref string) (renderRunReceipt, error) {
	idx, err := loadRenderRunIndex(scope)
	if err != nil {
		return renderRunReceipt{}, err
	}
	needle := strings.TrimSpace(ref)
	if needle == "" || needle == "latest" {
		needle = idx.Latest
	} else if idx.Aliases != nil {
		if runID := idx.Aliases[needle]; runID != "" {
			needle = runID
		}
	}
	for _, run := range idx.Runs {
		if run.RunID == needle || run.Name == needle {
			return run, nil
		}
	}
	return renderRunReceipt{}, &domain.CommandError{Code: "render_run_not_found", Message: "Render run not found", Hint: "Use template inspect --runs or note show --runs to view available runs"}
}

func templateRenderRunScope(root, templateName string) (string, error) {
	name, err := cleanTemplateName(templateName)
	if err != nil {
		return "", err
	}
	return safeJoin(root, filepath.ToSlash(filepath.Join(".pinax", "renders", "templates", name)))
}

func noteRenderRunScope(root, notePath string) (string, error) {
	mirror := strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(notePath), "notes/"), filepath.Ext(notePath))
	if mirror == "" || strings.HasPrefix(mirror, "../") || strings.Contains(mirror, "/../") {
		return "", &domain.CommandError{Code: "render_scope_invalid", Message: "Render run note scope is invalid", Hint: "Use a note path inside the vault"}
	}
	return safeJoin(root, filepath.ToSlash(filepath.Join(".pinax", "renders", mirror)))
}

func loadRenderRunIndex(scope string) (renderRunIndex, error) {
	b, err := os.ReadFile(filepath.Join(scope, "index.json"))
	if err != nil {
		return renderRunIndex{}, err
	}
	var idx renderRunIndex
	if err := json.Unmarshal(b, &idx); err != nil {
		return renderRunIndex{}, err
	}
	return idx, nil
}

func saveRenderRunIndex(scope string, idx renderRunIndex) error {
	if err := os.MkdirAll(scope, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(scope, "index.json"), append(b, '\n'), 0o644)
}

func readRenderRunReceipt(path string) (renderRunReceipt, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return renderRunReceipt{}, err
	}
	var receipt renderRunReceipt
	if err := json.Unmarshal(b, &receipt); err != nil {
		return renderRunReceipt{}, err
	}
	return receipt, nil
}

func templateRunArgs(req TemplateRequest) map[string]string {
	args := map[string]string{}
	if req.Title != "" {
		args["title"] = req.Title
	}
	if req.Project != "" {
		args["project"] = req.Project
	}
	if len(req.Tags) > 0 {
		args["tags"] = strings.Join(req.Tags, ",")
	}
	for key, value := range req.Vars {
		args["var."+key] = value
	}
	return args
}

func applyTemplateRunArgs(req TemplateRequest, args map[string]string) TemplateRequest {
	if req.Title == "" {
		req.Title = args["title"]
	}
	if req.Project == "" {
		req.Project = args["project"]
	}
	if len(req.Tags) == 0 && args["tags"] != "" {
		req.Tags = strings.Split(args["tags"], ",")
	}
	if req.Vars == nil {
		req.Vars = map[string]string{}
	}
	for key, value := range args {
		if strings.HasPrefix(key, "var.") {
			name := strings.TrimPrefix(key, "var.")
			if req.Vars[name] == "" {
				req.Vars[name] = value
			}
		}
	}
	return req
}

func redactRunArgs(args map[string]string) map[string]string {
	if len(args) == 0 {
		return nil
	}
	redacted := make(map[string]string, len(args))
	for key, value := range args {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") {
			redacted[key] = "[REDACTED]"
			continue
		}
		redacted[key] = value
	}
	return redacted
}

func aliasesForExistingRuns(aliases map[string]string, runs []renderRunReceipt) map[string]string {
	allowed := map[string]bool{}
	for _, run := range runs {
		allowed[run.RunID] = true
	}
	out := map[string]string{}
	for name, runID := range aliases {
		if allowed[runID] {
			out[name] = runID
		}
	}
	return out
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func newRenderRunID(now time.Time, seed string) string {
	sum := sha256.Sum256([]byte(now.Format(time.RFC3339Nano) + seed))
	return "run_" + now.Format("20060102T150405Z") + "_" + hex.EncodeToString(sum[:])[:8]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
