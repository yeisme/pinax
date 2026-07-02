package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishDocLarkWorkflowCreatesMappingAndStatus(t *testing.T) {
	root := t.TempDir()
	writePublishNoteFixture(t, root, "notes/index/alpha.md", map[string]string{"note_id": "note_alpha", "title": "Alpha", "kind": "concept", "status": "active", "publish": "public"}, "# Alpha\n\nPublic body for Lark doc publishing.\n")

	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakePublishCLI(t, filepath.Join(fakeBin, "lark-cli"), "lark")
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	profileOut := runCLI(t, "publish", "doc", "profile", "set", "lark-doc", "--space", "spc_test", "--folder", "fld_test", "--as", "user", "--vault", root, "--json")
	profileEnvelope := parsePublishEnvelope(t, profileOut)
	if profileEnvelope["command"] != "publish.doc.profile.set" || profileEnvelope["status"] != "success" || profileEnvelope["facts"].(map[string]any)["as"] != "user" {
		t.Fatalf("profile envelope = %#v", profileEnvelope)
	}
	doctorOut := runCLI(t, "publish", "doc", "provider", "doctor", "--target", "lark-doc", "--vault", root, "--json")
	doctorEnvelope := parsePublishEnvelope(t, doctorOut)
	if doctorEnvelope["command"] != "publish.doc.provider.doctor" || doctorEnvelope["status"] != "success" || doctorEnvelope["facts"].(map[string]any)["as"] != "user" {
		t.Fatalf("doctor envelope = %#v", doctorEnvelope)
	}

	prepareOut := runCLI(t, "publish", "doc", "prepare", "--note", "note_alpha", "--target", "lark-doc", "--vault", root, "--json")
	prepareEnvelope := parsePublishEnvelope(t, prepareOut)
	prepareFacts := prepareEnvelope["facts"].(map[string]any)
	if prepareEnvelope["command"] != "publish.doc.prepare" || prepareFacts["note_id"] != "note_alpha" || prepareFacts["target"] != "lark-doc" {
		t.Fatalf("prepare envelope = %#v", prepareEnvelope)
	}
	packageID := prepareFacts["package_id"].(string)
	if packageID == "" {
		t.Fatalf("package id missing: %#v", prepareFacts)
	}

	dryRunOut := runCLI(t, "publish", "doc", "push", "--package", packageID, "--target", "lark-doc", "--vault", root, "--dry-run", "--json")
	dryRunEnvelope := parsePublishEnvelope(t, dryRunOut)
	if dryRunEnvelope["command"] != "publish.doc.push" || dryRunEnvelope["facts"].(map[string]any)["dry_run"] != "true" {
		t.Fatalf("dry-run envelope = %#v", dryRunEnvelope)
	}

	pushOut := runCLI(t, "publish", "doc", "push", "--package", packageID, "--target", "lark-doc", "--vault", root, "--json")
	pushEnvelope := parsePublishEnvelope(t, pushOut)
	pushFacts := pushEnvelope["facts"].(map[string]any)
	externalURL, _ := pushFacts["external_url"].(string)
	if pushEnvelope["command"] != "publish.doc.push" || pushFacts["publish_status"] != "published" || externalURL == "" {
		t.Fatalf("push envelope = %#v", pushEnvelope)
	}
	if strings.Contains(pushOut, "secret") || strings.Contains(pushOut, root) {
		t.Fatalf("push output leaked sensitive content or local path:\n%s", pushOut)
	}

	statusOut := runCLI(t, "publish", "doc", "status", "--note", "note_alpha", "--vault", root, "--json")
	statusEnvelope := parsePublishEnvelope(t, statusOut)
	statusFacts := statusEnvelope["facts"].(map[string]any)
	if statusEnvelope["command"] != "publish.doc.status" || statusFacts["publish_status"] != "published" || statusFacts["target"] != "lark-doc" {
		t.Fatalf("status envelope = %#v", statusEnvelope)
	}

	listOut := runCLI(t, "publish", "doc", "list", "--target", "lark-doc", "--vault", root, "--json")
	listEnvelope := parsePublishEnvelope(t, listOut)
	if listEnvelope["command"] != "publish.doc.list" || listEnvelope["facts"].(map[string]any)["mappings"] != "1" {
		t.Fatalf("list envelope = %#v", listEnvelope)
	}
}

func writeFakePublishCLI(t *testing.T, path, provider string) {
	t.Helper()
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"drive\" ] && [ \"$2\" = \"+create-folder\" ]; then\n" +
		"  echo '{\"status\":\"ok\",\"id\":\"fld_fake_child\",\"url\":\"https://example.test/folder/fld_fake_child\"}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [ \"$1\" = \"drive\" ] && [ \"$2\" = \"+move\" ]; then\n" +
		"  echo '{\"status\":\"ok\"}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [ \"$1\" = \"auth\" ] && [ \"$2\" = \"status\" ]; then\n" +
		"  echo '{\"identities\":{\"user\":{\"status\":\"ready\",\"available\":true},\"bot\":{\"status\":\"ready\",\"available\":true}}}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"case \"$1:$2\" in\n" +
		"  markdown:+create|markdown:+overwrite) echo '{\"status\":\"ok\",\"id\":\"ext_test_doc\",\"url\":\"https://example.test/doc/ext_test_doc\"}' ;;\n" +
		"  *) echo '{\"status\":\"ok\",\"provider\":\"" + provider + "\"}' ;;\n" +
		"esac\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake publish cli: %v", err)
	}
}

func TestPublishDocLarkDoctorFailsWhenRequestedUserIdentityMissing(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"auth\" ] && [ \"$2\" = \"status\" ]; then\n" +
		"  echo '{\"identities\":{\"user\":{\"status\":\"missing\",\"available\":false},\"bot\":{\"status\":\"ready\",\"available\":true}}}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo '{\"status\":\"ok\"}'\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "lark-cli"), []byte(body), 0o755); err != nil {
		t.Fatalf("write fake lark-cli: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	runCLI(t, "publish", "doc", "profile", "set", "lark-doc", "--folder", "fld_test", "--as", "user", "--vault", root, "--json")
	out, err := runCLIExpectError("publish", "doc", "provider", "doctor", "--target", "lark-doc", "--vault", root, "--agent")
	if err == nil {
		t.Fatalf("doctor unexpectedly succeeded:\n%s", out)
	}
	for _, want := range []string{"command=publish.doc.provider.doctor", "status=failed", "error.code=provider_auth_failed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestPublishDocProviderListNotionProfileAndAgentOutput(t *testing.T) {
	root := t.TempDir()
	writePublishNoteFixture(t, root, "notes/index/beta.md", map[string]string{"note_id": "note_beta", "title": "Beta", "kind": "concept", "status": "active", "publish": "public"}, "# Beta\n\nBody for Notion profile contract.\n")

	listOut := runCLI(t, "publish", "doc", "provider", "list", "--vault", root, "--json")
	listEnvelope := parsePublishEnvelope(t, listOut)
	if listEnvelope["command"] != "publish.doc.provider.list" || listEnvelope["facts"].(map[string]any)["providers"] != "2" {
		t.Fatalf("provider list envelope = %#v", listEnvelope)
	}

	profileOut := runCLI(t, "publish", "doc", "profile", "set", "notion-page", "--workspace", "ws_test", "--parent-page", "pg_parent", "--vault", root, "--json")
	profileEnvelope := parsePublishEnvelope(t, profileOut)
	facts := profileEnvelope["facts"].(map[string]any)
	if profileEnvelope["command"] != "publish.doc.profile.set" || facts["target"] != "notion-page" || facts["provider"] != "notion" {
		t.Fatalf("notion profile envelope = %#v", profileEnvelope)
	}

	prepareOut := runCLI(t, "publish", "doc", "prepare", "--note", "note_beta", "--target", "notion-page", "--vault", root, "--json")
	prepareEnvelope := parsePublishEnvelope(t, prepareOut)
	packageID := prepareEnvelope["facts"].(map[string]any)["package_id"].(string)
	if packageID == "" {
		t.Fatalf("package id missing: %#v", prepareEnvelope)
	}

	agentOut, err := runCLIExpectError("publish", "doc", "push", "--package", packageID, "--target", "notion-page", "--vault", root, "--agent")
	if err == nil {
		t.Fatalf("notion push unexpectedly succeeded without fake notion cli:\n%s", agentOut)
	}
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=publish.doc.push", "status=failed"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("agent output missing %q:\n%s", want, agentOut)
		}
	}
	if strings.Contains(agentOut, root) || strings.Contains(agentOut, "Body for Notion") {
		t.Fatalf("agent output leaked local path or note body:\n%s", agentOut)
	}
}
