package app

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/syncops"
	"github.com/yeisme/pinax/internal/domain"
)

const (
	activityEntrySchemaVersion = "pinax.activity_event.v1"
	activityDefaultLimit       = 50
	activityMaxLimit           = 200
)

type ActivityRequest struct {
	VaultPath string
	EventID   string
	Source    string
	Query     string
	Status    string
	Object    string
	Since     string
	Until     string
	Limit     int
}

type ActivityEntry struct {
	SchemaVersion string            `json:"schema_version"`
	EventID       string            `json:"event_id"`
	Source        string            `json:"source"`
	Kind          string            `json:"kind"`
	Status        string            `json:"status,omitempty"`
	Severity      string            `json:"severity,omitempty"`
	Summary       string            `json:"summary"`
	ObjectRef     string            `json:"object_ref,omitempty"`
	Path          string            `json:"path,omitempty"`
	RunID         string            `json:"run_id,omitempty"`
	Timestamp     string            `json:"ts,omitempty"`
	DurationMS    int64             `json:"duration_ms,omitempty"`
	Facts         map[string]string `json:"facts,omitempty"`
	Evidence      []string          `json:"evidence,omitempty"`
	Actions       []domain.Action   `json:"actions,omitempty"`
}

type activityWarning struct {
	Source  string `json:"source"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

type activitySourceStatus struct {
	Source        string `json:"source"`
	Path          string `json:"path"`
	Available     bool   `json:"available"`
	Entries       int    `json:"entries"`
	Warnings      int    `json:"warnings"`
	EstimatedSize int64  `json:"estimated_size_bytes"`
	Readonly      bool   `json:"readonly"`
	Prunable      bool   `json:"prunable"`
}

type activityQueryResult struct {
	Root     string
	Entries  []ActivityEntry
	Sources  []activitySourceStatus
	Warnings []activityWarning
	Filters  map[string]string
}

func (s *Service) ActivitySources(_ context.Context, req ActivityRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("activity.sources", err), err
	}
	result := readActivity(root, ActivityRequest{VaultPath: root, Source: "all", Limit: activityMaxLimit})
	projection := activityProjection("activity.sources", "Activity sources inspected.", result)
	projection.Data = map[string]any{"schema_version": activityEntrySchemaVersion, "sources": result.Sources, "warnings": result.Warnings}
	return projection, nil
}

func (s *Service) ActivityList(_ context.Context, req ActivityRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("activity.list", err), err
	}
	result, err := filterActivity(root, req)
	if err != nil {
		return errorProjection("activity.list", err), err
	}
	projection := activityProjection("activity.list", "Activity entries listed.", result)
	projection.Data = map[string]any{"schema_version": activityEntrySchemaVersion, "entries": result.Entries, "sources": result.Sources, "filters": result.Filters, "warnings": result.Warnings}
	if len(result.Entries) > 0 {
		projection.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax activity show %s --vault %s --json", result.Entries[0].EventID, shellQuote(root))}}
	}
	return projection, nil
}

func (s *Service) ActivityTail(ctx context.Context, req ActivityRequest) (domain.Projection, error) {
	projection, err := s.ActivityList(ctx, req)
	projection.Command = "activity.tail"
	if projection.Status == "success" {
		projection.Summary = "Recent activity entries read."
	}
	return projection, err
}

func (s *Service) ActivityShow(_ context.Context, req ActivityRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("activity.show", err), err
	}
	result := readActivity(root, ActivityRequest{VaultPath: root, Source: "all"})
	for _, entry := range result.Entries {
		if entry.EventID == strings.TrimSpace(req.EventID) {
			result.Entries = []ActivityEntry{entry}
			projection := activityProjection("activity.show", "Activity entry read.", result)
			projection.Facts["event_id"] = entry.EventID
			projection.Facts["source"] = entry.Source
			projection.Facts["kind"] = entry.Kind
			projection.Data = map[string]any{"schema_version": activityEntrySchemaVersion, "entry": entry, "warnings": result.Warnings}
			projection.Evidence = entry.Evidence
			return projection, nil
		}
	}
	commandErr := &domain.CommandError{Code: "activity_event_not_found", Message: "activity event was not found", Hint: "Run pinax activity list --vault <vault> --json"}
	return domain.NewErrorProjection("activity.show", commandErr), commandErr
}

func (s *Service) ActivityManage(_ context.Context, req ActivityRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("activity.manage", err), err
	}
	result := readActivity(root, ActivityRequest{VaultPath: root, Source: "all", Limit: activityMaxLimit})
	projection := activityProjection("activity.manage", "Activity log management summary generated.", result)
	projection.Data = map[string]any{"schema_version": activityEntrySchemaVersion, "sources": result.Sources, "warnings": result.Warnings, "readonly": true}
	projection.Actions = []domain.Action{{Name: "prune-sync-runs", Command: fmt.Sprintf("pinax sync logs prune --vault %s --keep 200 --max-age-days 90 --yes --json", shellQuote(root))}}
	return projection, nil
}

func filterActivity(root string, req ActivityRequest) (activityQueryResult, error) {
	result := readActivity(root, req)
	since, err := parseActivityTime(req.Since)
	if err != nil {
		return result, err
	}
	until, err := parseActivityTime(req.Until)
	if err != nil {
		return result, err
	}
	sources := activitySourceSet(req.Source)
	query := strings.ToLower(strings.TrimSpace(req.Query))
	status := strings.ToLower(strings.TrimSpace(req.Status))
	object := strings.ToLower(strings.TrimSpace(req.Object))
	filtered := make([]ActivityEntry, 0, len(result.Entries))
	for _, entry := range result.Entries {
		if len(sources) > 0 && !sources[entry.Source] {
			continue
		}
		if status != "" && strings.ToLower(entry.Status) != status {
			continue
		}
		if !since.IsZero() && activityEntryTime(entry).Before(since) {
			continue
		}
		if !until.IsZero() && activityEntryTime(entry).After(until) {
			continue
		}
		if object != "" && !activityEntryMatchesObject(entry, object) {
			continue
		}
		if query != "" && !activityEntryMatchesQuery(entry, query) {
			continue
		}
		filtered = append(filtered, entry)
	}
	limit := activityLimit(req.Limit)
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	result.Entries = filtered
	result.Filters = map[string]string{"source": defaultString(strings.TrimSpace(req.Source), "all"), "limit": strconv.Itoa(limit)}
	for key, value := range map[string]string{"query": req.Query, "status": req.Status, "object": req.Object, "since": req.Since, "until": req.Until} {
		if strings.TrimSpace(value) != "" {
			result.Filters[key] = strings.TrimSpace(value)
		}
	}
	return result, nil
}

func readActivity(root string, req ActivityRequest) activityQueryResult {
	result := activityQueryResult{Root: root, Filters: map[string]string{}}
	readers := []func(string, *activityQueryResult){
		readVaultActivityEvents,
		readMonitorEventsActivity,
		readSyncRunActivity,
		readSyncDaemonActivity,
		readAPIAuditActivity,
		readRecordLedgerActivity,
	}
	for _, reader := range readers {
		reader(root, &result)
	}
	sort.SliceStable(result.Entries, func(i, j int) bool {
		ti := activityEntryTime(result.Entries[i])
		tj := activityEntryTime(result.Entries[j])
		if ti.Equal(tj) {
			return result.Entries[i].EventID > result.Entries[j].EventID
		}
		return ti.After(tj)
	})
	return result
}

func readVaultActivityEvents(root string, result *activityQueryResult) {
	path := filepath.Join(root, ".pinax", "events.jsonl")
	readJSONLActivity(root, path, "vault_events", result, func(line []byte, lineNo int) (ActivityEntry, bool, error) {
		var event struct {
			Type   string            `json:"type"`
			Status string            `json:"status"`
			TS     string            `json:"ts"`
			Facts  map[string]string `json:"facts"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			return ActivityEntry{}, false, err
		}
		if strings.TrimSpace(event.Type) == "" {
			return ActivityEntry{}, false, nil
		}
		facts := sanitizeActivityFacts(event.Facts)
		pathValue := firstActivityNonEmpty(facts["path"], facts["source_path"], facts["target_path"], facts["folder_path"])
		entry := newActivityEntry("vault_events", event.Type, event.Status, event.TS, line)
		entry.Path = pathValue
		entry.ObjectRef = firstActivityNonEmpty(pathValue, facts["project"], facts["item_id"], facts["run_id"])
		entry.RunID = facts["run_id"]
		entry.Summary = activitySummary(event.Type, event.Status, pathValue)
		entry.Facts = facts
		entry.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "events.jsonl"))}
		return entry, true, nil
	})
}

func readSyncRunActivity(root string, result *activityQueryResult) {
	source := "sync_runs"
	base := filepath.Join(root, ".pinax", "sync-runs")
	status := newActivitySourceStatus(root, source, base, true)
	defer func() { result.Sources = append(result.Sources, status) }()
	if _, err := os.Stat(base); err != nil {
		if !os.IsNotExist(err) {
			addActivityWarning(result, source, base, 0, err)
			status.Warnings++
		}
		return
	}
	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			addActivityWarning(result, source, path, 0, err)
			status.Warnings++
			return nil
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		if info, statErr := d.Info(); statErr == nil {
			status.EstimatedSize += info.Size()
		}
		b, err := os.ReadFile(path)
		if err != nil {
			addActivityWarning(result, source, path, 0, err)
			status.Warnings++
			return nil
		}
		var receipt SyncRunReceipt
		if err := json.Unmarshal(b, &receipt); err != nil {
			addActivityWarning(result, source, path, 0, err)
			status.Warnings++
			return nil
		}
		if receipt.SchemaVersion != syncRunSchemaVersion {
			return nil
		}
		facts := map[string]string{"target": receipt.Target, "direction": receipt.Direction, "backend_kind": receipt.BackendKind, "remote_write": fmt.Sprint(receipt.RemoteWrite), "local_write": fmt.Sprint(receipt.LocalWrite)}
		if receipt.RevisionID != "" {
			facts["revision_id"] = syncops.SanitizeString(receipt.RevisionID)
		}
		entry := newActivityEntry(source, "sync.run", receipt.Status, receipt.CreatedAt, []byte(receipt.RunID+path))
		entry.RunID = receipt.RunID
		entry.ObjectRef = receipt.RunID
		entry.Summary = activitySummary("sync.run", receipt.Status, receipt.RunID)
		entry.Facts = facts
		entry.DurationMS = receipt.TimingsMS["total"]
		entry.Evidence = []string{relativeEvidence(root, path)}
		entry.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", receipt.RunID, shellQuote(root))}}
		result.Entries = append(result.Entries, entry)
		status.Entries++
		return nil
	})
}

func readSyncDaemonActivity(root string, result *activityQueryResult) {
	path := filepath.Join(root, ".pinax", "sync-daemon", "events.jsonl")
	readJSONLActivity(root, path, "sync_daemon", result, func(line []byte, lineNo int) (ActivityEntry, bool, error) {
		var event struct {
			Type           string `json:"type"`
			Status         string `json:"status"`
			Target         string `json:"target"`
			Path           string `json:"path"`
			ErrorCode      string `json:"error_code"`
			Message        string `json:"message"`
			Trigger        string `json:"trigger"`
			Direction      string `json:"direction"`
			DurationMS     int64  `json:"duration_ms"`
			RemoteRevision string `json:"remote_revision"`
			RevisionID     string `json:"revision_id"`
			SyncRunID      string `json:"sync_run_id"`
			CreatedAt      string `json:"created_at"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			return ActivityEntry{}, false, err
		}
		if strings.TrimSpace(event.Type) == "" {
			return ActivityEntry{}, false, nil
		}
		entry := newActivityEntry("sync_daemon", event.Type, event.Status, event.CreatedAt, line)
		entry.Path = syncops.RedactPath(event.Path, "default")
		entry.RunID = syncops.SanitizeString(event.SyncRunID)
		entry.ObjectRef = firstActivityNonEmpty(entry.RunID, entry.Path, event.Target)
		entry.DurationMS = event.DurationMS
		entry.Summary = activitySummary(event.Type, event.Status, entry.ObjectRef)
		entry.Facts = sanitizeActivityFacts(map[string]string{"target": event.Target, "trigger": event.Trigger, "direction": event.Direction, "error_code": event.ErrorCode, "message": event.Message, "remote_revision": event.RemoteRevision, "revision_id": event.RevisionID})
		entry.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "sync-daemon", "events.jsonl"))}
		return entry, true, nil
	})
}

func readAPIAuditActivity(root string, result *activityQueryResult) {
	path := filepath.Join(root, ".pinax", "events", "api-audit.jsonl")
	readJSONLActivity(root, path, "api_audit", result, func(line []byte, lineNo int) (ActivityEntry, bool, error) {
		var event struct {
			TS      string `json:"ts"`
			TokenID string `json:"token_id"`
			Method  string `json:"method"`
			Path    string `json:"path"`
			Scope   string `json:"scope"`
			Group   string `json:"group"`
			Status  int    `json:"status"`
		}
		if err := json.Unmarshal(line, &event); err != nil {
			return ActivityEntry{}, false, err
		}
		if strings.TrimSpace(event.Method) == "" && strings.TrimSpace(event.Path) == "" {
			return ActivityEntry{}, false, nil
		}
		status := "success"
		if event.Status >= 400 {
			status = "failed"
		}
		kind := "api." + strings.ToLower(defaultString(event.Method, "request"))
		entry := newActivityEntry("api_audit", kind, status, event.TS, line)
		entry.Path = syncops.SanitizeString(event.Path)
		entry.ObjectRef = entry.Path
		entry.Summary = activitySummary(kind, status, entry.Path)
		entry.Facts = sanitizeActivityFacts(map[string]string{"method": event.Method, "path": event.Path, "scope": event.Scope, "group": event.Group, "status_code": strconv.Itoa(event.Status), "token_id": event.TokenID})
		entry.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "events", "api-audit.jsonl"))}
		return entry, true, nil
	})
}

func readRecordLedgerActivity(root string, result *activityQueryResult) {
	path := filepath.Join(root, ".pinax", "records", "events.jsonl")
	readJSONLActivity(root, path, "record_ledger", result, func(line []byte, lineNo int) (ActivityEntry, bool, error) {
		var event domain.RecordEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return ActivityEntry{}, false, err
		}
		if strings.TrimSpace(string(event.Kind)) == "" {
			return ActivityEntry{}, false, nil
		}
		entry := newActivityEntry("record_ledger", string(event.Kind), "success", event.CreatedAt, line)
		if event.EventID != "" {
			entry.EventID = "record_ledger:" + event.EventID
		}
		entry.Path = syncops.RedactPath(event.Path, "default")
		entry.ObjectRef = firstActivityNonEmpty(event.NoteID, entry.Path, event.Title)
		entry.Summary = activitySummary(string(event.Kind), "success", entry.ObjectRef)
		entry.Facts = sanitizeActivityFacts(map[string]string{"note_id": event.NoteID, "old_path": event.OldPath, "title": event.Title, "ledger_seq": fmt.Sprint(event.Seq), "event_id": event.EventID})
		entry.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "records", "events.jsonl"))}
		return entry, true, nil
	})
}

type activityLineMapper func(line []byte, lineNo int) (ActivityEntry, bool, error)

func readJSONLActivity(root, path, source string, result *activityQueryResult, mapper activityLineMapper) {
	status := newActivitySourceStatus(root, source, path, false)
	defer func() { result.Sources = append(result.Sources, status) }()
	file, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			addActivityWarning(result, source, path, 0, err)
			status.Warnings++
		}
		return
	}
	defer func() { _ = file.Close() }()
	if info, statErr := file.Stat(); statErr == nil {
		status.Available = true
		status.EstimatedSize = info.Size()
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		entry, ok, err := mapper([]byte(line), lineNo)
		if err != nil {
			addActivityWarning(result, source, path, lineNo, err)
			status.Warnings++
			continue
		}
		if !ok {
			continue
		}
		result.Entries = append(result.Entries, entry)
		status.Entries++
	}
	if err := scanner.Err(); err != nil {
		addActivityWarning(result, source, path, lineNo, err)
		status.Warnings++
	}
}

func activityProjection(command, summary string, result activityQueryResult) domain.Projection {
	projection := domain.NewProjection(command, summary)
	if len(result.Warnings) > 0 {
		projection.Status = "partial"
	}
	projection.Facts["entries"] = fmt.Sprint(len(result.Entries))
	projection.Facts["sources"] = fmt.Sprint(len(result.Sources))
	projection.Facts["warnings"] = fmt.Sprint(len(result.Warnings))
	projection.Facts["schema_version"] = activityEntrySchemaVersion
	addProjectionFilterFacts(&projection, result.Filters)
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "events.jsonl")), filepath.ToSlash(filepath.Join(".pinax", "monitor", "events.jsonl")), filepath.ToSlash(filepath.Join(".pinax", "sync-daemon", "events.jsonl")), filepath.ToSlash(filepath.Join(".pinax", "events", "api-audit.jsonl")), filepath.ToSlash(filepath.Join(".pinax", "records", "events.jsonl"))}
	return projection
}

func addProjectionFilterFacts(projection *domain.Projection, filters map[string]string) {
	if projection == nil {
		return
	}
	if projection.Facts == nil {
		projection.Facts = map[string]string{}
	}
	for key, value := range filters {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		projection.Facts["filter."+key] = value
	}
}

func newActivityEntry(source, kind, status, ts string, seed []byte) ActivityEntry {
	if strings.TrimSpace(status) == "" {
		status = "success"
	}
	return ActivityEntry{SchemaVersion: activityEntrySchemaVersion, EventID: activityEventID(source, seed), Source: source, Kind: syncops.SanitizeString(kind), Status: syncops.SanitizeString(status), Severity: activitySeverity(status), Timestamp: syncops.SanitizeString(ts)}
}

func activityEventID(source string, seed []byte) string {
	sum := sha256.Sum256(append([]byte(source+":"), seed...))
	return source + ":" + hex.EncodeToString(sum[:])[:16]
}

func activitySummary(kind, status, object string) string {
	kind = syncops.SanitizeString(kind)
	status = defaultString(syncops.SanitizeString(status), "success")
	object = syncops.SanitizeString(object)
	if object == "" {
		return fmt.Sprintf("%s %s", kind, status)
	}
	return fmt.Sprintf("%s %s: %s", kind, status, object)
}

func activitySeverity(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error":
		return "error"
	case "partial", "degraded", "warning", "approval_required":
		return "warning"
	default:
		return "info"
	}
}

func activityEntryTime(entry ActivityEntry) time.Time {
	t, _ := time.Parse(time.RFC3339, entry.Timestamp)
	return t
}

func parseActivityTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, &domain.CommandError{Code: "invalid_activity_time", Message: "activity time must use RFC3339", Hint: "Use a timestamp such as 2026-06-27T10:00:00Z"}
	}
	return t, nil
}

func activityLimit(limit int) int {
	if limit <= 0 {
		return activityDefaultLimit
	}
	if limit > activityMaxLimit {
		return activityMaxLimit
	}
	return limit
}

func activitySourceSet(value string) map[string]bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "all") {
		return nil
	}
	set := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" && !strings.EqualFold(part, "all") {
			set[part] = true
		}
	}
	return set
}

func activityEntryMatchesQuery(entry ActivityEntry, query string) bool {
	fields := []string{entry.Source, entry.Kind, entry.Status, entry.Summary, entry.ObjectRef, entry.Path, entry.RunID}
	for _, value := range fields {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	for key, value := range entry.Facts {
		if strings.Contains(strings.ToLower(key), query) || strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func activityEntryMatchesObject(entry ActivityEntry, object string) bool {
	for _, value := range []string{entry.ObjectRef, entry.Path, entry.RunID, entry.EventID} {
		if strings.Contains(strings.ToLower(value), object) {
			return true
		}
	}
	return false
}

func sanitizeActivityFacts(facts map[string]string) map[string]string {
	clean := map[string]string{}
	for key, value := range facts {
		key = strings.TrimSpace(key)
		if key == "" || activitySensitiveKey(key) {
			continue
		}
		value = syncops.SanitizeString(value)
		if value != "" {
			clean[key] = value
		}
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}

func activitySensitiveKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie") || strings.Contains(lower, "password") || strings.Contains(lower, "raw_prompt") || strings.Contains(lower, "provider_payload") || strings.Contains(lower, "system_prompt")
}

func addActivityWarning(result *activityQueryResult, source, path string, line int, err error) {
	result.Warnings = append(result.Warnings, activityWarning{Source: source, Path: relativeEvidence(result.Root, path), Line: line, Message: syncops.SanitizeString(err.Error())})
}

func newActivitySourceStatus(root, source, path string, prunable bool) activitySourceStatus {
	available := false
	size := int64(0)
	if info, err := os.Stat(path); err == nil {
		available = true
		if !info.IsDir() {
			size = info.Size()
		}
	}
	return activitySourceStatus{Source: source, Path: relativeEvidence(root, path), Available: available, EstimatedSize: size, Readonly: true, Prunable: prunable}
}

func relativeEvidence(root, path string) string {
	if root == "" || path == "" {
		return filepath.ToSlash(path)
	}
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

func firstActivityNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
