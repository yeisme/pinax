package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/semantic"
)

func TestKBDoctorProjectsActiveGenerationIdentityAndFreshness(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "notes"))
	writeReviewTruthNote(t, root)
	notes, err := scanNotes(root)
	if err != nil {
		t.Fatalf("scan notes: %v", err)
	}
	manifest := validKBGenerationManifest()
	manifest.GenerationID = "gen-doctor"
	manifest.SourceDigest = kbSourceDigest(notes)
	manifest.SourceSnapshot = kbSourceSnapshot(manifest.SourceDigest)
	if _, err := WriteKBGenerationManifest(root, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	mustMkdir(t, filepath.Join(root, ".pinax", "kb", "generations", manifest.GenerationID, "lancedb"))
	ref := validKBActivationRef(manifest.GenerationID)
	ref.SourceDigest = manifest.SourceDigest
	ref.SourceSnapshot = manifest.SourceSnapshot
	ref.GenerationManifestHash = HashKBGenerationManifest(manifest)
	if err := CommitKBActivation(root, 0, KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: ref, ActivatedAt: "2026-08-02T00:00:00Z"}); err != nil {
		t.Fatalf("commit activation: %v", err)
	}

	projection, err := NewService().KBDoctor(context.Background(), KBIndexRequest{VaultPath: root, Backend: semantic.DefaultBackend, SidecarExecutable: writeStagingFakeSidecar(t)})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	for key, want := range map[string]string{
		"generation_id":         manifest.GenerationID,
		"generation_status":     string(manifest.Status),
		"index_status":          "fresh",
		"protocol":              manifest.Protocol,
		"provider":              manifest.Provider,
		"model":                 manifest.Model,
		"embedding_dim":         fmt.Sprint(manifest.EmbeddingDim),
		"source_snapshot":       manifest.SourceSnapshot,
		"source_digest":         manifest.SourceDigest,
		"model_manifest_digest": manifest.ModelManifestDigest,
		"profile_hash":          manifest.ProfileHash,
		"rollback_available":    "false",
	} {
		if projection.Facts[key] != want {
			t.Errorf("doctor facts[%q] = %q, want %q; facts=%#v", key, projection.Facts[key], want, projection.Facts)
		}
	}
	if projection.Facts["retrieval_ready"] != "true" {
		if _, present := projection.Facts["retrieval_ready"]; present {
			t.Fatalf("doctor inferred retrieval readiness: %#v", projection.Facts)
		}
	}
}

func TestKBDoctorKeepsGenerationFactsWhenSidecarIsUnavailable(t *testing.T) {
	root := t.TempDir()
	manifest := validKBGenerationManifest()
	manifest.GenerationID = "gen-doctor-error"
	if _, err := WriteKBGenerationManifest(root, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	ref := validKBActivationRef(manifest.GenerationID)
	ref.SourceDigest = manifest.SourceDigest
	ref.SourceSnapshot = manifest.SourceSnapshot
	ref.GenerationManifestHash = HashKBGenerationManifest(manifest)
	if err := CommitKBActivation(root, 0, KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: ref, ActivatedAt: "2026-08-02T00:00:00Z"}); err != nil {
		t.Fatalf("commit activation: %v", err)
	}

	projection, err := NewService().KBDoctor(context.Background(), KBIndexRequest{VaultPath: root, Backend: semantic.DefaultBackend, SidecarExecutable: filepath.Join(root, "missing-sidecar")})
	if err == nil || projection.Error == nil || projection.Error.Code != "kb_sidecar_unavailable" {
		t.Fatalf("doctor error = %v projection=%#v, want kb_sidecar_unavailable", err, projection)
	}
	if projection.Facts["generation_id"] != manifest.GenerationID || projection.Facts["index_status"] == "" || projection.Facts["rollback_available"] != "false" {
		t.Fatalf("doctor error lost generation facts: %#v", projection.Facts)
	}
}

func TestKBDoctorGenerationProjectionDoesNotExposeAbsoluteStorePath(t *testing.T) {
	root := t.TempDir()
	manifest := validKBGenerationManifest()
	manifest.GenerationID = "gen-doctor-redaction"
	if _, err := WriteKBGenerationManifest(root, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	ref := validKBActivationRef(manifest.GenerationID)
	ref.SourceDigest = manifest.SourceDigest
	ref.SourceSnapshot = manifest.SourceSnapshot
	ref.GenerationManifestHash = HashKBGenerationManifest(manifest)
	if err := CommitKBActivation(root, 0, KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: ref, ActivatedAt: "2026-08-02T00:00:00Z"}); err != nil {
		t.Fatalf("commit activation: %v", err)
	}
	projection, err := NewService().KBDoctor(context.Background(), KBIndexRequest{VaultPath: root, Backend: semantic.DefaultBackend, SidecarExecutable: writeStagingFakeSidecar(t)})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	payload := projection.Summary + projection.Facts["source_snapshot"] + projection.Facts["source_digest"]
	if strings.Contains(payload, root) || strings.Contains(payload, filepath.Join(root, ".pinax", "kb")) {
		t.Fatalf("doctor leaked absolute store path: %#v", projection)
	}
}
