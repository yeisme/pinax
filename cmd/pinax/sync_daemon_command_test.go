package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncDaemonCommandHelp(t *testing.T) {
	out := runCLI(t, "sync", "daemon", "--help")
	for _, want := range []string{"run", "start", "status", "stop", "logs"} {
		if !strings.Contains(out, want) {
			t.Fatalf("sync daemon help missing %q:\n%s", want, out)
		}
	}
	runHelp := runCLI(t, "sync", "daemon", "run", "--help")
	for _, want := range []string{"--target", "--poll-interval", "--sync-timeout", "--yes"} {
		if !strings.Contains(runHelp, want) {
			t.Fatalf("sync daemon run help missing %q:\n%s", want, runHelp)
		}
	}
}

func TestSyncDaemonOutputContract(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	statusOut := runCLI(t, "sync", "daemon", "status", "--vault", root, "--json")
	assertJSONCommandStatus(t, statusOut, "sync.daemon.status", "success")
	for _, want := range []string{`"daemon_status":"stopped"`, `"runtime_dir":".pinax/sync-daemon"`, `"remote_write"`} {
		if strings.Contains(statusOut, "Authorization") || strings.Contains(statusOut, "Bearer") {
			t.Fatalf("sync daemon status leaked sensitive data:\n%s", statusOut)
		}
		if want == `"remote_write"` {
			continue
		}
		if !strings.Contains(statusOut, want) {
			t.Fatalf("sync daemon status missing %q:\n%s", want, statusOut)
		}
	}

	logsOut := runCLI(t, "sync", "daemon", "logs", "--vault", root, "--agent")
	for _, want := range []string{"command=sync.daemon.logs", "status=success", "fact.daemon_status=stopped"} {
		if !strings.Contains(logsOut, want) {
			t.Fatalf("sync daemon logs agent missing %q:\n%s", want, logsOut)
		}
	}
}

func TestSyncDaemonRequiresApprovalForWrites(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	for _, args := range [][]string{{"sync", "daemon", "run", "--once", "--target", "cloud", "--vault", root, "--json"}, {"sync", "daemon", "start", "--target", "cloud", "--vault", root, "--json"}} {
		out, err := runCLIExpectError(args...)
		if err == nil {
			t.Fatalf("sync daemon command should require approval: %v\n%s", args, out)
		}
		assertJSONErrorCode(t, out, "approval_required")
	}
}

func TestSyncDaemonRunLiveOutputModes(t *testing.T) {
	root := t.TempDir()
	store := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "live.md"), "# Live\n\ndaemon live output\n")
	runCLI(t, "cloud", "login", "--endpoint", "file://"+store, "--workspace", "ws", "--device", "dev", "--secret-ref", "test-secret", "--vault", root, "--json")

	humanOut := runCLI(t, "sync", "daemon", "run", "--once", "--target", "cloud", "--vault", root, "--yes")
	for _, want := range []string{"sync_started", "push_completed"} {
		if !strings.Contains(humanOut, want) {
			t.Fatalf("human daemon output missing %q:\n%s", want, humanOut)
		}
	}
	if strings.Contains(humanOut, "Authorization") || strings.Contains(humanOut, "Bearer") {
		t.Fatalf("human daemon output leaked sensitive data:\n%s", humanOut)
	}

	eventsOut := runCLI(t, "sync", "daemon", "run", "--once", "--target", "cloud", "--vault", root, "--yes", "--events")
	events := parseNDJSONEvents(t, eventsOut)
	for _, want := range []string{"start", "sync_started", "push_completed", "end"} {
		if !hasEventType(events, want) {
			t.Fatalf("daemon events missing %q:\n%s", want, eventsOut)
		}
	}
	for _, event := range events {
		if event["command"] != "sync.daemon.run" || event["mode"] != "events" {
			t.Fatalf("event contract invalid: %#v", event)
		}
	}

	jsonOut := runCLI(t, "sync", "daemon", "run", "--once", "--target", "cloud", "--vault", root, "--yes", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("json mode mixed non-json output: %v\n%s", err, jsonOut)
	}
	if envelope["command"] != "sync.daemon.run" || envelope["mode"] != "json" {
		t.Fatalf("json envelope invalid: %#v", envelope)
	}
}
