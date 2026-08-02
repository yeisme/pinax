package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestKBCheckDiskAdmissionRejectsHighWaterWithoutOverride(t *testing.T) {
	original := kbStatfs
	t.Cleanup(func() { kbStatfs = original })
	kbStatfs = func(string) (kbFilesystemStats, error) {
		return kbFilesystemStats{Blocks: 100, FreeBlocks: 4, BlockSize: 1024}, nil
	}

	_, err := CheckKBDiskAdmission(t.TempDir(), 1, DefaultKBDiskAdmissionPolicy())
	var cmdErr *domain.CommandError
	if err == nil || !errors.As(err, &cmdErr) || cmdErr.Code != "kb_disk_high_water" {
		t.Fatalf("disk admission error = %v, want kb_disk_high_water", err)
	}
}

func TestKBCheckDiskAdmissionAllowsExplicitOverride(t *testing.T) {
	original := kbStatfs
	t.Cleanup(func() { kbStatfs = original })
	kbStatfs = func(string) (kbFilesystemStats, error) {
		return kbFilesystemStats{Blocks: 100, FreeBlocks: 4, BlockSize: 1024}, nil
	}

	policy := DefaultKBDiskAdmissionPolicy()
	policy.AllowHighWater = true
	result, err := CheckKBDiskAdmission(t.TempDir(), 1, policy)
	if err != nil {
		t.Fatalf("override should allow high-water disk: %v", err)
	}
	if !result.HighWater || !result.Override {
		t.Fatalf("disk admission result = %#v, want high-water override", result)
	}
}

func TestKBCheckDiskAdmissionRejectsStatFailure(t *testing.T) {
	original := kbStatfs
	t.Cleanup(func() { kbStatfs = original })
	kbStatfs = func(string) (kbFilesystemStats, error) { return kbFilesystemStats{}, errors.New("stat failed") }

	_, err := CheckKBDiskAdmission(t.TempDir(), 1, DefaultKBDiskAdmissionPolicy())
	var cmdErr *domain.CommandError
	if err == nil || !errors.As(err, &cmdErr) || cmdErr.Code != "kb_disk_stat_failed" {
		t.Fatalf("disk stat error = %v, want kb_disk_stat_failed", err)
	}
}

func TestKBRebuildRejectsDiskHighWaterBeforeProviderOrSidecar(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "disk.md"), []byte("---\nschema_version: pinax.note.v1\nnote_id: disk-note\ntitle: Disk\nkind: reference\nstatus: active\n---\n\n# Disk\n\nAdmission.\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	original := kbStatfs
	t.Cleanup(func() { kbStatfs = original })
	kbStatfs = func(string) (kbFilesystemStats, error) {
		return kbFilesystemStats{Blocks: 100, FreeBlocks: 4, BlockSize: 1024}, nil
	}
	policy := DefaultKBDiskAdmissionPolicy()
	_, err := NewService().KBRebuild(context.Background(), KBIndexRequest{
		VaultPath:         root,
		Backend:           "lancedb",
		Provider:          "fake",
		Model:             "fake-hash-v1",
		SidecarExecutable: filepath.Join(root, "missing-sidecar"),
		DiskAdmission:     &policy,
	})
	if err == nil || !strings.Contains(err.Error(), "kb_disk_high_water") {
		t.Fatalf("rebuild error = %v, want kb_disk_high_water", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".pinax", "kb", "generations")); statErr != nil {
		t.Fatalf("disk admission should leave generation evidence root: %v", statErr)
	}
}

func TestKBCheckDiskAdmissionRejectsGenerationBudget(t *testing.T) {
	root := t.TempDir()
	generationPath := filepath.Join(root, ".pinax", "kb", "generations", "gen-old")
	if err := os.MkdirAll(generationPath, 0o700); err != nil {
		t.Fatalf("mkdir generation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generationPath, "payload.bin"), []byte("12345"), 0o600); err != nil {
		t.Fatalf("write generation payload: %v", err)
	}
	original := kbStatfs
	t.Cleanup(func() { kbStatfs = original })
	kbStatfs = func(string) (kbFilesystemStats, error) {
		return kbFilesystemStats{Blocks: 100, FreeBlocks: 99, BlockSize: 1024}, nil
	}
	policy := DefaultKBDiskAdmissionPolicy()
	policy.GenerationBudgetBytes = 4
	_, err := CheckKBDiskAdmission(root, 1, policy)
	var cmdErr *domain.CommandError
	if err == nil || !errors.As(err, &cmdErr) || cmdErr.Code != "kb_generation_budget_exceeded" {
		t.Fatalf("generation budget error = %v, want kb_generation_budget_exceeded", err)
	}
}

func TestKBCheckDiskAdmissionRejectsEvidenceBudget(t *testing.T) {
	root := t.TempDir()
	evidencePath := filepath.Join(root, ".pinax", "kb", "evaluations", "run-old")
	if err := os.MkdirAll(evidencePath, 0o700); err != nil {
		t.Fatalf("mkdir evidence: %v", err)
	}
	if err := os.WriteFile(filepath.Join(evidencePath, "receipt.json"), []byte("12345"), 0o600); err != nil {
		t.Fatalf("write evidence: %v", err)
	}
	original := kbStatfs
	t.Cleanup(func() { kbStatfs = original })
	kbStatfs = func(string) (kbFilesystemStats, error) {
		return kbFilesystemStats{Blocks: 100, FreeBlocks: 99, BlockSize: 1024}, nil
	}
	policy := DefaultKBDiskAdmissionPolicy()
	policy.EvidenceBudgetBytes = 4
	_, err := CheckKBDiskAdmission(root, 1, policy)
	var cmdErr *domain.CommandError
	if err == nil || !errors.As(err, &cmdErr) || cmdErr.Code != "kb_evidence_budget_exceeded" {
		t.Fatalf("evidence budget error = %v, want kb_evidence_budget_exceeded", err)
	}
}

func TestKBCheckDiskAdmissionRejectsConfiguredModelCacheBudget(t *testing.T) {
	root := t.TempDir()
	modelPath := filepath.Join(root, "ollama-models")
	if err := os.MkdirAll(modelPath, 0o700); err != nil {
		t.Fatalf("mkdir model cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modelPath, "blob"), []byte("12345"), 0o600); err != nil {
		t.Fatalf("write model cache: %v", err)
	}
	original := kbStatfs
	t.Cleanup(func() { kbStatfs = original })
	kbStatfs = func(string) (kbFilesystemStats, error) {
		return kbFilesystemStats{Blocks: 100, FreeBlocks: 99, BlockSize: 1024}, nil
	}
	policy := DefaultKBDiskAdmissionPolicy()
	policy.ModelCachePath = modelPath
	policy.ModelCacheBudgetBytes = 4
	_, err := CheckKBDiskAdmission(root, 1, policy)
	var cmdErr *domain.CommandError
	if err == nil || !errors.As(err, &cmdErr) || cmdErr.Code != "kb_model_cache_budget_exceeded" {
		t.Fatalf("model cache budget error = %v, want kb_model_cache_budget_exceeded", err)
	}
}
