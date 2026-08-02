package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestKBReviewOverviewDistinguishesFreshActiveGeneration(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	note := "---\nschema_version: pinax.note.v1\nnote_id: note_review_state\ntitle: Review State\nkind: reference\nstatus: active\n---\n\n# State\n\nThe review projection reports a fresh active generation.\n"
	if err := os.WriteFile(filepath.Join(root, "notes", "state.md"), []byte(note), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	notes, err := scanNotes(root)
	if err != nil {
		t.Fatalf("scan notes: %v", err)
	}
	manifest := validKBGenerationManifest()
	manifest.SourceDigest = kbSourceDigest(notes)
	manifest.SourceSnapshot = kbSourceSnapshot(manifest.SourceDigest)
	if _, err := WriteKBGenerationManifest(root, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	storePath := filepath.Join(root, ".pinax", "kb", "generations", manifest.GenerationID, "lancedb")
	if err := os.MkdirAll(storePath, 0o700); err != nil {
		t.Fatalf("mkdir store: %v", err)
	}
	ref := validKBActivationRef(manifest.GenerationID)
	ref.SourceDigest = manifest.SourceDigest
	ref.SourceSnapshot = manifest.SourceSnapshot
	ref.GenerationManifestHash = HashKBGenerationManifest(manifest)
	if err := CommitKBActivation(root, 0, KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: ref, ActivatedAt: "2026-08-02T00:00:00Z"}); err != nil {
		t.Fatalf("commit activation: %v", err)
	}
	projection, err := NewService().KBReviewOverview(context.Background(), KBReviewRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("review overview: %v", err)
	}
	if projection.Facts["index_status"] != "fresh" || projection.Facts["active_generation"] != manifest.GenerationID || projection.Facts["rollback_available"] != "false" {
		t.Fatalf("review facts = %#v", projection.Facts)
	}
	if projection.Facts["protocol"] != "inferrum.sidecar.v1" || projection.Facts["provider"] != manifest.Provider {
		t.Fatalf("review identity facts = %#v", projection.Facts)
	}
}
