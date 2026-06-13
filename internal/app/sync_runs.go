package app

import (
	"bufio"
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

	"github.com/yeisme/pinax/internal/cloudsync"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/redaction"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
	syncplan "github.com/yeisme/pinax/internal/sync"
)

const (
	syncRunSchemaVersion   = "pinax.sync_run.v1"
	syncStateSchemaVersion = "pinax.sync_state.v1"
	defaultSyncRunKeep     = 200
	defaultSyncRunMaxAge   = 90
)

type SyncLogsRequest struct {
	VaultPath  string
	RunID      string
	Limit      int
	Keep       int
	MaxAgeDays int
	Yes        bool
}

type SyncRunReceipt struct {
	SchemaVersion        string               `json:"schema_version"`
	RunID                string               `json:"run_id"`
	Command              string               `json:"command"`
	Target               string               `json:"target"`
	Direction            string               `json:"direction"`
	Status               string               `json:"status"`
	RemoteWrite          bool                 `json:"remote_write"`
	LocalWrite           bool                 `json:"local_write"`
	BackendKind          string               `json:"backend_kind"`
	Transport            string               `json:"transport"`
	WorkspaceID          string               `json:"workspace_id"`
	VaultID              string               `json:"vault_id"`
	DeviceID             string               `json:"device_id"`
	RequestID            string               `json:"request_id"`
	BaseRevision         string               `json:"base_revision"`
	RemoteRevisionBefore string               `json:"remote_revision_before"`
	RevisionID           string               `json:"revision_id"`
	ManifestBlobID       string               `json:"manifest_blob_id"`
	Counts               map[string]int       `json:"counts"`
	TimingsMS            map[string]int64     `json:"timings_ms"`
	Error                *domain.CommandError `json:"error"`
	Actions              []domain.Action      `json:"actions"`
	Redaction            SyncRunRedaction     `json:"redaction"`
	Operations           []syncplan.Operation `json:"operations,omitempty"`
	CreatedAt            string               `json:"created_at"`
}

type SyncRunRedaction struct {
	PathPolicy   string `json:"path_policy"`
	SecretPolicy string `json:"secret_policy"`
}

type currentSyncState struct {
	SchemaVersion      string `json:"schema_version"`
	Target             string `json:"target"`
	BackendKind        string `json:"backend_kind"`
	Endpoint           string `json:"endpoint"`
	WorkspaceID        string `json:"workspace_id"`
	VaultID            string `json:"vault_id"`
	DeviceID           string `json:"device_id"`
	LastSyncedRevision string `json:"last_synced_revision,omitempty"`
	LastSyncRunID      string `json:"last_sync_run_id"`
	LastDirection      string `json:"last_direction"`
	LastStatus         string `json:"last_status"`
	UpdatedAt          string `json:"updated_at"`
}

type syncRunRecord struct {
	Receipt SyncRunReceipt `json:"receipt"`
	Path    string         `json:"path"`
}

func syncRunStart(command string, direction syncplan.Direction, state pinaxcloud.State, pathPolicy string) SyncRunReceipt {
	createdAt := time.Now().UTC()
	runID := "sync_" + createdAt.Format("20060102T150405.000000000")
	policy := normalizeSyncPathPolicy(pathPolicy)
	return SyncRunReceipt{
		SchemaVersion: syncRunSchemaVersion,
		RunID:         runID,
		Command:       command,
		Target:        "cloud",
		Direction:     string(direction),
		Status:        "success",
		BackendKind:   directBackendKind(state),
		Transport:     syncTransportName(state),
		WorkspaceID:   sanitizeSyncString(state.Config.WorkspaceID),
		VaultID:       syncVaultID(state, state.Config.WorkspaceID),
		DeviceID:      sanitizeSyncString(state.Config.DeviceID),
		RequestID:     "pinax-" + createdAt.Format("20060102T150405.000000000"),
		Counts:        map[string]int{},
		TimingsMS:     map[string]int64{},
		Actions:       []domain.Action{},
		Redaction:     SyncRunRedaction{PathPolicy: policy, SecretPolicy: "cloud"},
		CreatedAt:     createdAt.Format(time.RFC3339),
	}
}

func normalizeSyncPathPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "hash", "hashed":
		return "hash"
	case "omit", "omitted", "none":
		return "omitted"
	default:
		return "default"
	}
}

func syncTransportName(state pinaxcloud.State) string {
	endpoint := strings.TrimSpace(state.Config.Endpoint)
	if strings.HasPrefix(endpoint, "file://") {
		return "file"
	}
	if strings.HasPrefix(endpoint, "s3://") || state.Config.BackendKind == "s3-direct" {
		return "s3"
	}
	if strings.HasPrefix(endpoint, "rclone://") || state.Config.BackendKind == "rclone-direct" {
		return "rclone"
	}
	return "server"
}

func syncVaultID(state pinaxcloud.State, fallback string) string {
	if strings.TrimSpace(fallback) != "" {
		return sanitizeSyncString(fallback)
	}
	sum := sha256.Sum256([]byte(state.Config.Endpoint + "|" + state.Config.WorkspaceID))
	return "vault_" + hex.EncodeToString(sum[:])[:16]
}

func syncRunReceiptPath(root string, receipt SyncRunReceipt) string {
	createdAt, err := time.Parse(time.RFC3339, receipt.CreatedAt)
	if err != nil {
		createdAt = time.Now().UTC()
	}
	return filepath.Join(root, ".pinax", "sync-runs", createdAt.Format("2006"), createdAt.Format("01"), receipt.RunID+".json")
}

func writeSyncRunReceipt(root string, receipt SyncRunReceipt) (string, error) {
	receipt.WorkspaceID = sanitizeSyncString(receipt.WorkspaceID)
	receipt.DeviceID = sanitizeSyncString(receipt.DeviceID)
	receipt.RequestID = sanitizeSyncString(receipt.RequestID)
	if receipt.Counts == nil {
		receipt.Counts = map[string]int{}
	}
	if receipt.TimingsMS == nil {
		receipt.TimingsMS = map[string]int64{}
	}
	path := syncRunReceiptPath(root, receipt)
	return filepath.ToSlash(strings.TrimPrefix(path, root+string(os.PathSeparator))), writeJSONAsset(path, receipt)
}

func writeCurrentSyncState(root string, state pinaxcloud.State, receipt SyncRunReceipt, syncedRevision string) error {
	previous, _ := readCurrentSyncState(root)
	if strings.TrimSpace(syncedRevision) == "" {
		syncedRevision = previous.LastSyncedRevision
	}
	current := currentSyncState{
		SchemaVersion:      syncStateSchemaVersion,
		Target:             "cloud",
		BackendKind:        directBackendKind(state),
		Endpoint:           sanitizeSyncString(state.Config.Endpoint),
		WorkspaceID:        sanitizeSyncString(state.Config.WorkspaceID),
		VaultID:            syncVaultID(state, state.Config.WorkspaceID),
		DeviceID:           sanitizeSyncString(state.Config.DeviceID),
		LastSyncedRevision: sanitizeSyncString(syncedRevision),
		LastSyncRunID:      receipt.RunID,
		LastDirection:      receipt.Direction,
		LastStatus:         receipt.Status,
		UpdatedAt:          time.Now().UTC().Format(time.RFC3339),
	}
	return writeJSONAsset(filepath.Join(root, ".pinax", "sync-state.json"), current)
}

func readCurrentSyncState(root string) (currentSyncState, error) {
	b, err := os.ReadFile(filepath.Join(root, ".pinax", "sync-state.json"))
	if err != nil {
		return currentSyncState{}, err
	}
	var state currentSyncState
	if err := json.Unmarshal(b, &state); err != nil {
		return currentSyncState{}, err
	}
	return state, nil
}

func finishSyncRun(root string, state pinaxcloud.State, receipt SyncRunReceipt, plan syncplan.Plan, status string, commandErr *domain.CommandError, actions []domain.Action, pathPolicy string, started time.Time) (SyncRunReceipt, string, error) {
	receipt.Status = status
	receipt.BaseRevision = sanitizeSyncString(plan.BaseRevision)
	receipt.RemoteRevisionBefore = sanitizeSyncString(plan.RemoteRevision)
	receipt.RemoteWrite = plan.RemoteWrite
	receipt.Counts = syncRunCounts(plan, receipt.Counts)
	receipt.TimingsMS["total"] = time.Since(started).Milliseconds()
	receipt.Error = sanitizeCommandError(commandErr)
	receipt.Actions = sanitizeActions(actions)
	receipt.Operations = sanitizeSyncOperations(plan.Operations, pathPolicy)
	path, err := writeSyncRunReceipt(root, receipt)
	if err != nil {
		return receipt, path, err
	}
	if err := appendSyncRunEvent(root, receipt); err != nil {
		return receipt, path, err
	}
	return receipt, path, nil
}

func syncRunCounts(plan syncplan.Plan, base map[string]int) map[string]int {
	counts := map[string]int{}
	for key, value := range base {
		counts[key] = value
	}
	counts["operations"] = len(plan.Operations)
	counts["conflicts"] = len(plan.ConflictQueue)
	for _, op := range plan.Operations {
		switch op.Kind {
		case "upload_blob":
			counts["upload_blobs"]++
		case "download_blob":
			counts["download_blobs"]++
		case "conflict":
			counts["conflicts"]++
		}
	}
	return counts
}

func appendSyncRunEvent(root string, receipt SyncRunReceipt) error {
	facts := map[string]string{
		"run_id":       receipt.RunID,
		"command":      receipt.Command,
		"backend_kind": receipt.BackendKind,
		"remote_write": fmt.Sprint(receipt.RemoteWrite),
		"conflicts":    fmt.Sprint(receipt.Counts["conflicts"]),
	}
	if receipt.RevisionID != "" {
		facts["revision_id"] = receipt.RevisionID
	}
	if receipt.Error != nil {
		facts["error_code"] = receipt.Error.Code
	}
	return appendEvent(root, "sync.run", receipt.Status, facts)
}

func sanitizeSyncOperations(ops []syncplan.Operation, policy string) []syncplan.Operation {
	if len(ops) == 0 {
		return nil
	}
	policy = normalizeSyncPathPolicy(policy)
	out := make([]syncplan.Operation, 0, len(ops))
	for _, op := range ops {
		op.Path = redactSyncPath(op.Path, policy)
		if policy == "hash" && op.PathHash == "" && op.Path != "" {
			op.PathHash = op.Path
		}
		if policy == "omitted" {
			op.PathHash = ""
		}
		out = append(out, op)
	}
	return out
}

func sanitizeSyncPlan(plan syncplan.Plan, policy string) syncplan.Plan {
	plan.Operations = sanitizeSyncOperations(plan.Operations, policy)
	return plan
}

func redactSyncPath(pathValue, policy string) string {
	pathValue = filepath.ToSlash(strings.TrimSpace(pathValue))
	if pathValue == "" {
		return ""
	}
	switch normalizeSyncPathPolicy(policy) {
	case "hash":
		sum := sha256.Sum256([]byte(pathValue))
		return "path_sha256:" + hex.EncodeToString(sum[:])
	case "omitted":
		return ""
	default:
		return sanitizeSyncString(pathValue)
	}
}

func sanitizeCommandError(err *domain.CommandError) *domain.CommandError {
	if err == nil {
		return nil
	}
	return &domain.CommandError{Code: sanitizeSyncString(err.Code), Message: sanitizeSyncString(err.Message), Hint: sanitizeSyncString(err.Hint)}
}

func sanitizeActions(actions []domain.Action) []domain.Action {
	out := make([]domain.Action, 0, len(actions))
	for _, action := range actions {
		out = append(out, domain.Action{Name: sanitizeSyncString(action.Name), Command: sanitizeSyncString(action.Command)})
	}
	return out
}

func sanitizeSyncString(value string) string {
	value = redaction.Cloud(value)
	for _, marker := range []string{"Authorization", "Cookie", "provider payload", "provider stderr"} {
		value = strings.ReplaceAll(value, marker, "[REDACTED]")
	}
	return value
}

func commandErrorFromError(err error) *domain.CommandError {
	if err == nil {
		return nil
	}
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) {
		return commandErr
	}
	if errors.Is(err, cloudsync.ErrLockHeld) {
		return &domain.CommandError{Code: "lock_held", Message: "cloud commit lock is held", Hint: "Retry after the current or expired direct backend commit lock clears"}
	}
	if errors.Is(err, cloudsync.ErrRevisionConflict) {
		return &domain.CommandError{Code: "revision_conflict", Message: "cloud revision conflict", Hint: "Pull remote changes and resolve conflicts before retrying"}
	}
	msg := err.Error()
	if strings.Contains(msg, "rclone ") || strings.Contains(msg, "NOTICE:") || strings.Contains(msg, "CRITICAL:") || strings.Contains(msg, "provider stderr") {
		return &domain.CommandError{Code: "transport_unavailable", Message: "cloud transport is unavailable", Hint: "Run pinax cloud doctor --vault <vault> --json and verify provider credentials"}
	}
	return &domain.CommandError{Code: "internal_error", Message: sanitizeSyncString(msg)}
}

func writeApprovalRequiredSyncRun(root string, req SyncRequest, command string, direction syncplan.Direction, commandErr *domain.CommandError, projection *domain.Projection) error {
	state, err := cloudStateForSync(root, req)
	if err != nil {
		return err
	}
	started := time.Now()
	pathPolicy := normalizeSyncPathPolicy(req.PathPolicy)
	receipt := syncRunStart(command, direction, state, pathPolicy)
	plan := syncplan.Plan{SchemaVersion: syncplan.PlanSchemaVersion, Status: "approval_required", Direction: direction, Target: "cloud", DryRun: req.DryRun, RequiresApproval: true, RemoteWrite: false}
	if projection.Actions == nil {
		projection.Actions = []domain.Action{{Name: "dry_run", Command: fmt.Sprintf("pinax %s --target cloud --dry-run --vault %s --json", strings.ReplaceAll(command, ".", " "), shellQuote(root))}}
	}
	receipt, receiptPath, finishErr := finishSyncRun(root, state, receipt, plan, "approval_required", commandErr, projection.Actions, pathPolicy, started)
	if finishErr != nil {
		return finishErr
	}
	_ = writeCurrentSyncState(root, state, receipt, "")
	projection.Facts["run_id"] = receipt.RunID
	projection.Facts["remote_write"] = "false"
	projection.Facts["target"] = "cloud"
	projection.Evidence = []string{receiptPath}
	projection.Data = map[string]any{"receipt": receipt}
	return nil
}

func readSyncRunReceipts(root string) ([]syncRunRecord, error) {
	base := filepath.Join(root, ".pinax", "sync-runs")
	records := []syncRunRecord{}
	if _, err := os.Stat(base); err != nil {
		if os.IsNotExist(err) {
			return records, nil
		}
		return nil, err
	}
	if err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var receipt SyncRunReceipt
		if err := json.Unmarshal(b, &receipt); err != nil {
			return err
		}
		if receipt.SchemaVersion == syncRunSchemaVersion {
			records = append(records, syncRunRecord{Receipt: receipt, Path: path})
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sortSyncRunRecords(records)
	return records, nil
}

func sortSyncRunRecords(records []syncRunRecord) {
	sort.Slice(records, func(i, j int) bool {
		return records[i].Receipt.CreatedAt > records[j].Receipt.CreatedAt
	})
}
func (s *Service) SyncLogsList(_ context.Context, req SyncLogsRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.logs.list", err), err
	}
	records, err := readSyncRunReceipts(root)
	if err != nil {
		return errorProjection("sync.logs.list", err), err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if len(records) > limit {
		records = records[:limit]
	}
	projection := domain.NewProjection("sync.logs.list", "Sync run logs listed.")
	projection.Facts["runs"] = fmt.Sprint(len(records))
	projection.Data = map[string]any{"schema_version": syncRunSchemaVersion, "runs": receiptSummaries(records)}
	if len(records) > 0 {
		projection.Actions = []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax sync logs show %s --vault %s --json", records[0].Receipt.RunID, shellQuote(root))}}
	}
	return projection, nil
}

func (s *Service) SyncLogsShow(_ context.Context, req SyncLogsRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.logs.show", err), err
	}
	record, err := findSyncRunReceipt(root, req.RunID)
	if err != nil {
		return errorProjection("sync.logs.show", err), err
	}
	projection := domain.NewProjection("sync.logs.show", "Sync run receipt read.")
	projection.Facts["run_id"] = record.Receipt.RunID
	projection.Facts["status"] = record.Receipt.Status
	projection.Facts["remote_write"] = fmt.Sprint(record.Receipt.RemoteWrite)
	projection.Facts["backend_kind"] = record.Receipt.BackendKind
	projection.Data = map[string]any{"receipt": record.Receipt}
	projection.Evidence = []string{filepath.ToSlash(strings.TrimPrefix(record.Path, root+string(os.PathSeparator)))}
	return projection, nil
}

func (s *Service) SyncLogsTail(_ context.Context, req SyncLogsRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.logs.tail", err), err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	events, err := tailSyncEvents(root, limit)
	if err != nil {
		return errorProjection("sync.logs.tail", err), err
	}
	projection := domain.NewProjection("sync.logs.tail", "Sync event timeline read.")
	projection.Facts["events"] = fmt.Sprint(len(events))
	if len(events) > 0 {
		if runID, _ := events[len(events)-1]["run_id"].(string); runID != "" {
			projection.Facts["run_id"] = runID
		}
	}
	projection.Data = map[string]any{"events": events}
	return projection, nil
}

func (s *Service) SyncLogsPrune(_ context.Context, req SyncLogsRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.logs.prune", err), err
	}
	records, err := readSyncRunReceipts(root)
	if err != nil {
		return errorProjection("sync.logs.prune", err), err
	}
	keep := req.Keep
	if keep <= 0 {
		keep = defaultSyncRunKeep
	}
	maxAgeDays := req.MaxAgeDays
	if maxAgeDays <= 0 {
		maxAgeDays = defaultSyncRunMaxAge
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -maxAgeDays)
	candidates := make([]syncRunRecord, 0)
	for i, record := range records {
		createdAt, parseErr := time.Parse(time.RFC3339, record.Receipt.CreatedAt)
		tooOld := parseErr == nil && createdAt.Before(cutoff)
		tooMany := i >= keep
		if tooOld || tooMany {
			candidates = append(candidates, record)
		}
	}
	deleted := 0
	if req.Yes {
		for _, candidate := range candidates {
			if err := os.Remove(candidate.Path); err != nil && !os.IsNotExist(err) {
				return errorProjection("sync.logs.prune", err), err
			}
			deleted++
		}
	}
	projection := domain.NewProjection("sync.logs.prune", "Sync run log prune preview generated.")
	if req.Yes {
		projection.Summary = "Sync run logs pruned."
	}
	projection.Facts["dry_run"] = fmt.Sprint(!req.Yes)
	projection.Facts["delete_candidates"] = fmt.Sprint(len(candidates))
	projection.Facts["deleted"] = fmt.Sprint(deleted)
	projection.Facts["keep"] = fmt.Sprint(keep)
	projection.Facts["max_age_days"] = fmt.Sprint(maxAgeDays)
	projection.Data = map[string]any{"dry_run": !req.Yes, "delete_candidates": receiptSummaries(candidates), "deleted": deleted, "keep": keep, "max_age_days": maxAgeDays}
	return projection, nil
}

func receiptSummaries(records []syncRunRecord) []map[string]any {
	summaries := make([]map[string]any, 0, len(records))
	for _, record := range records {
		r := record.Receipt
		summary := map[string]any{"schema_version": r.SchemaVersion, "run_id": r.RunID, "command": r.Command, "status": r.Status, "direction": r.Direction, "backend_kind": r.BackendKind, "remote_write": r.RemoteWrite, "revision_id": r.RevisionID, "created_at": r.CreatedAt}
		if r.Error != nil {
			summary["error_code"] = r.Error.Code
		}
		summaries = append(summaries, summary)
	}
	return summaries
}

func findSyncRunReceipt(root, runID string) (syncRunRecord, error) {
	records, err := readSyncRunReceipts(root)
	if err != nil {
		return syncRunRecord{}, err
	}
	for _, record := range records {
		if record.Receipt.RunID == runID {
			return record, nil
		}
	}
	return syncRunRecord{}, &domain.CommandError{Code: "sync_run_not_found", Message: "sync run receipt was not found", Hint: "Run pinax sync logs list --json"}
}

func tailSyncEvents(root string, limit int) ([]map[string]any, error) {
	path := filepath.Join(root, ".pinax", "events.jsonl")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var events []map[string]any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event struct {
			Type   string            `json:"type"`
			Status string            `json:"status"`
			TS     string            `json:"ts"`
			Facts  map[string]string `json:"facts"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil || event.Type != "sync.run" {
			continue
		}
		row := map[string]any{"type": event.Type, "status": event.Status, "ts": event.TS}
		for key, value := range event.Facts {
			row[key] = value
		}
		events = append(events, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events, nil
}
