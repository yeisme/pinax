package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

const KBActivationReceiptSchema = "pinax.kb.activation-receipt.v1"

type KBActivateRequest struct {
	VaultPath        string
	GenerationID     string
	Suite            string
	RunID            string
	ExpectedSequence *uint64
	ActivatedAt      string
}

type KBRollbackRequest struct {
	VaultPath        string
	ExpectedSequence *uint64
	ActivatedAt      string
}

type KBActivationReceipt struct {
	SchemaVersion         string `json:"schema_version"`
	Action                string `json:"action"`
	Sequence              uint64 `json:"sequence"`
	GenerationID          string `json:"generation_id"`
	PreviousGenerationID  string `json:"previous_generation_id,omitempty"`
	EvaluationReceiptHash string `json:"evaluation_receipt_hash,omitempty"`
	CreatedAt             string `json:"created_at"`
}

func (s *Service) KBActivate(_ context.Context, req KBActivateRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("kb.activate", err), err
	}
	generationID := strings.TrimSpace(req.GenerationID)
	if !validKBToken(generationID) {
		err := &domain.CommandError{Code: "kb_generation_id_invalid", Message: "KB generation id is invalid", Hint: "Use --generation <candidate-id>"}
		return domain.NewErrorProjection("kb.activate", err), err
	}
	suite, suiteRef, err := readKBEvaluationSuite(root, req.Suite)
	if err != nil {
		return commandErrorProjection("kb.activate", err)
	}
	manifest, err := ReadKBGenerationManifest(root, generationID)
	if err != nil {
		return commandErrorProjection("kb.activate", err)
	}
	runID := strings.TrimSpace(req.RunID)
	if !validKBToken(runID) {
		err := &domain.CommandError{Code: "kb_evaluation_run_required", Message: "KB evaluation run is required", Hint: "Pass the run id returned by pinax kb evaluate"}
		return domain.NewErrorProjection("kb.activate", err), err
	}
	receipt, err := ReadKBEvaluationReceipt(root, runID)
	if err != nil {
		return commandErrorProjection("kb.activate", err)
	}
	current, err := ReadKBActivationDescriptor(root)
	if err != nil {
		return commandErrorProjection("kb.activate", err)
	}
	expected := current.Sequence
	if req.ExpectedSequence != nil {
		expected = *req.ExpectedSequence
	}
	activatedAt := strings.TrimSpace(req.ActivatedAt)
	if activatedAt == "" {
		activatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := ActivateKBCandidate(root, expected, manifest, suite, receipt, kbEvaluationGateConfigHash(), activatedAt); err != nil {
		return commandErrorProjection("kb.activate", err)
	}
	descriptor, err := ReadKBActivationDescriptor(root)
	if err != nil {
		return commandErrorProjection("kb.activate", err)
	}
	previousID := ""
	if descriptor.Previous != nil {
		previousID = descriptor.Previous.GenerationID
	}
	receiptPath, receiptErr := writeKBActivationReceipt(root, KBActivationReceipt{SchemaVersion: KBActivationReceiptSchema, Action: "activate", Sequence: descriptor.Sequence, GenerationID: manifest.GenerationID, PreviousGenerationID: previousID, EvaluationReceiptHash: receiptHash(receipt), CreatedAt: activatedAt})
	projection := domain.NewProjection("kb.activate", "KB candidate generation activated.")
	projection.Facts["status"] = "active"
	projection.Facts["generation_id"] = manifest.GenerationID
	projection.Facts["generation_status"] = string(manifest.Status)
	projection.Facts["activation_sequence"] = fmt.Sprint(descriptor.Sequence)
	projection.Facts["provider"] = manifest.Provider
	projection.Facts["model"] = manifest.Model
	projection.Facts["protocol"] = manifest.Protocol
	projection.Facts["embedding_dim"] = fmt.Sprint(manifest.EmbeddingDim)
	projection.Facts["model_manifest_digest"] = manifest.ModelManifestDigest
	projection.Facts["profile_hash"] = manifest.ProfileHash
	projection.Facts["source_snapshot"] = manifest.SourceSnapshot
	projection.Facts["source_digest"] = manifest.SourceDigest
	projection.Facts["daemon_version"] = manifest.DaemonVersion
	if manifest.BaseModelDigest != "" {
		projection.Facts["base_model_digest"] = manifest.BaseModelDigest
	}
	projection.Facts["evaluation_receipt_hash"] = receiptHash(receipt)
	projection.Facts["previous_generation_id"] = previousID
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "kb", "activation.json")), receiptPath, suiteRef}
	projection.Data = map[string]any{"generation_id": manifest.GenerationID, "previous_generation_id": previousID, "activation_sequence": descriptor.Sequence, "evaluation_receipt_hash": receiptHash(receipt), "receipt_path": receiptPath}
	if receiptErr != nil {
		projection.Warnings = []domain.ProjectionWarning{{Code: "kb_activation_receipt_write_failed", Message: "KB activation committed but its local mutation receipt could not be written", Hint: receiptErr.Error()}}
	}
	return projection, nil
}

func (s *Service) KBRollback(_ context.Context, req KBRollbackRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("kb.rollback", err), err
	}
	current, err := ReadKBActivationDescriptor(root)
	if err != nil {
		return commandErrorProjection("kb.rollback", err)
	}
	expected := current.Sequence
	if req.ExpectedSequence != nil {
		expected = *req.ExpectedSequence
	}
	activatedAt := strings.TrimSpace(req.ActivatedAt)
	if activatedAt == "" {
		activatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := RollbackKBActivation(root, expected, activatedAt); err != nil {
		return commandErrorProjection("kb.rollback", err)
	}
	descriptor, err := ReadKBActivationDescriptor(root)
	if err != nil {
		return commandErrorProjection("kb.rollback", err)
	}
	previousID := ""
	if descriptor.Previous != nil {
		previousID = descriptor.Previous.GenerationID
	}
	receiptPath, receiptErr := writeKBActivationReceipt(root, KBActivationReceipt{SchemaVersion: KBActivationReceiptSchema, Action: "rollback", Sequence: descriptor.Sequence, GenerationID: descriptor.Active.GenerationID, PreviousGenerationID: previousID, CreatedAt: activatedAt})
	projection := domain.NewProjection("kb.rollback", "KB active generation rolled back.")
	projection.Facts["status"] = "rolled_back"
	projection.Facts["generation_id"] = descriptor.Active.GenerationID
	projection.Facts["activation_sequence"] = fmt.Sprint(descriptor.Sequence)
	projection.Facts["previous_generation_id"] = previousID
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "kb", "activation.json")), receiptPath}
	projection.Data = map[string]any{"generation_id": descriptor.Active.GenerationID, "previous_generation_id": previousID, "activation_sequence": descriptor.Sequence, "receipt_path": receiptPath}
	if receiptErr != nil {
		projection.Warnings = []domain.ProjectionWarning{{Code: "kb_activation_receipt_write_failed", Message: "KB rollback committed but its local mutation receipt could not be written", Hint: receiptErr.Error()}}
	}
	return projection, nil
}

func receiptHash(receipt KBEvaluationReceipt) string {
	return HashKBEvaluationReceipt(receipt)
}

func writeKBActivationReceipt(root string, receipt KBActivationReceipt) (string, error) {
	if receipt.SchemaVersion != KBActivationReceiptSchema || (receipt.Action != "activate" && receipt.Action != "rollback") || !validKBToken(receipt.GenerationID) || receipt.Sequence == 0 {
		return "", &domain.CommandError{Code: "kb_activation_receipt_invalid", Message: "KB activation receipt is invalid", Hint: "Activation must commit a complete generation descriptor before writing a receipt"}
	}
	path := filepath.Join(kbRoot(root), "activation-receipts", fmt.Sprintf("%06d-%s.json", receipt.Sequence, receipt.Action))
	if err := atomicKBJSONWrite(path, receipt); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(".pinax", "kb", "activation-receipts", fmt.Sprintf("%06d-%s.json", receipt.Sequence, receipt.Action))), nil
}
