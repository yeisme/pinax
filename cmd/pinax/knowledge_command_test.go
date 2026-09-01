package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func knowledgeNote(id, title, extraFrontmatter, body string) string {
	return "---\nschema_version: pinax.note.v1\nnote_id: " + id + "\ntitle: " + title + "\n" + extraFrontmatter + "---\n\n# " + title + "\n\n" + body + "\n"
}

func sha256Digest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func parseKnowledgeEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json invalid: %v\n%s", err, out)
	}
	return envelope
}

func knowledgePackageFromEnvelope(t *testing.T, envelope map[string]any) map[string]any {
	t.Helper()
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data: %#v", envelope)
	}
	pkg, ok := data["package"].(map[string]any)
	if !ok {
		t.Fatalf("missing package: %#v", data)
	}
	return pkg
}

func knowledgeEntries(t *testing.T, pkg map[string]any) []map[string]any {
	t.Helper()
	raw, _ := pkg["entries"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("entry is not an object: %#v", item)
		}
		out = append(out, entry)
	}
	return out
}

func snapshotTree(t *testing.T, root string) map[string]fileSnap {
	t.Helper()
	snaps := map[string]fileSnap{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snaps[filepath.ToSlash(rel)] = fileSnap{mod: info.ModTime(), size: info.Size(), digest: sha256Digest(string(content))}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot tree: %v", err)
	}
	return snaps
}

type fileSnap struct {
	mod    time.Time
	size   int64
	digest string
}

func assertTreeUnchanged(t *testing.T, before, after map[string]fileSnap) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("tree file count changed: before=%d after=%d", len(before), len(after))
	}
	for path, snap := range before {
		got, ok := after[path]
		if !ok {
			t.Fatalf("tree lost %s", path)
		}
		if got.digest != snap.digest || got.size != snap.size || !got.mod.Equal(snap.mod) {
			t.Fatalf("tree mutated %s", path)
		}
	}
}

func TestKnowledgeExportProjectionDefaultEmptyAllowlist(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	secretBody := knowledgeNote("note_secret", "Secret", "", "do-not-export this body")
	writeCLIFixture(t, filepath.Join(root, "notes", "secret.md"), secretBody)
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	writeCLIFixture(t, indexPath, "fake-index")
	inferrumDB := filepath.Join(t.TempDir(), "inferrum.db")
	writeCLIFixture(t, inferrumDB, "fake-inferrum")
	beforeVault := snapshotTree(t, root)
	beforeDB, err := os.ReadFile(inferrumDB)
	if err != nil {
		t.Fatalf("read inferrum db: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "empty.json")
	out := runCLI(t, "knowledge", "export-projection", "--output", outPath, "--vault", root, "--json")
	assertJSONCommandStatus(t, out, "knowledge.export-projection", "success")
	envelope := parseKnowledgeEnvelope(t, out)
	facts := envelope["facts"].(map[string]any)
	if facts["empty_reason"] != "allowlist_empty" || facts["entries"] != "0" || facts["read_note_bodies"] != "0" {
		t.Fatalf("empty export facts = %#v\n%s", facts, out)
	}
	if facts["vault_write"] != "false" || facts["index_write"] != "false" {
		t.Fatalf("write facts = %#v", facts)
	}
	pkg := knowledgePackageFromEnvelope(t, envelope)
	if pkg["empty_reason"] != "allowlist_empty" {
		t.Fatalf("package empty_reason = %#v", pkg["empty_reason"])
	}
	if len(knowledgeEntries(t, pkg)) != 0 {
		t.Fatalf("empty package still has entries: %#v", pkg["entries"])
	}
	raw := readCLIFile(t, outPath)
	if strings.Contains(raw, "do-not-export") || strings.Contains(raw, "Secret") || strings.Contains(out, "do-not-export") {
		t.Fatalf("empty export leaked note body:\n%s\n%s", out, raw)
	}
	assertTreeUnchanged(t, beforeVault, snapshotTree(t, root))
	afterDB, err := os.ReadFile(inferrumDB)
	if err != nil {
		t.Fatalf("reread inferrum db: %v", err)
	}
	if string(afterDB) != string(beforeDB) {
		t.Fatal("inferrum db mutated")
	}
}

func TestKnowledgeExportProjectionDualConditionAndSchema(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	allowed := knowledgeNote("note_public", "Public", "knowledge_export: allow\n", "allowlisted body")
	pathOnly := knowledgeNote("note_path", "Path Only", "", "path only body")
	markerOnly := knowledgeNote("note_marker", "Marker Only", "knowledge_export: allow\n", "marker only body")
	writeCLIFixture(t, filepath.Join(root, "notes", "public.md"), allowed)
	writeCLIFixture(t, filepath.Join(root, "notes", "path-only.md"), pathOnly)
	writeCLIFixture(t, filepath.Join(root, "notes", "private", "marker-only.md"), markerOnly)
	beforeVault := snapshotTree(t, root)

	outPath := filepath.Join(t.TempDir(), "allowlisted.json")
	out := runCLI(t, "knowledge", "export-projection",
		"--output", outPath,
		"--allowlist", "notes/public.md",
		"--allowlist", "notes/path-only.md",
		"--vault", root, "--json")
	assertJSONCommandStatus(t, out, "knowledge.export-projection", "success")
	envelope := parseKnowledgeEnvelope(t, out)
	facts := envelope["facts"].(map[string]any)
	if facts["empty_reason"] != "" || facts["included"] != "1" || facts["omitted_missing_marker"] != "1" || facts["read_note_bodies"] != "2" {
		t.Fatalf("allowlisted facts = %#v\n%s", facts, out)
	}
	pkg := knowledgePackageFromEnvelope(t, envelope)
	entries := knowledgeEntries(t, pkg)
	if len(entries) != 1 {
		t.Fatalf("entries = %#v", entries)
	}
	entry := entries[0]
	if entry["kind"] != "note" {
		t.Fatalf("kind = %#v", entry["kind"])
	}
	refs := entry["refs"].(map[string]any)
	if refs["path"] != "notes/public.md" || refs["note_id"] != "note_public" || refs["source"] != "pinax.note" {
		t.Fatalf("refs = %#v", refs)
	}
	digest, _ := entry["digest"].(string)
	if digest == "" {
		t.Fatalf("missing digest: %#v", entry)
	}
	permission := entry["permission"].(map[string]any)
	if permission["export"] != "allowlisted" {
		t.Fatalf("permission = %#v", permission)
	}
	citation := entry["citation"].(map[string]any)
	if citation["path"] != "notes/public.md" || citation["note_id"] != "note_public" {
		t.Fatalf("citation = %#v", citation)
	}
	freshness := entry["freshness"].(map[string]any)
	changedAt, _ := freshness["changed_at"].(string)
	if freshness["content_digest"] != digest || changedAt == "" {
		t.Fatalf("freshness = %#v", freshness)
	}
	raw := readCLIFile(t, outPath)
	if strings.Contains(raw, "allowlisted body") || strings.Contains(raw, "path only body") || strings.Contains(raw, "marker only body") {
		t.Fatalf("package leaked note bodies:\n%s", raw)
	}
	if strings.Contains(raw, "notes/private/marker-only.md") || strings.Contains(raw, "notes/path-only.md") {
		t.Fatalf("package included dual-condition miss:\n%s", raw)
	}
	assertTreeUnchanged(t, beforeVault, snapshotTree(t, root))
}

func TestKnowledgeExportProjectionTombstoneIncremental(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	keep := knowledgeNote("note_keep", "Keep", "knowledge_export: allow\n", "keep body")
	gone := knowledgeNote("note_gone", "Gone", "knowledge_export: allow\n", "gone body")
	writeCLIFixture(t, filepath.Join(root, "notes", "keep.md"), keep)
	writeCLIFixture(t, filepath.Join(root, "notes", "gone.md"), gone)

	firstPath := filepath.Join(t.TempDir(), "first.json")
	firstOut := runCLI(t, "knowledge", "export-projection",
		"--output", firstPath,
		"--allowlist", "notes/keep.md",
		"--allowlist", "notes/gone.md",
		"--vault", root, "--json")
	assertJSONCommandStatus(t, firstOut, "knowledge.export-projection", "success")
	firstPkg := knowledgePackageFromEnvelope(t, parseKnowledgeEnvelope(t, firstOut))
	if len(knowledgeEntries(t, firstPkg)) != 2 {
		t.Fatalf("first export entries = %#v", firstPkg["entries"])
	}

	if err := os.Remove(filepath.Join(root, "notes", "gone.md")); err != nil {
		t.Fatalf("delete gone note: %v", err)
	}
	beforeVault := snapshotTree(t, root)
	secondPath := filepath.Join(t.TempDir(), "second.json")
	secondOut := runCLI(t, "knowledge", "export-projection",
		"--output", secondPath,
		"--allowlist", "notes/keep.md",
		"--allowlist", "notes/gone.md",
		"--from-package", firstPath,
		"--vault", root, "--json")
	assertJSONCommandStatus(t, secondOut, "knowledge.export-projection", "success")
	envelope := parseKnowledgeEnvelope(t, secondOut)
	facts := envelope["facts"].(map[string]any)
	if facts["tombstones"] != "1" || facts["unchanged"] != "1" || facts["included"] != "0" {
		t.Fatalf("incremental facts = %#v\n%s", facts, secondOut)
	}
	entries := knowledgeEntries(t, knowledgePackageFromEnvelope(t, envelope))
	if len(entries) != 1 {
		t.Fatalf("incremental entries = %#v", entries)
	}
	entry := entries[0]
	if entry["kind"] != "tombstone" {
		t.Fatalf("expected tombstone, got %#v", entry)
	}
	refs := entry["refs"].(map[string]any)
	if refs["path"] != "notes/gone.md" {
		t.Fatalf("tombstone path = %#v", refs)
	}
	revocation := entry["revocation"].(map[string]any)
	if revocation["status"] != "tombstone" {
		t.Fatalf("revocation = %#v", revocation)
	}
	raw := readCLIFile(t, secondPath)
	if strings.Contains(raw, "keep body") || strings.Contains(raw, "gone body") {
		t.Fatalf("incremental package leaked note bodies:\n%s", raw)
	}
	if strings.Contains(raw, `"kind": "note"`) {
		t.Fatalf("unchanged note re-emitted as content:\n%s", raw)
	}
	assertTreeUnchanged(t, beforeVault, snapshotTree(t, root))
}

func TestKnowledgeExportProjectionRejectsVaultOutput(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out, err := runCLIExpectError("knowledge", "export-projection", "--output", filepath.Join(root, "leak.json"), "--vault", root, "--json")
	if err == nil {
		t.Fatalf("expected vault output to fail:\n%s", out)
	}
	assertJSONErrorCode(t, out, "unsafe_path")
	if fileExists(filepath.Join(root, "leak.json")) {
		t.Fatal("rejected export still wrote inside the vault")
	}
}

func TestKnowledgeExportProjectionHelpUsesLongFlags(t *testing.T) {
	t.Parallel()
	out := runCLI(t, "knowledge", "export-projection", "--help")
	for _, want := range []string{"--output", "--allowlist", "--allowlist-file", "--from-package"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, " -o ") || strings.Contains(out, " -a ") {
		t.Fatalf("unexpected lowercase short alias in help:\n%s", out)
	}
}
