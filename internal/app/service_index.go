package app

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
	pinaxversion "github.com/yeisme/pinax/internal/version"
)

// Index operations: sync/lookup/refresh/repair/rebuild/init/doctor/summary/explain/status
// over the local SQLite/GORM note index, plus index summary/action helpers.
// Extracted from service.go to isolate the index-maintenance surface.

func (s *Service) SyncIndex(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.sync", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("index.sync", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("index.sync", err), err
	}
	result, err := noteindex.Sync(root, notes)
	if err != nil {
		return errorProjection("index.sync", err), err
	}
	_ = appendEvent(root, "index.sync", "success", map[string]string{"created": fmt.Sprint(result.Created), "changed": fmt.Sprint(result.Changed), "moved": fmt.Sprint(result.Moved), "deleted": fmt.Sprint(result.Deleted)})
	projection := domain.NewProjection("index.sync", "Local index synced.")
	projection.Facts["created"] = fmt.Sprint(result.Created)
	projection.Facts["changed"] = fmt.Sprint(result.Changed)
	projection.Facts["moved"] = fmt.Sprint(result.Moved)
	projection.Facts["deleted"] = fmt.Sprint(result.Deleted)
	projection.Facts["restored"] = "0"
	projection.Facts["skipped"] = fmt.Sprint(result.Skipped)
	projection.Facts["candidates"] = "0"
	projection.Facts["failed"] = fmt.Sprint(result.Failed)
	projection.Facts["index_status"] = "fresh"
	projection.Data = result
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	return projection, nil
}

func (s *Service) IndexLookup(_ context.Context, req IndexLookupRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.lookup", err), err
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "index lookup requires a query", Hint: "pinax index lookup <query> --vault <vault>"}
		return domain.NewErrorProjection("index.lookup", err), err
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = "registered"
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "all"
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("index.lookup", err), err
	}
	status, _ := noteindex.Inspect(root, notes)
	candidates := []VaultObjectCandidate{}
	if scopeAllows(scope, "registered") && kindAllows(kind, "note") {
		for _, note := range notes {
			if fields, score := noteCandidateMatch(note, query); score > 0 {
				candidates = append(candidates, VaultObjectCandidate{ObjectKind: "note", Path: note.Path, Title: note.Title, NoteID: note.ID, ManagedStatus: "registered", MatchFields: fields, Score: score, IndexStatus: status.Status})
			}
		}
	}
	if scopeAllows(scope, "adoptable") && kindAllows(kind, "file") {
		files, err := adoptableMarkdownCandidates(root, notes, query, status.Status)
		if err != nil {
			return errorProjection("index.lookup", err), err
		}
		candidates = append(candidates, files...)
	}
	if scopeAllows(scope, "assets") && kindAllows(kind, "asset") {
		manifest, err := pinaxassets.Load(root)
		if err != nil {
			return errorProjection("index.lookup", err), err
		}
		for _, asset := range manifest.Assets {
			if fields, score := assetCandidateMatch(asset, query); score > 0 {
				candidates = append(candidates, VaultObjectCandidate{ObjectKind: "asset", Path: asset.Path, AssetID: asset.ID, ManagedStatus: asset.ManagedStatus, MatchFields: fields, Score: score, MediaType: asset.MediaType, IndexStatus: status.Status})
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].Path < candidates[j].Path
	})
	projection := domain.NewProjection("index.lookup", "Vault object lookup completed.")
	if len(candidates) > 1 {
		projection.Status = "partial"
	}
	projection.Facts["query"] = query
	projection.Facts["scope"] = scope
	projection.Facts["kind"] = kind
	projection.Facts["candidates"] = fmt.Sprint(len(candidates))
	projection.Facts["index_status"] = status.Status
	for i, candidate := range candidates {
		prefix := fmt.Sprintf("candidate.%d.", i+1)
		projection.Facts[prefix+"object_kind"] = candidate.ObjectKind
		projection.Facts[prefix+"path"] = candidate.Path
		projection.Facts[prefix+"managed_status"] = candidate.ManagedStatus
	}
	projection.Actions = []domain.Action{{Name: "refresh", Command: fmt.Sprintf("pinax index refresh --vault %s --json", shellQuote(root))}}
	projection.Evidence = []string{status.Path}
	projection.Data = map[string]any{"candidates": candidates}
	return projection, nil
}

func scopeAllows(scope, target string) bool {
	switch scope {
	case "all":
		return true
	case "registered_or_adoptable":
		return target == "registered" || target == "adoptable"
	default:
		return scope == target
	}
}

func kindAllows(kind, target string) bool {
	return kind == "" || kind == "all" || kind == target
}

func noteCandidateMatch(note domain.Note, query string) ([]string, int) {
	q := strings.ToLower(query)
	checks := []struct {
		field    string
		value    string
		exact    int
		contains int
	}{{"note_id", note.ID, 100, 60}, {"path", note.Path, 95, 55}, {"filename", filepath.Base(note.Path), 90, 50}, {"stem", strings.TrimSuffix(filepath.Base(note.Path), filepath.Ext(note.Path)), 90, 50}, {"title", note.Title, 85, 45}, {"journal_alias", journalNoteShellFriendlyAlias(note), 85, 45}}
	return matchFields(q, checks)
}

func assetCandidateMatch(asset pinaxassets.Asset, query string) ([]string, int) {
	q := strings.ToLower(query)
	checks := []struct {
		field    string
		value    string
		exact    int
		contains int
	}{{"asset_id", asset.ID, 100, 60}, {"path", asset.Path, 95, 55}, {"filename", asset.Filename, 90, 50}, {"stem", asset.Stem, 90, 50}}
	return matchFields(q, checks)
}

func matchFields(q string, checks []struct {
	field    string
	value    string
	exact    int
	contains int
}) ([]string, int) {
	type fieldScore struct {
		field string
		score int
		order int
	}
	matchedFields := map[string]fieldScore{}
	score := 0
	for order, check := range checks {
		value := strings.ToLower(strings.TrimSpace(check.value))
		if value == "" {
			continue
		}
		matched := 0
		if value == q {
			matched = check.exact
		} else if strings.Contains(value, q) {
			matched = check.contains
		}
		if matched == 0 {
			continue
		}
		current, ok := matchedFields[check.field]
		if !ok || matched > current.score {
			matchedFields[check.field] = fieldScore{field: check.field, score: matched, order: order}
		}
		if matched > score {
			score = matched
		}
	}
	fields := make([]fieldScore, 0, len(matchedFields))
	for _, field := range matchedFields {
		fields = append(fields, field)
	}
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].score != fields[j].score {
			return fields[i].score > fields[j].score
		}
		return fields[i].order < fields[j].order
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		out = append(out, field.field)
	}
	return out, score
}

func adoptableMarkdownCandidates(root string, notes []domain.Note, query, indexStatus string) ([]VaultObjectCandidate, error) {
	registered := map[string]bool{}
	for _, note := range notes {
		registered[note.Path] = true
	}
	candidates := []VaultObjectCandidate{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel == ".git" || strings.HasPrefix(rel, ".pinax") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(rel) != ".md" || registered[rel] {
			return nil
		}
		fields, score := fileCandidateMatch(rel, query)
		if score == 0 {
			return nil
		}
		candidates = append(candidates, VaultObjectCandidate{ObjectKind: "file", Path: rel, ManagedStatus: "adoptable", MatchFields: fields, Score: score, IndexStatus: indexStatus})
		return nil
	})
	return candidates, err
}

func fileCandidateMatch(path, query string) ([]string, int) {
	q := strings.ToLower(query)
	checks := []struct {
		field    string
		value    string
		exact    int
		contains int
	}{{"path", path, 95, 55}, {"filename", filepath.Base(path), 90, 50}, {"stem", strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), 90, 50}}
	return matchFields(q, checks)
}
func (s *Service) IndexRefresh(ctx context.Context, req IndexRefreshRequest) (projection domain.Projection, err error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.refresh", err), err
	}
	rec := startMonitorRun(root, "index.refresh", map[string]string{"changed_since": strings.TrimSpace(req.ChangedSince)})
	defer func() {
		runID, evidence := rec.Finish(projection.Status, err)
		addMonitorProjectionFacts(&projection, runID, evidence)
	}()
	endStep := rec.BeginStep("vault_assets.ensure", nil)
	if err := ensureVaultAssets(root); err != nil {
		endStep(err)
		return errorProjection("index.refresh", err), err
	}
	endStep(nil)
	endStep = rec.BeginStep("notes.scan", nil)
	validNotes, failedPaths, err := scanIndexRefreshNotes(root)
	if err != nil {
		endStep(err)
		return errorProjection("index.refresh", err), err
	}
	endStep(nil)
	scanned := len(validNotes) + len(failedPaths)
	changedSince := strings.TrimSpace(req.ChangedSince)
	changedCandidates := []pinaxversion.ChangedPath{}
	var result noteindex.RefreshResult
	if changedSince != "" {
		endStep = rec.BeginStep("version.changed_since", nil)
		changedCandidates, err = s.versionBackend.ChangedSince(ctx, pinaxversion.ChangedSinceRequest{Root: root, SinceRevision: changedSince})
		if err != nil {
			endStep(err)
			return errorProjection("index.refresh", err), err
		}
		endStep(nil)
		endStep = rec.BeginStep("index.refresh_changed", map[string]string{"changed_candidates": fmt.Sprint(len(changedCandidates))})
		result, err = noteindex.RefreshChanged(root, validNotes, changedCandidates, noteindex.RefreshOptions{})
	} else {
		endStep = rec.BeginStep("index.refresh", map[string]string{"notes": fmt.Sprint(len(validNotes))})
		result, err = noteindex.Refresh(root, validNotes, noteindex.RefreshOptions{})
		result.Scanned = scanned
	}
	if err != nil {
		endStep(err)
		return errorProjection("index.refresh", err), err
	}
	endStep(nil)
	result.Failed += len(failedPaths)
	result.FailedPaths = append(result.FailedPaths, failedPaths...)
	if result.Failed > 0 {
		result.IndexStatus = "partial"
	}
	status := "success"
	if result.IndexStatus == "partial" {
		status = "partial"
	}
	_ = appendEvent(root, "index.refresh", status, map[string]string{"scanned": fmt.Sprint(result.Scanned), "indexed": fmt.Sprint(result.Indexed), "failed": fmt.Sprint(result.Failed)})
	projection = domain.NewProjection("index.refresh", "Local index refresh completed.")
	projection.Status = status
	projection.Facts["scanned"] = fmt.Sprint(result.Scanned)
	projection.Facts["changed"] = fmt.Sprint(result.Changed)
	projection.Facts["skipped"] = fmt.Sprint(result.Skipped)
	projection.Facts["indexed"] = fmt.Sprint(result.Indexed)
	projection.Facts["created"] = fmt.Sprint(result.Created)
	projection.Facts["moved"] = fmt.Sprint(result.Moved)
	projection.Facts["deleted"] = fmt.Sprint(result.Deleted)
	projection.Facts["failed"] = fmt.Sprint(result.Failed)
	if changedSince != "" {
		projection.Facts["changed_since"] = changedSince
		projection.Facts["changed_candidates"] = fmt.Sprint(len(changedCandidates))
	}
	if len(result.FailedPaths) > 0 {
		projection.Facts["failed_paths"] = strings.Join(result.FailedPaths, ",")
	}
	projection.Facts["batches"] = fmt.Sprint(result.Batches)
	projection.Facts["duration_ms"] = fmt.Sprint(result.DurationMillis)
	projection.Facts["index_status"] = result.IndexStatus
	projection.Facts["schema_version"] = noteindex.SchemaVersion
	projection.Facts["path"] = filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))
	projection.Evidence = append([]string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}, result.FailedPaths...)
	if status == "partial" {
		projection.Actions = []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax index doctor --vault %s", shellQuote(root))}, {Name: "rebuild", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))}}
	}
	projection.Data = result
	return projection, nil
}

func refreshableIndexNotes(notes []domain.Note) ([]domain.Note, []string) {
	valid := make([]domain.Note, 0, len(notes))
	failedPaths := make([]string, 0)
	for _, note := range notes {
		if strings.TrimSpace(note.Path) == "" || strings.TrimSpace(note.ID) == "" {
			failedPaths = append(failedPaths, note.Path)
			continue
		}
		valid = append(valid, note)
	}
	return valid, failedPaths
}

func (s *Service) IndexRepair(_ context.Context, req IndexRepairRequest) (projection domain.Projection, err error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.repair", err), err
	}
	rec := startMonitorRun(root, "index.repair", map[string]string{"kind": strings.TrimSpace(req.Kind), "dry_run": fmt.Sprint(req.DryRun)})
	defer func() {
		runID, evidence := rec.Finish(projection.Status, err)
		addMonitorProjectionFacts(&projection, runID, evidence)
	}()
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "recreate"
	}
	if kind != "recreate" {
		err := &domain.CommandError{Code: "index_repair_kind_invalid", Message: "index repair kind is unsupported", Hint: "Use --kind recreate"}
		return domain.NewErrorProjection("index.repair", err), err
	}
	indexRel := filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))
	operation := map[string]string{"kind": kind, "mode": repairMode(req), "risk": "low", "path": indexRel, "reason": "recreate local projection"}
	if req.DryRun {
		projection = domain.NewProjection("index.repair", "Index repair plan generated.")
		projection.Facts["dry_run"] = "true"
		projection.Facts["writes"] = "false"
		projection.Facts["operations"] = "1"
		projection.Facts["kind"] = kind
		projection.Facts["risk.low"] = "1"
		projection.Evidence = []string{indexRel}
		projection.Data = map[string]any{"operations": []map[string]string{operation}}
		return projection, nil
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "approval_required", Message: "index repair requires --yes or --dry-run", Hint: fmt.Sprintf("Run pinax index repair --vault %s --kind recreate --dry-run first, then add --yes after confirming", shellQuote(root))}
		return domain.NewErrorProjection("index.repair", err), err
	}
	endStep := rec.BeginStep("index.backup", nil)
	backupRel, err := backupIndexProjection(root)
	if err != nil {
		endStep(err)
		return errorProjection("index.repair", err), err
	}
	endStep(nil)
	endStep = rec.BeginStep("notes.scan", nil)
	notes, err := scanNotes(root)
	if err != nil {
		endStep(err)
		return errorProjection("index.repair", err), err
	}
	notes = ordinaryNotes(notes)
	endStep(nil)
	validNotes, _ := refreshableIndexNotes(notes)
	endStep = rec.BeginStep("index.rebuild", map[string]string{"notes": fmt.Sprint(len(validNotes))})
	counts, err := noteindex.Rebuild(root, validNotes)
	if err != nil {
		endStep(err)
		return errorProjection("index.repair", err), err
	}
	endStep(nil)
	_ = appendEvent(root, "index.repair", "success", map[string]string{"kind": kind, "writes": "true"})
	projection = domain.NewProjection("index.repair", "Index projection repaired.")
	projection.Facts["dry_run"] = "false"
	projection.Facts["writes"] = "true"
	projection.Facts["operations"] = "1"
	projection.Facts["kind"] = kind
	projection.Facts["risk.low"] = "1"
	projection.Facts["index_status"] = "fresh"
	projection.Facts["notes"] = fmt.Sprint(counts.Notes)
	projection.Facts["path"] = indexRel
	projection.Evidence = []string{indexRel, backupRel}
	projection.Data = map[string]any{"operations": []map[string]string{operation}, "backup_path": backupRel, "counts": counts}
	return projection, nil
}

func repairMode(req IndexRepairRequest) string {
	if req.DryRun || !req.Yes {
		return "preview"
	}
	return "apply"
}

func backupIndexProjection(root string) (string, error) {
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if _, err := os.Stat(indexPath); err != nil {
		if os.IsNotExist(err) {
			return filepath.ToSlash(filepath.Join(".pinax", "index.sqlite")), nil
		}
		return "", err
	}
	backupDir := filepath.Join(root, ".pinax", "index-backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	backupRel := filepath.ToSlash(filepath.Join(".pinax", "index-backups", "index-"+time.Now().UTC().Format("20060102T150405.000000000")+".sqlite"))
	backupPath := filepath.Join(root, filepath.FromSlash(backupRel))
	if err := os.Rename(indexPath, backupPath); err != nil {
		return "", err
	}
	return backupRel, nil
}

func (s *Service) RebuildIndex(_ context.Context, req VaultRequest) (projection domain.Projection, err error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.rebuild", err), err
	}
	rec := startMonitorRun(root, "index.rebuild", nil)
	defer func() {
		runID, evidence := rec.Finish(projection.Status, err)
		addMonitorProjectionFacts(&projection, runID, evidence)
	}()
	endStep := rec.BeginStep("vault_assets.ensure", nil)
	if err := ensureVaultAssets(root); err != nil {
		endStep(err)
		return errorProjection("index.rebuild", err), err
	}
	endStep(nil)
	endStep = rec.BeginStep("notes.scan", nil)
	notes, err := scanNotes(root)
	if err != nil {
		endStep(err)
		return errorProjection("index.rebuild", err), err
	}
	notes = ordinaryNotes(notes)
	endStep(nil)
	endStep = rec.BeginStep("index.rebuild", map[string]string{"notes": fmt.Sprint(len(notes))})
	counts, err := noteindex.Rebuild(root, notes)
	if err != nil {
		endStep(err)
		return errorProjection("index.rebuild", err), err
	}
	endStep(nil)
	_ = appendEvent(root, "index.rebuild", "success", map[string]string{"notes": fmt.Sprint(counts.Notes)})
	projection = domain.NewProjection("index.rebuild", "Local index rebuilt.")
	projection.Facts["notes"] = fmt.Sprint(counts.Notes)
	projection.Facts["tags"] = fmt.Sprint(counts.Tags)
	projection.Facts["links"] = fmt.Sprint(counts.Links)
	projection.Facts["tokens"] = fmt.Sprint(counts.Tokens)
	projection.Facts["attachments"] = fmt.Sprint(counts.Attachments)
	projection.Facts["dimensions"] = fmt.Sprint(counts.Dimensions)
	projection.Facts["folders"] = fmt.Sprint(counts.Folders)
	projection.Facts["schema_version"] = noteindex.SchemaVersion
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"counts": counts}
	return projection, nil
}

func (s *Service) InitIndex(_ context.Context, req VaultRequest) (projection domain.Projection, err error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.init", err), err
	}
	rec := startMonitorRun(root, "index.init", nil)
	defer func() {
		runID, evidence := rec.Finish(projection.Status, err)
		addMonitorProjectionFacts(&projection, runID, evidence)
	}()
	endStep := rec.BeginStep("vault_assets.ensure", nil)
	if err := ensureVaultAssets(root); err != nil {
		endStep(err)
		return errorProjection("index.init", err), err
	}
	endStep(nil)
	endStep = rec.BeginStep("index.init", nil)
	status, err := noteindex.Init(root)
	if err != nil {
		endStep(err)
		return errorProjection("index.init", err), err
	}
	endStep(nil)
	projection = domain.NewProjection("index.init", "Local index database initialized.")
	projection.Facts["path"] = status.Path
	projection.Facts["index_status"] = status.Status
	projection.Facts["schema_version"] = status.SchemaVersion
	projection.Evidence = []string{status.Path}
	projection.Data = status
	return projection, nil
}

func (s *Service) IndexDoctor(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.doctor", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("index.doctor", err), err
	}
	notes = ordinaryNotes(notes)
	status, err := noteindex.Inspect(root, notes)
	if err != nil {
		return errorProjection("index.doctor", err), err
	}
	issues := indexDoctorIssues(root, status)
	report := domain.VaultDoctorReport{VaultPath: root, Issues: issues, Counts: countIssuesBySeverity(issues), Stats: domain.VaultStats{VaultPath: root, NoteCount: len(notes), IndexStatus: status.Status, IndexPath: status.Path}}
	projection := domain.NewProjection("index.doctor", "Local index diagnostics completed.")
	projection.Facts["path"] = status.Path
	projection.Facts["index_status"] = status.Status
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["issues.total"] = fmt.Sprint(len(issues))
	if status.SchemaVersion != "" {
		projection.Facts["schema_version"] = status.SchemaVersion
	} else {
		projection.Facts["schema_version"] = noteindex.SchemaVersion
	}
	for severity, count := range report.Counts {
		projection.Facts["issues."+severity] = fmt.Sprint(count)
	}
	if len(issues) > 0 {
		projection.Status = "partial"
		projection.Facts["issue_codes"] = indexIssueCodes(issues)
		projection.Actions = nextActionsFromIssues(issues)
	}
	projection.Evidence = append([]string{status.Path}, status.Evidence...)
	projection.Data = report
	return projection, nil
}

func indexDoctorIssues(root string, status noteindex.Status) []domain.VaultIssue {
	switch status.Status {
	case "fresh":
		return nil
	case "missing":
		return []domain.VaultIssue{{Code: "index_missing", Severity: "warning", Path: status.Path, Message: "Local index missing", Evidence: append([]string{"index_status=missing"}, status.Evidence...), NextActions: []domain.Action{{Name: "refresh", Command: fmt.Sprintf("pinax index refresh --vault %s", shellQuote(root))}}}}
	case "stale":
		return []domain.VaultIssue{{Code: "index_stale", Severity: "warning", Path: status.Path, Message: "Local index stale", Evidence: append([]string{"index_status=stale"}, status.Evidence...), NextActions: []domain.Action{{Name: "refresh", Command: fmt.Sprintf("pinax index refresh --vault %s", shellQuote(root))}}}}
	case "unreadable":
		return []domain.VaultIssue{{Code: "index_unreadable", Severity: "error", Path: status.Path, Message: "Local index unreadable", Evidence: append([]string{"index_status=unreadable"}, status.Evidence...), NextActions: []domain.Action{{Name: "repair", Command: fmt.Sprintf("pinax index repair --vault %s --kind recreate --dry-run", shellQuote(root))}, {Name: "rebuild", Command: fmt.Sprintf("pinax index rebuild --vault %s", shellQuote(root))}}}}
	default:
		return []domain.VaultIssue{{Code: "index_" + status.Status, Severity: "warning", Path: status.Path, Message: "Local index status needs review", Evidence: append([]string{"index_status=" + status.Status}, status.Evidence...), NextActions: []domain.Action{{Name: "doctor", Command: fmt.Sprintf("pinax index doctor --vault %s", shellQuote(root))}}}}
	}
}

func indexIssueCodes(issues []domain.VaultIssue) string {
	codes := make([]string, 0, len(issues))
	seen := map[string]bool{}
	for _, issue := range issues {
		if issue.Code == "" || seen[issue.Code] {
			continue
		}
		seen[issue.Code] = true
		codes = append(codes, issue.Code)
	}
	sort.Strings(codes)
	return strings.Join(codes, ",")
}

func (s *Service) IndexSummary(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.summary", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("index.summary", err), err
	}
	status, err := noteindex.Inspect(root, notes)
	if err != nil {
		return errorProjection("index.summary", err), err
	}
	action := recommendedIndexAction(root, status.Status)
	projection := domain.NewProjection("index.summary", indexSummaryText(status.Status))
	if status.Status != "fresh" {
		projection.Status = "partial"
	}
	if action.Command != "" {
		projection.Actions = []domain.Action{action}
		projection.Facts["recommended_action"] = action.Command
	}
	projection.Facts["path"] = status.Path
	projection.Facts["index_status"] = status.Status
	schemaVersion := status.SchemaVersion
	if schemaVersion == "" {
		schemaVersion = noteindex.SchemaVersion
	}
	projection.Facts["schema_version"] = schemaVersion
	projection.Facts["notes"] = fmt.Sprint(len(notes))
	projection.Facts["writes"] = "false"
	projection.Facts["affected_workflows"] = "search,query,note_list,organize"
	projection.Evidence = append([]string{status.Path}, status.Evidence...)
	projection.Data = status
	return projection, nil
}

func (s *Service) IndexExplain(ctx context.Context, req VaultRequest) (domain.Projection, error) {
	projection, err := s.IndexSummary(ctx, req)
	if err != nil {
		return errorProjection("index.explain", err), err
	}
	projection.Command = "index.explain"
	projection.Summary = "Local index explanation generated."
	projection.Facts["explains"] = "index projection status"
	return projection, nil
}

func (s *Service) IndexStatus(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("index.status", err), err
	}
	notes, err := scanNotes(root)
	if err != nil {
		return errorProjection("index.status", err), err
	}
	notes = ordinaryNotes(notes)
	status, err := noteindex.Inspect(root, notes)
	if err != nil {
		return errorProjection("index.status", err), err
	}
	projection := domain.NewProjection("index.status", "Local index status checked.")
	if status.Status != "fresh" {
		projection.Status = "partial"
		action := recommendedIndexAction(root, status.Status)
		if action.Command != "" {
			projection.Actions = []domain.Action{action}
		}
	}
	projection.Facts["path"] = status.Path
	projection.Facts["index_status"] = status.Status
	if status.SchemaVersion != "" {
		projection.Facts["schema_version"] = status.SchemaVersion
	}
	if status.Notes > 0 {
		projection.Facts["notes"] = fmt.Sprint(status.Notes)
	}
	projection.Evidence = append([]string{status.Path}, status.Evidence...)
	projection.Data = status
	return projection, nil
}

func indexSummaryText(status string) string {
	switch status {
	case "fresh":
		return "Local index is available. Recommended next step: continue searching or querying."
	case "missing", "stale":
		return "Local index needs maintenance. Recommended next step: run low-cost refresh."
	case "unreadable":
		return "Local index cannot be read. Recommended next step: run doctor or repair dry-run first."
	default:
		return "Local index status summarized. See recommended next steps below."
	}
}

func recommendedIndexAction(root, status string) domain.Action {
	quotedRoot := shellQuote(root)
	switch status {
	case "fresh":
		return domain.Action{Name: "search", Command: fmt.Sprintf("pinax search <query> --vault %s", quotedRoot)}
	case "missing", "stale":
		return domain.Action{Name: "refresh", Command: fmt.Sprintf("pinax index refresh --vault %s", quotedRoot)}
	case "unreadable":
		return domain.Action{Name: "repair", Command: fmt.Sprintf("pinax index repair --vault %s --kind recreate --dry-run", quotedRoot)}
	default:
		return domain.Action{Name: "doctor", Command: fmt.Sprintf("pinax index doctor --vault %s", quotedRoot)}
	}
}

func refreshIndex(root string) error {
	notes, err := scanNotes(root)
	if err != nil {
		return err
	}
	notes = ordinaryNotes(notes)
	_, err = noteindex.Rebuild(root, notes)
	return err
}
