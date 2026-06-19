package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestAPIServeMachineModesAreQuietAndWriteModeConflictIsStable(t *testing.T) {
	root := t.TempDir()
	for _, mode := range []string{"--json", "--agent"} {
		stdout, stderr, err := runAPIServeUntilCanceled(t, root, "api", "serve", "--port", "0", "--vault", root, mode)
		if err == nil || stderr != "" {
			t.Fatalf("api serve %s should fail without diagnostics on stderr: err=%v stderr=%q stdout=%s", mode, err, stderr, stdout)
		}
		if !strings.Contains(stdout, "unsupported_output_mode") || strings.Contains(stdout, "Pinax local API") || strings.Contains(stdout, "http://127.0.0.1:") {
			t.Fatalf("api serve %s stdout contract violated: %s", mode, stdout)
		}
	}
	stdout, stderr, err := runCLISeparate("api", "serve", "--readonly", "--allow-write", "--vault", root, "--json")
	if err == nil || stderr != "" || !strings.Contains(stdout, "write_mode_conflict") {
		t.Fatalf("api serve write mode conflict err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
}

func TestAPIServeLifecycleOutput(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, err := runAPIServeUntilCanceled(t, root, "api", "serve", "--port", "0", "--vault", root)
	if err != nil || stdout != "" {
		t.Fatalf("api serve default err=%v stdout=%q stderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "pinax api ready") || !strings.Contains(stderr, "http://127.0.0.1:") || !strings.Contains(stderr, "auth_mode") {
		t.Fatalf("api serve default stderr missing zap startup log: %s", stderr)
	}

	stdout, stderr, err = runAPIServeUntilCanceled(t, root, "api", "serve", "--readonly", "--port", "0", "--vault", root, "--events")
	if err != nil || stderr != "" {
		t.Fatalf("api serve events err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	events := parseNDJSONEvents(t, stdout)
	for _, want := range []string{"start", "ready", "shutdown"} {
		if !hasEventType(events, want) {
			t.Fatalf("api serve events missing %s: %#v\n%s", want, events, stdout)
		}
	}
	for _, event := range events {
		if event["type"] == "ready" && !strings.Contains(fmt.Sprint(event["url"]), "http://127.0.0.1:") {
			t.Fatalf("ready event missing localhost URL: %#v", event)
		}
		if strings.Contains(fmt.Sprint(event["message"]), "Temp token:") {
			t.Fatalf("events leaked temp token log: %#v", event)
		}
	}
}

func TestAPIRoutesHumanOutputListsEndpointsCLI(t *testing.T) {
	root := t.TempDir()
	out := runCLI(t, "api", "routes", "--vault", root)
	for _, want := range []string{"GET /v1/projects/{slug}/board", "CALL Pinax.Note.Read", "project.board.show"} {
		if !strings.Contains(out, want) {
			t.Fatalf("api routes human output missing %q:\n%s", want, out)
		}
	}
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("api routes human output should not be JSON:\n%s", out)
	}
}
