package app

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/internal/records"
)

// Written before replacement. No body, template or caller prompt is retained.
type dailyEditResult struct {
	NoteID        string `json:"note_id"`
	Path          string `json:"path"`
	ContentDigest string `json:"content_digest"`
	RecordDigest  string `json:"record_digest"`
}

func (s *Service) dailyEditReconcileEvidence(ctx context.Context, root string, row operation.OperationRow) (operation.ReconcileEvidence, error) {
	var empty operation.ReconcileEvidence
	if row.CapabilityID != writerDailyEditCapability {
		return empty, nil
	}
	if err := dailyStorageSafe(ctx, root); err != nil {
		return empty, err
	}
	var proof dailyEditResult
	if json.Unmarshal(row.ResultJSON, &proof) != nil || proof.NoteID == "" || !readerDailyVisiblePath(proof.Path) || !strings.HasPrefix(row.ReceiptRef, "snapshot:") {
		return empty, nil
	}
	hash, err := hex.DecodeString(strings.TrimPrefix(proof.ContentDigest, "sha256:"))
	if err != nil || len(hash) != 32 || !strings.HasPrefix(proof.ContentDigest, "sha256:") || proof.RecordDigest == "" {
		return empty, nil
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		return empty, err
	}
	defer func() { _ = opened.Close() }()
	raw, err := readWriterEditorCandidate(opened, proof.Path)
	if err != nil {
		return empty, nil
	}
	if len(raw) > CollaborationMaxBodyBytes || collaborationRevision(string(raw)) != proof.ContentDigest {
		return empty, nil
	}
	note := parseNote(proof.Path, string(raw))
	if note.ID != proof.NoteID || note.Kind != "daily" {
		return empty, nil
	}
	event, found, err := records.NewService(root).FindEventByIdempotency(ctx, row.OperationID)
	if err != nil {
		return empty, err
	}
	if !found || event.Kind != domain.RecordEventNoteMetadataUpdated || event.NoteID != proof.NoteID || filepath.ToSlash(event.Path) != proof.Path || event.ContentRevision.Hash != proof.RecordDigest {
		return empty, nil
	}
	result, _ := json.Marshal(proof)
	receipt, _ := json.Marshal(map[string]string{"operation_id": row.OperationID, "snapshot": row.ReceiptRef, "record_event_id": event.EventID, "proof": "fixed_source_and_original_record"})
	return operation.ReconcileEvidence{Applied: true, ProofRef: "record:" + event.EventID, Result: result, Receipt: receipt, ReceiptRef: row.ReceiptRef, ResourceRef: "pinax://note/" + proof.NoteID, RevisionAfter: proof.ContentDigest}, nil
}
