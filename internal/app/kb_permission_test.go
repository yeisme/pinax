package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/semantic"
)

func TestKBSearchExplicitEmptyPermissionIsFailClosed(t *testing.T) {
	projection, err := (&Service{}).KBSearch(context.Background(), KBIndexRequest{
		VaultPath:          t.TempDir(),
		Backend:            semantic.DefaultBackend,
		Provider:           "fake",
		Model:              semantic.FakeProviderModel,
		Query:              "private query",
		PermissionResolved: true,
		AllowedIDs:         []string{},
	})
	if err != nil {
		t.Fatalf("explicit empty permission should return zero results: %v", err)
	}
	if got := projection.Facts["matches"]; got != "0" {
		t.Fatalf("matches = %q, want 0; projection=%#v", got, projection)
	}
	if got := projection.Facts["total"]; got != "0" {
		t.Fatalf("total = %q, want 0; projection=%#v", got, projection)
	}
}

func TestKBSearchRejectsUnresolvedPermission(t *testing.T) {
	_, err := (&Service{}).KBSearch(context.Background(), KBIndexRequest{
		VaultPath:          t.TempDir(),
		Backend:            semantic.DefaultBackend,
		Provider:           "fake",
		Model:              semantic.FakeProviderModel,
		Query:              "private query",
		PermissionResolved: true,
		AllowedIDs:         nil,
	})
	var cmdErr *domain.CommandError
	if !errors.As(err, &cmdErr) || cmdErr.Code != "permission_unresolved" {
		t.Fatalf("error = %#v, want permission_unresolved", err)
	}
}

func TestKBSearchResolvesEmptyPersonalVaultBeforeSidecar(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(t.TempDir(), "sidecar-called")
	bin := filepath.Join(t.TempDir(), "inferrum-lancedb-sidecar")
	script := "#!/bin/sh\ntouch " + marker + "\nprintf '%s\\n' '{\"schema_version\":\"inferrum.sidecar.v1\",\"status\":\"success\",\"backend\":\"lancedb\",\"total\":0}'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sidecar: %v", err)
	}

	projection, err := (&Service{}).KBSearch(context.Background(), KBIndexRequest{
		VaultPath:         root,
		Backend:           semantic.DefaultBackend,
		Provider:          "fake",
		Model:             semantic.FakeProviderModel,
		Query:             "private query",
		SidecarExecutable: bin,
	})
	if err != nil {
		t.Fatalf("empty personal vault should return zero results: %v", err)
	}
	if projection.Facts["permission_status"] != "empty" || projection.Facts["matches"] != "0" {
		t.Fatalf("projection = %#v, want permission_status=empty and zero matches", projection)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("empty personal permission set invoked the sidecar")
	}
}

func TestKBSearchRejectsPinnedGenerationIdentityDrift(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	note := "---\nschema_version: pinax.note.v1\nnote_id: note_identity\ntitle: Identity\nkind: reference\nstatus: active\n---\n\n# Identity\n\nPinned model identity must remain stable.\n"
	if err := os.WriteFile(filepath.Join(root, "notes", "identity.md"), []byte(note), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	manifest := validKBGenerationManifest()
	manifest.GenerationID = "gen-identity-drift"
	manifest.Provider = "fake"
	manifest.Model = semantic.FakeProviderModel
	manifest.BaseModelDigest = ""
	manifest.ModelManifestDigest = "sha256:wrong"
	manifest.ProfileHash = "sha256:wrong-profile"
	manifest.EmbeddingDim = 32
	manifest.Documents = 1
	manifest.Chunks = 1
	manifest.RowCount = 1
	if _, err := WriteKBGenerationManifest(root, manifest); err != nil {
		t.Fatalf("write generation manifest: %v", err)
	}
	ref := validKBActivationRef(manifest.GenerationID)
	ref.Provider = manifest.Provider
	ref.Model = manifest.Model
	ref.BaseModelDigest = ""
	ref.ModelManifestDigest = manifest.ModelManifestDigest
	ref.ProfileHash = manifest.ProfileHash
	ref.EmbeddingDim = manifest.EmbeddingDim
	ref.GenerationManifestHash = HashKBGenerationManifest(manifest)
	if err := CommitKBActivation(root, 0, KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: ref, ActivatedAt: "2026-08-02T00:00:00Z"}); err != nil {
		t.Fatalf("commit active generation: %v", err)
	}
	_, err := (&Service{}).KBSearch(context.Background(), KBIndexRequest{VaultPath: root, Backend: semantic.DefaultBackend, Query: "identity", SidecarExecutable: filepath.Join(root, "missing-sidecar")})
	var cmdErr *domain.CommandError
	if !errors.As(err, &cmdErr) || cmdErr.Code != "kb_model_mismatch" {
		t.Fatalf("identity drift error = %#v, want kb_model_mismatch", err)
	}
}
