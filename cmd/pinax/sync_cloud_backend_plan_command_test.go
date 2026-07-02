package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackendProviderCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	help := runCLI(t, "backend", "--help")
	for _, want := range []string{"list", "add", "show", "doctor", "capabilities", "diff", "push", "pull", "remove", "object"} {
		if !strings.Contains(help, want) {
			t.Fatalf("backend help missing %q:\n%s", want, help)
		}
	}

	// backend add s3
	addOut := runCLI(t, "backend", "add", "s3", "work-s3", "--bucket", "notes", "--region", "us-east-1", "--prefix", "pinax/", "--profile", "work", "--vault", root, "--json")
	var addEnvelope map[string]any
	if err := json.Unmarshal([]byte(addOut), &addEnvelope); err != nil {
		t.Fatalf("backend add json invalid: %v\n%s", err, addOut)
	}
	if addEnvelope["command"] != "backend.add" || addEnvelope["status"] != "success" {
		t.Fatalf("backend add envelope = %#v", addEnvelope)
	}
	addFacts := addEnvelope["facts"].(map[string]any)
	if addFacts["name"] != "work-s3" || addFacts["kind"] != "s3" {
		t.Fatalf("backend add facts = %#v", addFacts)
	}
	if strings.Contains(strings.ToLower(addOut), "secret") || strings.Contains(strings.ToLower(addOut), "access_key") {
		t.Fatalf("backend add output leaked secret-like material:\n%s", addOut)
	}

	// backend list
	listOut := runCLI(t, "backend", "list", "--vault", root, "--json")
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("backend list json invalid: %v\n%s", err, listOut)
	}
	if listEnvelope["command"] != "backend.list" || listEnvelope["status"] != "success" {
		t.Fatalf("backend list envelope = %#v", listEnvelope)
	}
	listFacts := listEnvelope["facts"].(map[string]any)
	if listFacts["backends"] != "1" || listFacts["default_backend"] != "work-s3" {
		t.Fatalf("backend list facts = %#v", listFacts)
	}

	// backend ls is the short alias for backend list, not object listing.
	lsAliasOut := runCLI(t, "backend", "ls", "--vault", root, "--json")
	var lsAliasEnvelope map[string]any
	if err := json.Unmarshal([]byte(lsAliasOut), &lsAliasEnvelope); err != nil {
		t.Fatalf("backend ls alias json invalid: %v\n%s", err, lsAliasOut)
	}
	if lsAliasEnvelope["command"] != "backend.list" || lsAliasEnvelope["status"] != "success" {
		t.Fatalf("backend ls alias envelope = %#v", lsAliasEnvelope)
	}
	if lsAliasEnvelope["facts"].(map[string]any)["backends"] != "1" {
		t.Fatalf("backend ls alias facts = %#v", lsAliasEnvelope["facts"])
	}

	legacyLS, err := runCLIExpectError("backend", "ls", "--name", "work-s3", "--vault", root, "--json")
	if err == nil || !strings.Contains(legacyLS, "flag_error") {
		t.Fatalf("backend ls --name should not keep legacy object semantics err=%v out=%s", err, legacyLS)
	}

	objectListMissing, err := runCLIExpectError("backend", "object", "list", "--vault", root, "--json")
	if err == nil || !strings.Contains(objectListMissing, "argument_required") || !strings.Contains(objectListMissing, "backend object list <name> [prefix]") {
		t.Fatalf("backend object list missing name err=%v out=%s", err, objectListMissing)
	}

	// backend show
	statusOut := runCLI(t, "backend", "show", "work-s3", "--vault", root, "--json")
	var statusEnvelope map[string]any
	if err := json.Unmarshal([]byte(statusOut), &statusEnvelope); err != nil {
		t.Fatalf("backend show json invalid: %v\n%s", err, statusOut)
	}
	if statusEnvelope["command"] != "backend.show" {
		t.Fatalf("backend show envelope = %#v", statusEnvelope)
	}

	// backend doctor
	doctorOut := runCLI(t, "backend", "doctor", "work-s3", "--vault", root, "--json")
	var doctorEnvelope map[string]any
	if err := json.Unmarshal([]byte(doctorOut), &doctorEnvelope); err != nil {
		t.Fatalf("backend doctor json invalid: %v\n%s", err, doctorOut)
	}
	if doctorEnvelope["command"] != "backend.doctor" {
		t.Fatalf("backend doctor envelope = %#v", doctorEnvelope)
	}

	// backend capabilities
	capOut := runCLI(t, "backend", "capabilities", "work-s3", "--vault", root, "--agent")
	for _, want := range []string{"command=backend.capabilities", "status=success", "fact.name=work-s3", "fact.kind=s3"} {
		if !strings.Contains(capOut, want) {
			t.Fatalf("backend capabilities agent output missing %q:\n%s", want, capOut)
		}
	}

	// backend diff
	diffOut := runCLI(t, "backend", "diff", "work-s3", "--vault", root, "--json")
	var diffEnvelope map[string]any
	if err := json.Unmarshal([]byte(diffOut), &diffEnvelope); err != nil {
		t.Fatalf("backend diff json invalid: %v\n%s", err, diffOut)
	}
	if diffEnvelope["command"] != "backend.diff" {
		t.Fatalf("backend diff envelope = %#v", diffEnvelope)
	}

	// backend push dry-run
	pushDryRun := runCLI(t, "backend", "push", "work-s3", "--dry-run", "--vault", root, "--json")
	if !strings.Contains(pushDryRun, "backend.push") || !strings.Contains(pushDryRun, `"dry_run":true`) {
		t.Fatalf("backend push dry-run output invalid:\n%s", pushDryRun)
	}

	// backend push without approval
	pushFail, err := runCLIExpectError("backend", "push", "work-s3", "--vault", root, "--json")
	if err == nil || !strings.Contains(pushFail, "approval_required") {
		t.Fatalf("backend push without approval err=%v out=%s", err, pushFail)
	}

	// backend add rclone
	rcloneOut := runCLI(t, "backend", "add", "rclone", "work-drive", "--remote", "workdrive:pinax", "--vault", root, "--json")
	if !strings.Contains(rcloneOut, "backend.add") {
		t.Fatalf("backend add rclone output = %s", rcloneOut)
	}

	// backend remove
	removeOut := runCLI(t, "backend", "remove", "work-drive", "--vault", root, "--json")
	if !strings.Contains(removeOut, "backend.remove") {
		t.Fatalf("backend remove output = %s", removeOut)
	}
	// verify removed
	listAfterRemove := runCLI(t, "backend", "list", "--vault", root, "--json")
	var listAfterEnvelope map[string]any
	if err := json.Unmarshal([]byte(listAfterRemove), &listAfterEnvelope); err != nil {
		t.Fatalf("backend list after remove json invalid: %v\n%s", err, listAfterRemove)
	}
	if listAfterEnvelope["facts"].(map[string]any)["backends"] != "1" {
		t.Fatalf("expected 1 backend after remove: %s", listAfterRemove)
	}

	// backend add without name
	noName, err := runCLIExpectError("backend", "add", "s3", "--bucket", "b", "--region", "r", "--vault", root, "--json")
	if err == nil || !strings.Contains(noName, "argument_required") {
		t.Fatalf("backend add without name err=%v out=%s", err, noName)
	}

	// backend add invalid kind
	badKind, err := runCLIExpectError("backend", "add", "ftp", "x", "--vault", root, "--json")
	if err == nil || !strings.Contains(badKind, "backend_kind_invalid") {
		t.Fatalf("backend add invalid kind err=%v out=%s", err, badKind)
	}

	// backend add s3 missing required fields
	missingS3, err := runCLIExpectError("backend", "add", "s3", "bad-s3", "--vault", root, "--json")
	if err == nil || !strings.Contains(missingS3, "backend_config_incomplete") {
		t.Fatalf("backend add s3 missing fields err=%v out=%s", err, missingS3)
	}

	// backend show not found
	notFound, err := runCLIExpectError("backend", "show", "nonexistent", "--vault", root, "--json")
	if err == nil || !strings.Contains(notFound, "backend_not_found") {
		t.Fatalf("backend show not found err=%v out=%s", err, notFound)
	}

	// legacy storage compatibility: storage commands still work
	storageOut := runCLI(t, "storage", "set-s3", "--bucket", "legacy-bucket", "--region", "us-east-1", "--vault", root, "--json")
	if !strings.Contains(storageOut, "storage.set_s3") {
		t.Fatalf("storage set-s3 still works:\n%s", storageOut)
	}
}

func TestPlanDailyTaskBridgeWritesMarkdownBlockThroughCLI(t *testing.T) {
	t.Setenv("PINAX_TEST_NOW", "2026-06-21T15:30:00Z")
	installFakeTaskBridgeCLI(t)
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	dryOut, dryErr, err := runCLISeparate("plan", "daily", "--taskbridge", "--dry-run", "--vault", root, "--json")
	if err != nil || dryErr != "" {
		t.Fatalf("plan daily taskbridge dry-run err=%v stderr=%q stdout=%s", err, dryErr, dryOut)
	}
	var dryEnvelope map[string]any
	if err := json.Unmarshal([]byte(dryOut), &dryEnvelope); err != nil {
		t.Fatalf("dry-run json invalid: %v\n%s", err, dryOut)
	}
	dryFacts := dryEnvelope["facts"].(map[string]any)
	if dryFacts["source"] != "taskbridge" || dryFacts["captured_at"] != "2026-06-21T15:30:00Z" || dryFacts["target_note"] != "daily/2026-06-21.md" {
		t.Fatalf("dry-run facts = %#v", dryFacts)
	}
	if _, statErr := os.Stat(filepath.Join(root, "daily", "2026-06-21.md")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("dry-run should not create daily note, stat=%v", statErr)
	}

	applyOut := runCLI(t, "plan", "daily", "--taskbridge", "--save", "--yes", "--vault", root, "--json")
	var applyEnvelope map[string]any
	if err := json.Unmarshal([]byte(applyOut), &applyEnvelope); err != nil {
		t.Fatalf("apply json invalid: %v\n%s", err, applyOut)
	}
	applyFacts := applyEnvelope["facts"].(map[string]any)
	if applyFacts["managed_block"] != "planning-daily" || applyFacts["saved_path"] == "" {
		t.Fatalf("apply facts = %#v", applyFacts)
	}
	daily := readCLIFile(t, filepath.Join(root, "daily", "2026-06-21.md"))
	for _, want := range []string{"<!-- pinax:managed name=planning-daily -->", "Captured at: 2026-06-21T15:30:00Z", "CLI task", "task_cli_1"} {
		if !strings.Contains(daily, want) {
			t.Fatalf("daily note missing %q:\n%s", want, daily)
		}
	}

	actionsOut := runCLI(t, "plan", "actions", "--from", "daily", "--taskbridge", "--save", "--vault", root, "--json")
	var actionsEnvelope map[string]any
	if err := json.Unmarshal([]byte(actionsOut), &actionsEnvelope); err != nil {
		t.Fatalf("taskbridge actions json invalid: %v\n%s", err, actionsOut)
	}
	actionsFacts := actionsEnvelope["facts"].(map[string]any)
	if actionsFacts["source"] != "taskbridge" || actionsFacts["tasks"] != "1" || actionsFacts["saved_path"] == "" {
		t.Fatalf("taskbridge actions facts = %#v", actionsFacts)
	}
	draft := readCLIFile(t, filepath.Join(root, actionsFacts["saved_path"].(string)))
	if !strings.Contains(draft, `"task_id": "task_cli_2"`) || strings.Contains(draft, "--confirm") {
		t.Fatalf("taskbridge action draft invalid:\n%s", draft)
	}
}

func installFakeTaskBridgeCLI(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	path := filepath.Join(binDir, "taskbridge")
	script := `#!/bin/sh
if [ "$1 $2" != "agent today" ]; then echo unexpected args: "$@" >&2; exit 2; fi
cat <<'JSON'
{"schema":"taskbridge.agent-result.v1","status":"ok","request_id":"req_cli","dry_run":false,"requires_confirmation":false,"result":{"schema":"taskbridge.today.v1","date":"2026-06-21","status":"ok","summary":{"must_do":1,"at_risk":1,"inbox":0,"overdue":0,"project_next":0,"sync_warnings":0},"sections":[{"id":"must_do","title":"Must do today","tasks":[{"id":"task_cli_1","title":"CLI task","status":"todo","source":"local","priority":"high","reason":"Due today"}]},{"id":"at_risk","title":"At risk","tasks":[{"id":"task_cli_2","title":"CLI deferred task","status":"todo","source":"local","priority":"low","reason":"Too large"}]}],"suggested_actions":[{"id":"act_cli_1","type":"defer_task","task_id":"task_cli_2","reason":"Too large for today","requires_confirmation":true}],"warnings":[]},"warnings":[],"errors":[]}
JSON
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake taskbridge: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestFeishuDeliveryCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out := runCLI(t, "briefing", "deliver", "feishu", "--webhook", "https://open.feishu.cn/open-apis/bot/v2/hook/raw-token", "--secret-ref", "env://FEISHU_WEBHOOK", "--title", "Daily briefing", "--text", "AI tooling update", "--dry-run", "--vault", root, "--json")
	assertJSONCommandStatus(t, out, "briefing.deliver.feishu", "success")
	if strings.Contains(out, "raw-token") || strings.Contains(out, "FEISHU_WEBHOOK") {
		t.Fatalf("feishu dry-run leaked secret:\n%s", out)
	}
	if !strings.Contains(out, "\"remote_write\":\"false\"") {
		t.Fatalf("feishu dry-run missing remote_write false:\n%s", out)
	}
}

func TestBriefingRecipeCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	initOut := runCLI(t, "briefing", "recipe", "init", "--vault", root, "--json")
	assertJSONCommandStatus(t, initOut, "briefing.recipe.init", "success")
	if !strings.Contains(initOut, "AI research") || strings.Contains(initOut, "webhook") {
		t.Fatalf("recipe init output invalid:\n%s", initOut)
	}
	setOut := runCLI(t, "briefing", "recipe", "set", "--topic", "AI tooling", "--limit", "7", "--source", "fake:ai", "--vault", root, "--json")
	assertJSONCommandStatus(t, setOut, "briefing.recipe.set", "success")
	showOut := runCLI(t, "briefing", "recipe", "show", "--vault", root, "--agent")
	for _, want := range []string{"command=briefing.recipe.show", "fact.topic=\"AI tooling\"", "fact.limit=7", "fact.sources=2"} {
		if !strings.Contains(showOut, want) {
			t.Fatalf("recipe show missing %q:\n%s", want, showOut)
		}
	}
}

func TestCloudOutputContractModes(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "# Alpha\nsecret-token body\n")
	runCLI(t, "cloud", "login", "--endpoint", "https://cloud.example.test", "--workspace", "ws_123", "--device", "dev_laptop", "--secret-ref", "op://pinax/cloud-token", "--vault", root, "--json")
	agentOut := runCLI(t, "cloud", "status", "--vault", root, "--agent")
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=cloud.status", "status=success", "fact.configured=true", "fact.session_status=active"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("cloud status --agent missing %q:\n%s", want, agentOut)
		}
	}
	assertMachineOutputClean(t, agentOut)
	eventsOut := runCLI(t, "cloud", "doctor", "--vault", root, "--events")
	assertNDJSONEvents(t, eventsOut, "cloud.doctor")
	assertMachineOutputClean(t, eventsOut)
	explainOut := runCLI(t, "sync", "push", "--target", "cloud", "--dry-run", "--base-revision", "rev_1", "--remote-revision", "rev_1", "--vault", root, "--explain")
	if !strings.Contains(explainOut, "Conclusion") || !strings.Contains(explainOut, "Evidence") || strings.Contains(explainOut, "secret-token") || strings.Contains(explainOut, "cloud-token") {
		t.Fatalf("sync explain contract invalid:\n%s", explainOut)
	}
	conflict, err := runCLIExpectError("sync", "push", "--target", "cloud", "--yes", "--base-revision", "rev_1", "--remote-revision", "rev_2", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("conflict sync succeeded: %s", conflict)
	}
	assertJSONErrorCode(t, conflict, "REVISION_CONFLICT")
	assertMachineOutputClean(t, conflict)
}

func TestSyncRunReceiptsLogsStatusAndRedactionCLI(t *testing.T) {
	root := t.TempDir()
	objectRoot := t.TempDir()
	rawPath := "notes/raw-secret-path.md"
	forbidden := []string{"PLAINTEXT_NOTE_BODY", "raw-secret-path.md", "raw-token-123", "Authorization", "Cookie", "op://pinax/secret-ref", "provider payload", "provider stderr"}
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, rawPath), "# Secret\n\nPLAINTEXT_NOTE_BODY Authorization: Bearer raw-token-123 Cookie: session=abc provider payload provider stderr\n")
	runCLI(t, "cloud", "login", "--endpoint", "file://"+objectRoot, "--workspace", "ws_secret", "--device", "dev_secret", "--secret-ref", "op://pinax/secret-ref", "--vault", root, "--json")

	stdout, stderr, err := runCLISeparate("sync", "push", "--target", "cloud", "--yes", "--path-policy", "hash", "--vault", root, "--json")
	if err != nil || stderr != "" {
		t.Fatalf("sync push err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	assertJSONCommandStatus(t, stdout, "sync.push", "success")
	assertNoForbiddenSyncLeak(t, stdout+stderr, forbidden)

	statePath := filepath.Join(root, ".pinax", "sync-state.json")
	state := readCLIFile(t, statePath)
	assertNoForbiddenSyncLeak(t, state, forbidden)
	var stateJSON map[string]any
	if err := json.Unmarshal([]byte(state), &stateJSON); err != nil {
		t.Fatalf("sync-state json invalid: %v\n%s", err, state)
	}
	runID, _ := stateJSON["last_sync_run_id"].(string)
	if stateJSON["schema_version"] != "pinax.sync_state.v1" || runID == "" || stateJSON["last_synced_revision"] == "" || stateJSON["runs"] != nil || stateJSON["remote_write"] != nil {
		t.Fatalf("sync-state not current-state only: %#v", stateJSON)
	}

	receiptPath := filepath.Join(root, ".pinax", "sync-runs", time.Now().UTC().Format("2006"), time.Now().UTC().Format("01"), runID+".json")
	receipt := readCLIFile(t, receiptPath)
	assertNoForbiddenSyncLeak(t, receipt, forbidden)
	var receiptJSON map[string]any
	if err := json.Unmarshal([]byte(receipt), &receiptJSON); err != nil {
		t.Fatalf("receipt json invalid: %v\n%s", err, receipt)
	}
	for _, key := range []string{"run_id", "command", "target", "direction", "status", "remote_write", "local_write", "backend_kind", "transport", "workspace_id", "vault_id", "device_id", "request_id", "revision_id", "manifest_blob_id", "counts", "timings_ms", "actions", "redaction", "created_at"} {
		if _, ok := receiptJSON[key]; !ok {
			t.Fatalf("receipt missing %s: %#v", key, receiptJSON)
		}
	}
	if receiptJSON["schema_version"] != "pinax.sync_run.v1" || receiptJSON["status"] != "success" || receiptJSON["remote_write"] != true {
		t.Fatalf("receipt schema/status invalid: %#v", receiptJSON)
	}

	events := readCLIFile(t, filepath.Join(root, ".pinax", "events.jsonl"))
	assertNoForbiddenSyncLeak(t, events, forbidden)
	if !strings.Contains(events, runID) || strings.Contains(events, "manifest_blob_id") || strings.Contains(events, "provider payload") {
		t.Fatalf("events are not a safe run-linked summary:\n%s", events)
	}

	var objectText strings.Builder
	if err := filepath.WalkDir(objectRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		objectText.WriteString(filepath.ToSlash(path))
		objectText.WriteByte('\n')
		objectText.WriteString(readCLIFile(t, path))
		objectText.WriteByte('\n')
		return nil
	}); err != nil {
		t.Fatalf("walk object store: %v", err)
	}
	assertNoForbiddenSyncLeak(t, objectText.String(), forbidden)

	for _, mode := range [][]string{{"--json"}, {"--agent"}, {"--events"}, {"--explain"}} {
		out := runCLI(t, append([]string{"sync", "logs", "show", runID, "--vault", root}, mode...)...)
		assertNoForbiddenSyncLeak(t, out, forbidden)
		if !strings.Contains(out, runID) {
			t.Fatalf("logs show %v missing run id:\n%s", mode, out)
		}
	}
	for _, args := range [][]string{
		{"sync", "logs", "list", "--vault", root},
		{"sync", "logs", "tail", "--limit", "5", "--vault", root},
		{"sync", "logs", "prune", "--keep", "200", "--vault", root},
	} {
		for _, mode := range []string{"--json", "--agent", "--events", "--explain"} {
			out := runCLI(t, append(args, mode)...)
			assertNoForbiddenSyncLeak(t, out, forbidden)
		}
	}
	statusOut := runCLI(t, "sync", "status", "--vault", root, "--json")
	assertNoForbiddenSyncLeak(t, statusOut, forbidden)
	if !strings.Contains(statusOut, runID) {
		t.Fatalf("sync status output missing run id:\n%s", statusOut)
	}
}

func TestSyncRunReceiptsCoverPartialFailedApprovalAndPruneCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "# Alpha\nbody\n")
	runCLI(t, "cloud", "login", "--endpoint", "https://cloud.example.test", "--workspace", "ws", "--device", "dev", "--secret-ref", "env://PINAX_SECRET", "--vault", root, "--json")

	approvalOut, approvalErr := runCLIExpectError("sync", "push", "--target", "cloud", "--vault", root, "--json")
	if approvalErr == nil || !strings.Contains(approvalOut, "approval_required") {
		t.Fatalf("approval-required sync output invalid err=%v out=%s", approvalErr, approvalOut)
	}
	unavailableOut, unavailableErr := runCLIExpectError("sync", "push", "--target", "cloud", "--yes", "--base-revision", "rev_1", "--remote-revision", "rev_1", "--vault", root, "--json")
	if unavailableErr == nil || !strings.Contains(unavailableOut, "cloud_secret_unavailable") || strings.Contains(unavailableOut, "PINAX_SECRET") {
		t.Fatalf("unavailable sync output invalid err=%v out=%s", unavailableErr, unavailableOut)
	}
	failedOut, failedErr := runCLIExpectError("sync", "push", "--target", "cloud", "--yes", "--base-revision", "rev_1", "--remote-revision", "rev_2", "--vault", root, "--json")
	if failedErr == nil || !strings.Contains(failedOut, "REVISION_CONFLICT") {
		t.Fatalf("failed conflict sync output invalid err=%v out=%s", failedErr, failedOut)
	}

	logsOut := runCLI(t, "sync", "logs", "list", "--vault", root, "--json")
	for _, want := range []string{"approval_required", "cloud_secret_unavailable", "failed", "pinax.sync_run.v1"} {
		if !strings.Contains(logsOut, want) {
			t.Fatalf("logs list missing %q:\n%s", want, logsOut)
		}
	}
	preview := runCLI(t, "sync", "logs", "prune", "--keep", "1", "--vault", root, "--json")
	if !strings.Contains(preview, "\"dry_run\":true") || !strings.Contains(preview, "delete_candidates") {
		t.Fatalf("prune preview invalid:\n%s", preview)
	}
	pruned := runCLI(t, "sync", "logs", "prune", "--keep", "1", "--yes", "--vault", root, "--json")
	if !strings.Contains(pruned, "\"deleted\":") {
		t.Fatalf("prune apply invalid:\n%s", pruned)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "sync-state.json")); err != nil {
		t.Fatalf("prune deleted sync-state: %v", err)
	}
}

func TestSyncRunPathRedactionPoliciesCLI(t *testing.T) {
	for _, tc := range []struct {
		policy      string
		wantPath    bool
		wantHash    bool
		wantOmitted bool
	}{
		{policy: "default", wantPath: true},
		{policy: "hash", wantHash: true},
		{policy: "omitted", wantOmitted: true},
	} {
		t.Run(tc.policy, func(t *testing.T) {
			root := t.TempDir()
			runCLI(t, "init", root, "--title", "Vault", "--json")
			writeCLIFixture(t, filepath.Join(root, "notes", "policy-secret.md"), "# Policy\nbody\n")
			runCLI(t, "cloud", "login", "--endpoint", "https://cloud.example.test", "--workspace", "ws", "--device", "dev", "--secret-ref", "env://PINAX_SECRET", "--vault", root, "--json")
			out := runCLI(t, "sync", "diff", "--target", "cloud", "--path-policy", tc.policy, "--vault", root, "--json")
			state := readCLIFile(t, filepath.Join(root, ".pinax", "sync-state.json"))
			var stateJSON map[string]any
			if err := json.Unmarshal([]byte(state), &stateJSON); err != nil {
				t.Fatalf("state json invalid: %v", err)
			}
			runID := stateJSON["last_sync_run_id"].(string)
			receipt := readCLIFile(t, filepath.Join(root, ".pinax", "sync-runs", time.Now().UTC().Format("2006"), time.Now().UTC().Format("01"), runID+".json"))
			combined := out + receipt
			if tc.wantPath && !strings.Contains(combined, "notes/policy-secret.md") {
				t.Fatalf("default policy omitted path:\n%s", combined)
			}
			if !tc.wantPath && strings.Contains(combined, "policy-secret.md") {
				t.Fatalf("%s policy leaked path:\n%s", tc.policy, combined)
			}
			if tc.wantHash && !strings.Contains(combined, "path_sha256:") {
				t.Fatalf("hash policy missing hash:\n%s", combined)
			}
			if tc.wantOmitted && strings.Contains(combined, "path_sha256:") {
				t.Fatalf("omitted policy kept hash:\n%s", combined)
			}
		})
	}
}

func TestSyncCloudPlannerCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "# Alpha\nbody\n")
	serverRevision := "rev_1"
	writeServerJSON := func(w http.ResponseWriter, status int, payload any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Fatalf("write server json: %v", err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer cloud-token" {
			t.Fatalf("server transport authorization header = %q", got)
		}
		workspacePath := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(workspacePath, "/head"):
			writeServerJSON(w, http.StatusOK, map[string]any{"revision_id": serverRevision, "manifest_blob_id": "manifest_initial"})
		case r.Method == http.MethodPost && strings.HasSuffix(workspacePath, "/blobs:batch-check"):
			writeServerJSON(w, http.StatusOK, map[string]any{"missing_blob_ids": []string{"blob_"}})
		case r.Method == http.MethodPost && strings.HasSuffix(workspacePath, "/blobs:sign-upload"):
			var req struct {
				BlobID   string `json:"blob_id"`
				BlobHash string `json:"blob_hash"`
				Size     int64  `json:"size"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode sign-upload request: %v", err)
			}
			if req.BlobID == "" || req.BlobHash == "" || req.Size < 0 {
				t.Fatalf("invalid sign-upload request: %#v", req)
			}
			writeServerJSON(w, http.StatusOK, map[string]any{"blob_id": req.BlobID, "object_key": "vaults/ws_123/" + req.BlobID, "method": "PUT", "url": "https://objects.example.local/" + req.BlobID})
		case r.Method == http.MethodPut && strings.Contains(workspacePath, "/blobs/"):
			writeServerJSON(w, http.StatusCreated, map[string]any{"status": "stored"})
		case r.Method == http.MethodPost && strings.HasSuffix(workspacePath, "/revisions"):
			if got := r.Header.Get("Idempotency-Key"); got == "" {
				t.Fatalf("server transport missing idempotency key")
			}
			serverRevision = "rev_server"
			writeServerJSON(w, http.StatusOK, map[string]any{"revision_id": serverRevision, "manifest_blob_id": "manifest_server"})
		default:
			t.Fatalf("unexpected server transport request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	runCLI(t, "cloud", "login", "--endpoint", server.URL, "--workspace", "ws_123", "--device", "dev_laptop", "--secret-ref", "plain:cloud-token", "--vault", root, "--json")
	diffOut := runCLI(t, "sync", "diff", "--target", "cloud", "--dry-run", "--base-revision", "rev_1", "--remote-revision", "rev_1", "--vault", root, "--json")
	assertJSONCommandStatus(t, diffOut, "sync.diff", "success")
	if !strings.Contains(diffOut, "\"dry_run\":\"true\"") || !strings.Contains(diffOut, "upload_blob") {
		t.Fatalf("sync diff missing dry-run plan:\n%s", diffOut)
	}
	pushDryRun := runCLI(t, "sync", "push", "--target", "cloud", "--dry-run", "--base-revision", "rev_1", "--remote-revision", "rev_1", "--vault", root, "--json")
	assertJSONCommandStatus(t, pushDryRun, "sync.push", "success")
	if !strings.Contains(pushDryRun, "upload_manifest") || strings.Contains(pushDryRun, "\"remote_write\":true") {
		t.Fatalf("sync push dry-run plan invalid:\n%s", pushDryRun)
	}
	pushApply := runCLI(t, "sync", "push", "--target", "cloud", "--yes", "--base-revision", "rev_1", "--remote-revision", "rev_1", "--vault", root, "--json")
	assertJSONCommandStatus(t, pushApply, "sync.push", "success")
	if !strings.Contains(pushApply, "\"remote_write\":true") || strings.Contains(pushApply, "cloud_api_unimplemented") || !strings.Contains(pushApply, "rev_server") {
		t.Fatalf("sync push --yes did not use server transport durable commit:\n%s", pushApply)
	}

	objectRoot := t.TempDir()
	runCLI(t, "cloud", "login", "--endpoint", "file://"+objectRoot, "--workspace", "ws_file", "--device", "dev_file", "--secret-ref", "env://PINAX_TEST_SECRET", "--vault", root, "--json")
	directPush := runCLI(t, "sync", "push", "--target", "cloud", "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, directPush, "sync.push", "success")
	if !strings.Contains(directPush, "\"remote_write\":true") || strings.Contains(directPush, "cloud_api_unimplemented") {
		t.Fatalf("direct cloud push did not complete durable remote write:\n%s", directPush)
	}
	stateReceipt := readCLIFile(t, filepath.Join(root, ".pinax", "sync-state.json"))
	if !strings.Contains(stateReceipt, "\"last_sync_run_id\"") || !strings.Contains(stateReceipt, "\"backend_kind\": \"embedded\"") || strings.Contains(stateReceipt, "\"remote_write\"") {
		t.Fatalf("direct cloud push current state invalid:\n%s", stateReceipt)
	}

	profileRoot := filepath.Join(root, "xdg")
	t.Setenv("XDG_CONFIG_HOME", profileRoot)
	runCLI(t, "profile", "add", "cloud-work", "--endpoint", server.URL, "--workspace", "ws_profile", "--device", "dev_profile", "--secret-ref", "plain:cloud-token")
	profilePush := runCLI(t, "sync", "push", "--target", "cloud-work", "--yes", "--base-revision", "rev_1", "--remote-revision", "rev_1", "--vault", root, "--json")
	assertJSONCommandStatus(t, profilePush, "sync.push", "success")
	if !strings.Contains(profilePush, "ws_profile") || !strings.Contains(profilePush, "dev_profile") || !strings.Contains(profilePush, "\"remote_write\":true") || strings.Contains(profilePush, "cloud_api_unimplemented") {
		t.Fatalf("sync push with profile target did not use server transport:\n%s", profilePush)
	}
	pullDryRun := runCLI(t, "sync", "pull", "--target", "cloud", "--dry-run", "--base-revision", "rev_1", "--remote-revision", "rev_1", "--vault", root, "--json")
	assertJSONCommandStatus(t, pullDryRun, "sync.pull", "success")
	if !strings.Contains(pullDryRun, "download_manifest") {
		t.Fatalf("sync pull dry-run plan invalid:\n%s", pullDryRun)
	}
	conflict, err := runCLIExpectError("sync", "push", "--target", "cloud", "--yes", "--base-revision", "rev_1", "--remote-revision", "rev_2", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("conflict push succeeded: %s", conflict)
	}
	assertJSONErrorCode(t, conflict, "REVISION_CONFLICT")
	for _, want := range []string{"pinax sync conflicts list --vault " + root + " --json", "pinax sync conflicts diff <file>", "pinax sync conflicts resolve <file>"} {
		if !strings.Contains(conflict, want) {
			t.Fatalf("revision conflict json missing action %q:\n%s", want, conflict)
		}
	}
	conflictAgent, agentErr := runCLIExpectError("sync", "push", "--target", "cloud", "--yes", "--base-revision", "rev_1", "--remote-revision", "rev_2", "--vault", root, "--agent")
	if agentErr == nil {
		t.Fatalf("conflict push agent succeeded: %s", conflictAgent)
	}
	for _, want := range []string{"command=sync.push", "error.code=REVISION_CONFLICT", "action.list=", "pinax sync conflicts list --vault " + root + " --json", "action.diff=", "pinax sync conflicts diff <file>", "action.resolve=", "pinax sync conflicts resolve <file>"} {
		if !strings.Contains(conflictAgent, want) {
			t.Fatalf("revision conflict agent missing action %q:\n%s", want, conflictAgent)
		}
	}
}

func TestSyncTargetCompletionAndInitUsesExistingCloudConfigCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	targetCompletion := runCLI(t, "__complete", "sync", "pull", "--target", "")
	for _, want := range []string{"cloud\tconfigured Cloud Sync backend", "s3\tS3-compatible direct backend", "git\tGit backend"} {
		if !strings.Contains(targetCompletion, want) {
			t.Fatalf("sync target completion missing %q:\n%s", want, targetCompletion)
		}
	}
	runCLI(t, "cloud", "backend", "set", "s3", "--bucket", "notes", "--region", "us-east-1", "--prefix", "pinax-sync/", "--endpoint", "http://127.0.0.1:9000", "--workspace", "ec", "--device", "dev", "--vault", root, "--json")
	initOut := runCLI(t, "sync", "init", "--vault", root, "--json")
	assertJSONCommandStatus(t, initOut, "sync.init", "success")
	for _, want := range []string{"\"backend_kind\":\"s3-direct\"", "s3://notes/pinax-sync", "\"workspace\":\"ec\"", "\"device\":\"dev\""} {
		if !strings.Contains(initOut, want) {
			t.Fatalf("sync init did not reuse cloud config %q:\n%s", want, initOut)
		}
	}
}

func TestCloudStateCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	missing, err := runCLIExpectError("cloud", "status", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("cloud status without config succeeded: %s", missing)
	}
	assertJSONErrorCode(t, missing, "cloud_not_configured")
	loginOut := runCLI(t, "cloud", "login", "--endpoint", "https://cloud.example.test", "--workspace", "ws_123", "--device", "dev_laptop", "--secret-ref", "op://pinax/cloud-token", "--vault", root, "--json")
	if strings.Contains(loginOut, "cloud-token") || strings.Contains(loginOut, "Authorization") {
		t.Fatalf("cloud login leaked secret reference/token:\n%s", loginOut)
	}
	assertJSONCommandStatus(t, loginOut, "cloud.login", "success")
	statusOut := runCLI(t, "cloud", "status", "--vault", root, "--json")
	assertJSONCommandStatus(t, statusOut, "cloud.status", "success")
	if !strings.Contains(statusOut, "\"configured\":\"true\"") || !strings.Contains(statusOut, "dev_laptop") {
		t.Fatalf("cloud status missing facts:\n%s", statusOut)
	}
	doctorOut := runCLI(t, "cloud", "doctor", "--vault", root, "--json")
	assertJSONCommandStatus(t, doctorOut, "cloud.doctor", "success")
	logoutOut := runCLI(t, "cloud", "logout", "--vault", root, "--json")
	assertJSONCommandStatus(t, logoutOut, "cloud.logout", "success")
	loggedOut := runCLI(t, "cloud", "status", "--vault", root, "--json")
	if !strings.Contains(loggedOut, "logged_out") {
		t.Fatalf("cloud status after logout missing logged_out:\n%s", loggedOut)
	}
}

func TestCloudBackendSetS3CLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out := runCLI(t, "cloud", "backend", "set", "s3", "--bucket", "notes", "--region", "us-east-1", "--prefix", "pinax-sync/", "--endpoint", "http://10.10.1.102:9010", "--profile", "work", "--workspace", "personal", "--device", "laptop", "--vault", root, "--json")
	assertJSONCommandStatus(t, out, "cloud.backend.set", "success")
	for _, want := range []string{"\"backend_kind\":\"s3-direct\"", "s3://notes/pinax-sync", "\"s3\":{", "\"endpoint\":\"http://10.10.1.102:9010\"", "\"path_style\":true", "personal", "laptop"} {
		if !strings.Contains(out, want) {
			t.Fatalf("cloud backend set s3 missing %q:\n%s", want, out)
		}
	}
	for _, leaked := range []string{"access_key", "secret_key", "Authorization", "Cookie", "AKIA", "refresh_token"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("cloud backend set s3 leaked %q:\n%s", leaked, out)
		}
	}
	configYAML := readCLIFile(t, filepath.Join(root, ".pinax", "cloud", "config.yaml"))
	for _, want := range []string{"backend_kind: s3-direct", "bucket: notes", "prefix: pinax-sync/", "endpoint: http://10.10.1.102:9010", "profile: work", "path_style: true", "secret_ref: profile://work"} {
		if !strings.Contains(configYAML, want) {
			t.Fatalf("cloud yaml config missing %q:\n%s", want, configYAML)
		}
	}
	for _, escaped := range []string{"http%3A", "?endpoint=", "&profile="} {
		if strings.Contains(configYAML, escaped) {
			t.Fatalf("cloud yaml config contains escaped endpoint fragment %q:\n%s", escaped, configYAML)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "cloud", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("cloud backend set s3 should write config.yaml as primary config, err=%v", err)
	}
	status := runCLI(t, "cloud", "status", "--vault", root, "--json")
	assertJSONCommandStatus(t, status, "cloud.status", "success")
	if !strings.Contains(status, "\"backend_kind\":\"s3-direct\"") || !strings.Contains(status, "s3://notes/pinax-sync") {
		t.Fatalf("cloud status missing s3 backend facts:\n%s", status)
	}
	doctor := runCLI(t, "cloud", "doctor", "--vault", root, "--json")
	assertJSONCommandStatus(t, doctor, "cloud.doctor", "success")
	for _, want := range []string{"\"backend_kind\":\"s3-direct\"", "\"auth_boundary\":\"provider_credentials\"", "\"server_audit\":false"} {
		if !strings.Contains(doctor, want) {
			t.Fatalf("cloud doctor missing direct boundary %q:\n%s", want, doctor)
		}
	}
}

func TestCloudBackendSetRcloneCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out := runCLI(t, "cloud", "backend", "set", "rclone", "--remote", "onedrive:PinaxSync", "--workspace", "personal", "--device", "laptop", "--vault", root, "--json")
	assertJSONCommandStatus(t, out, "cloud.backend.set", "success")
	for _, want := range []string{"\"backend_kind\":\"rclone-direct\"", "rclone://onedrive/PinaxSync", "personal", "laptop"} {
		if !strings.Contains(out, want) {
			t.Fatalf("cloud backend set rclone missing %q:\n%s", want, out)
		}
	}
	for _, leaked := range []string{"refresh_token", "Authorization", "Cookie", "client_secret"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("cloud backend set rclone leaked %q:\n%s", leaked, out)
		}
	}
	doctor := runCLI(t, "cloud", "doctor", "--vault", root, "--json")
	assertJSONCommandStatus(t, doctor, "cloud.doctor", "success")
	if !strings.Contains(doctor, "\"auth_boundary\":\"provider_credentials\"") || !strings.Contains(doctor, "\"server_audit\":false") {
		t.Fatalf("cloud doctor missing rclone boundary:\n%s", doctor)
	}
}

func TestDirectCloudPushPullCLI(t *testing.T) {
	objectRoot := t.TempDir()
	deviceA := t.TempDir()
	deviceB := t.TempDir()
	runCLI(t, "init", deviceA, "--title", "Device A", "--json")
	runCLI(t, "init", deviceB, "--title", "Device B", "--json")
	writeCLIFixture(t, filepath.Join(deviceA, "notes", "alpha.md"), "# Alpha\n\nfrom device A\n")
	runCLI(t, "cloud", "login", "--endpoint", "file://"+objectRoot, "--workspace", "personal", "--device", "laptop", "--secret-ref", "env://PINAX_TEST_SECRET", "--vault", deviceA, "--json")
	runCLI(t, "cloud", "login", "--endpoint", "file://"+objectRoot, "--workspace", "personal", "--device", "desktop", "--secret-ref", "env://PINAX_TEST_SECRET", "--vault", deviceB, "--json")
	push := runCLI(t, "sync", "push", "--target", "cloud", "--yes", "--vault", deviceA, "--json")
	assertJSONCommandStatus(t, push, "sync.push", "success")
	pull := runCLI(t, "sync", "pull", "--target", "cloud", "--yes", "--vault", deviceB, "--json")
	assertJSONCommandStatus(t, pull, "sync.pull", "success")
	if !strings.Contains(pull, "\"remote_write\":false") || !strings.Contains(pull, "\"files_applied\":1") {
		t.Fatalf("direct pull output invalid:\n%s", pull)
	}
	got := readCLIFile(t, filepath.Join(deviceB, "notes", "alpha.md"))
	if !strings.Contains(got, "from device A") {
		t.Fatalf("pulled note missing remote body:\n%s", got)
	}
	writeCLIFixture(t, filepath.Join(deviceB, "notes", "alpha.md"), "# Alpha\n\nlocal desktop edit\n")
	writeCLIFixture(t, filepath.Join(deviceA, "notes", "alpha.md"), "# Alpha\n\nupdated from device A\n")
	pushAgain := runCLI(t, "sync", "push", "--target", "cloud", "--yes", "--vault", deviceA, "--json")
	assertJSONCommandStatus(t, pushAgain, "sync.push", "success")
	pullConflict := runCLI(t, "sync", "pull", "--target", "cloud", "--yes", "--vault", deviceB, "--json")
	assertJSONCommandStatus(t, pullConflict, "sync.pull", "success")
	updated := readCLIFile(t, filepath.Join(deviceB, "notes", "alpha.md"))
	if !strings.Contains(updated, "updated from device A") {
		t.Fatalf("pulled trunk missing update:\n%s", updated)
	}
	conflicts, err := filepath.Glob(filepath.Join(deviceB, "notes", "alpha.*.conflict.md"))
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("expected one conflict copy, got %v err=%v", conflicts, err)
	}
	conflictBody := readCLIFile(t, conflicts[0])
	if !strings.Contains(conflictBody, "local desktop edit") {
		t.Fatalf("conflict copy lost local edit:\n%s", conflictBody)
	}
}

func TestSyncConflictsCommandsUseProjectionOutputModesAndReceiptsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	mainRel := filepath.ToSlash(filepath.Join("notes", "alpha.md"))
	conflictRel := filepath.ToSlash(filepath.Join("notes", "alpha.20260612010203.conflict.md"))
	writeCLIFixture(t, filepath.Join(root, filepath.FromSlash(mainRel)), "# Alpha\n\nremote trunk\n")
	writeCLIFixture(t, filepath.Join(root, filepath.FromSlash(conflictRel)), "# Alpha\n\nlocal edit\n")
	listDefault := runCLI(t, "sync", "conflicts", "list", "--vault", root)
	if strings.HasPrefix(strings.TrimSpace(listDefault), "{") || !strings.Contains(listDefault, conflictRel) || !strings.Contains(listDefault, "pinax sync conflicts list --vault "+root+" --json") {
		t.Fatalf("conflict list default output invalid:\n%s", listDefault)
	}

	listJSON := runCLI(t, "sync", "conflicts", "list", "--vault", root, "--json")
	assertJSONCommandStatus(t, listJSON, "sync.conflicts.list", "success")
	if !strings.Contains(listJSON, conflictRel) || !strings.Contains(listJSON, mainRel) || !strings.Contains(listJSON, "pinax sync conflicts diff "+conflictRel) || !strings.Contains(listJSON, "pinax sync conflicts resolve "+conflictRel) {
		t.Fatalf("conflict list json missing conflict paths/actions:\n%s", listJSON)
	}

	listAgent := runCLI(t, "sync", "conflicts", "list", "--vault", root, "--agent")
	for _, want := range []string{"mode=agent", "command=sync.conflicts.list", "fact.conflict.1.file=" + conflictRel, "action.diff=", "action.resolve="} {
		if !strings.Contains(listAgent, want) {
			t.Fatalf("conflict list agent missing %q:\n%s", want, listAgent)
		}
	}

	diffJSON := runCLI(t, "sync", "conflicts", "diff", conflictRel, "--vault", root, "--json")
	assertJSONCommandStatus(t, diffJSON, "sync.conflicts.diff", "success")
	if !strings.Contains(diffJSON, "--- "+mainRel) || !strings.Contains(diffJSON, "+++ "+conflictRel) || !strings.Contains(diffJSON, "pinax sync conflicts resolve "+conflictRel) {
		t.Fatalf("conflict diff json missing stable diff/actions:\n%s", diffJSON)
	}

	showEvents := runCLI(t, "sync", "conflicts", "show", conflictRel, "--vault", root, "--events")
	events := parseNDJSONEvents(t, showEvents)
	if !hasEventType(events, "start") || !hasEventType(events, "end") || !strings.Contains(showEvents, "sync.conflicts.show") || strings.Contains(showEvents, "local edit") {
		t.Fatalf("conflict show events should be structural and redacted: %#v\n%s", events, showEvents)
	}

	showExplain := runCLI(t, "sync", "conflicts", "show", conflictRel, "--vault", root, "--explain")
	if !strings.Contains(showExplain, "Recommended next step: pinax sync conflicts diff "+conflictRel) || strings.Contains(showExplain, "local edit") {
		t.Fatalf("conflict show explain missing next action or leaked body:\n%s", showExplain)
	}

	withoutYes, err := runCLIExpectError("sync", "conflicts", "resolve", conflictRel, "--keep-remote", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("conflict resolve without --yes succeeded: %s", withoutYes)
	}
	assertJSONErrorCode(t, withoutYes, "approval_required")
	if !fileExists(filepath.Join(root, filepath.FromSlash(conflictRel))) {
		t.Fatalf("conflict file was removed without --yes")
	}

	resolveJSON := runCLI(t, "sync", "conflicts", "resolve", conflictRel, "--keep-remote", "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, resolveJSON, "sync.conflicts.resolve", "success")
	if !strings.Contains(resolveJSON, "\"receipt_path\"") || !strings.Contains(resolveJSON, "sync-conflicts/receipts/") || strings.Contains(resolveJSON, "local edit") || strings.Contains(resolveJSON, "remote trunk") {
		t.Fatalf("conflict resolve json missing safe receipt or leaked body:\n%s", resolveJSON)
	}
	if fileExists(filepath.Join(root, filepath.FromSlash(conflictRel))) {
		t.Fatalf("conflict file still exists after keep-remote resolve")
	}
	eventLog := readCLIFile(t, filepath.Join(root, ".pinax", "events.jsonl"))
	if !strings.Contains(eventLog, "sync.conflict.resolve") || strings.Contains(eventLog, "local edit") || strings.Contains(eventLog, "remote trunk") {
		t.Fatalf("resolve event missing or leaked body:\n%s", eventLog)
	}
}

func TestSyncConflictNextActionsAppearInSyncJSONAndAgentOutputsCLI(t *testing.T) {
	objectRoot := t.TempDir()
	deviceA := t.TempDir()
	deviceB := t.TempDir()
	runCLI(t, "init", deviceA, "--title", "Device A", "--json")
	runCLI(t, "init", deviceB, "--title", "Device B", "--json")
	writeCLIFixture(t, filepath.Join(deviceA, "notes", "alpha.md"), "# Alpha\n\nfrom A\n")
	runCLI(t, "cloud", "login", "--endpoint", "file://"+objectRoot, "--workspace", "personal", "--device", "laptop", "--secret-ref", "env://PINAX_TEST_SECRET", "--vault", deviceA, "--json")
	runCLI(t, "cloud", "login", "--endpoint", "file://"+objectRoot, "--workspace", "personal", "--device", "desktop", "--secret-ref", "env://PINAX_TEST_SECRET", "--vault", deviceB, "--json")
	runCLI(t, "sync", "push", "--target", "cloud", "--yes", "--vault", deviceA, "--json")
	runCLI(t, "sync", "pull", "--target", "cloud", "--yes", "--vault", deviceB, "--json")

	writeCLIFixture(t, filepath.Join(deviceB, "notes", "alpha.md"), "# Alpha\n\nlocal JSON conflict\n")
	writeCLIFixture(t, filepath.Join(deviceA, "notes", "alpha.md"), "# Alpha\n\nremote JSON conflict\n")
	runCLI(t, "sync", "push", "--target", "cloud", "--yes", "--vault", deviceA, "--json")
	pullJSON := runCLI(t, "sync", "pull", "--target", "cloud", "--yes", "--vault", deviceB, "--json")
	assertJSONCommandStatus(t, pullJSON, "sync.pull", "success")
	for _, want := range []string{"\"conflicts\":\"1\"", "pinax sync conflicts list --vault " + deviceB + " --json", "pinax sync conflicts diff notes/alpha.", "pinax sync conflicts resolve notes/alpha."} {
		if !strings.Contains(pullJSON, want) {
			t.Fatalf("sync pull json missing conflict action %q:\n%s", want, pullJSON)
		}
	}

	writeCLIFixture(t, filepath.Join(deviceB, "notes", "alpha.md"), "# Alpha\n\nlocal agent conflict\n")
	writeCLIFixture(t, filepath.Join(deviceA, "notes", "alpha.md"), "# Alpha\n\nremote agent conflict\n")
	runCLI(t, "sync", "push", "--target", "cloud", "--yes", "--vault", deviceA, "--json")
	pullAgent := runCLI(t, "sync", "pull", "--target", "cloud", "--yes", "--vault", deviceB, "--agent")
	for _, want := range []string{"command=sync.pull", "fact.conflicts=1", "action.list=", "pinax sync conflicts list --vault " + deviceB + " --json", "action.diff=", "action.resolve="} {
		if !strings.Contains(pullAgent, want) {
			t.Fatalf("sync pull agent missing conflict action %q:\n%s", want, pullAgent)
		}
	}
}

func TestSyncConflictsCobraLayerDoesNotResolveFilesDirectlyCLI(t *testing.T) {
	source := readCLIFile(t, filepath.Join("..", "..", "internal", "cli", "sync_conflicts_cmd.go"))
	for _, forbidden := range []string{"os.Rename", "os.Remove", "fmt.Println", "fmt.Printf"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("sync conflicts Cobra layer still contains %s", forbidden)
		}
	}
}

func TestBackendLegacyStorageProjection(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	// Only set legacy storage.json, no backends.json yet.
	runCLI(t, "storage", "set-s3", "--bucket", "notes", "--region", "us-east-1", "--vault", root, "--json")
	// Remove any backends.json that might have been created.
	if err := os.Remove(filepath.Join(root, ".pinax", "backends.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove backends.json: %v", err)
	}
	listOut := runCLI(t, "backend", "list", "--vault", root, "--json")
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("backend list legacy json invalid: %v\n%s", err, listOut)
	}
	facts := listEnvelope["facts"].(map[string]any)
	if facts["backends"] != "1" {
		t.Fatalf("expected 1 backend from legacy storage: %s", listOut)
	}
}

func TestPlanningWorkflowsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	help := runCLI(t, "plan", "--help")
	for _, want := range []string{"daily", "weekly", "monthly", "actions", "snapshot"} {
		if !strings.Contains(help, want) {
			t.Fatalf("plan help missing %q:\n%s", want, help)
		}
	}

	// plan daily dry-run
	dailyDryRun := runCLI(t, "plan", "daily", "--dry-run", "--vault", root, "--json")
	var dailyEnvelope map[string]any
	if err := json.Unmarshal([]byte(dailyDryRun), &dailyEnvelope); err != nil {
		t.Fatalf("plan daily json invalid: %v\n%s", err, dailyDryRun)
	}
	if dailyEnvelope["command"] != "plan.daily" || dailyEnvelope["status"] != "success" {
		t.Fatalf("plan daily envelope = %#v", dailyEnvelope)
	}
	dailyFacts := dailyEnvelope["facts"].(map[string]any)
	if dailyFacts["period"] != "daily" || dailyFacts["dry_run"] != "true" || dailyFacts["max_commitments"] != "3" {
		t.Fatalf("plan daily facts = %#v", dailyFacts)
	}

	// plan daily with save
	dailyOut := runCLI(t, "plan", "daily", "--save", "--yes", "--vault", root, "--json")
	var dailySaveEnvelope map[string]any
	if err := json.Unmarshal([]byte(dailyOut), &dailySaveEnvelope); err != nil {
		t.Fatalf("plan daily save json invalid: %v\n%s", err, dailyOut)
	}
	if dailySaveEnvelope["command"] != "plan.daily" {
		t.Fatalf("plan daily save envelope = %#v", dailySaveEnvelope)
	}
	dailySaveFacts := dailySaveEnvelope["facts"].(map[string]any)
	if dailySaveFacts["dry_run"] != nil {
		t.Fatalf("plan daily save should not be dry_run: %#v", dailySaveFacts)
	}
	if dailySaveFacts["saved_path"] == "" || !strings.Contains(dailySaveFacts["saved_path"].(string), ".pinax/planning/snapshots/") {
		t.Fatalf("plan daily saved_path invalid: %#v", dailySaveFacts)
	}

	// plan weekly
	weeklyOut := runCLI(t, "plan", "weekly", "--dry-run", "--vault", root, "--json")
	var weeklyEnvelope map[string]any
	if err := json.Unmarshal([]byte(weeklyOut), &weeklyEnvelope); err != nil {
		t.Fatalf("plan weekly json invalid: %v\n%s", err, weeklyOut)
	}
	weeklyFacts := weeklyEnvelope["facts"].(map[string]any)
	if weeklyFacts["period"] != "weekly" || weeklyFacts["max_commitments"] != "7" {
		t.Fatalf("plan weekly facts = %#v", weeklyFacts)
	}

	// plan monthly
	monthlyOut := runCLI(t, "plan", "monthly", "--dry-run", "--vault", root, "--json")
	var monthlyEnvelope map[string]any
	if err := json.Unmarshal([]byte(monthlyOut), &monthlyEnvelope); err != nil {
		t.Fatalf("plan monthly json invalid: %v\n%s", err, monthlyOut)
	}
	monthlyFacts := monthlyEnvelope["facts"].(map[string]any)
	if monthlyFacts["period"] != "monthly" || monthlyFacts["max_commitments"] != "15" {
		t.Fatalf("plan monthly facts = %#v", monthlyFacts)
	}

	// plan actions dry-run
	actionsDryRun := runCLI(t, "plan", "actions", "--from", "daily", "--vault", root, "--json")
	var actionsEnvelope map[string]any
	if err := json.Unmarshal([]byte(actionsDryRun), &actionsEnvelope); err != nil {
		t.Fatalf("plan actions json invalid: %v\n%s", err, actionsDryRun)
	}
	if actionsEnvelope["command"] != "plan.actions" {
		t.Fatalf("plan actions envelope = %#v", actionsEnvelope)
	}

	// plan actions with save
	actionsSave := runCLI(t, "plan", "actions", "--from", "daily", "--save", "--vault", root, "--json")
	var actionsSaveEnvelope map[string]any
	if err := json.Unmarshal([]byte(actionsSave), &actionsSaveEnvelope); err != nil {
		t.Fatalf("plan actions save json invalid: %v\n%s", err, actionsSave)
	}
	actionsSaveFacts := actionsSaveEnvelope["facts"].(map[string]any)
	if actionsSaveFacts["saved_path"] == "" || !strings.Contains(actionsSaveFacts["saved_path"].(string), ".pinax/planning/actions/") {
		t.Fatalf("plan actions saved_path invalid: %#v", actionsSaveFacts)
	}

	// plan snapshot
	snapshotOut := runCLI(t, "plan", "snapshot", "--vault", root, "--json")
	var snapshotEnvelope map[string]any
	if err := json.Unmarshal([]byte(snapshotOut), &snapshotEnvelope); err != nil {
		t.Fatalf("plan snapshot json invalid: %v\n%s", err, snapshotOut)
	}
	if snapshotEnvelope["command"] != "plan.snapshot" {
		t.Fatalf("plan snapshot envelope = %#v", snapshotEnvelope)
	}
	snapshotFacts := snapshotEnvelope["facts"].(map[string]any)
	if snapshotFacts["snapshot_id"] == "" || snapshotFacts["saved_path"] == "" {
		t.Fatalf("plan snapshot facts = %#v", snapshotFacts)
	}

	// plan daily agent output
	agentOut := runCLI(t, "plan", "daily", "--dry-run", "--vault", root, "--agent")
	for _, want := range []string{"command=plan.daily", "status=success", "fact.period=daily", "fact.dry_run=true"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("plan daily agent output missing %q:\n%s", want, agentOut)
		}
	}
}

func TestBackendObjectListAndStatCommandsReadLocalBlobStore(t *testing.T) {
	root := t.TempDir()
	backendRoot := filepath.Join(root, "backend-store")
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "backend", "add", "local", "work-local", "--root", backendRoot, "--vault", root, "--json")
	if err := os.MkdirAll(filepath.Join(backendRoot, "pinax"), 0o700); err != nil {
		t.Fatalf("mkdir backend fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backendRoot, "pinax", "manifest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write backend fixture: %v", err)
	}

	lsOut := runCLI(t, "backend", "object", "list", "work-local", "pinax", "--vault", root, "--json")
	var lsEnvelope map[string]any
	if err := json.Unmarshal([]byte(lsOut), &lsEnvelope); err != nil {
		t.Fatalf("backend ls json invalid: %v\n%s", err, lsOut)
	}
	if lsEnvelope["command"] != "backend.object.list" || !strings.Contains(lsOut, "manifest.json") {
		t.Fatalf("backend object list did not include local object: %#v\n%s", lsEnvelope, lsOut)
	}

	statOut := runCLI(t, "backend", "object", "stat", "work-local", "pinax/manifest.json", "--vault", root, "--json")
	var statEnvelope map[string]any
	if err := json.Unmarshal([]byte(statOut), &statEnvelope); err != nil {
		t.Fatalf("backend stat json invalid: %v\n%s", err, statOut)
	}
	if statEnvelope["command"] != "backend.object.stat" || !strings.Contains(statOut, "revision") {
		t.Fatalf("backend object stat did not include revision: %#v\n%s", statEnvelope, statOut)
	}
}
