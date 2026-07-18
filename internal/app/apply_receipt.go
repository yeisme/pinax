package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/records"
)

func writeObjectApplyReceipt(ctx context.Context, root, command, planID, snapshotID string, before map[string]managedObjectPlanBinding, changedPaths []string) (domain.ApplyReceipt, error) {
	afterByPathBindings, err := managedNotePlanBindings(ctx, root)
	if err != nil {
		return domain.ApplyReceipt{}, err
	}
	after := managedBindingsByObject(afterByPathBindings)
	beforeByPath := make(map[string]managedObjectPlanBinding, len(before))
	afterByPath := make(map[string]managedObjectPlanBinding, len(after))
	for _, binding := range before {
		beforeByPath[binding.ObservedPath] = binding
	}
	for _, binding := range after {
		afterByPath[binding.ObservedPath] = binding
	}
	selected := map[string]struct{}{}
	for _, path := range changedPaths {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if binding, ok := beforeByPath[path]; ok {
			selected[binding.ObjectID] = struct{}{}
		}
		if binding, ok := afterByPath[path]; ok {
			selected[binding.ObjectID] = struct{}{}
		}
	}
	for objectID := range before {
		if _, ok := after[objectID]; ok && before[objectID].ObservedPath != after[objectID].ObservedPath {
			selected[objectID] = struct{}{}
		}
	}
	ledger, ledgerErr := records.NewService(root).ReplayReadOnly(ctx)
	if ledgerErr != nil {
		return domain.ApplyReceipt{}, ledgerErr
	}
	objectIDs := make([]string, 0, len(selected))
	for objectID := range selected {
		objectIDs = append(objectIDs, objectID)
	}
	sort.Strings(objectIDs)
	objects := make([]domain.ApplyReceiptObject, 0, len(objectIDs))
	for _, objectID := range objectIDs {
		beforeBinding := before[objectID]
		afterBinding := after[objectID]
		object := domain.ApplyReceiptObject{ObjectID: objectID, ObjectKind: "note", BeforePath: beforeBinding.ObservedPath, AfterPath: afterBinding.ObservedPath, ExpectedRecordVersion: beforeBinding.ExpectedRecordVersion, BeforeRevision: beforeBinding.ExpectedContentRevision, AfterRevision: afterBinding.ExpectedContentRevision}
		if record, ok := ledger.Records[objectID]; ok {
			object.AfterRecordVersion = record.RecordVersion
		}
		objects = append(objects, object)
	}
	changedPaths = normalizedReceiptPaths(changedPaths)
	audit, auditErr := buildSyncManifestIdentityAudit(root, "receipt-device")
	syncReady := auditErr == nil && audit.Eligible
	syncReason := "identity_ready"
	if !syncReady {
		syncReason = "manifest_identity_migration_required"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	receiptID := "apply-" + shortSyncManifestDigest(command+"\x00"+planID+"\x00"+now)
	rel := filepath.ToSlash(filepath.Join(".pinax", "receipts", receiptID+".json"))
	receipt := domain.ApplyReceipt{SchemaVersion: domain.ApplyReceiptSchemaVersion, ReceiptID: receiptID, Command: command, PlanID: planID, Status: "applied", SnapshotID: snapshotID, LedgerSeq: ledger.Version.LastSeq, ChangedPaths: changedPaths, Objects: objects, SyncReady: syncReady, SyncReason: syncReason, SavedPath: rel, CreatedAt: now}
	if err := writeJSONAsset(filepath.Join(root, filepath.FromSlash(rel)), receipt); err != nil {
		return domain.ApplyReceipt{}, err
	}
	return receipt, nil
}

func addApplyReceiptProjection(projection *domain.Projection, receipt domain.ApplyReceipt) {
	if projection == nil {
		return
	}
	projection.Facts["receipt_id"] = receipt.ReceiptID
	projection.Facts["ledger_seq"] = fmt.Sprint(receipt.LedgerSeq)
	projection.Facts["changed_paths"] = fmt.Sprint(len(receipt.ChangedPaths))
	projection.Facts["objects"] = fmt.Sprint(len(receipt.Objects))
	projection.Facts["sync_ready"] = fmt.Sprint(receipt.SyncReady)
	projection.Facts["sync_reason"] = receipt.SyncReason
	if receipt.SnapshotID != "" {
		projection.Facts["snapshot_id"] = receipt.SnapshotID
	}
	projection.Evidence = append(projection.Evidence, receipt.SavedPath)
}

func managedBindingsByObject(bindings map[string]managedObjectPlanBinding) map[string]managedObjectPlanBinding {
	result := make(map[string]managedObjectPlanBinding, len(bindings))
	for _, binding := range bindings {
		result[binding.ObjectID] = binding
	}
	return result
}

func normalizedReceiptPaths(paths []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}
