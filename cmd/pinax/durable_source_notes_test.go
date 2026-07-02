package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceTemplateCreatesGitHubSourceNote(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	stdout, stderr, err := runCLISeparate("note", "add", "iptv-org/iptv", "--template", "source.github", "--var", "url=https://github.com/iptv-org/iptv", "--vault", root, "--json")
	if err != nil || stderr != "" {
		t.Fatalf("source template command failed: err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("source template json invalid: %v\n%s", err, stdout)
	}
	if envelope["command"] != "note.new" || envelope["mode"] != "json" || envelope["status"] != "success" {
		t.Fatalf("source template envelope = %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	for key, want := range map[string]string{
		"template":                 "source.github",
		"path":                     "sources/github/iptv-org-iptv.md",
		"kind":                     "source",
		"status":                   "active",
		"tags":                     "source/github,reference/source",
		"template.defaults_source": "source.github",
	} {
		if facts[key] != want {
			t.Fatalf("fact %s = %#v, want %q; facts=%#v", key, facts[key], want, facts)
		}
	}
	content := readCLIFile(t, filepath.Join(root, "sources", "github", "iptv-org-iptv.md"))
	for _, want := range []string{"kind: source", "status: active", "tags: [source/github, reference/source]", "Source facts", "Canonical URLs", "Use decision", "Risk and boundary", "Verification", "Related notes", "Next actions", "https://github.com/iptv-org/iptv"} {
		if !strings.Contains(content, want) {
			t.Fatalf("source note missing %q:\n%s", want, content)
		}
	}
}

func TestSourceTemplateExplicitFieldsOverrideDefaults(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	stdout := runCLI(t, "note", "add", "iptv-org/iptv", "--template", "source.github", "--var", "url=https://github.com/iptv-org/iptv", "--dir", "custom", "--kind", "reference", "--status", "draft", "--tags", "custom/tag", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("override json invalid: %v\n%s", err, stdout)
	}
	facts := envelope["facts"].(map[string]any)
	for key, want := range map[string]string{
		"path":               "notes/custom/iptv-org-iptv.md",
		"kind":               "reference",
		"status":             "draft",
		"tags":               "custom/tag",
		"template":           "source.github",
		"template.overrides": "kind,status,path,tags",
	} {
		if facts[key] != want {
			t.Fatalf("fact %s = %#v, want %q; facts=%#v", key, facts[key], want, facts)
		}
	}
	content, err := os.ReadFile(filepath.Join(root, "notes", "custom", "iptv-org-iptv.md"))
	if err != nil {
		t.Fatalf("read override note: %v", err)
	}
	for _, want := range []string{"kind: reference", "status: draft", "tags: [custom/tag]"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("override note missing %q:\n%s", want, string(content))
		}
	}
}

func TestDurableSourceMetadataAndOrganizePlans(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "add", "iptv-org/iptv", "--body", "# iptv-org/iptv\n\nSource: https://github.com/iptv-org/iptv\n", "--kind", "reference", "--tags", "github", "--dir", "research", "--vault", root, "--json")

	metadata := runCLI(t, "metadata", "plan", "iptv-org/iptv", "--vault", root, "--json")
	for _, want := range []string{"source_metadata", "source_url=https://github.com/iptv-org/iptv", "kind=source", "tags=source/github,reference/source", "writes\":\"false"} {
		if !strings.Contains(metadata, want) {
			t.Fatalf("metadata plan missing %q:\n%s", want, metadata)
		}
	}

	organize := runCLI(t, "organize", "plan", "--vault", root, "--json")
	for _, want := range []string{"source_move", "sources/github/iptv-org-iptv.md", "source_review", "Missing durable source sections", "manual_review"} {
		if !strings.Contains(organize, want) {
			t.Fatalf("organize plan missing %q:\n%s", want, organize)
		}
	}
}

func TestSearchSourceNotes(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "add", "iptv-org/iptv", "--template", "source.github", "--var", "url=https://github.com/iptv-org/iptv", "--vault", root, "--json")

	path := filepath.Join(root, "sources", "github", "iptv-org-iptv.md")
	content := readCLIFile(t, path)
	content = strings.Replace(content, "tags: [source/github, reference/source]\n", "tags: [source/github, reference/source]\nsource_url: https://github.com/iptv-org/iptv\nlast_checked_at: 2026-06-20\nsource_license: unknown\nreview_after: 2026-09-20\n", 1)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write source note metadata: %v", err)
	}
	runCLI(t, "index", "refresh", "--vault", root, "--json")

	listOut := runCLI(t, "note", "list", "--kind", "source", "--tag", "source/github", "--vault", root, "--json")
	for _, want := range []string{"sources/github/iptv-org-iptv.md", "source/github", "kind\":\"source"} {
		if !strings.Contains(listOut, want) {
			t.Fatalf("source note list missing %q:\n%s", want, listOut)
		}
	}
	searchOut := runCLI(t, "search", "iptv", "--kind", "source", "--tag", "source/github", "--vault", root, "--json")
	for _, want := range []string{"sources/github/iptv-org-iptv.md", "filter.kind", "filter.tag"} {
		if !strings.Contains(searchOut, want) {
			t.Fatalf("source note search missing %q:\n%s", want, searchOut)
		}
	}
}

func TestDurableSourceGraphChecks(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "add", "M3U Playlist Format", "--body", "# M3U Playlist Format\n", "--kind", "concept", "--vault", root, "--json")
	runCLI(t, "note", "add", "iptv-org/iptv", "--template", "source.github", "--var", "url=https://github.com/iptv-org/iptv", "--body", "# iptv-org/iptv\n\n## Related notes\n\n- [[M3U Playlist Format]]\n", "--vault", root, "--json")

	links := runCLI(t, "note", "links", "sources/github/iptv-org-iptv.md", "--vault", root, "--json")
	if !strings.Contains(links, "M3U Playlist Format") || !strings.Contains(links, "resolved") {
		t.Fatalf("source links output = %s", links)
	}
	backlinks := runCLI(t, "note", "backlinks", "M3U Playlist Format", "--vault", root, "--json")
	if !strings.Contains(backlinks, "sources/github/iptv-org-iptv.md") {
		t.Fatalf("concept backlinks output = %s", backlinks)
	}
}
