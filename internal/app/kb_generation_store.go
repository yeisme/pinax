package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

const kbActivationLockStaleAfter = 10 * time.Minute

func kbRoot(root string) string {
	return filepath.Join(root, ".pinax", "kb")
}

func kbGenerationManifestPath(root, generationID string) string {
	return filepath.Join(kbRoot(root), "generations", generationID, "generation.json")
}

func kbEvaluationReceiptPath(root, runID string) string {
	return filepath.Join(kbRoot(root), "evaluations", runID, "receipt.json")
}

// WriteKBGenerationManifest writes one immutable generation manifest. A
// repeated write of byte-equivalent identity is idempotent; a changed write
// is rejected so a generation directory can never be repurposed.
func WriteKBGenerationManifest(root string, manifest KBGenerationManifest) (string, error) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	path := kbGenerationManifestPath(root, manifest.GenerationID)
	if existing, err := ReadKBGenerationManifest(root, manifest.GenerationID); err == nil {
		if existing == manifest {
			return path, nil
		}
		return "", &domain.CommandError{Code: "kb_generation_immutable", Message: "KB generation manifest already exists with different identity", Hint: "Use a new generation id instead of replacing an existing generation"}
	} else if !errors.Is(err, os.ErrNotExist) {
		var cmdErr *domain.CommandError
		if errors.As(err, &cmdErr) {
			return "", err
		}
		return "", err
	}
	if err := atomicKBJSONWrite(path, manifest); err != nil {
		if errors.Is(err, os.ErrExist) {
			return WriteKBGenerationManifest(root, manifest)
		}
		return "", err
	}
	return path, nil
}

func ReadKBGenerationManifest(root, generationID string) (KBGenerationManifest, error) {
	if !validKBToken(generationID) {
		return KBGenerationManifest{}, invalidKBGenerationManifest("generation_id is invalid")
	}
	payload, err := os.ReadFile(kbGenerationManifestPath(root, generationID))
	if err != nil {
		return KBGenerationManifest{}, err
	}
	var manifest KBGenerationManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return KBGenerationManifest{}, invalidKBGenerationManifest("generation.json is not valid JSON")
	}
	if manifest.GenerationID != generationID {
		return KBGenerationManifest{}, invalidKBGenerationManifest("generation.json id does not match its directory")
	}
	if err := manifest.Validate(); err != nil {
		return KBGenerationManifest{}, err
	}
	return manifest, nil
}

func WriteKBEvaluationReceipt(root string, receipt KBEvaluationReceipt) (string, error) {
	if err := ValidateKBEvaluationReceipt(receipt); err != nil {
		return "", err
	}
	path := kbEvaluationReceiptPath(root, receipt.RunID)
	if existing, err := ReadKBEvaluationReceipt(root, receipt.RunID); err == nil {
		if reflect.DeepEqual(existing, receipt) {
			return path, nil
		}
		return "", &domain.CommandError{Code: "kb_evaluation_receipt_immutable", Message: "KB evaluation receipt already exists with different identity", Hint: "Use a new run id instead of replacing a completed receipt"}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := atomicKBJSONWrite(path, receipt); err != nil {
		if errors.Is(err, os.ErrExist) {
			return WriteKBEvaluationReceipt(root, receipt)
		}
		return "", err
	}
	return path, nil
}

func ReadKBEvaluationReceipt(root, runID string) (KBEvaluationReceipt, error) {
	if !validKBToken(runID) {
		return KBEvaluationReceipt{}, invalidKBEvaluationReceipt("run_id is invalid")
	}
	payload, err := os.ReadFile(kbEvaluationReceiptPath(root, runID))
	if err != nil {
		return KBEvaluationReceipt{}, err
	}
	var receipt KBEvaluationReceipt
	if err := json.Unmarshal(payload, &receipt); err != nil {
		return KBEvaluationReceipt{}, invalidKBEvaluationReceipt("receipt.json is not valid JSON")
	}
	if receipt.RunID != runID {
		return KBEvaluationReceipt{}, invalidKBEvaluationReceipt("receipt run id does not match its directory")
	}
	if err := ValidateKBEvaluationReceipt(receipt); err != nil {
		return KBEvaluationReceipt{}, err
	}
	return receipt, nil
}

// ReadKBActivationDescriptor returns the empty sequence-zero descriptor when
// the vault has not activated a generation yet.
func ReadKBActivationDescriptor(root string) (KBActivationDescriptor, error) {
	payload, err := os.ReadFile(filepath.Join(kbRoot(root), "activation.json"))
	if errors.Is(err, os.ErrNotExist) {
		return KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema}, nil
	}
	if err != nil {
		return KBActivationDescriptor{}, err
	}
	var descriptor KBActivationDescriptor
	if err := json.Unmarshal(payload, &descriptor); err != nil {
		return KBActivationDescriptor{}, invalidKBActivation("activation.json is not valid JSON")
	}
	if err := descriptor.Validate(); err != nil {
		return KBActivationDescriptor{}, err
	}
	return descriptor, nil
}

// CommitKBActivation performs a vault-scoped lock plus expected-sequence CAS
// and atomically replaces activation.json with a complete descriptor.
func CommitKBActivation(root string, expectedSequence uint64, next KBActivationDescriptor) error {
	if err := next.Validate(); err != nil {
		return err
	}
	if next.Sequence != expectedSequence+1 {
		return &domain.CommandError{Code: "kb_activation_sequence_invalid", Message: "KB activation sequence must advance exactly once", Hint: "Read the current activation descriptor and retry with the next sequence"}
	}
	lockPath := filepath.Join(kbRoot(root), "activation.lock")
	if err := acquireKBActivationLock(lockPath); err != nil {
		return err
	}
	defer func() { _ = os.Remove(lockPath) }()

	current, err := ReadKBActivationDescriptor(root)
	if err != nil {
		return err
	}
	if current.Sequence != expectedSequence {
		return &domain.CommandError{Code: "kb_activation_sequence_conflict", Message: "KB activation sequence is stale", Hint: "Refresh the activation descriptor before activating or rolling back"}
	}
	return atomicKBJSONWrite(filepath.Join(kbRoot(root), "activation.json"), next)
}

// ActivateKBCandidate is the candidate-bound transition used by the future
// evaluation command. It validates the exact generation/evaluation identity
// before constructing the one authoritative active+previous descriptor.
func ActivateKBCandidate(root string, expectedSequence uint64, manifest KBGenerationManifest, suite KBEvaluationSuite, receipt KBEvaluationReceipt, gateConfigHash, activatedAt string) error {
	if manifest.Status != KBGenerationStatusReady {
		return invalidKBEvaluationGate("only a ready generation may be activated")
	}
	if err := ValidateKBEvaluationReceiptForCandidate(receipt, manifest, suite, gateConfigHash); err != nil {
		return err
	}
	current, err := ReadKBActivationDescriptor(root)
	if err != nil {
		return err
	}
	ref := &KBActivationRef{
		GenerationID:           manifest.GenerationID,
		Protocol:               manifest.Protocol,
		Provider:               manifest.Provider,
		Model:                  manifest.Model,
		BaseModelDigest:        manifest.BaseModelDigest,
		ModelManifestDigest:    manifest.ModelManifestDigest,
		ProfileHash:            manifest.ProfileHash,
		DaemonVersion:          manifest.DaemonVersion,
		EmbeddingDim:           manifest.EmbeddingDim,
		SourceSnapshot:         manifest.SourceSnapshot,
		SourceDigest:           manifest.SourceDigest,
		GenerationManifestHash: HashKBGenerationManifest(manifest),
		EvaluationReceiptHash:  HashKBEvaluationReceipt(receipt),
	}
	next := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: expectedSequence + 1, Active: ref, ActivatedAt: activatedAt}
	if current.Active != nil {
		next.Previous = current.Active
	}
	return CommitKBActivation(root, expectedSequence, next)
}

func RollbackKBActivation(root string, expectedSequence uint64, activatedAt string) error {
	current, err := ReadKBActivationDescriptor(root)
	if err != nil {
		return err
	}
	if current.Active == nil || current.Previous == nil {
		return &domain.CommandError{Code: "kb_activation_previous_unavailable", Message: "KB previous generation is not available for rollback", Hint: "Keep an active generation and previous generation before requesting rollback"}
	}
	next := KBActivationDescriptor{
		SchemaVersion: KBActivationDescriptorSchema,
		Sequence:      expectedSequence + 1,
		Active:        current.Previous,
		Previous:      current.Active,
		ActivatedAt:   activatedAt,
	}
	return CommitKBActivation(root, expectedSequence, next)
}

func acquireKBActivationLock(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return acquireKBActivationLockAttempt(path, false)
}

func acquireKBActivationLockAttempt(path string, recovered bool) error {
	payload := []byte(fmt.Sprintf("{\"pid\":%d,\"acquired_at\":%q}\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339)))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			if !recovered && staleKBActivationLock(path) {
				if removeErr := os.Remove(path); removeErr == nil || errors.Is(removeErr, os.ErrNotExist) {
					return acquireKBActivationLockAttempt(path, true)
				}
			}
			return &domain.CommandError{Code: "kb_activation_lock_held", Message: "KB activation is already being mutated", Hint: "Retry after the current activation or rollback finishes"}
		}
		return err
	}
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	return file.Close()
}

func staleKBActivationLock(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if time.Since(info.ModTime()) < kbActivationLockStaleAfter {
		return false
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	var lock struct {
		PID        int    `json:"pid"`
		AcquiredAt string `json:"acquired_at"`
	}
	if json.Unmarshal(payload, &lock) != nil || lock.PID <= 0 || strings.TrimSpace(lock.AcquiredAt) == "" {
		return true
	}
	acquired, err := time.Parse(time.RFC3339, lock.AcquiredAt)
	if err != nil {
		return true
	}
	return time.Since(acquired) >= kbActivationLockStaleAfter
}

func atomicKBJSONWrite(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	removeTemp := true
	defer func() {
		_ = file.Close()
		if removeTemp {
			_ = os.Remove(tmp)
		}
	}()
	if err := runKBAtomicWriteFailure("write"); err != nil {
		return err
	}
	if _, err := file.Write(payload); err != nil {
		return err
	}
	if err := runKBAtomicWriteFailure("file_sync"); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := runKBAtomicWriteFailure("file_close"); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := runKBAtomicWriteFailure("rename"); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	removeTemp = false
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

// kbAtomicWriteFailure is intentionally package-private and nil in normal
// operation. Tests use it to exercise the pre-rename failure boundaries
// without replacing filesystem primitives or mutating production state.
var kbAtomicWriteFailure func(stage string) error

func runKBAtomicWriteFailure(stage string) error {
	if kbAtomicWriteFailure == nil {
		return nil
	}
	return kbAtomicWriteFailure(stage)
}
