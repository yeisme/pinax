package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/semantic"
)

func TestKBRebuildStagesGenerationWithoutReplacingActivation(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	note := "---\nschema_version: pinax.note.v1\nnote_id: note_stage\ntitle: Staging Note\nkind: reference\nstatus: active\n---\n\n# Staging\n\nGeneration staging must not replace the active descriptor.\n"
	if err := os.WriteFile(filepath.Join(root, "notes", "stage.md"), []byte(note), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	sidecar := writeStagingFakeSidecar(t)
	projection, err := NewService().KBRebuild(context.Background(), KBIndexRequest{
		VaultPath:         root,
		Backend:           "lancedb",
		Provider:          "fake",
		Model:             "fake-hash-v1",
		SidecarExecutable: sidecar,
	})
	if err != nil {
		t.Fatalf("rebuild staging: %v", err)
	}
	if projection.Facts["generation_status"] != "ready" || projection.Facts["generation_id"] == "" {
		t.Fatalf("staging facts = %#v", projection.Facts)
	}
	generationID := projection.Facts["generation_id"]
	storePath := filepath.Join(root, ".pinax", "kb", "generations", generationID, "lancedb", "sidecar.jsonl")
	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("staged sidecar store missing: %v", err)
	}
	descriptor, err := ReadKBActivationDescriptor(root)
	if err != nil {
		t.Fatalf("read activation descriptor: %v", err)
	}
	if descriptor.Sequence != 0 || descriptor.Active != nil || descriptor.Previous != nil {
		t.Fatalf("rebuild changed activation descriptor: %#v", descriptor)
	}
	manifest, err := ReadKBGenerationManifest(root, generationID)
	if err != nil {
		t.Fatalf("read generation manifest: %v", err)
	}
	if manifest.Status != KBGenerationStatusReady || manifest.RowCount != manifest.Chunks || manifest.SourceDigest == "" || manifest.ModelManifestDigest == "" {
		t.Fatalf("generation manifest = %#v", manifest)
	}
	if strings.Contains(projection.Summary, filepath.Clean(root)) {
		t.Fatalf("projection leaked absolute vault path: %#v", projection)
	}
}

func writeStagingFakeSidecar(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inferrum-lancedb-sidecar")
	body := `#!/usr/bin/env python3
import json, pathlib, sys
req = json.load(sys.stdin)
store = pathlib.Path(req["store_uri"])
store.mkdir(parents=True, exist_ok=True)
records = req.get("records", [])
(store / "sidecar.jsonl").write_text("".join(json.dumps(row) + "\n" for row in records), encoding="utf-8")
print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"success","backend":"lancedb","rows":len(records)}))
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake staging sidecar: %v", err)
	}
	return path
}

func TestKBGenerationManifestProjectionDoesNotPersistNoteBody(t *testing.T) {
	manifest := KBGenerationManifest{
		SchemaVersion:       KBGenerationManifestSchema,
		GenerationID:        "gen-safe",
		Status:              KBGenerationStatusReady,
		Protocol:            "inferrum.sidecar.v1",
		Backend:             "lancedb",
		Provider:            "fake",
		Model:               "fake-hash-v1",
		ModelManifestDigest: "sha256:model",
		ProfileHash:         "sha256:profile",
		SourceSnapshot:      "snapshot-safe",
		SourceDigest:        "sha256:source",
		EmbeddingDim:        32,
		Documents:           1,
		Chunks:              1,
		RowCount:            1,
		CreatedAt:           "2026-08-02T00:00:00Z",
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if strings.Contains(string(payload), "body") || strings.Contains(string(payload), "vector") || strings.Contains(string(payload), "SECRET") {
		t.Fatalf("manifest leaked forbidden fields: %s", payload)
	}
}

func TestValidateStagedChunksRejectsUnsafeCitationPaths(t *testing.T) {
	base := semantic.Chunk{
		ChunkID:      "chunk-safe",
		VaultPath:    "notes/topic.md",
		Preview:      "bounded preview",
		EmbeddingDim: 2,
		Vector:       []float64{0.1, 0.2},
	}
	if err := validateStagedChunks([]semantic.Chunk{base}, 2); err != nil {
		t.Fatalf("valid relative citation rejected: %v", err)
	}
	for _, path := range []string{"", "/private/vault/secret.md", "../secret.md", `..\\secret.md`, `C:\\vault\\secret.md`} {
		chunk := base
		chunk.ChunkID = "chunk-" + strings.NewReplacer("/", "_", "\\", "_").Replace(path)
		chunk.VaultPath = path
		if err := validateStagedChunks([]semantic.Chunk{chunk}, 2); err == nil || !strings.Contains(err.Error(), "kb_generation_validation_failed") {
			t.Fatalf("unsafe citation path %q error = %v, want kb_generation_validation_failed", path, err)
		}
	}
}

func TestValidateStagedChunksRejectsNonFiniteVectors(t *testing.T) {
	base := semantic.Chunk{
		ChunkID:      "chunk-finite",
		VaultPath:    "notes/topic.md",
		Preview:      "bounded preview",
		EmbeddingDim: 2,
		Vector:       []float64{0.1, 0.2},
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		chunk := base
		chunk.ChunkID = "chunk-nonfinite"
		chunk.Vector = []float64{value, 0.2}
		if err := validateStagedChunks([]semantic.Chunk{chunk}, 2); err == nil || !strings.Contains(err.Error(), "kb_generation_validation_failed") {
			t.Fatalf("non-finite vector %v error = %v, want kb_generation_validation_failed", value, err)
		}
	}
}

func TestKBRebuildSourceDriftKeepsExistingActivation(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	notePath := filepath.Join(root, "notes", "drift.md")
	original := "---\nschema_version: pinax.note.v1\nnote_id: note_drift\ntitle: Drift\nkind: reference\nstatus: active\n---\n\n# Drift\n\nOriginal source.\n"
	if err := os.WriteFile(notePath, []byte(original), 0o644); err != nil {
		t.Fatalf("write original note: %v", err)
	}
	if err := CommitKBActivation(root, 0, KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: validKBActivationRef("gen-existing"), ActivatedAt: "2026-08-02T00:00:00Z"}); err != nil {
		t.Fatalf("write existing activation: %v", err)
	}
	before, err := ReadKBActivationDescriptor(root)
	if err != nil {
		t.Fatalf("read existing activation: %v", err)
	}
	sidecar := writeSourceDriftFakeSidecar(t, notePath)
	_, err = NewService().KBRebuild(context.Background(), KBIndexRequest{VaultPath: root, Backend: "lancedb", Provider: "fake", Model: "fake-hash-v1", SidecarExecutable: sidecar})
	if err == nil || !strings.Contains(err.Error(), "kb_source_drift") {
		t.Fatalf("source drift error = %v, want kb_source_drift", err)
	}
	after, err := ReadKBActivationDescriptor(root)
	if err != nil {
		t.Fatalf("read activation after drift: %v", err)
	}
	if after.Sequence != before.Sequence || after.Active == nil || before.Active == nil || after.Active.GenerationID != before.Active.GenerationID {
		t.Fatalf("source drift changed activation: before=%#v after=%#v", before, after)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".pinax", "kb", "generations"))
	if err != nil {
		t.Fatalf("read generation failures: %v", err)
	}
	foundFailure := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, statErr := os.Stat(filepath.Join(root, ".pinax", "kb", "generations", entry.Name(), "failure.json")); statErr == nil {
			foundFailure = true
			break
		}
	}
	if !foundFailure {
		t.Fatalf("source drift did not leave a redacted failure receipt")
	}
}

func TestKBRebuildSidecarFailureKeepsExistingActivation(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	note := "---\nschema_version: pinax.note.v1\nnote_id: note_sidecar_failure\ntitle: Sidecar failure\nkind: reference\nstatus: active\n---\n\n# Failure\n\nThe sidecar failure must leave active unchanged.\n"
	if err := os.WriteFile(filepath.Join(root, "notes", "failure.md"), []byte(note), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	before := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: validKBActivationRef("gen-existing"), ActivatedAt: "2026-08-02T00:00:00Z"}
	if err := CommitKBActivation(root, 0, before); err != nil {
		t.Fatalf("write existing activation: %v", err)
	}
	_, err := NewService().KBRebuild(context.Background(), KBIndexRequest{
		VaultPath:         root,
		Backend:           "lancedb",
		Provider:          "fake",
		Model:             "fake-hash-v1",
		SidecarExecutable: writeFailingStagingSidecar(t),
	})
	if err == nil || !strings.Contains(err.Error(), "kb_sidecar_failed") {
		t.Fatalf("sidecar failure error = %v, want kb_sidecar_failed", err)
	}
	after, err := ReadKBActivationDescriptor(root)
	if err != nil {
		t.Fatalf("read activation after sidecar failure: %v", err)
	}
	if after.Sequence != before.Sequence || after.Active == nil || after.Active.GenerationID != before.Active.GenerationID {
		t.Fatalf("sidecar failure changed activation: before=%#v after=%#v", before, after)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".pinax", "kb", "generations"))
	if err != nil {
		t.Fatalf("read generation failures: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("sidecar failure should retain exactly one candidate directory, got %d", len(entries))
	}
	failurePath := filepath.Join(root, ".pinax", "kb", "generations", entries[0].Name(), "failure.json")
	payload, err := os.ReadFile(failurePath)
	if err != nil {
		t.Fatalf("read sidecar failure receipt: %v", err)
	}
	var failure KBGenerationFailure
	if err := json.Unmarshal(payload, &failure); err != nil {
		t.Fatalf("decode sidecar failure receipt: %v", err)
	}
	if failure.SchemaVersion != KBGenerationFailureSchema || failure.Stage != "indexing" || failure.Code != "kb_sidecar_failed" {
		t.Fatalf("sidecar failure receipt = %#v", failure)
	}
	for _, forbidden := range []string{"/home/", "raw_prompt", "provider_payload", "secret", "vector"} {
		if strings.Contains(strings.ToLower(string(payload)), strings.ToLower(forbidden)) {
			t.Fatalf("sidecar failure receipt leaked %q: %s", forbidden, payload)
		}
	}
}

func TestKBRebuildSidecarFaultsFailClosedWithoutReplacingActivation(t *testing.T) {
	cases := []struct {
		name       string
		executable func(*testing.T) string
		timeout    time.Duration
		wantCode   string
	}{
		{name: "missing", executable: func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing-sidecar") }, wantCode: "kb_sidecar_unavailable"},
		{name: "timeout", executable: writeSlowStagingSidecar, timeout: 100 * time.Millisecond, wantCode: "kb_sidecar_timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
				t.Fatalf("mkdir notes: %v", err)
			}
			note := "---\nschema_version: pinax.note.v1\nnote_id: note_fault\ntitle: Fault\nkind: reference\nstatus: active\n---\n\n# Fault\n\nSidecar faults must preserve the active generation.\n"
			if err := os.WriteFile(filepath.Join(root, "notes", "fault.md"), []byte(note), 0o644); err != nil {
				t.Fatalf("write note: %v", err)
			}
			before := KBActivationDescriptor{SchemaVersion: KBActivationDescriptorSchema, Sequence: 1, Active: validKBActivationRef("gen-existing"), ActivatedAt: "2026-08-02T00:00:00Z"}
			if err := CommitKBActivation(root, 0, before); err != nil {
				t.Fatalf("write existing activation: %v", err)
			}
			_, err := NewService().KBRebuild(context.Background(), KBIndexRequest{
				VaultPath:         root,
				Backend:           semantic.DefaultBackend,
				Provider:          "fake",
				Model:             semantic.FakeProviderModel,
				SidecarExecutable: tc.executable(t),
				SidecarTimeout:    tc.timeout,
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantCode) {
				t.Fatalf("fault error = %v, want %s", err, tc.wantCode)
			}
			after, readErr := ReadKBActivationDescriptor(root)
			if readErr != nil {
				t.Fatalf("read activation after fault: %v", readErr)
			}
			if after.Sequence != before.Sequence || after.Active == nil || before.Active == nil || after.Active.GenerationID != before.Active.GenerationID {
				t.Fatalf("fault changed activation: before=%#v after=%#v", before, after)
			}
			entries, readErr := os.ReadDir(filepath.Join(root, ".pinax", "kb", "generations"))
			if readErr != nil {
				t.Fatalf("read failure generations: %v", readErr)
			}
			foundFailure := false
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				payload, readFailureErr := os.ReadFile(filepath.Join(root, ".pinax", "kb", "generations", entry.Name(), "failure.json"))
				if readFailureErr != nil {
					continue
				}
				var failure KBGenerationFailure
				if err := json.Unmarshal(payload, &failure); err != nil {
					t.Fatalf("decode failure receipt: %v", err)
				}
				if failure.Code != tc.wantCode {
					t.Fatalf("failure receipt = %#v, want %s", failure, tc.wantCode)
				}
				foundFailure = true
			}
			if !foundFailure {
				t.Fatalf("fault did not leave a bounded failure receipt")
			}
		})
	}
}

func writeFailingStagingSidecar(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inferrum-lancedb-sidecar")
	body := `#!/usr/bin/env python3
import json
print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"failed","backend":"lancedb","error":{"code":"operation_failed","message":"simulated sidecar failure"}}))
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write failing sidecar: %v", err)
	}
	return path
}

func writeSlowStagingSidecar(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inferrum-lancedb-sidecar")
	body := `#!/usr/bin/env python3
import time
time.sleep(0.5)
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write slow staging sidecar: %v", err)
	}
	return path
}

func writeSourceDriftFakeSidecar(t *testing.T, notePath string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inferrum-lancedb-sidecar")
	body := fmt.Sprintf(`#!/usr/bin/env python3
import json, pathlib, sys
req = json.load(sys.stdin)
store = pathlib.Path(req["store_uri"])
store.mkdir(parents=True, exist_ok=True)
if sys.argv[1] == "rebuild":
    records = req.get("records", [])
    (store / "sidecar.jsonl").write_text("".join(json.dumps(row) + "\n" for row in records), encoding="utf-8")
    pathlib.Path(%q).write_text(pathlib.Path(%q).read_text(encoding="utf-8") + "\nChanged while staging.\n", encoding="utf-8")
print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"success","backend":"lancedb","rows":len(req.get("records", []))}))
`, notePath, notePath)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write source drift sidecar: %v", err)
	}
	return path
}
