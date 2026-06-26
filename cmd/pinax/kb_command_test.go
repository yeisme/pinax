package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKBRebuildSearchAndContextUseLanceDBSidecar(t *testing.T) {
	root := t.TempDir()
	binDir := t.TempDir()
	sidecar := writeFakeKBSidecar(t, binDir)
	t.Setenv("PINAX_KB_SIDECAR", sidecar)
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "cloud-sync.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_cloud_sync\ntitle: Cloud Sync Design\ntags: [pinax, sync]\nkind: reference\n---\n\n# Cloud Sync Design\n\nPinax uses MinIO S3-compatible Cloud Sync for encrypted revisions.\n\n## LanceDB\n\nEach device rebuilds a local LanceDB semantic projection after pull.\n\nSECRET_BODY_SENTINEL should stay out of bounded agent context.\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "daily.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_daily\ntitle: Daily\ntags: [daily]\nkind: reference\n---\n\n# Daily\n\nLunch notes unrelated to vector search.\n")

	rebuildOut := runCLI(t, "kb", "rebuild", "--vault", root, "--backend", "lancedb", "--provider", "fake", "--json")
	var rebuild map[string]any
	if err := json.Unmarshal([]byte(rebuildOut), &rebuild); err != nil {
		t.Fatalf("kb rebuild json invalid: %v\n%s", err, rebuildOut)
	}
	if rebuild["command"] != "kb.rebuild" || rebuild["status"] != "success" {
		t.Fatalf("kb rebuild envelope = %#v", rebuild)
	}
	facts := rebuild["facts"].(map[string]any)
	if facts["backend"] != "lancedb" || facts["provider"] != "fake" || facts["documents"] != "2" {
		t.Fatalf("kb rebuild facts = %#v", facts)
	}
	if !fileExists(filepath.Join(root, ".pinax", "kb", "lancedb", "sidecar.jsonl")) {
		t.Fatalf("kb rebuild did not call sidecar")
	}

	searchOut := runCLI(t, "kb", "search", "LanceDB semantic projection", "--vault", root, "--agent")
	for _, want := range []string{"command=kb.search", "fact.backend=lancedb", "fact.matches=", "fact.provider=fake"} {
		if !strings.Contains(searchOut, want) {
			t.Fatalf("kb search agent missing %q:\n%s", want, searchOut)
		}
	}

	contextOut := runCLI(t, "kb", "context", "how should devices rebuild semantic search", "--vault", root, "--limit", "1", "--json")
	var contextEnvelope map[string]any
	if err := json.Unmarshal([]byte(contextOut), &contextEnvelope); err != nil {
		t.Fatalf("kb context json invalid: %v\n%s", err, contextOut)
	}
	if contextEnvelope["command"] != "kb.context" || contextEnvelope["status"] != "success" {
		t.Fatalf("kb context envelope = %#v", contextEnvelope)
	}
	if strings.Contains(contextOut, "SECRET_BODY_SENTINEL") || strings.Contains(contextOut, "raw_body") || strings.Contains(contextOut, "\"body\"") {
		t.Fatalf("kb context leaked full body field/content:\n%s", contextOut)
	}
}

func TestKBLanceDBRequiresSidecar(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PINAX_KB_SIDECAR", filepath.Join(root, "missing-sidecar"))
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out, err := runCLIExpectError("kb", "rebuild", "--vault", root, "--backend", "lancedb", "--provider", "fake", "--json")
	if err == nil {
		t.Fatalf("kb rebuild should require lancedb sidecar:\n%s", out)
	}
	assertJSONErrorCode(t, out, "kb_sidecar_unavailable")
}

func TestKBRejectsUnknownProvider(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out, err := runCLIExpectError("kb", "rebuild", "--vault", root, "--backend", "fake", "--provider", "gemni", "--json")
	if err == nil {
		t.Fatalf("kb rebuild should reject unknown provider:\n%s", out)
	}
	assertJSONErrorCode(t, out, "provider_invalid")
}

func TestKBProviderListAndDoctorContracts(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	runCLI(t, "init", root, "--title", "Vault", "--json")

	listOut := runCLI(t, "kb", "provider", "list", "--vault", root, "--json")
	assertJSONCommandStatus(t, listOut, "kb.provider.list", "success")
	for _, want := range []string{`"name":"gemini"`, `"name":"openai"`, `"name":"ollama"`, `"name":"fake"`, `"credential_source":"env:OPENAI_API_KEY"`, `"local_only":true`} {
		if !strings.Contains(listOut, want) {
			t.Fatalf("provider list missing %q:\n%s", want, listOut)
		}
	}
	for _, forbidden := range []string{"sk-", "Authorization", "Bearer"} {
		if strings.Contains(listOut, forbidden) {
			t.Fatalf("provider list leaked %q:\n%s", forbidden, listOut)
		}
	}

	agentOut := runCLI(t, "kb", "provider", "list", "--vault", root, "--agent")
	for _, want := range []string{"command=kb.provider.list", "fact.providers=4", "fact.default_provider=gemini"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("provider list agent missing %q:\n%s", want, agentOut)
		}
	}

	fakeDoctor := runCLI(t, "kb", "provider", "doctor", "fake", "--vault", root, "--json")
	assertJSONCommandStatus(t, fakeDoctor, "kb.provider.doctor", "success")
	if !strings.Contains(fakeDoctor, `"available":true`) || !strings.Contains(fakeDoctor, `"provider":"fake"`) {
		t.Fatalf("fake doctor output invalid:\n%s", fakeDoctor)
	}

	missingOpenAI, err := runCLIExpectError("kb", "provider", "doctor", "openai", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("openai doctor without key should fail:\n%s", missingOpenAI)
	}
	assertJSONErrorCode(t, missingOpenAI, "provider_not_configured")
	for _, forbidden := range []string{"Authorization", "Bearer", "raw_provider_payload", "provider_payload"} {
		if strings.Contains(missingOpenAI, forbidden) {
			t.Fatalf("provider doctor leaked %q:\n%s", forbidden, missingOpenAI)
		}
	}
}

func TestKBImportTextCopiesIntoVaultAndKeepsDryRunReadOnly(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(source, "idea.txt"), "Local-first semantic notebook with Gemini embeddings.\n")

	dryRun := runCLI(t, "kb", "import", source, "--include", "*.txt", "--vault", root, "--dry-run", "--json")
	assertJSONCommandStatus(t, dryRun, "kb.import", "success")
	if strings.Contains(runCLI(t, "note", "list", "--vault", root, "--json"), "idea") {
		t.Fatalf("kb import dry-run wrote note")
	}

	importOut := runCLI(t, "kb", "import", source, "--include", "*.txt", "--vault", root, "--yes", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(importOut), &envelope); err != nil {
		t.Fatalf("kb import json invalid: %v\n%s", err, importOut)
	}
	if envelope["command"] != "kb.import" || envelope["status"] != "success" {
		t.Fatalf("kb import envelope = %#v", envelope)
	}
	if envelope["facts"].(map[string]any)["imported"] != "1" {
		t.Fatalf("kb import facts = %#v", envelope["facts"])
	}
}

func TestKBImportDuplicateTitlesDoNotOverwrite(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(source, "a", "idea.txt"), "Alpha")
	writeCLIFixture(t, filepath.Join(source, "b", "idea.txt"), "Beta")

	importOut := runCLI(t, "kb", "import", source, "--include", "*.txt", "--vault", root, "--yes", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(importOut), &envelope); err != nil {
		t.Fatalf("kb import json invalid: %v\n%s", err, importOut)
	}
	if envelope["facts"].(map[string]any)["imported"] != "2" {
		t.Fatalf("kb import facts = %#v", envelope["facts"])
	}
	matches, err := filepath.Glob(filepath.Join(root, "notes", "kb", "imports", "idea*.md"))
	if err != nil || len(matches) != 2 {
		t.Fatalf("imported files = %#v err=%v", matches, err)
	}
}

func writeFakeKBSidecar(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "pinax-lancedb-sidecar")
	body := `#!/usr/bin/env python3
import json, pathlib, sys

op = sys.argv[1] if len(sys.argv) > 1 else ""
req = json.load(sys.stdin)
store = pathlib.Path(req["store_uri"])
store.mkdir(parents=True, exist_ok=True)
sidecar_path = store / "sidecar.jsonl"

if op == "doctor":
    print(json.dumps({"schema_version":"pinax.kb.sidecar.v1","status":"success","backend":"lancedb","dependency":"fake-sidecar"}))
elif op == "rebuild":
    chunks = req.get("chunks", [])
    with sidecar_path.open("w", encoding="utf-8") as f:
        for chunk in chunks:
            assert "chunk_text" not in chunk
            f.write(json.dumps(chunk, ensure_ascii=False) + "\n")
    print(json.dumps({"schema_version":"pinax.kb.sidecar.v1","status":"success","backend":"lancedb","chunks":len(chunks),"documents":req.get("documents",0)}))
elif op == "search":
    rows = []
    if sidecar_path.exists():
        rows = [json.loads(line) for line in sidecar_path.read_text(encoding="utf-8").splitlines() if line.strip()]
    hits = []
    for idx, row in enumerate(rows[:req.get("limit", 8) or 8]):
        hits.append({"chunk_id": row["chunk_id"], "note_id": row.get("note_id", ""), "path": row["vault_path"], "title": row["title"], "heading_path": row.get("heading_path", ""), "preview": row["preview"], "score": 1.0 - idx * 0.01, "provider": row["provider"], "model": row["embedding_model"], "tags": row.get("tags", []), "kind": row.get("kind", ""), "status": row.get("status", "")})
    print(json.dumps({"schema_version":"pinax.kb.sidecar.v1","status":"success","backend":"lancedb","total":len(rows),"hits":hits}))
else:
    print(json.dumps({"schema_version":"pinax.kb.sidecar.v1","status":"failed","error":{"code":"operation_invalid","message":"unknown operation"}}))
    sys.exit(2)
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake kb sidecar: %v", err)
	}
	return path
}
