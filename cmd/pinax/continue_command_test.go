package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestContinueCommand_HelpExists verifies the experimental `pinax continue`
// intent facade exposes English help text describing the bounded continuity
// pack and its flags.
func TestContinueCommand_HelpExists(t *testing.T) {
	help := runCLI(t, "continue", "--help")
	for _, want := range []string{
		"Compile a bounded, permission-first continuity pack",
		"experimental additive facade",
		"Usage",
		"Flags",
		"--task",
		"--intent",
		"--max-items",
		"--max-chars",
		"--handoff",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("continue --help missing %q:\n%s", want, help)
		}
	}
}

// TestContinueCommand_InCommandTree verifies the root help lists the additive
// `continue` entry point alongside the existing command tree.
func TestContinueCommand_InCommandTree(t *testing.T) {
	rootHelp := runCLI(t, "--help")
	if !strings.Contains(rootHelp, "  continue") {
		t.Fatalf("root --help should list continue:\n%s", rootHelp)
	}
}

// TestContinueCommand_JSONOutput verifies `pinax continue --json` emits a
// well-formed agent-protocol projection envelope with the expected command,
// status, mode and the continuity facts surface.
func TestContinueCommand_JSONOutput(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	out := runCLI(t, "continue", "--task", "ship the continuity facade", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("continue --json invalid envelope: %v\n%s", err, out)
	}
	if envelope["command"] != "continue" {
		t.Fatalf("command = %#v, want continue", envelope["command"])
	}
	if envelope["status"] != "success" {
		t.Fatalf("status = %#v, want success", envelope["status"])
	}
	if envelope["mode"] != "json" {
		t.Fatalf("mode = %#v, want json", envelope["mode"])
	}
	facts, ok := envelope["facts"].(map[string]any)
	if !ok {
		t.Fatalf("facts missing or wrong type: %#v", envelope["facts"])
	}
	for _, key := range []string{"schema_version", "section_count", "handoff_status", "truncated", "source_coverage"} {
		if _, ok := facts[key]; !ok {
			t.Fatalf("continue facts missing %s: %#v", key, facts)
		}
	}
	if envelope["summary"] == nil || envelope["summary"] == "" {
		t.Fatalf("continue projection missing summary: %#v", envelope)
	}
}

// TestContinueCommand_ExperimentalFlag verifies the continuity projection is
// explicitly tagged experimental=true so machine consumers can gate on the
// additive, not-yet-stable surface.
func TestContinueCommand_ExperimentalFlag(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	out := runCLI(t, "continue", "--vault", root, "--json")
	facts := jsonParseFacts(t, out)
	if facts["experimental"] != "true" {
		t.Fatalf("experimental fact = %#v, want \"true\": %#v", facts["experimental"], facts)
	}
}

// TestContinueCommand_NoOldCommandChange verifies the experimental `continue`
// facade is purely additive: the pre-existing agent, memory, and brain entry
// points remain registered and their help trees are unchanged.
func TestContinueCommand_NoOldCommandChange(t *testing.T) {
	rootHelp := runCLI(t, "--help")
	for _, cmd := range []string{"agent", "memory", "brain"} {
		if !strings.Contains(rootHelp, "  "+cmd) {
			t.Fatalf("root --help should still list %q:\n%s", cmd, rootHelp)
		}
	}

	agentHelp := runCLI(t, "agent", "--help")
	for _, want := range []string{"context", "memory", "handoff", "feedback", "status"} {
		if !strings.Contains(agentHelp, want) {
			t.Fatalf("agent --help missing %q (existing tree changed):\n%s", want, agentHelp)
		}
	}

	memoryHelp := runCLI(t, "memory", "--help")
	for _, want := range []string{"capture", "list", "recall", "context"} {
		if !strings.Contains(memoryHelp, want) {
			t.Fatalf("memory --help missing %q (existing tree changed):\n%s", want, memoryHelp)
		}
	}

	brainHelp := runCLI(t, "brain", "--help")
	for _, want := range []string{"answer", "maintain"} {
		if !strings.Contains(brainHelp, want) {
			t.Fatalf("brain --help missing %q (existing tree changed):\n%s", want, brainHelp)
		}
	}
}
