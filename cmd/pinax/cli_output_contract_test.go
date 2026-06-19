package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLITreeHelpSmoke(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		want   []string
		absent []string
	}{
		{args: []string{"--help"}, want: []string{"Local vault", "Note workflows", "Organization and search", "Automation and integrations", "Configuration and maintenance", "vault", "journal", "storage", "organize", "note", "folder", "template"}, absent: []string{"\n  daily ", "\n  weekly ", "\n  monthly ", "\n  stats ", "\n  validate ", "\n  doctor ", "\n  dashboard ", "\n  tag ", "\n  kind ", "\n  group ", "\n  schema "}},
		{args: []string{"vault", "--help"}, want: []string{"stats", "validate", "doctor", "dashboard"}},
		{args: []string{"journal", "--help"}, want: []string{"daily", "weekly", "monthly"}},
		{args: []string{"storage", "--help"}, want: []string{"set", "status", "doctor"}, absent: []string{"\n  set-local ", "\n  set-s3 "}},
		{args: []string{"storage", "set", "--help"}, want: []string{"local", "s3"}},
		{args: []string{"note", "--help"}, want: []string{"new", "show", "tags", "folders", "kinds", "groups"}},
		{args: []string{"organize", "--help"}, want: []string{"plan", "list", "apply"}, absent: []string{"\n  suggest "}},
	} {
		out := runCLI(t, tc.args...)
		for _, want := range tc.want {
			if !strings.Contains(out, want) {
				t.Fatalf("help %v missing %q:\n%s", tc.args, want, out)
			}
		}
		for _, absent := range tc.absent {
			if strings.Contains(out, absent) {
				t.Fatalf("help %v should hide compatibility command %q:\n%s", tc.args, strings.TrimSpace(absent), out)
			}
		}
	}
}

func TestCLITreePrimaryPathAliases(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	rootStats := runCLI(t, "stats", "--vault", root, "--json")
	vaultStats := runCLI(t, "vault", "stats", "--vault", root, "--json")
	assertSameCommandAndFacts(t, rootStats, vaultStats, "vault.stats")

	rootValidate := runCLI(t, "validate", "--vault", root, "--json")
	vaultValidate := runCLI(t, "vault", "validate", "--vault", root, "--json")
	assertSameCommandAndFacts(t, rootValidate, vaultValidate, "vault.validate")

	runCLI(t, "daily", "append", "--body", "alias", "--vault", root, "--json")
	dailyRoot := runCLI(t, "daily", "show", "--vault", root, "--json")
	dailyPrimary := runCLI(t, "journal", "daily", "show", "--vault", root, "--json")
	assertSameCommandAndFacts(t, dailyRoot, dailyPrimary, "daily.show")

	legacyStorage := runCLI(t, "storage", "set-local", "--root", root, "--vault", root, "--json")
	primaryStorage := runCLI(t, "storage", "set", "local", "--root", root, "--vault", root, "--json")
	assertSameCommandAndFacts(t, legacyStorage, primaryStorage, "storage.set_local")

	rootSchema := runCLI(t, "schema", "export", "--format", "openapi", "--vault", root, "--json")
	apiSchema := runCLI(t, "api", "schema", "export", "--format", "openapi", "--vault", root, "--json")
	assertSameCommandAndFacts(t, rootSchema, apiSchema, "api.schema.export")
	schemaHelp := runCLI(t, "schema", "--help")
	for _, want := range []string{"pinax schema export", "Export the local API schema"} {
		if !strings.Contains(schemaHelp, want) {
			t.Fatalf("schema help missing %q:\n%s", want, schemaHelp)
		}
	}
}

func TestCLIRemoteModeForwardsSupportedCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rpc" || r.Method != http.MethodPost {
			t.Fatalf("unexpected remote request %s %s", r.Method, r.URL.Path)
		}
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode remote request: %v", err)
		}
		if req.Method != "Pinax.Folder.List" || req.Params["include_empty"] != true || req.Params["purpose"] != "notes" {
			t.Fatalf("remote request = %#v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": "1.0", "mode": "json", "command": "folder.list", "status": "success", "facts": map[string]string{"remote": "true"}})
	}))
	defer server.Close()

	out := runCLI(t, "--api-url", server.URL, "folder", "list", "--purpose", "notes", "--include-empty", "--json")
	assertJSONCommandStatus(t, out, "folder.list", "success")
	if !strings.Contains(out, `"remote":"true"`) {
		t.Fatalf("remote output missing returned projection facts: %s", out)
	}
}

func TestCLIRemoteModeEnvironmentAndAgentOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode remote request: %v", err)
		}
		if req.Method != "Pinax.Inbox.List" {
			t.Fatalf("remote method = %s", req.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": "1.0", "mode": "json", "command": "inbox.list", "status": "success", "facts": map[string]string{"count": "0"}})
	}))
	defer server.Close()
	t.Setenv("PINAX_API_URL", server.URL)

	out := runCLI(t, "inbox", "list", "--agent")
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=inbox.list", "status=success", "fact.count=0"} {
		if !strings.Contains(out, want) {
			t.Fatalf("agent output missing %q:\n%s", want, out)
		}
	}
}

func TestCLIRemoteModeRejectsVaultConflictAndUnsupportedCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("remote server should not be called for invalid remote mode command")
	}))
	defer server.Close()

	out, err := runCLIExpectError("--api-url", server.URL, "--vault", t.TempDir(), "folder", "list", "--json")
	if err == nil || !strings.Contains(out, "remote_vault_conflict") {
		t.Fatalf("expected remote_vault_conflict, err=%v out=%s", err, out)
	}
	out, err = runCLIExpectError("--api-url", server.URL, "version", "--json")
	if err == nil || !strings.Contains(out, "remote_command_unsupported") {
		t.Fatalf("expected remote_command_unsupported, err=%v out=%s", err, out)
	}
	tokenFile := filepath.Join(t.TempDir(), "token.txt")
	writeCLIFixture(t, tokenFile, "file-token")
	out, err = runCLIExpectError("--api-url", server.URL, "--api-token", "inline-token", "--api-token-file", tokenFile, "folder", "list", "--json")
	if err == nil || !strings.Contains(out, "remote_token_conflict") || strings.Contains(out, "inline-token") || strings.Contains(out, "file-token") {
		t.Fatalf("expected redacted remote_token_conflict, err=%v out=%s", err, out)
	}
}

func TestCLIRemoteModeTokenSourcesStayRedacted(t *testing.T) {
	const secret = "pinax-remote-secret"
	tokenFile := filepath.Join(t.TempDir(), "token.txt")
	writeCLIFixture(t, tokenFile, secret+"\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+secret {
			t.Fatalf("authorization header = %q", got)
		}
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": "1.0", "mode": "json", "command": "folder.create", "status": "failed", "error": map[string]string{"code": "write_disabled", "message": "remote writes disabled"}})
	}))
	defer server.Close()

	out, err := runCLIExpectError("--api-url", server.URL, "--api-token-file", tokenFile, "folder", "create", "secret-folder", "--yes", "--json")
	if err == nil || !strings.Contains(out, "write_disabled") {
		t.Fatalf("expected remote projection error, err=%v out=%s", err, out)
	}
	if strings.Contains(out, secret) || strings.Contains(out, "Authorization") {
		t.Fatalf("remote output leaked token/header: %s", out)
	}
}

func TestAgentOutputMode(t *testing.T) {
	root := t.TempDir()
	out := runCLI(t, "init", root, "--title", "Vault", "--agent")
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=vault.init", "status=success"} {
		if !strings.Contains(out, want) {
			t.Fatalf("agent output missing %q:\n%s", want, out)
		}
	}
}

func TestMissingRequiredArgReturnsHelpfulProjection(t *testing.T) {
	out, err := runCLIExpectError("note", "show", "--json")
	if err == nil {
		t.Fatalf("note show without arg succeeded: %s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json error output invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "failed" || envelope["command"] != "note.show" {
		t.Fatalf("error envelope = %#v", envelope)
	}
	errorObject, ok := envelope["error"].(map[string]any)
	if !ok || errorObject["code"] != "argument_required" {
		t.Fatalf("error object = %#v", envelope["error"])
	}
	if !strings.Contains(errorObject["hint"].(string), "pinax note show <note>") {
		t.Fatalf("error hint = %#v", errorObject["hint"])
	}
}

func TestFlagErrorReturnsHelpfulProjection(t *testing.T) {
	out, err := runCLIExpectError("validate", "--json", "--bogus")
	if err == nil {
		t.Fatalf("unknown flag succeeded: %s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json flag error output invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "failed" || envelope["command"] != "cli.flag" {
		t.Fatalf("flag error envelope = %#v", envelope)
	}
	errorObject, ok := envelope["error"].(map[string]any)
	if !ok || errorObject["code"] != "flag_error" {
		t.Fatalf("flag error object = %#v", envelope["error"])
	}
}

func TestOutputModesAreMutuallyExclusive(t *testing.T) {
	out, err := runCLIExpectError("version", "--json", "--agent")
	if err == nil {
		t.Fatalf("conflicting output modes succeeded: %s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json mode conflict output invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "failed" || envelope["command"] != "cli.output_mode" {
		t.Fatalf("mode conflict envelope = %#v", envelope)
	}
	errorObject, ok := envelope["error"].(map[string]any)
	if !ok || errorObject["code"] != "output_mode_conflict" {
		t.Fatalf("mode conflict error = %#v", envelope["error"])
	}
	if !strings.Contains(errorObject["hint"].(string), "Keep only one output mode") {
		t.Fatalf("mode conflict hint = %#v", errorObject["hint"])
	}
}

func TestApplyHelpDocumentsSafetyFlags(t *testing.T) {
	out := runCLI(t, "organize", "apply", "--help")
	for _, want := range []string{"--yes", "--snapshot-message", "saved and reviewed plan from pinax organize plan --save", "version snapshot"} {
		if !strings.Contains(out, want) {
			t.Fatalf("organize apply help missing %q:\n%s", want, out)
		}
	}
}

func TestEventsAndExplainOutputModes(t *testing.T) {
	root := t.TempDir()
	events := runCLI(t, "init", root, "--events")
	lines := strings.Split(strings.TrimSpace(events), "\n")
	if len(lines) != 2 {
		t.Fatalf("events lines = %q", events)
	}
	for i, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("event %d invalid: %v\n%s", i, err, events)
		}
		if _, ok := event["error"]; ok && event["error"] == nil {
			t.Fatalf("event %d contains null error field: %#v", i, event)
		}
	}
	if !strings.Contains(lines[0], `"type":"start"`) || !strings.Contains(lines[1], `"type":"end"`) {
		t.Fatalf("events missing start/end:\n%s", events)
	}

	explain := runCLI(t, "validate", "--vault", root, "--explain")
	for _, want := range []string{"Conclusion:", "Evidence:", "Confidence:", "Recommended next step:"} {
		if !strings.Contains(explain, want) {
			t.Fatalf("explain output missing %q:\n%s", want, explain)
		}
	}
}

func TestHumanOutputIsPolishedForNotebookViewsAndHelp(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "Work", "--notes-prefix", "notes/work", "--vault", root, "--json")
	runCLI(t, "project", "create", "personal", "--name", "Personal", "--notes-prefix", "notes/personal", "--vault", root, "--json")
	runCLI(t, "note", "new", "Work Note", "--group", "work", "--kind", "reference", "--tags", "work", "--body", "body", "--vault", root, "--json")
	runCLI(t, "note", "new", "Personal Note", "--group", "personal", "--kind", "reference", "--tags", "personal", "--body", "body", "--vault", root, "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "raw.md"), "# Raw Note\n\nbody\n")

	groupOut := runCLI(t, "group", "list", "--vault", root)
	for _, want := range []string{"━━━━━━━━", "────────", "Highlights", "Metric", "Value", "Group", "Count", "work", "personal"} {
		if !strings.Contains(groupOut, want) {
			t.Fatalf("group list polished output missing %q:\n%s", want, groupOut)
		}
	}
	for _, old := range []string{"摘要", "统计", "列表", "状态:", "事实:", "dimension=group, dimensions=", "dimension=group"} {
		if strings.Contains(groupOut, old) {
			t.Fatalf("group list still uses old prose %q:\n%s", old, groupOut)
		}
	}

	helpOut := runCLI(t, "metadata")
	for _, want := range []string{"Summary", "Usage", "Available Commands", "Flags", "Global Flags", "pinax metadata [command] --help"} {
		if !strings.Contains(helpOut, want) {
			t.Fatalf("metadata help missing %q:\n%s", want, helpOut)
		}
	}
	for _, old := range []string{"简介", "用法", "可用命令", "参数", "全局参数"} {
		if strings.Contains(helpOut, old) {
			t.Fatalf("metadata help still contains English cobra heading %q:\n%s", old, helpOut)
		}
	}

	writeCLIFixture(t, filepath.Join(root, "notes", "needs-metadata.md"), "---\nschema_version: pinax.note.v1\ntitle: Needs Metadata\n---\n\n# Needs Metadata\n\nbody\n")
	planOut := runCLI(t, "metadata", "plan", "--vault", root)
	for _, want := range []string{"━━━━━━━━", "────────", "Highlights", "Metadata plan generated.", "Metric", "Value", "Planned updates", "Next step"} {
		if !strings.Contains(planOut, want) {
			t.Fatalf("metadata plan polished output missing %q:\n%s", want, planOut)
		}
	}
	for _, old := range []string{"Pinax", "摘要", "统计", "状态:", "重点:", "事实: planned_updates=", "planned_updates="} {
		if strings.Contains(planOut, old) {
			t.Fatalf("metadata plan still uses old prose %q:\n%s", old, planOut)
		}
	}
}
