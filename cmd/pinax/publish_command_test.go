package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPublishProfileInitValidateShowListCommands(t *testing.T) {
	root := t.TempDir()

	initOut := runCLI(t, "publish", "profile", "init", "public", "--target", "github-pages", "--renderer", "hugo", "--title", "Knowledge Base", "--base-url", "https://example.github.io/kb/", "--theme", "builtin:pinax-encyclopedia", "--vault", root, "--json")
	initEnvelope := parsePublishEnvelope(t, initOut)
	if initEnvelope["command"] != "publish.profile.init" || initEnvelope["status"] != "success" {
		t.Fatalf("init envelope = %#v", initEnvelope)
	}
	facts := initEnvelope["facts"].(map[string]any)
	if facts["profile"] != "public" || facts["target"] != "github-pages" || facts["renderer"] != "hugo" {
		t.Fatalf("init facts = %#v", facts)
	}
	profilePath := filepath.Join(root, ".pinax", "publish", "profiles", "public.yaml")
	profileBody, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("profile not written: %v", err)
	}
	var profile map[string]any
	if err := yaml.Unmarshal(profileBody, &profile); err != nil {
		t.Fatalf("profile yaml invalid: %v\n%s", err, profileBody)
	}
	if profile["schema_version"] != "pinax.publish_profile.v1" || profile["name"] != "public" {
		t.Fatalf("profile identity = %#v", profile)
	}

	validateOut := runCLI(t, "publish", "profile", "validate", "public", "--vault", root, "--json")
	validateEnvelope := parsePublishEnvelope(t, validateOut)
	if validateEnvelope["command"] != "publish.profile.validate" || validateEnvelope["status"] != "success" {
		t.Fatalf("validate envelope = %#v", validateEnvelope)
	}
	if validateEnvelope["facts"].(map[string]any)["issues"] != "0" {
		t.Fatalf("validate facts = %#v", validateEnvelope["facts"])
	}

	showOut := runCLI(t, "publish", "profile", "show", "public", "--vault", root, "--json")
	showEnvelope := parsePublishEnvelope(t, showOut)
	if showEnvelope["command"] != "publish.profile.show" || showEnvelope["facts"].(map[string]any)["profile"] != "public" {
		t.Fatalf("show envelope = %#v", showEnvelope)
	}

	listOut := runCLI(t, "publish", "profile", "list", "--vault", root, "--json")
	listEnvelope := parsePublishEnvelope(t, listOut)
	if listEnvelope["command"] != "publish.profile.list" || listEnvelope["facts"].(map[string]any)["profiles"] != "1" {
		t.Fatalf("list envelope = %#v", listEnvelope)
	}
}

func publishVaultFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writePublishNoteFixture(t, root, "notes/alpha.md", map[string]string{"note_id": "note_alpha", "title": "Alpha", "kind": "concept", "status": "active", "publish": "public"}, "# Alpha\n\nPublic body.\n")
	return root
}

func TestPublishGistTargetBuildsMarkdownAndDeploysThroughGh(t *testing.T) {
	root := publishVaultFixture(t)
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	ghLog := filepath.Join(root, "gh.log")
	fakeGH := filepath.Join(fakeBin, "gh")
	if err := os.WriteFile(fakeGH, []byte("#!/bin/sh\nprintf '%s\n' \"$*\" >> \"$PINAX_TEST_GH_LOG\"\necho https://gist.github.com/fake/pinax\n"), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PINAX_TEST_GH_LOG", ghLog)

	runCLI(t, "publish", "profile", "init", "gist", "--target", "github-gist", "--renderer", "none", "--vault", root, "--json")
	outDir := filepath.Join(root, "dist", "gist")
	buildOut := runCLI(t, "publish", "build", "--profile", "gist", "--target", "github-gist", "--out", outDir, "--vault", root, "--json")
	buildEnvelope := parsePublishEnvelope(t, buildOut)
	if buildEnvelope["command"] != "publish.build" || buildEnvelope["facts"].(map[string]any)["target"] != "github-gist" {
		t.Fatalf("gist build envelope = %#v", buildEnvelope)
	}
	if !fileExists(filepath.Join(outDir, "pinax-gist.md")) || !fileExists(filepath.Join(outDir, "pinax-publish-manifest.json")) {
		t.Fatalf("gist output missing expected files")
	}

	deployOut := runCLI(t, "publish", "deploy", "--profile", "gist", "--target", "github-gist", "--out", outDir, "--yes", "--vault", root, "--json")
	deployEnvelope := parsePublishEnvelope(t, deployOut)
	deployFacts := deployEnvelope["facts"].(map[string]any)
	if deployEnvelope["command"] != "publish.deploy" || deployFacts["mode"] != "gist" || deployFacts["target"] != "github-gist" {
		t.Fatalf("gist deploy envelope = %#v", deployEnvelope)
	}
	logBody, err := os.ReadFile(ghLog)
	if err != nil {
		t.Fatalf("gh log missing: %v", err)
	}
	if !strings.Contains(string(logBody), "gist create") || strings.Contains(deployOut, root) {
		t.Fatalf("gist deploy did not call gh safely: log=%s out=%s", logBody, deployOut)
	}
}

func TestPublishHTTPDeployPostsScannedOutputToEndpoint(t *testing.T) {
	root := publishVaultFixture(t)
	var receivedPath string
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedBody = r.FormValue("manifest") + r.FormValue("content")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"ok","url":"https://share.example.test/pinax"}`)
	}))
	defer server.Close()

	runCLI(t, "publish", "profile", "init", "http", "--target", "http", "--renderer", "none", "--vault", root, "--json")
	outDir := filepath.Join(root, "dist", "http")
	runCLI(t, "publish", "build", "--profile", "http", "--target", "http", "--out", outDir, "--vault", root, "--json")
	deployOut := runCLI(t, "publish", "deploy", "--profile", "http", "--target", "http", "--out", outDir, "--endpoint", server.URL+"/publish", "--yes", "--vault", root, "--json")
	deployEnvelope := parsePublishEnvelope(t, deployOut)
	facts := deployEnvelope["facts"].(map[string]any)
	if facts["mode"] != "http" || facts["target"] != "http" || facts["http_status"] != "200" {
		t.Fatalf("http deploy facts = %#v", facts)
	}
	if receivedPath != "/publish" || !strings.Contains(receivedBody, "pinax.publish_manifest.v1") || strings.Contains(deployOut, root) {
		t.Fatalf("http deploy request/output invalid path=%s body=%s out=%s", receivedPath, receivedBody, deployOut)
	}
}

func TestPublishServePreviewsBuiltOutputOnLoopback(t *testing.T) {
	root := publishVaultFixture(t)
	runCLI(t, "publish", "profile", "init", "wiki", "--target", "github-wiki", "--renderer", "none", "--vault", root, "--json")
	outDir := filepath.Join(root, "dist", "wiki")
	runCLI(t, "publish", "build", "--profile", "wiki", "--target", "github-wiki", "--out", outDir, "--vault", root, "--json")

	serveOut := runCLI(t, "publish", "serve", "--profile", "wiki", "--out", outDir, "--host", "127.0.0.1", "--port", "0", "--once", "--vault", root, "--json")
	envelope := parsePublishEnvelope(t, serveOut)
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "publish.serve" || facts["host"] != "127.0.0.1" || facts["served"] != "true" {
		t.Fatalf("serve envelope = %#v", envelope)
	}
	if strings.Contains(serveOut, root) {
		t.Fatalf("serve output leaked local root:\n%s", serveOut)
	}
}

func TestPublishThemeListAndEjectCommands(t *testing.T) {
	root := t.TempDir()
	listOut := runCLI(t, "publish", "theme", "list", "--vault", root, "--json")
	listEnvelope := parsePublishEnvelope(t, listOut)
	if listEnvelope["command"] != "publish.theme.list" || listEnvelope["status"] != "success" {
		t.Fatalf("theme list envelope = %#v", listEnvelope)
	}
	listFacts := listEnvelope["facts"].(map[string]any)
	if listFacts["themes"] != "1" || listFacts["theme.1.name"] != "pinax-encyclopedia" || listFacts["theme.1.contract"] != "pinax.publish_theme.v1" {
		t.Fatalf("theme list facts = %#v", listFacts)
	}

	outDir := filepath.Join(root, "ejected-theme")
	ejectOut := runCLI(t, "publish", "theme", "eject", "pinax-encyclopedia", "--out", outDir, "--vault", root, "--json")
	ejectEnvelope := parsePublishEnvelope(t, ejectOut)
	if ejectEnvelope["command"] != "publish.theme.eject" || ejectEnvelope["status"] != "success" {
		t.Fatalf("theme eject envelope = %#v", ejectEnvelope)
	}
	ejectFacts := ejectEnvelope["facts"].(map[string]any)
	if ejectFacts["theme"] != "pinax-encyclopedia" || ejectFacts["contract"] != "pinax.publish_theme.v1" || ejectFacts["files"] == "0" {
		t.Fatalf("theme eject facts = %#v", ejectFacts)
	}
	for _, rel := range []string{"theme.toml", "layouts/_default/baseof.html", "layouts/_default/single.html", "assets/css/pinax.css", "assets/js/pinax-search.js"} {
		if !fileExists(filepath.Join(outDir, filepath.FromSlash(rel))) {
			t.Fatalf("ejected theme missing %s", rel)
		}
	}
	if strings.Contains(ejectOut, root) {
		t.Fatalf("theme eject leaked local root:\n%s", ejectOut)
	}
}

func TestPublishThemeAndDeployOutputModesExposeStableProjection(t *testing.T) {
	root := t.TempDir()
	jsonOut := runCLI(t, "publish", "theme", "list", "--vault", root, "--json")
	if parsePublishEnvelope(t, jsonOut)["command"] != "publish.theme.list" {
		t.Fatalf("theme list json = %s", jsonOut)
	}
	agentOut := runCLI(t, "publish", "theme", "list", "--vault", root, "--agent")
	for _, want := range []string{"command=publish.theme.list", "status=success", "fact.themes=1", "fact.theme.1.name=pinax-encyclopedia"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("theme list agent missing %q:\n%s", want, agentOut)
		}
	}
	eventsOut := runCLI(t, "publish", "theme", "list", "--vault", root, "--events")
	if !strings.Contains(eventsOut, "\"type\":\"start\"") || !strings.Contains(eventsOut, "\"type\":\"end\"") || !strings.Contains(eventsOut, "publish.theme.list") {
		t.Fatalf("theme list events invalid:\n%s", eventsOut)
	}
	explainOut := runCLI(t, "publish", "theme", "list", "--vault", root, "--explain")
	if !strings.Contains(explainOut, "Conclusion:") || !strings.Contains(explainOut, "themes=1") {
		t.Fatalf("theme list explain invalid:\n%s", explainOut)
	}

	runCLI(t, "publish", "profile", "init", "pages", "--target", "github-pages", "--renderer", "hugo", "--vault", root, "--json")
	deployOut, deployErr := runCLIExpectError("publish", "deploy", "--profile", "pages", "--target", "github-pages", "--out", filepath.Join(root, "dist", "site"), "--repo", filepath.Join(t.TempDir(), "repo"), "--vault", root, "--json")
	if deployErr == nil || !strings.Contains(deployOut, "approval_required") || strings.Contains(deployOut, root) {
		t.Fatalf("deploy approval json invalid or leaked root: out=%s err=%v", deployOut, deployErr)
	}
}

func TestPublishProfileAgentOutputIsStableAndClean(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "publish", "profile", "init", "public", "--target", "github-wiki", "--renderer", "none", "--vault", root, "--json")

	agentOut := runCLI(t, "publish", "profile", "validate", "public", "--vault", root, "--agent")
	for _, want := range []string{"command=publish.profile.validate", "status=success", "fact.profile=public", "fact.target=github-wiki", "fact.issues=0", "action.plan="} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("agent output missing %q:\n%s", want, agentOut)
		}
	}
	if strings.Contains(agentOut, root) || strings.Contains(agentOut, "状态") || strings.Contains(agentOut, "Knowledge") {
		t.Fatalf("agent output leaked local path or prose:\n%s", agentOut)
	}
}

func TestPublishProfileValidateRejectsUnsafeHandWrittenProfile(t *testing.T) {
	root := t.TempDir()
	profileDir := filepath.Join(root, ".pinax", "publish", "profiles")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	badProfile := "schema_version: pinax.publish_profile.v1\n" +
		"name: bad\n" +
		"target: ftp\n" +
		"renderer: jekyll\n" +
		"body_policy: all-notes\n" +
		"output:\n  path: ../public\n" +
		"safety:\n  block_secrets: false\n  block_private_bodies: true\n  block_pinax_internals: true\n"
	if err := os.WriteFile(filepath.Join(profileDir, "bad.yaml"), []byte(badProfile), 0o644); err != nil {
		t.Fatal(err)
	}
	before := mustReadFile(t, filepath.Join(profileDir, "bad.yaml"))

	out, err := runCLIExpectError("publish", "profile", "validate", "bad", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("expected validation error, got output %s", out)
	}
	for _, code := range []string{"publish_target_invalid", "publish_renderer_invalid", "publish_body_policy_invalid", "publish_output_path_unsafe", "publish_safety_gate_disabled"} {
		if !strings.Contains(out, code) {
			t.Fatalf("validate output missing %s:\n%s", code, out)
		}
	}
	after := mustReadFile(t, filepath.Join(profileDir, "bad.yaml"))
	if before != after {
		t.Fatalf("validate modified profile")
	}
}

func TestPublishProfileValidateRejectsUnknownFieldsWithStableCode(t *testing.T) {
	root := t.TempDir()
	profileDir := filepath.Join(root, ".pinax", "publish", "profiles")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatal(err)
	}
	profile := "schema_version: pinax.publish_profile.v1\n" +
		"name: unknown\n" +
		"target: github-pages\n" +
		"renderer: hugo\n" +
		"body_policy: published-notes-only\n" +
		"unknown_secret_ref: ${TOKEN}\n"
	if err := os.WriteFile(filepath.Join(profileDir, "unknown.yaml"), []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runCLIExpectError("publish", "profile", "validate", "unknown", "--vault", root, "--json")
	if err == nil || !strings.Contains(out, "publish_profile_unknown_field") {
		t.Fatalf("unknown field err=%v out=%s", err, out)
	}
}

func TestPublishPlanSelectsSkipsAndBlocksWithoutBodyLeak(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "publish", "profile", "init", "public", "--target", "github-pages", "--renderer", "hugo", "--vault", root, "--json")
	writePublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Public Note", "kind": "concept", "status": "active", "publish": "public"}, "# Public\n\nAllowed body with ![Diagram](../assets/diagram.png).\n")
	writePublishNoteFixture(t, root, "notes/draft.md", map[string]string{"note_id": "note_draft", "title": "Draft Note", "kind": "concept", "status": "draft", "publish": "public"}, "# Draft\n\nDRAFT BODY MUST NOT LEAK.\n")
	writePublishNoteFixture(t, root, "notes/private.md", map[string]string{"note_id": "note_private", "title": "Private Note", "kind": "concept", "status": "active", "publish": "public", "privacy": "private"}, "# Private\n\nPRIVATE BODY MUST NOT LEAK.\n")
	writePublishNoteFixture(t, root, "notes/unpublished.md", map[string]string{"note_id": "note_unpublished", "title": "Unpublished Note", "kind": "concept", "status": "active", "publish": "false"}, "# Unpublished\n\nUNPUBLISHED BODY MUST NOT LEAK.\n")
	writePublishNoteFixture(t, root, "notes/secret.md", map[string]string{"note_id": "note_secret", "title": "Secret Note", "kind": "concept", "status": "active", "publish": "public"}, "# Secret\n\nAuthorization: Bearer SECRET_SENTINEL_TOKEN\n")
	writeCLIFixture(t, filepath.Join(root, "assets", "diagram.png"), "fake image")

	out := runCLI(t, "publish", "plan", "--profile", "public", "--target", "github-pages", "--vault", root, "--json")
	envelope := parsePublishEnvelope(t, out)
	if envelope["command"] != "publish.plan" || envelope["status"] != "partial" {
		t.Fatalf("plan envelope = %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["selected_count"] != "1" || facts["skipped_count"] != "3" || facts["blocking_count"] != "1" {
		t.Fatalf("plan facts = %#v", facts)
	}
	for _, forbidden := range []string{"SECRET_SENTINEL_TOKEN", "PRIVATE BODY MUST NOT LEAK", "DRAFT BODY MUST NOT LEAK", "UNPUBLISHED BODY MUST NOT LEAK"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("plan output leaked %q:\n%s", forbidden, out)
		}
	}

	agentOut := runCLI(t, "publish", "plan", "--profile", "public", "--target", "github-pages", "--vault", root, "--agent")
	for _, want := range []string{"command=publish.plan", "status=partial", "fact.selected_count=1", "fact.skipped_count=3", "fact.blocking_count=1", "fact.manual_review_count=0"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("agent output missing %q:\n%s", want, agentOut)
		}
	}
	if strings.Contains(agentOut, "SECRET_SENTINEL_TOKEN") || strings.Contains(agentOut, root) {
		t.Fatalf("agent output leaked secret or local path:\n%s", agentOut)
	}
}

func TestPublishPlanClassifiesLinkedAssets(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "publish", "profile", "init", "public", "--target", "github-pages", "--renderer", "hugo", "--vault", root, "--json")
	writePublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Asset Note", "kind": "concept", "status": "active", "publish": "public"}, "# Asset Note\n\n![Diagram](../assets/diagram.png)\n![Raw](../assets/raw.exe)\n![Missing](../assets/missing.pdf)\n")
	writeCLIFixture(t, filepath.Join(root, "assets", "diagram.png"), "fake image")
	writeCLIFixture(t, filepath.Join(root, "assets", "raw.exe"), "not publishable")

	out := runCLI(t, "publish", "plan", "--profile", "public", "--target", "github-pages", "--vault", root, "--json")
	envelope := parsePublishEnvelope(t, out)
	if envelope["command"] != "publish.plan" || envelope["status"] != "partial" {
		t.Fatalf("plan envelope = %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["selected_count"] != "1" || facts["selected_asset_count"] != "1" || facts["asset_violation_count"] != "2" || facts["blocking_count"] != "2" {
		t.Fatalf("plan facts = %#v", facts)
	}
	if !strings.Contains(out, `"kind":"asset"`) || !strings.Contains(out, `"source_path":"assets/diagram.png"`) {
		t.Fatalf("plan did not include allowed linked asset:\n%s", out)
	}
	if count := strings.Count(out, `"class":"asset_not_allowed"`); count != 2 {
		t.Fatalf("asset_not_allowed count = %d, output:\n%s", count, out)
	}
	if strings.Contains(out, root) {
		t.Fatalf("plan output leaked local root:\n%s", out)
	}
}

func TestPublishPlanOutputModesExposeStableProjection(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "publish", "profile", "init", "public", "--target", "github-pages", "--renderer", "hugo", "--vault", root, "--json")
	writePublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Output Modes", "kind": "concept", "status": "active", "publish": "public"}, "# Output Modes\n")

	summaryOut := runCLI(t, "publish", "plan", "--profile", "public", "--target", "github-pages", "--vault", root)
	for _, want := range []string{"发布计划已生成", "selected count", "pinax publish build --profile public --target github-pages --vault <vault> --json"} {
		if !strings.Contains(summaryOut, want) {
			t.Fatalf("summary output missing %q:\n%s", want, summaryOut)
		}
	}

	jsonOut := runCLI(t, "publish", "plan", "--profile", "public", "--target", "github-pages", "--vault", root, "--json")
	jsonEnvelope := parsePublishEnvelope(t, jsonOut)
	if jsonEnvelope["mode"] != "json" || jsonEnvelope["status"] != "success" || jsonEnvelope["facts"].(map[string]any)["manual_review_count"] != "0" {
		t.Fatalf("json envelope = %#v", jsonEnvelope)
	}

	agentOut := runCLI(t, "publish", "plan", "--profile", "public", "--target", "github-pages", "--vault", root, "--agent")
	for _, want := range []string{"mode=agent", "command=publish.plan", "status=success", "fact.selected_count=1", "fact.manual_review_count=0", "action.build="} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("agent output missing %q:\n%s", want, agentOut)
		}
	}

	eventsOut := runCLI(t, "publish", "plan", "--profile", "public", "--target", "github-pages", "--vault", root, "--events")
	assertNDJSONEvents(t, eventsOut, "publish.plan")
	if !strings.Contains(eventsOut, `"type":"start"`) || !strings.Contains(eventsOut, `"type":"end"`) || !strings.Contains(eventsOut, `"selected_count":"1"`) {
		t.Fatalf("events output missing start/end/facts:\n%s", eventsOut)
	}

	explainOut := runCLI(t, "publish", "plan", "--profile", "public", "--target", "github-pages", "--vault", root, "--explain")
	for _, want := range []string{"Conclusion", "Evidence", "Recommended next step", "selected_count=1", "pinax publish build --profile public --target github-pages --vault <vault> --json"} {
		if !strings.Contains(explainOut, want) {
			t.Fatalf("explain output missing %q:\n%s", want, explainOut)
		}
	}
}

func TestPublishPlanIncludesSourceInfoAndLinkGraph(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "publish", "profile", "init", "public", "--target", "github-pages", "--renderer", "hugo", "--vault", root, "--json")
	writePublishNoteFixture(t, root, "notes/alpha.md", map[string]string{"note_id": "note_alpha", "title": "Alpha", "kind": "concept", "status": "active", "publish": "public"}, "# Alpha\n\nSee [[Beta]] and [[Missing]].\nPRIVATE_LINK_BODY_SENTINEL\n")
	writePublishNoteFixture(t, root, "notes/beta.md", map[string]string{"note_id": "note_beta", "title": "Beta", "kind": "concept", "status": "active", "publish": "public"}, "# Beta\n")

	out := runCLI(t, "publish", "plan", "--profile", "public", "--target", "github-pages", "--vault", root, "--json")
	envelope := parsePublishEnvelope(t, out)
	facts := envelope["facts"].(map[string]any)
	if facts["source_count"] != "2" || facts["link_count"] != "2" || facts["broken_link_count"] != "1" {
		t.Fatalf("plan facts = %#v\n%s", facts, out)
	}
	plan := envelope["data"].(map[string]any)["plan"].(map[string]any)
	if len(plan["sources"].([]any)) != 2 || len(plan["link_graph"].([]any)) != 2 {
		t.Fatalf("plan source/link projection = %#v", plan)
	}
	if !strings.Contains(out, `"source_path":"notes/alpha.md"`) || !strings.Contains(out, `"target":"Missing"`) || !strings.Contains(out, `"status":"broken"`) {
		t.Fatalf("plan did not include safe source/link facts:\n%s", out)
	}
	if strings.Contains(out, "PRIVATE_LINK_BODY_SENTINEL") {
		t.Fatalf("plan source/link projection leaked note body:\n%s", out)
	}
}

func TestPublishBuildGitHubWikiWritesMarkdownManifestAndReceipt(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "dist", "wiki")
	runCLI(t, "publish", "profile", "init", "wiki", "--target", "github-wiki", "--renderer", "none", "--vault", root, "--json")
	writePublishNoteFixture(t, root, "notes/alpha.md", map[string]string{"note_id": "note_alpha", "title": "Alpha", "kind": "concept", "status": "active", "publish": "public", "tags": "wiki,alpha"}, "# Alpha\n\nSee [[Beta]], [[Missing Target]], and ![Diagram](../assets/diagram.png).\n")
	writePublishNoteFixture(t, root, "notes/beta.md", map[string]string{"note_id": "note_beta", "title": "Beta", "kind": "source", "status": "active", "publish": "public", "tags": "wiki"}, "# Beta\n")
	writeCLIFixture(t, filepath.Join(root, "assets", "diagram.png"), "fake image")

	out := runCLI(t, "publish", "build", "--profile", "wiki", "--target", "github-wiki", "--out", outDir, "--vault", root, "--json")
	envelope := parsePublishEnvelope(t, out)
	if envelope["command"] != "publish.build" || envelope["status"] != "success" {
		t.Fatalf("build envelope = %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["target"] != "github-wiki" || facts["selected_count"] != "2" || facts["asset_count"] != "1" || facts["scan_findings"] != "0" {
		t.Fatalf("build facts = %#v", facts)
	}
	for _, rel := range []string{"Home.md", "alpha.md", "beta.md", "Tags.md", "Types.md", "Sources.md", "_Sidebar.md", "pinax-publish-manifest.json", "assets/diagram.png"} {
		if _, err := os.Stat(filepath.Join(outDir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing wiki output %s: %v", rel, err)
		}
	}
	alpha := mustReadFile(t, filepath.Join(outDir, "alpha.md"))
	if !strings.Contains(alpha, "See [[Beta|beta]]") || !strings.Contains(alpha, "](assets/diagram.png)") || strings.Contains(alpha, "[[Missing Target]]") || !strings.Contains(alpha, "Missing Target (unpublished)") || strings.Contains(alpha, "Authorization") {
		t.Fatalf("alpha wiki page invalid:\n%s", alpha)
	}
	if !strings.Contains(mustReadFile(t, filepath.Join(outDir, "Tags.md")), "wiki") || !strings.Contains(mustReadFile(t, filepath.Join(outDir, "Types.md")), "source") || !strings.Contains(mustReadFile(t, filepath.Join(outDir, "Sources.md")), "Beta") {
		t.Fatalf("wiki indexes missing expected entries")
	}
	if !strings.Contains(mustReadFile(t, filepath.Join(outDir, "pinax-publish-manifest.json")), "note_alpha") {
		t.Fatalf("manifest missing published note")
	}
	receipts, err := filepath.Glob(filepath.Join(root, ".pinax", "publish", "runs", "*", "receipt.json"))
	if err != nil || len(receipts) != 1 {
		t.Fatalf("receipt files = %#v err=%v", receipts, err)
	}
	if strings.Contains(out, root) {
		t.Fatalf("build output leaked local root:\n%s", out)
	}
}

func TestPublishBuildGitHubPagesUsesFakeHugoAndScansOutput(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "dist", "site")
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeHugo := filepath.Join(fakeBin, "hugo")
	if err := os.WriteFile(fakeHugo, []byte("#!/bin/sh\nif [ \"$1\" = \"version\" ]; then echo 'hugo v0.130.0'; exit 0; fi\nwhile [ $# -gt 0 ]; do case \"$1\" in --destination) shift; dest=\"$1\" ;; esac; shift; done\nmkdir -p \"$dest\"\nprintf '%s\n' '<html>public</html>' > \"$dest/index.html\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	runCLI(t, "publish", "profile", "init", "pages", "--target", "github-pages", "--renderer", "hugo", "--title", "Knowledge", "--base-url", "https://example.github.io/kb/", "--vault", root, "--json")
	writePublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Public", "kind": "concept", "status": "active", "publish": "public", "tags": "pages"}, "# Public\n")

	out := runCLI(t, "publish", "build", "--profile", "pages", "--target", "github-pages", "--out", outDir, "--vault", root, "--json")
	envelope := parsePublishEnvelope(t, out)
	if envelope["command"] != "publish.build" || envelope["status"] != "success" {
		t.Fatalf("pages build envelope = %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["target"] != "github-pages" || facts["renderer"] != "hugo" || facts["selected_count"] != "1" || facts["scan_findings"] != "0" {
		t.Fatalf("pages build facts = %#v", facts)
	}
	if _, err := os.Stat(filepath.Join(outDir, "index.html")); err != nil {
		t.Fatalf("fake hugo output missing: %v", err)
	}
	if !strings.Contains(mustReadFile(t, filepath.Join(outDir, "index.html")), "public") {
		t.Fatalf("fake hugo output invalid")
	}
	receipts, err := filepath.Glob(filepath.Join(root, ".pinax", "publish", "runs", "*", "receipt.json"))
	if err != nil || len(receipts) != 1 {
		t.Fatalf("receipt files = %#v err=%v", receipts, err)
	}
	if strings.Contains(out, root) {
		t.Fatalf("pages build output leaked local root:\n%s", out)
	}
}

func TestPublishBuildGitHubWikiBlocksDisallowedLinkedAsset(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "dist", "wiki")
	runCLI(t, "publish", "profile", "init", "wiki", "--target", "github-wiki", "--renderer", "none", "--vault", root, "--json")
	writePublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Public", "kind": "concept", "status": "active", "publish": "public"}, "# Public\n\n![Raw](../assets/raw.exe)\n")
	writeCLIFixture(t, filepath.Join(root, "assets", "raw.exe"), "not publishable")

	out, err := runCLIExpectError("publish", "build", "--profile", "wiki", "--target", "github-wiki", "--out", outDir, "--vault", root, "--json")
	if err == nil {
		t.Fatalf("expected disallowed asset build failure, got %s", out)
	}
	if !strings.Contains(out, "publish_plan_blocked") || !strings.Contains(out, "asset_not_allowed") {
		t.Fatalf("build failure missing plan block/asset finding:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(outDir, "assets", "raw.exe")); err == nil {
		t.Fatalf("disallowed asset was copied")
	}
}

func TestPublishBuildGitHubWikiFailsWhenOutputScanFindsLeak(t *testing.T) {
	root := t.TempDir()
	outDir := filepath.Join(root, "dist", "wiki")
	runCLI(t, "publish", "profile", "init", "wiki", "--target", "github-wiki", "--renderer", "none", "--vault", root, "--json")
	writePublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Public", "kind": "concept", "status": "active", "publish": "public"}, "# Public\n")
	writeCLIFixture(t, filepath.Join(outDir, "leak.md"), "token=RAW_OUTPUT_TOKEN")

	out, err := runCLIExpectError("publish", "build", "--profile", "wiki", "--target", "github-wiki", "--out", outDir, "--vault", root, "--json")
	if err == nil {
		t.Fatalf("expected output scan failure, got %s", out)
	}
	if !strings.Contains(out, "publish_leak_detected") || !strings.Contains(out, "secret_pattern") {
		t.Fatalf("build leak failure missing structured code/finding:\n%s", out)
	}
	if strings.Contains(out, "RAW_OUTPUT_TOKEN") {
		t.Fatalf("build leak failure echoed secret:\n%s", out)
	}
}

func TestPublishBuildRejectsPinaxInternalOutputDirectory(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "publish", "profile", "init", "wiki", "--target", "github-wiki", "--renderer", "none", "--vault", root, "--json")
	writePublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Public", "kind": "concept", "status": "active", "publish": "public"}, "# Public\n")

	out, err := runCLIExpectError("publish", "build", "--profile", "wiki", "--target", "github-wiki", "--out", filepath.Join(root, ".pinax"), "--vault", root, "--json")
	if err == nil || !strings.Contains(out, "publish_out_unsafe") {
		t.Fatalf("publish build should reject exact .pinax output: out=%s err=%v", out, err)
	}
	if fileExists(filepath.Join(root, ".pinax", "Home.md")) || fileExists(filepath.Join(root, ".pinax", "pinax-publish-manifest.json")) {
		t.Fatalf("publish build wrote into .pinax despite unsafe output rejection")
	}
}

func TestPublishBuildGitHubPagesFailsWhenFakeHugoOutputLeaks(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeHugo := filepath.Join(fakeBin, "hugo")
	if err := os.WriteFile(fakeHugo, []byte("#!/bin/sh\nwhile [ $# -gt 0 ]; do case \"$1\" in --destination) shift; dest=\"$1\" ;; esac; shift; done\nmkdir -p \"$dest\"\nprintf '%s\n' 'token=RAW_THEME_TOKEN' > \"$dest/index.html\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	runCLI(t, "publish", "profile", "init", "pages", "--target", "github-pages", "--renderer", "hugo", "--vault", root, "--json")
	writePublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Public", "kind": "concept", "status": "active", "publish": "public"}, "# Public\n\nSafe body.")

	out, err := runCLIExpectError("publish", "build", "--profile", "pages", "--target", "github-pages", "--out", filepath.Join(root, "dist", "site"), "--vault", root, "--json")
	if err == nil {
		t.Fatalf("expected pages output scan failure, got %s", out)
	}
	if !strings.Contains(out, "publish_leak_detected") || !strings.Contains(out, "secret_pattern") {
		t.Fatalf("pages leak failure missing structured code/finding:\n%s", out)
	}
	if strings.Contains(out, "RAW_THEME_TOKEN") || fileExists(filepath.Join(root, ".pinax", "publish", "runs")) {
		t.Fatalf("pages leak echoed secret or wrote success receipt:\n%s", out)
	}
}

func TestPublishBuildGitHubPagesRealHugoSmokeWhenAvailable(t *testing.T) {
	if _, err := exec.LookPath("hugo"); err != nil {
		t.Skip("hugo executable not available; skipping optional real Hugo smoke")
	}
	root := t.TempDir()
	outDir := filepath.Join(root, "dist", "site")
	runCLI(t, "publish", "profile", "init", "pages", "--target", "github-pages", "--renderer", "hugo", "--title", "Knowledge", "--base-url", "/", "--vault", root, "--json")
	writePublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Public", "kind": "concept", "status": "active", "publish": "public", "tags": "pages"}, "# Public\n\nSafe body for real Hugo smoke.")

	out := runCLI(t, "publish", "build", "--profile", "pages", "--target", "github-pages", "--out", outDir, "--vault", root, "--json")
	envelope := parsePublishEnvelope(t, out)
	if envelope["command"] != "publish.build" || envelope["status"] != "success" {
		t.Fatalf("real hugo build envelope = %#v", envelope)
	}
	index := mustReadFile(t, filepath.Join(outDir, "index.html"))
	for _, want := range []string{"pinax-search-data", "pinax-graph-data", "Public"} {
		if !strings.Contains(index, want) {
			t.Fatalf("real hugo index missing %q:\n%s", want, index)
		}
	}
	for _, forbidden := range []string{"https://", "http://", "//cdn", "analytics", "fonts.googleapis"} {
		if strings.Contains(strings.ToLower(index), forbidden) {
			t.Fatalf("real hugo index contains external resource marker %q:\n%s", forbidden, index)
		}
	}
	stagingSearch, err := filepath.Glob(filepath.Join(root, ".pinax", "publish", "staging", "*", "data", "pinax", "search-index.json"))
	if err != nil || len(stagingSearch) == 0 || len(mustReadFile(t, stagingSearch[0])) == 0 {
		t.Fatalf("real hugo staging search index missing: files=%#v err=%v", stagingSearch, err)
	}
	stagingGraph, err := filepath.Glob(filepath.Join(root, ".pinax", "publish", "staging", "*", "data", "pinax", "graph.json"))
	if err != nil || len(stagingGraph) == 0 || len(mustReadFile(t, stagingGraph[0])) == 0 {
		t.Fatalf("real hugo staging graph missing: files=%#v err=%v", stagingGraph, err)
	}
}

func TestPublishDeployRequiresApprovalAndCommitsLocalRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
	root := t.TempDir()
	outDir := filepath.Join(root, "dist", "site")
	runCLI(t, "publish", "profile", "init", "pages", "--target", "github-pages", "--renderer", "hugo", "--vault", root, "--json")
	repoDir := filepath.Join(t.TempDir(), "deploy-repo")

	approvalOut, approvalErr := runCLIExpectError("publish", "deploy", "--profile", "pages", "--target", "github-pages", "--out", outDir, "--repo", repoDir, "--branch", "gh-pages", "--vault", root, "--json")
	if approvalErr == nil || !strings.Contains(approvalOut, "approval_required") || fileExists(repoDir) {
		t.Fatalf("deploy without --yes should require approval and not write: out=%s err=%v", approvalOut, approvalErr)
	}
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeHugo := filepath.Join(fakeBin, "hugo")
	if err := os.WriteFile(fakeHugo, []byte("#!/bin/sh\nwhile [ $# -gt 0 ]; do case \"$1\" in --destination) shift; dest=\"$1\" ;; esac; shift; done\nmkdir -p \"$dest\"\nprintf '%s\n' '<html>published</html>' > \"$dest/index.html\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	writePublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Public", "kind": "concept", "status": "active", "publish": "public"}, "# Public\n")
	runCLI(t, "publish", "build", "--profile", "pages", "--target", "github-pages", "--out", outDir, "--vault", root, "--json")

	out := runCLI(t, "publish", "deploy", "--profile", "pages", "--target", "github-pages", "--out", outDir, "--repo", repoDir, "--branch", "gh-pages", "--yes", "--vault", root, "--json")
	envelope := parsePublishEnvelope(t, out)
	if envelope["command"] != "publish.deploy" || envelope["status"] != "success" {
		t.Fatalf("deploy envelope = %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["mode"] != "git" || facts["branch"] != "gh-pages" || facts["files"] != "1" {
		t.Fatalf("deploy facts = %#v", facts)
	}
	if !strings.Contains(mustReadFile(t, filepath.Join(repoDir, "index.html")), "published") {
		t.Fatalf("deployed index missing")
	}
	cmd := exec.Command("git", "-C", repoDir, "log", "--oneline", "-1")
	log, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(log), "pinax publish deploy") {
		t.Fatalf("deploy commit missing: %v\n%s", err, log)
	}
	if strings.Contains(out, root) {
		t.Fatalf("deploy output leaked local root:\n%s", out)
	}
}

func TestPublishDeployRequiresReceiptAndCleanScan(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
	root := t.TempDir()
	outDir := filepath.Join(root, "dist", "site")
	writeCLIFixture(t, filepath.Join(outDir, "index.html"), "<html>published</html>")
	runCLI(t, "publish", "profile", "init", "pages", "--target", "github-pages", "--renderer", "hugo", "--vault", root, "--json")
	repoDir := filepath.Join(t.TempDir(), "deploy-repo")

	out, err := runCLIExpectError("publish", "deploy", "--profile", "pages", "--target", "github-pages", "--out", outDir, "--repo", repoDir, "--branch", "gh-pages", "--yes", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("expected deploy validation failure, got %s", out)
	}
	if !strings.Contains(out, "publish_deploy_validation_failed") || fileExists(repoDir) {
		t.Fatalf("deploy should require receipt before writing: out=%s", out)
	}
}

func TestPublishDeployMatrixCoversWikiVaultRootAndRemoteRedaction(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
	root := t.TempDir()
	wikiOut := filepath.Join(root, "dist", "wiki")
	runCLI(t, "publish", "profile", "init", "wiki", "--target", "github-wiki", "--renderer", "none", "--vault", root, "--json")
	writePublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Public", "kind": "concept", "status": "active", "publish": "public"}, "# Public\n")
	runCLI(t, "publish", "build", "--profile", "wiki", "--target", "github-wiki", "--out", wikiOut, "--vault", root, "--json")

	rootDeployOut, rootDeployErr := runCLIExpectError("publish", "deploy", "--profile", "wiki", "--target", "github-wiki", "--out", wikiOut, "--repo", root, "--yes", "--vault", root, "--json")
	if rootDeployErr == nil || !strings.Contains(rootDeployOut, "publish_deploy_policy_invalid") {
		t.Fatalf("deploy to vault root should be rejected: out=%s err=%v", rootDeployOut, rootDeployErr)
	}

	remoteOut, remoteErr := runCLIExpectError("publish", "deploy", "--profile", "wiki", "--target", "github-wiki", "--out", wikiOut, "--repo", "https://user:RAW_REMOTE_TOKEN@example.invalid/wiki.git", "--yes", "--vault", root, "--json")
	if remoteErr == nil || !strings.Contains(remoteOut, "publish_deploy_remote_unsupported") || strings.Contains(remoteOut, "RAW_REMOTE_TOKEN") || strings.Contains(remoteOut, "user:") {
		t.Fatalf("remote deploy error should be stable and redacted: out=%s err=%v", remoteOut, remoteErr)
	}

	repoDir := filepath.Join(t.TempDir(), "wiki-repo")
	deployOut := runCLI(t, "publish", "deploy", "--profile", "wiki", "--target", "github-wiki", "--out", wikiOut, "--repo", repoDir, "--yes", "--vault", root, "--json")
	envelope := parsePublishEnvelope(t, deployOut)
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "publish.deploy" || facts["target"] != "github-wiki" || facts["branch"] != "master" || facts["files"] == "0" {
		t.Fatalf("wiki deploy envelope = %#v", envelope)
	}
	if !fileExists(filepath.Join(repoDir, "Home.md")) {
		t.Fatalf("wiki deploy did not copy Home.md")
	}
}

func TestPublishDoctorDetectsFakeHugoAndProfile(t *testing.T) {
	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeHugo := filepath.Join(fakeBin, "hugo")
	if err := os.WriteFile(fakeHugo, []byte("#!/bin/sh\necho 'hugo v0.130.0'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	runCLI(t, "publish", "profile", "init", "public", "--target", "github-pages", "--renderer", "hugo", "--vault", root, "--json")

	out := runCLI(t, "publish", "doctor", "--profile", "public", "--target", "github-pages", "--out", filepath.Join(root, "dist", "site"), "--vault", root, "--json")
	envelope := parsePublishEnvelope(t, out)
	if envelope["command"] != "publish.doctor" || envelope["status"] != "success" {
		t.Fatalf("doctor envelope = %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	for key, want := range map[string]string{"profile": "public", "target": "github-pages", "renderer": "hugo", "hugo_available": "true", "profile_issues": "0", "out_safe": "true"} {
		if facts[key] != want {
			t.Fatalf("doctor fact %s = %v want %s; facts=%#v", key, facts[key], want, facts)
		}
	}
	if strings.Contains(out, root) {
		t.Fatalf("doctor output leaked local root:\n%s", out)
	}
}

func parsePublishEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("publish output is not JSON: %v\n%s", err, out)
	}
	return envelope
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func writePublishNoteFixture(t *testing.T, root, rel string, meta map[string]string, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("schema_version: pinax.note.v1\n")
	for _, key := range []string{"note_id", "title", "kind", "status", "publish", "privacy", "tags"} {
		if value := meta[key]; value != "" {
			b.WriteString(key)
			b.WriteString(": ")
			b.WriteString(value)
			b.WriteString("\n")
		}
	}
	b.WriteString("---\n\n")
	b.WriteString(body)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
