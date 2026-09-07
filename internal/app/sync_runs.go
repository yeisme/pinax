package app

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/app/syncops"
	"github.com/yeisme/pinax/internal/domain"
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
	VaultPath    string
	RunID        string
	Limit        int
	Keep         int
	MaxAgeDays   int
	Yes          bool
	PollInterval time.Duration
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
	LastManifestBlobID string `json:"last_manifest_blob_id,omitempty"`
	LastManifestCache  string `json:"last_manifest_cache,omitempty"`
	LastKeyID          string `json:"last_key_id,omitempty"`
	LastSyncRunID      string `json:"last_sync_run_id"`
	LastDirection      string `json:"last_direction"`
	LastStatus         string `json:"last_status"`
	UpdatedAt          string `json:"updated_at"`
}

type syncRunRecord struct {
	Receipt SyncRunReceipt `json:"receipt"`
	Path    string         `json:"path"`
}

func syncRunStart(command string, direction syncplan.Direction, state pinaxcloud.State, pathPolicy, target string) SyncRunReceipt {
	createdAt := time.Now().UTC()
	runID := "sync_" + createdAt.Format("20060102T150405.000000000")
	policy := syncops.NormalizePathPolicy(pathPolicy)
	outputTarget := syncOutputTarget(target)
	return SyncRunReceipt{
		SchemaVersion: syncRunSchemaVersion,
		RunID:         runID,
		Command:       command,
		Target:        outputTarget,
		Direction:     string(direction),
		Status:        "success",
		BackendKind:   directBackendKind(state),
		Transport:     syncTransportName(state),
		WorkspaceID:   syncops.SanitizeString(state.Config.WorkspaceID),
		VaultID:       syncVaultID(state, state.Config.WorkspaceID),
		DeviceID:      syncops.SanitizeString(state.Config.DeviceID),
		RequestID:     "pinax-" + createdAt.Format("20060102T150405.000000000"),
		Counts:        map[string]int{},
		TimingsMS:     map[string]int64{},
		Actions:       []domain.Action{},
		Redaction:     SyncRunRedaction{PathPolicy: policy, SecretPolicy: outputTarget},
		CreatedAt:     createdAt.Format(time.RFC3339),
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
		return syncops.SanitizeString(fallback)
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
	receipt.WorkspaceID = syncops.SanitizeString(receipt.WorkspaceID)
	receipt.DeviceID = syncops.SanitizeString(receipt.DeviceID)
	receipt.RequestID = syncops.SanitizeString(receipt.RequestID)
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
	manifestBlobID := strings.TrimSpace(receipt.ManifestBlobID)
	if manifestBlobID == "" {
		manifestBlobID = previous.LastManifestBlobID
	}
	manifestCache := previous.LastManifestCache
	if strings.TrimSpace(syncedRevision) != "" {
		candidate := syncManifestCacheRel(syncedRevision)
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(candidate))); err == nil {
			manifestCache = candidate
		}
	}
	current := currentSyncState{
		SchemaVersion:      syncStateSchemaVersion,
		Target:             syncOutputTarget(receipt.Target),
		BackendKind:        directBackendKind(state),
		Endpoint:           syncops.SanitizeString(state.Config.Endpoint),
		WorkspaceID:        syncops.SanitizeString(state.Config.WorkspaceID),
		VaultID:            syncVaultID(state, state.Config.WorkspaceID),
		DeviceID:           syncops.SanitizeString(state.Config.DeviceID),
		LastSyncedRevision: syncops.SanitizeString(syncedRevision),
		LastManifestBlobID: syncops.SanitizeString(manifestBlobID),
		LastManifestCache:  syncops.SanitizeString(manifestCache),
		LastKeyID:          syncops.SanitizeString(pinaxcloud.KeyID(pinaxcloud.EncryptionSecretRef(state.Config))),
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

func finishSyncRun(root string, receipt SyncRunReceipt, plan syncplan.Plan, status string, commandErr *domain.CommandError, actions []domain.Action, pathPolicy string, started time.Time) (SyncRunReceipt, string, error) {
	return finishSyncRunWithFileEvents(root, receipt, plan, status, commandErr, actions, pathPolicy, started, true)
}

// finishSyncRunWithFileEvents 收口一个 sync run。includeFileEvents=false
// 用于执行期已逐项发出 live sync.file 事件的 run：收口只补终态 sync.run
// 事件，避免同一项被事件流重复计数（status 重放按事件计数）。
func finishSyncRunWithFileEvents(root string, receipt SyncRunReceipt, plan syncplan.Plan, status string, commandErr *domain.CommandError, actions []domain.Action, pathPolicy string, started time.Time, includeFileEvents bool) (SyncRunReceipt, string, error) {
	receipt.Status = status
	receipt.BaseRevision = syncops.SanitizeString(plan.BaseRevision)
	receipt.RemoteRevisionBefore = syncops.SanitizeString(plan.RemoteRevision)
	receipt.RemoteWrite = plan.RemoteWrite
	receipt.Counts = syncRunCounts(plan, receipt.Counts)
	receipt.TimingsMS["total"] = time.Since(started).Milliseconds()
	receipt.Error = sanitizeCommandError(commandErr)
	receipt.Actions = sanitizeActions(actions)
	receipt.Operations = syncops.SanitizeOperations(plan.Operations, pathPolicy)
	path, err := writeSyncRunReceipt(root, receipt)
	if err != nil {
		return receipt, path, err
	}
	if err := appendSyncRunEvents(root, receipt, includeFileEvents); err != nil {
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
	for _, key := range []string{"added", "modified", "deleted", "renamed", "bytes_uploaded", "bytes_downloaded"} {
		if _, ok := counts[key]; !ok {
			counts[key] = 0
		}
	}
	for _, op := range plan.Operations {
		switch op.Kind {
		case "upload_blob":
			counts["upload_blobs"]++
			if op.BaseRevision == "" {
				counts["added"]++
			} else {
				counts["modified"]++
			}
		case "download_blob":
			counts["download_blobs"]++
			if op.BaseRevision == "" {
				counts["added"]++
			} else {
				counts["modified"]++
			}
		case "delete_local", "delete_remote":
			counts["deleted"]++
		case "move":
			counts["renamed"]++
		case "conflict":
			counts["conflicts"]++
		}
	}
	return counts
}

func appendSyncRunEvent(root string, receipt SyncRunReceipt) error {
	return appendSyncRunEvents(root, receipt, true)
}

func appendSyncRunEvents(root string, receipt SyncRunReceipt, includeFileEvents bool) error {
	if includeFileEvents {
		for _, operation := range receipt.Operations {
			if operation.Kind == "upload_manifest" || operation.Kind == "download_manifest" {
				continue
			}
			facts := map[string]string{
				"run_id":           receipt.RunID,
				"command":          receipt.Command,
				"direction":        receipt.Direction,
				"backend_kind":     receipt.BackendKind,
				"kind":             operation.Kind,
				"operation_status": operation.Status,
			}
			for key, value := range map[string]string{
				"path": operation.Path, "path_hash": operation.PathHash,
				"from_path": operation.FromPath, "to_path": operation.ToPath,
				"object_kind": operation.ObjectKind,
			} {
				if strings.TrimSpace(value) != "" {
					facts[key] = value
				}
			}
			if code := syncRunOperationChangeCode(operation); code != "" {
				facts["change_code"] = code
			}
			facts["change_state"] = syncRunOperationChangeState(operation.Status, receipt.Status)
			if err := appendEvent(root, "sync.file", receipt.Status, facts); err != nil {
				return err
			}
		}
	}
	facts := map[string]string{
		"run_id":       receipt.RunID,
		"command":      receipt.Command,
		"direction":    receipt.Direction,
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

func syncRunOperationChangeCode(operation syncplan.Operation) string {
	switch operation.Kind {
	case "upload_blob", "download_blob":
		if operation.BaseRevision == "" {
			return "A"
		}
		return "M"
	case "delete_local", "delete_remote":
		return "D"
	case "move":
		return "R"
	case "conflict", "revision_conflict", "path_collision":
		return "C"
	default:
		return ""
	}
}

func syncRunOperationChangeState(operationStatus, receiptStatus string) string {
	if operationStatus == "conflict" || receiptStatus == "conflict" {
		return "conflict"
	}
	switch receiptStatus {
	case "success":
		return "applied"
	case "failed", "partial":
		return "failed"
	default:
		return "planned"
	}
}

func sanitizeCommandError(err *domain.CommandError) *domain.CommandError {
	if err == nil {
		return nil
	}
	return &domain.CommandError{Code: syncops.SanitizeString(err.Code), Message: syncops.SanitizeString(err.Message), Hint: syncops.SanitizeString(err.Hint)}
}

func sanitizeActions(actions []domain.Action) []domain.Action {
	out := make([]domain.Action, 0, len(actions))
	for _, action := range actions {
		out = append(out, domain.Action{Name: syncops.SanitizeString(action.Name), Command: syncops.SanitizeString(action.Command)})
	}
	return out
}

func writeApprovalRequiredSyncRun(root string, req SyncRequest, command string, direction syncplan.Direction, commandErr *domain.CommandError, projection *domain.Projection) error {
	state, err := cloudStateForSync(root, req)
	if err != nil {
		return err
	}
	started := time.Now()
	pathPolicy := syncops.NormalizePathPolicy(req.PathPolicy)
	outputTarget := syncOutputTarget(req.Target)
	receipt := syncRunStart(command, direction, state, pathPolicy, outputTarget)
	plan := syncplan.Plan{SchemaVersion: syncplan.PlanSchemaVersion, Status: "approval_required", Direction: direction, Target: outputTarget, DryRun: req.DryRun, RequiresApproval: true, RemoteWrite: false}
	if projection.Actions == nil {
		projection.Actions = []domain.Action{{Name: "dry_run", Command: fmt.Sprintf("pinax %s --target %s --dry-run --vault %s --json", strings.ReplaceAll(command, ".", " "), outputTarget, shellQuote(root))}}
	}
	receipt, receiptPath, finishErr := finishSyncRun(root, receipt, plan, "approval_required", commandErr, projection.Actions, pathPolicy, started)
	if finishErr != nil {
		return finishErr
	}
	if err := writeCurrentSyncState(root, state, receipt, ""); err != nil {
		warnPersistFailure("sync state", err)
	}
	projection.Facts["run_id"] = receipt.RunID
	projection.Facts["remote_write"] = "false"
	projection.Facts["target"] = outputTarget
	addCapsaBridgeFacts(projection, req.Target)
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
	projection.Facts["limit"] = fmt.Sprint(limit)
	projection.Data = map[string]any{"schema_version": syncRunSchemaVersion, "runs": receiptSummaries(records), "limit": limit}
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
	data := map[string]any{"receipt": record.Receipt}
	viewResult := "planned"
	switch record.Receipt.Status {
	case "success":
		viewResult = "applied"
	case "failed":
		viewResult = "failed"
	case "partial":
		viewResult = "partial"
	}
	attachSyncOutputView(projection.Facts, data, buildSyncOutputView(syncplan.Plan{Direction: syncplan.Direction(record.Receipt.Direction), Target: record.Receipt.Target, BaseRevision: record.Receipt.BaseRevision, RemoteRevision: record.Receipt.RemoteRevisionBefore, Operations: record.Receipt.Operations}, pinaxcloud.Manifest{}, pinaxcloud.Manifest{}, pinaxcloud.Manifest{}, syncOutputViewOptions{Scope: "cached", Result: viewResult, RemoteAfter: record.Receipt.RevisionID, LocalAfter: record.Receipt.RevisionID, PathPolicy: record.Receipt.Redaction.PathPolicy}))
	projection.Data = data
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
	projection.Facts["limit"] = fmt.Sprint(limit)
	if len(events) > 0 {
		if runID, _ := events[len(events)-1]["run_id"].(string); runID != "" {
			projection.Facts["run_id"] = runID
		}
	}
	projection.Data = map[string]any{"events": events, "limit": limit}
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
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil || (event.Type != "sync.run" && event.Type != "sync.file") {
			continue
		}
		row := map[string]any{"type": event.Type, "status": event.Status, "ts": event.TS, "seq": len(events) + 1}
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

func (s *Service) SyncLogsFollow(ctx context.Context, req SyncLogsRequest, emit func(map[string]any) error) error {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	interval := req.PollInterval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	cursor := 0
	emitNew := func(events []map[string]any) error {
		for _, event := range events {
			seq, _ := event["seq"].(int)
			if seq <= cursor {
				continue
			}
			if err := emit(event); err != nil {
				return err
			}
			cursor = seq
		}
		return nil
	}
	initial, err := tailSyncEvents(root, limit)
	if err != nil {
		return err
	}
	if err := emitNew(initial); err != nil {
		return err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			events, err := tailSyncEvents(root, 10000)
			if err != nil {
				return err
			}
			if err := emitNew(events); err != nil {
				return err
			}
		}
	}
}

// SyncKeysStatus reports which key derivation the vault uses and which one the
// remote manifest envelope is encrypted under, so a re-encryption migration
// can be verified objectively: remote_derivation flips legacy→v2 after a push
// re-encrypts, and unknown means a foreign secret or corrupted state.
func (s *Service) SyncKeysStatus(ctx context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.keys", err), err
	}
	state, err := cloudStateForSync(root, SyncRequest{})
	if err != nil {
		projection, projErr := cloudStateErrorProjection("sync.keys", root, err)
		return projection, projErr
	}
	projection := domain.NewProjection("sync.keys", "Sync encryption key derivation status.")
	projection.Facts["configured"] = "true"
	if active, legacy, keyErr := pinaxcloud.SyncKeyVersions(pinaxcloud.EncryptionSecretRef(state.Config)); keyErr != nil {
		projection.Facts["configured"] = "false"
		projection.Data = map[string]any{"error": keyErr.Error()}
		return projection, nil
	} else {
		projection.Facts["active_key_id"] = active
		projection.Facts["legacy_key_id"] = legacy
		projection.Facts["derivation"] = "v2"
		projection.Data = map[string]any{"active_key_id": active, "legacy_key_id": legacy, "derivation": "v2", "iterations": 600000}
	}
	snapshot, snapErr := loadCloudRemoteSnapshot(ctx, root, state)
	if snapErr != nil {
		projection.Facts["remote_reachable"] = "false"
		if data, ok := projection.Data.(map[string]any); ok {
			data["remote_error"] = snapErr.Error()
		}
		return projection, snapErr
	}
	projection.Facts["remote_reachable"] = "true"
	if strings.TrimSpace(snapshot.RevisionID) == "" {
		projection.Facts["remote_state"] = "empty"
		if data, ok := projection.Data.(map[string]any); ok {
			data["remote_state"] = "empty"
		}
		return projection, nil
	}
	active := projection.Facts["active_key_id"]
	legacy := projection.Facts["legacy_key_id"]
	class := pinaxcloud.ClassifyKeyID(snapshot.ManifestKeyID, active, legacy)
	projection.Facts["remote_state"] = "present"
	projection.Facts["remote_derivation"] = class
	projection.Facts["remote_manifest_key_id"] = snapshot.ManifestKeyID
	projection.Facts["remote_manifest_blob_id"] = snapshot.ManifestBlobID
	projection.Facts["reencryption_required"] = fmt.Sprint(class != "v2")
	switch class {
	case "v2":
		projection.Summary = "Remote is fully encrypted with the v2 derivation."
	case "legacy":
		projection.Summary = "Remote manifest still uses the legacy derivation; push to re-encrypt."
		projection.Actions = []domain.Action{{Name: "reencrypt", Command: fmt.Sprintf("pinax sync push --target %s --vault %s --yes --json", syncConfigCommand(state.Config.Endpoint), shellQuote(root))}}
	default:
		projection.Summary = "Remote manifest key does not match this vault's derivations."
	}
	if data, ok := projection.Data.(map[string]any); ok {
		data["remote_derivation"] = class
		data["remote_manifest_key_id"] = snapshot.ManifestKeyID
		data["remote_manifest_blob_id"] = snapshot.ManifestBlobID
		data["remote_revision_id"] = snapshot.RevisionID
		data["reencryption_required"] = class != "v2"
	}
	return projection, nil
}
