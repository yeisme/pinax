package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishDocProfileSetRendererInvalidRejected(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	out, err := runCLIExpectError("publish", "doc", "profile", "set", "lark-doc", "--folder", "fld_test", "--renderer", "html", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("invalid renderer unexpectedly succeeded:\n%s", out)
	}
	// 用 --agent 输出校验稳定 error code
	agentOut, _ := runCLIExpectError("publish", "doc", "profile", "set", "lark-doc", "--folder", "fld_test", "--renderer", "html", "--vault", root, "--agent")
	for _, want := range []string{"status=failed", "publish_doc_renderer_invalid"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("agent output missing %q:\n%s", want, agentOut)
		}
	}
	// 失败时不写入 profile。
	if _, err := os.Stat(filepath.Join(root, ".pinax", "publish", "doc", "profiles", "lark-doc.yaml")); err == nil {
		t.Fatalf("profile should not be written for invalid renderer")
	}
}

func TestPublishDocProfileSetMarkdownFileRendererLegacy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	out := runCLI(t, "publish", "doc", "profile", "set", "lark-doc", "--folder", "fld_test", "--as", "user", "--renderer", "markdown-file", "--vault", root, "--json")
	env := parsePublishEnvelope(t, out)
	facts := env["facts"].(map[string]any)
	if facts["renderer"] != "markdown-file" {
		t.Fatalf("renderer fact = %v", facts["renderer"])
	}
}

func TestPublishDocNativePushCreatesDocxMapping(t *testing.T) {
	root := t.TempDir()
	writePublishNoteFixture(t, root, "notes/index/alpha.md", map[string]string{"note_id": "note_alpha", "title": "Alpha", "kind": "concept", "status": "active", "publish": "public"}, "# Alpha\n\nBody with native docx push.\n")

	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakePublishCLI(t, filepath.Join(fakeBin, "lark-cli"))
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	// 新 profile 默认 renderer=native-docx。
	runCLI(t, "publish", "doc", "profile", "set", "lark-doc", "--folder", "fld_test", "--as", "user", "--vault", root, "--json")

	prepareOut := runCLI(t, "publish", "doc", "prepare", "--note", "note_alpha", "--target", "lark-doc", "--vault", root, "--json")
	packageID := parsePublishEnvelope(t, prepareOut)["facts"].(map[string]any)["package_id"].(string)

	pushOut := runCLI(t, "publish", "doc", "push", "--package", packageID, "--target", "lark-doc", "--vault", root, "--json")
	pushEnv := parsePublishEnvelope(t, pushOut)
	pushFacts := pushEnv["facts"].(map[string]any)
	if pushFacts["renderer"] != "native-docx" {
		t.Fatalf("renderer fact = %v", pushFacts["renderer"])
	}
	if pushFacts["external_object_type"] != "docx" {
		t.Fatalf("external_object_type = %v", pushFacts["external_object_type"])
	}
	mapping := pushEnv["data"].(map[string]any)["mapping"].(map[string]any)
	if mapping["renderer"] != "native-docx" {
		t.Fatalf("mapping renderer = %v", mapping["renderer"])
	}
	if mapping["render_revision"] != "pinax.publish.render.v1" {
		t.Fatalf("mapping render_revision = %v", mapping["render_revision"])
	}
	extObj := mapping["external_object"].(map[string]any)
	if extObj["type"] != "docx" {
		t.Fatalf("external object type = %v", extObj["type"])
	}
}

func TestPublishDocNativePushRejectsMarkdownFileMappingMigration(t *testing.T) {
	root := t.TempDir()
	writePublishNoteFixture(t, root, "notes/index/beta.md", map[string]string{"note_id": "note_beta", "title": "Beta", "kind": "concept", "status": "active", "publish": "public"}, "# Beta\n\nMigration guard test.\n")

	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakePublishCLI(t, filepath.Join(fakeBin, "lark-cli"))
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	// 先用 markdown-file 发布，建立 file mapping。
	runCLI(t, "publish", "doc", "profile", "set", "lark-doc", "--folder", "fld_test", "--as", "user", "--renderer", "markdown-file", "--vault", root, "--json")
	prepareOut := runCLI(t, "publish", "doc", "prepare", "--note", "note_beta", "--target", "lark-doc", "--vault", root, "--json")
	packageID := parsePublishEnvelope(t, prepareOut)["facts"].(map[string]any)["package_id"].(string)
	runCLI(t, "publish", "doc", "push", "--package", packageID, "--target", "lark-doc", "--vault", root, "--json")

	// 切到 native-docx 再 push：迁移守卫拒绝原地改义。
	runCLI(t, "publish", "doc", "profile", "set", "lark-doc", "--folder", "fld_test", "--as", "user", "--renderer", "native-docx", "--vault", root, "--json")
	_, err := runCLIExpectError("publish", "doc", "push", "--package", packageID, "--target", "lark-doc", "--vault", root, "--agent")
	if err == nil {
		t.Fatalf("migration push unexpectedly succeeded")
	}
}

func TestPublishDocStatusShowsRendererAndObjectType(t *testing.T) {
	root := t.TempDir()
	writePublishNoteFixture(t, root, "notes/index/gamma.md", map[string]string{"note_id": "note_gamma", "title": "Gamma", "kind": "concept", "status": "active", "publish": "public"}, "# Gamma\n\nStatus output test.\n")

	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakePublishCLI(t, filepath.Join(fakeBin, "lark-cli"))
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	runCLI(t, "publish", "doc", "profile", "set", "lark-doc", "--folder", "fld_test", "--as", "user", "--vault", root, "--json")
	prepareOut := runCLI(t, "publish", "doc", "prepare", "--note", "note_gamma", "--target", "lark-doc", "--vault", root, "--json")
	packageID := parsePublishEnvelope(t, prepareOut)["facts"].(map[string]any)["package_id"].(string)
	runCLI(t, "publish", "doc", "push", "--package", packageID, "--target", "lark-doc", "--vault", root, "--json")

	statusOut := runCLI(t, "publish", "doc", "status", "--note", "note_gamma", "--target", "lark-doc", "--vault", root, "--json")
	statusEnv := parsePublishEnvelope(t, statusOut)
	facts := statusEnv["facts"].(map[string]any)
	if facts["renderer"] != "native-docx" {
		t.Fatalf("status renderer = %v", facts["renderer"])
	}
	if facts["external_object_type"] != "docx" {
		t.Fatalf("status external_object_type = %v", facts["external_object_type"])
	}
}

func TestPublishDocListInfersRendererForLegacyMappings(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePublishNoteFixture(t, root, "notes/index/legacy.md", map[string]string{"note_id": "note_legacy", "title": "Legacy", "kind": "concept", "status": "active", "publish": "public"}, "# Legacy\n\nLegacy linked mapping.\n")
	writePublishNoteFixture(t, root, "notes/index/native.md", map[string]string{"note_id": "note_native", "title": "Native", "kind": "concept", "status": "active", "publish": "public"}, "# Native\n\nNative linked mapping.\n")

	runCLI(t, "publish", "doc", "profile", "set", "lark-doc", "--folder", "fld_test", "--as", "user", "--renderer", "markdown-file", "--vault", root, "--json")
	runCLI(t, "publish", "doc", "link", "--note", "note_legacy", "--target", "lark-doc", "--external-url", "https://example.test/file/legacy", "--vault", root, "--json")
	runCLI(t, "publish", "doc", "profile", "set", "lark-doc", "--folder", "fld_test", "--as", "user", "--renderer", "native-docx", "--vault", root, "--json")
	runCLI(t, "publish", "doc", "link", "--note", "note_native", "--target", "lark-doc", "--external-url", "https://example.test/docx/native", "--vault", root, "--json")

	for _, noteID := range []string{"note_legacy", "note_native"} {
		body, err := os.ReadFile(filepath.Join(root, ".pinax", "publish", "doc", "mappings", noteID, "lark-doc.json"))
		if err != nil {
			t.Fatalf("read mapping %s: %v", noteID, err)
		}
		if strings.Contains(string(body), `"renderer"`) {
			t.Fatalf("mapping %s should not contain renderer field:\n%s", noteID, string(body))
		}
	}

	humanOut := runCLI(t, "publish", "doc", "list", "--target", "lark-doc", "--vault", root)
	for _, want := range []string{"Document publish mappings", "Renderer", "note_legacy", "markdown-file", "note_native", "native-docx"} {
		if !strings.Contains(humanOut, want) {
			t.Fatalf("doc list human missing %q:\n%s", want, humanOut)
		}
	}

	agentOut := runCLI(t, "publish", "doc", "list", "--target", "lark-doc", "--vault", root, "--agent")
	assertPublishDocListAgentMapping(t, agentOut, "note_legacy", "markdown-file")
	assertPublishDocListAgentMapping(t, agentOut, "note_native", "native-docx")
}

func assertPublishDocListAgentMapping(t *testing.T, out, noteID, renderer string) {
	t.Helper()
	for i := 1; i <= 2; i++ {
		prefix := "doc_mapping." + string(rune('0'+i)) + "."
		if strings.Contains(out, prefix+"note_id="+noteID) {
			if !strings.Contains(out, prefix+"renderer="+renderer) {
				t.Fatalf("doc list agent missing renderer %q for %s:\n%s", renderer, noteID, out)
			}
			return
		}
	}
	t.Fatalf("doc list agent missing mapping for %s:\n%s", noteID, out)
}

func TestPublishDocIndexPageNativeCreatesDocxIndex(t *testing.T) {
	root := t.TempDir()
	writePublishNoteFixture(t, root, "notes/index/delta.md", map[string]string{"note_id": "note_delta", "title": "Delta", "kind": "concept", "status": "active", "publish": "public"}, "# Delta\n\nIndex native test.\n")

	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakePublishCLI(t, filepath.Join(fakeBin, "lark-cli"))
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	// index-page + native-docx → index 应为原生文档。
	runCLI(t, "publish", "doc", "profile", "set", "lark-doc", "--folder", "fld_test", "--as", "user", "--index-page", "--vault", root, "--json")
	prepareOut := runCLI(t, "publish", "doc", "prepare", "--note", "note_delta", "--target", "lark-doc", "--vault", root, "--json")
	packageID := parsePublishEnvelope(t, prepareOut)["facts"].(map[string]any)["package_id"].(string)
	runCLI(t, "publish", "doc", "push", "--package", packageID, "--target", "lark-doc", "--vault", root, "--json")

	// profile 的 index_object 应记录 object type。
	profileBody, err := os.ReadFile(filepath.Join(root, ".pinax", "publish", "doc", "profiles", "lark-doc.yaml"))
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	profileStr := string(profileBody)
	if !strings.Contains(profileStr, "index_object:") {
		t.Fatalf("profile missing index_object")
	}
	if !strings.Contains(profileStr, "docx") {
		t.Fatalf("index_object should record docx type:\n%s", profileStr)
	}
}
