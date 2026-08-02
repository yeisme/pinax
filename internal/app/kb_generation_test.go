package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

func TestKBGenerationManifestValidatesSafeImmutableIdentity(t *testing.T) {
	manifest := validKBGenerationManifest()
	if err := manifest.Validate(); err != nil {
		t.Fatalf("valid generation manifest rejected: %v", err)
	}

	unsafe := manifest
	unsafe.SourceSnapshot = "/private/vault/snapshot"
	var cmdErr *domain.CommandError
	if !errors.As(unsafe.Validate(), &cmdErr) || cmdErr.Code != "kb_generation_manifest_invalid" {
		t.Fatalf("unsafe source snapshot error = %#v, want kb_generation_manifest_invalid", unsafe.Validate())
	}

	missing := manifest
	missing.EmbeddingDim = 0
	if !errors.As(missing.Validate(), &cmdErr) || cmdErr.Code != "kb_generation_manifest_invalid" {
		t.Fatalf("missing dimension error = %#v, want kb_generation_manifest_invalid", missing.Validate())
	}
}

func TestKBActivationDescriptorUsesOneAuthoritativePreviousRef(t *testing.T) {
	descriptor := KBActivationDescriptor{
		SchemaVersion: KBActivationDescriptorSchema,
		Sequence:      4,
		Active:        validKBActivationRef("gen-active"),
		Previous:      validKBActivationRef("gen-previous"),
		ActivatedAt:   "2026-08-01T00:00:00Z",
	}
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("valid activation descriptor rejected: %v", err)
	}
	payload, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatalf("marshal activation descriptor: %v", err)
	}
	text := string(payload)
	for _, forbidden := range []string{"previous.json", "body", "vector", "raw_prompt", "secret", "absolute"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("activation descriptor contains forbidden sentinel %q: %s", forbidden, text)
		}
	}
	if strings.Count(text, `"previous"`) != 1 {
		t.Fatalf("activation descriptor should contain one previous ref: %s", text)
	}
}

func TestKBActivationDescriptorRejectsPreviousWithoutActive(t *testing.T) {
	descriptor := KBActivationDescriptor{
		SchemaVersion: KBActivationDescriptorSchema,
		Sequence:      1,
		Previous:      validKBActivationRef("gen-previous"),
	}
	var cmdErr *domain.CommandError
	if !errors.As(descriptor.Validate(), &cmdErr) || cmdErr.Code != "kb_activation_invalid" {
		t.Fatalf("previous without active error = %#v, want kb_activation_invalid", descriptor.Validate())
	}
}

func TestKBGenerationStoreKeepsManifestImmutable(t *testing.T) {
	root := t.TempDir()
	manifest := validKBGenerationManifest()
	path, err := WriteKBGenerationManifest(root, manifest)
	if err != nil {
		t.Fatalf("write generation manifest: %v", err)
	}
	if want := filepath.Join(root, ".pinax", "kb", "generations", manifest.GenerationID, "generation.json"); path != want {
		t.Fatalf("manifest path = %q, want %q", path, want)
	}
	got, err := ReadKBGenerationManifest(root, manifest.GenerationID)
	if err != nil {
		t.Fatalf("read generation manifest: %v", err)
	}
	if got != manifest {
		t.Fatalf("read manifest = %#v, want %#v", got, manifest)
	}

	changed := manifest
	changed.Status = KBGenerationStatusActive
	if _, err := WriteKBGenerationManifest(root, changed); err == nil {
		t.Fatalf("rewriting an immutable generation with changed content should fail")
	}
}

func TestKBActivationCommitUsesSequenceCAS(t *testing.T) {
	root := t.TempDir()
	active := validKBActivationRef("gen-active")
	first := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: active, ActivatedAt: "2026-08-01T00:00:00Z"}
	if err := CommitKBActivation(root, 0, first); err != nil {
		t.Fatalf("initial activation commit: %v", err)
	}
	got, err := ReadKBActivationDescriptor(root)
	if err != nil {
		t.Fatalf("read activation descriptor: %v", err)
	}
	if got.Sequence != 1 || got.Active == nil || got.Active.GenerationID != "gen-active" {
		t.Fatalf("activation descriptor = %#v", got)
	}

	stale := first
	stale.Sequence = 2
	stale.Active = validKBActivationRef("gen-stale")
	if err := CommitKBActivation(root, 0, stale); err == nil {
		t.Fatalf("stale activation sequence should fail")
	}

	next := first
	next.Sequence = 2
	next.Previous = next.Active
	next.Active = validKBActivationRef("gen-next")
	next.ActivatedAt = "2026-08-01T00:01:00Z"
	if err := CommitKBActivation(root, 1, next); err != nil {
		t.Fatalf("second activation commit: %v", err)
	}
	got, err = ReadKBActivationDescriptor(root)
	if err != nil {
		t.Fatalf("read second activation descriptor: %v", err)
	}
	if got.Sequence != 2 || got.Active.GenerationID != "gen-next" || got.Previous.GenerationID != "gen-active" {
		t.Fatalf("second activation descriptor = %#v", got)
	}
}

func TestKBActivationRecoversStaleLockWithoutOverwritingFreshLock(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, ".pinax", "kb", "activation.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		t.Fatalf("mkdir activation dir: %v", err)
	}
	if err := os.WriteFile(lockPath, []byte(`{"pid":999999,"acquired_at":"2020-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}
	old := time.Now().Add(-kbActivationLockStaleAfter - time.Minute)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("age stale lock: %v", err)
	}
	active := validKBActivationRef("gen-stale-recovered")
	descriptor := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: active, ActivatedAt: "2026-08-01T00:00:00Z"}
	if err := CommitKBActivation(root, 0, descriptor); err != nil {
		t.Fatalf("commit after stale lock recovery: %v", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("activation lock should be released, stat err=%v", err)
	}
}

func TestKBActivationKeepsFreshLockHeld(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, ".pinax", "kb", "activation.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		t.Fatalf("mkdir activation dir: %v", err)
	}
	if err := os.WriteFile(lockPath, []byte(`{"pid":1234,"acquired_at":"`+time.Now().UTC().Format(time.RFC3339)+`"}`), 0o600); err != nil {
		t.Fatalf("write fresh lock: %v", err)
	}
	active := validKBActivationRef("gen-fresh-lock")
	descriptor := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: active, ActivatedAt: "2026-08-01T00:00:00Z"}
	var cmdErr *domain.CommandError
	if !errors.As(CommitKBActivation(root, 0, descriptor), &cmdErr) || cmdErr.Code != "kb_activation_lock_held" {
		t.Fatalf("fresh lock error = %#v, want kb_activation_lock_held", CommitKBActivation(root, 0, descriptor))
	}
}

func TestKBActivationConcurrentCASAllowsOnlyOneStaleWriter(t *testing.T) {
	root := t.TempDir()
	first := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: validKBActivationRef("gen-first-concurrent"), ActivatedAt: "2026-08-01T00:00:00Z"}
	var err1, err2 error
	done := make(chan struct{})
	go func() {
		err1 = CommitKBActivation(root, 0, first)
		close(done)
	}()
	err2 = CommitKBActivation(root, 0, KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: validKBActivationRef("gen-second-concurrent"), ActivatedAt: "2026-08-01T00:00:01Z"})
	<-done
	if (err1 == nil) == (err2 == nil) {
		t.Fatalf("concurrent stale writers should have exactly one success: err1=%v err2=%v", err1, err2)
	}
	descriptor, err := ReadKBActivationDescriptor(root)
	if err != nil {
		t.Fatalf("read concurrent descriptor: %v", err)
	}
	if descriptor.Sequence != 1 || descriptor.Active == nil {
		t.Fatalf("concurrent descriptor = %#v", descriptor)
	}
}

func TestKBActivationAtomicWriteFailuresPreservePreviousDescriptor(t *testing.T) {
	for _, stage := range []string{"write", "file_sync", "file_close", "rename"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			first := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: validKBActivationRef("gen-before-" + stage), ActivatedAt: "2026-08-01T00:00:00Z"}
			if err := CommitKBActivation(root, 0, first); err != nil {
				t.Fatalf("initial activation: %v", err)
			}
			before, err := os.ReadFile(filepath.Join(root, ".pinax", "kb", "activation.json"))
			if err != nil {
				t.Fatalf("read initial descriptor: %v", err)
			}
			originalHook := kbAtomicWriteFailure
			t.Cleanup(func() { kbAtomicWriteFailure = originalHook })
			kbAtomicWriteFailure = func(currentStage string) error {
				if currentStage == stage {
					return errors.New("injected atomic write failure")
				}
				return nil
			}
			next := first
			next.Sequence = 2
			next.Previous = first.Active
			next.Active = validKBActivationRef("gen-after-" + stage)
			err = CommitKBActivation(root, 1, next)
			if err == nil {
				t.Fatalf("injected %s failure should fail", stage)
			}
			after, readErr := os.ReadFile(filepath.Join(root, ".pinax", "kb", "activation.json"))
			if readErr != nil {
				t.Fatalf("read descriptor after %s failure: %v", stage, readErr)
			}
			if string(after) != string(before) {
				t.Fatalf("descriptor changed after %s failure: before=%s after=%s", stage, before, after)
			}
			entries, readDirErr := os.ReadDir(filepath.Join(root, ".pinax", "kb"))
			if readDirErr != nil {
				t.Fatalf("read KB directory: %v", readDirErr)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "activation.json.tmp-") {
					t.Fatalf("temporary activation file survived %s failure: %s", stage, entry.Name())
				}
			}
		})
	}
}

func TestKBRollbackAtomicWriteFailuresPreservePreviousDescriptor(t *testing.T) {
	for _, stage := range []string{"write", "file_sync", "file_close", "rename"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			first := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: validKBActivationRef("gen-rollback-before-" + stage), ActivatedAt: "2026-08-01T00:00:00Z"}
			if err := CommitKBActivation(root, 0, first); err != nil {
				t.Fatalf("initial activation: %v", err)
			}
			second := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 2, Active: validKBActivationRef("gen-rollback-active-" + stage), Previous: first.Active, ActivatedAt: "2026-08-01T00:01:00Z"}
			if err := CommitKBActivation(root, 1, second); err != nil {
				t.Fatalf("second activation: %v", err)
			}
			before, err := os.ReadFile(filepath.Join(root, ".pinax", "kb", "activation.json"))
			if err != nil {
				t.Fatalf("read descriptor before rollback: %v", err)
			}
			originalHook := kbAtomicWriteFailure
			t.Cleanup(func() { kbAtomicWriteFailure = originalHook })
			kbAtomicWriteFailure = func(currentStage string) error {
				if currentStage == stage {
					return errors.New("injected rollback atomic write failure")
				}
				return nil
			}
			if err := RollbackKBActivation(root, 2, "2026-08-01T00:02:00Z"); err == nil {
				t.Fatalf("injected %s rollback failure should fail", stage)
			}
			after, err := os.ReadFile(filepath.Join(root, ".pinax", "kb", "activation.json"))
			if err != nil {
				t.Fatalf("read descriptor after rollback %s failure: %v", stage, err)
			}
			if string(after) != string(before) {
				t.Fatalf("descriptor changed after rollback %s failure: before=%s after=%s", stage, before, after)
			}
			assertNoKBActivationTempFiles(t, root, stage)
		})
	}
}

func TestKBActivationMixedRollbackCASAllowsOnlyOneStaleWriter(t *testing.T) {
	root := t.TempDir()
	first := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: validKBActivationRef("gen-mixed-first"), ActivatedAt: "2026-08-01T00:00:00Z"}
	if err := CommitKBActivation(root, 0, first); err != nil {
		t.Fatalf("first activation: %v", err)
	}
	second := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 2, Active: validKBActivationRef("gen-mixed-second"), Previous: first.Active, ActivatedAt: "2026-08-01T00:01:00Z"}
	if err := CommitKBActivation(root, 1, second); err != nil {
		t.Fatalf("second activation: %v", err)
	}
	next := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 3, Active: validKBActivationRef("gen-mixed-activate"), Previous: second.Active, ActivatedAt: "2026-08-01T00:03:00Z"}
	var rollbackErr error
	done := make(chan struct{})
	go func() {
		rollbackErr = RollbackKBActivation(root, 2, "2026-08-01T00:02:00Z")
		close(done)
	}()
	activateErr := CommitKBActivation(root, 2, next)
	<-done
	if (activateErr == nil) == (rollbackErr == nil) {
		t.Fatalf("mixed activation/rollback stale writers should have exactly one success: activate=%v rollback=%v", activateErr, rollbackErr)
	}
	descriptor, err := ReadKBActivationDescriptor(root)
	if err != nil {
		t.Fatalf("read mixed descriptor: %v", err)
	}
	if descriptor.Sequence != 3 || descriptor.Active == nil || descriptor.Previous == nil {
		t.Fatalf("mixed descriptor = %#v", descriptor)
	}
	if descriptor.Active.GenerationID != "gen-mixed-activate" && descriptor.Active.GenerationID != "gen-mixed-second" {
		t.Fatalf("mixed active generation = %q", descriptor.Active.GenerationID)
	}
}

func TestKBActivateApplicationAtomicWriteFailuresPreserveDescriptorAndReceipt(t *testing.T) {
	for _, stage := range []string{"write", "file_sync", "file_close", "rename"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			manifest := validKBGenerationManifest()
			manifest.GenerationID = "gen-service-" + stage
			suite := validKBEvaluationSuite()
			receipt := validKBEvaluationReceipt(manifest, suite)
			receipt.RunID = "run-service-" + stage
			receipt.GateConfigHash = kbEvaluationGateConfigHash()
			if _, err := WriteKBGenerationManifest(root, manifest); err != nil {
				t.Fatalf("write manifest: %v", err)
			}
			if _, err := WriteKBEvaluationReceipt(root, receipt); err != nil {
				t.Fatalf("write evaluation receipt: %v", err)
			}
			suitePath := filepath.Join(root, ".pinax", "kb", "evaluation-suites", "personal-canary.json")
			payload, err := json.Marshal(suite)
			if err != nil {
				t.Fatalf("marshal suite: %v", err)
			}
			if err := os.MkdirAll(filepath.Dir(suitePath), 0o700); err != nil {
				t.Fatalf("mkdir suite: %v", err)
			}
			if err := os.WriteFile(suitePath, payload, 0o600); err != nil {
				t.Fatalf("write suite: %v", err)
			}
			first := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: validKBActivationRef("gen-service-before-" + stage), ActivatedAt: "2026-08-01T00:00:00Z"}
			if err := CommitKBActivation(root, 0, first); err != nil {
				t.Fatalf("initial activation: %v", err)
			}
			before, err := os.ReadFile(filepath.Join(root, ".pinax", "kb", "activation.json"))
			if err != nil {
				t.Fatalf("read descriptor before activation: %v", err)
			}
			originalHook := kbAtomicWriteFailure
			t.Cleanup(func() { kbAtomicWriteFailure = originalHook })
			kbAtomicWriteFailure = func(currentStage string) error {
				if currentStage == stage {
					return errors.New("injected application activation atomic write failure")
				}
				return nil
			}
			expected := uint64(1)
			projection, err := NewService().KBActivate(context.Background(), KBActivateRequest{VaultPath: root, GenerationID: manifest.GenerationID, Suite: ".pinax/kb/evaluation-suites/personal-canary.json", RunID: receipt.RunID, ExpectedSequence: &expected, ActivatedAt: "2026-08-01T00:02:00Z"})
			if err == nil || projection.Error == nil {
				t.Fatalf("injected %s application activation failure should return an error projection: err=%v projection=%#v", stage, err, projection)
			}
			after, err := os.ReadFile(filepath.Join(root, ".pinax", "kb", "activation.json"))
			if err != nil {
				t.Fatalf("read descriptor after activation %s failure: %v", stage, err)
			}
			if string(after) != string(before) {
				t.Fatalf("descriptor changed after application activation %s failure: before=%s after=%s", stage, before, after)
			}
			if _, err := os.Stat(filepath.Join(root, ".pinax", "kb", "activation-receipts", "000002-activate.json")); !os.IsNotExist(err) {
				t.Fatalf("activation receipt should not be written after %s failure, stat err=%v", stage, err)
			}
			assertNoKBActivationTempFiles(t, root, stage)
		})
	}
}

func TestKBRollbackApplicationAtomicWriteFailuresPreserveDescriptorAndReceipt(t *testing.T) {
	for _, stage := range []string{"write", "file_sync", "file_close", "rename"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			first := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: validKBActivationRef("gen-app-rollback-before-" + stage), ActivatedAt: "2026-08-01T00:00:00Z"}
			if err := CommitKBActivation(root, 0, first); err != nil {
				t.Fatalf("initial activation: %v", err)
			}
			second := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 2, Active: validKBActivationRef("gen-app-rollback-active-" + stage), Previous: first.Active, ActivatedAt: "2026-08-01T00:01:00Z"}
			if err := CommitKBActivation(root, 1, second); err != nil {
				t.Fatalf("second activation: %v", err)
			}
			before, err := os.ReadFile(filepath.Join(root, ".pinax", "kb", "activation.json"))
			if err != nil {
				t.Fatalf("read descriptor before rollback: %v", err)
			}
			originalHook := kbAtomicWriteFailure
			t.Cleanup(func() { kbAtomicWriteFailure = originalHook })
			kbAtomicWriteFailure = func(currentStage string) error {
				if currentStage == stage {
					return errors.New("injected application rollback atomic write failure")
				}
				return nil
			}
			projection, err := NewService().KBRollback(context.Background(), KBRollbackRequest{VaultPath: root, ExpectedSequence: uint64Ptr(2), ActivatedAt: "2026-08-01T00:02:00Z"})
			if err == nil || projection.Error == nil {
				t.Fatalf("injected %s application rollback failure should return an error projection: err=%v projection=%#v", stage, err, projection)
			}
			after, err := os.ReadFile(filepath.Join(root, ".pinax", "kb", "activation.json"))
			if err != nil {
				t.Fatalf("read descriptor after rollback %s failure: %v", stage, err)
			}
			if string(after) != string(before) {
				t.Fatalf("descriptor changed after application rollback %s failure: before=%s after=%s", stage, before, after)
			}
			if _, err := os.Stat(filepath.Join(root, ".pinax", "kb", "activation-receipts", "000003-rollback.json")); !os.IsNotExist(err) {
				t.Fatalf("rollback receipt should not be written after %s failure, stat err=%v", stage, err)
			}
			assertNoKBActivationTempFiles(t, root, stage)
		})
	}
}

func assertNoKBActivationTempFiles(t *testing.T, root, stage string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".pinax", "kb"))
	if err != nil {
		t.Fatalf("read KB directory after %s failure: %v", stage, err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "activation.json.tmp-") {
			t.Fatalf("temporary activation file survived %s failure: %s", stage, entry.Name())
		}
	}
}

func uint64Ptr(value uint64) *uint64 {
	return &value
}

func TestKBActivateApplicationLoadsMatchingReceiptAndReportsSequence(t *testing.T) {
	root := t.TempDir()
	manifest := validKBGenerationManifest()
	suite := validKBEvaluationSuite()
	receipt := validKBEvaluationReceipt(manifest, suite)
	receipt.GateConfigHash = kbEvaluationGateConfigHash()
	if _, err := WriteKBGenerationManifest(root, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, err := WriteKBEvaluationReceipt(root, receipt); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
	suitePath := filepath.Join(root, ".pinax", "kb", "evaluation-suites", "personal-canary.json")
	payload, err := json.Marshal(suite)
	if err != nil {
		t.Fatalf("marshal suite: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(suitePath), 0o700); err != nil {
		t.Fatalf("mkdir suite: %v", err)
	}
	if err := os.WriteFile(suitePath, payload, 0o600); err != nil {
		t.Fatalf("write suite: %v", err)
	}
	projection, err := NewService().KBActivate(context.Background(), KBActivateRequest{VaultPath: root, GenerationID: manifest.GenerationID, Suite: ".pinax/kb/evaluation-suites/personal-canary.json", RunID: receipt.RunID})
	if err != nil {
		t.Fatalf("activate application: %v", err)
	}
	if projection.Command != "kb.activate" || projection.Facts["status"] != "active" || projection.Facts["activation_sequence"] != "1" {
		t.Fatalf("activation projection = %#v", projection)
	}
}

func TestKBRollbackApplicationWritesRedactedReceipt(t *testing.T) {
	root := t.TempDir()
	first := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: validKBActivationRef("gen-first"), ActivatedAt: "2026-08-01T00:00:00Z"}
	if err := CommitKBActivation(root, 0, first); err != nil {
		t.Fatalf("first activation: %v", err)
	}
	second := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 2, Active: validKBActivationRef("gen-second"), Previous: validKBActivationRef("gen-first"), ActivatedAt: "2026-08-01T00:01:00Z"}
	if err := CommitKBActivation(root, 1, second); err != nil {
		t.Fatalf("second activation: %v", err)
	}
	projection, err := NewService().KBRollback(context.Background(), KBRollbackRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("rollback application: %v", err)
	}
	if projection.Command != "kb.rollback" || projection.Facts["status"] != "rolled_back" || projection.Facts["activation_sequence"] != "3" {
		t.Fatalf("rollback projection = %#v", projection)
	}
	receiptPath := filepath.Join(root, ".pinax", "kb", "activation-receipts", "000003-rollback.json")
	payload, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatalf("read rollback receipt: %v", err)
	}
	for _, forbidden := range []string{"body", "vector", "secret", "absolute", "permission"} {
		if strings.Contains(strings.ToLower(string(payload)), forbidden) {
			t.Fatalf("rollback receipt leaked %q: %s", forbidden, payload)
		}
	}
}

func TestRollbackKBActivationSwapsActiveAndPreviousAtomically(t *testing.T) {
	root := t.TempDir()
	first := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: validKBActivationRef("gen-first"), ActivatedAt: "2026-08-01T00:00:00Z"}
	if err := CommitKBActivation(root, 0, first); err != nil {
		t.Fatalf("first activation: %v", err)
	}
	second := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 2, Active: validKBActivationRef("gen-second"), Previous: validKBActivationRef("gen-first"), ActivatedAt: "2026-08-01T00:01:00Z"}
	if err := CommitKBActivation(root, 1, second); err != nil {
		t.Fatalf("second activation: %v", err)
	}
	if err := RollbackKBActivation(root, 2, "2026-08-01T00:02:00Z"); err != nil {
		t.Fatalf("rollback activation: %v", err)
	}
	got, err := ReadKBActivationDescriptor(root)
	if err != nil {
		t.Fatalf("read rollback descriptor: %v", err)
	}
	if got.Sequence != 3 || got.Active.GenerationID != "gen-first" || got.Previous.GenerationID != "gen-second" {
		t.Fatalf("rollback descriptor = %#v", got)
	}
}

func validKBGenerationManifest() KBGenerationManifest {
	return KBGenerationManifest{
		SchemaVersion:       KBGenerationManifestSchema,
		GenerationID:        "gen-20260801-001",
		Status:              KBGenerationStatusReady,
		Protocol:            "inferrum.sidecar.v1",
		Backend:             "lancedb",
		Provider:            "ollama",
		Model:               "pinax-qwen3-embedding:lowmem",
		BaseModelDigest:     "sha256:base",
		ModelManifestDigest: "sha256:derived",
		ProfileHash:         "sha256:profile",
		SourceSnapshot:      "snapshot-001",
		SourceDigest:        "sha256:source",
		EmbeddingDim:        1024,
		Documents:           2,
		Chunks:              3,
		RowCount:            3,
		CreatedAt:           "2026-08-01T00:00:00Z",
	}
}

func validKBActivationRef(id string) *KBActivationRef {
	return &KBActivationRef{
		GenerationID:           id,
		Protocol:               "inferrum.sidecar.v1",
		Provider:               "ollama",
		Model:                  "pinax-qwen3-embedding:lowmem",
		ModelManifestDigest:    "sha256:derived",
		ProfileHash:            "sha256:profile",
		EmbeddingDim:           1024,
		SourceSnapshot:         "snapshot-001",
		SourceDigest:           "sha256:source",
		GenerationManifestHash: "sha256:manifest",
		EvaluationReceiptHash:  "sha256:evaluation",
	}
}
