package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	gitstore "github.com/yeisme/pinax/internal/git"
)

// proofLoopCommand is a proof loop command shape exercised by the contract
// tests. Each entry names the subcommand and the flags needed to render it in
// json, agent and events modes against a freshly initialized vault.
type proofLoopCommand struct {
	name string
	args []string
}

// proofLoopCommands lists the commands that form the agent-safe proof loop and
// must render a stable shared projection across every output mode. The search
// query uses "contract" so it matches the note TITLE only — search snippets
// intentionally show bounded body excerpts around body matches, which is a
// designed feature, not a projection leak. Testing a title-only match isolates
// the envelope-structure and recursive-body-leak assertions from snippet behavior.
func proofLoopCommands(vault string) []proofLoopCommand {
	return []proofLoopCommand{
		{"vault.stats", []string{"vault", "stats", "--vault", vault}},
		{"vault.doctor", []string{"vault", "doctor", "--vault", vault}},
		{"note.search", []string{"search", "contract", "--vault", vault}},
		{"note.orphans", []string{"note", "orphans", "--vault", vault}},
		{"repair.plan", []string{"repair", "plan", "--vault", vault}},
		{"organize.plan", []string{"organize", "plan", "--vault", vault}},
		{"version.history", []string{"version", "history", "--vault", vault}},
	}
}

// proofBodySentinel is a distinctive marker placed in a note body. Bounded READ
// projections must never leak it at ANY nesting depth — not just top-level.
const proofBodySentinel = "PROOF_BODY_SENTINEL_MUST_NOT_LEAK"

// assertNoRecursiveBodyLeak walks an arbitrary JSON value and fails if any
// nested key named "body" (or "Body") holds a non-empty string, or if the
// sentinel marker appears anywhere in the serialized envelope. This catches
// leaks hidden inside data.notes[].body, data.results[].note.body, etc.
func assertNoRecursiveBodyLeak(t *testing.T, name string, raw []byte) {
	t.Helper()
	var envelope any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("%s: cannot parse envelope: %v\n%s", name, err, raw)
	}
	assertNoBodyField(t, name, envelope)
	if strings.Contains(string(raw), proofBodySentinel) {
		t.Fatalf("%s: sentinel body marker leaked into envelope:\n%s", name, raw)
	}
}

// assertNoBodyField recursively walks v and reports any map entry whose key is
// body/Body with a non-empty string value. Bounded projections may carry empty
// body fields for schema stability, but never populated body content.
func assertNoBodyField(t *testing.T, name string, v any) {
	t.Helper()
	switch val := v.(type) {
	case map[string]any:
		for key, child := range val {
			lk := strings.ToLower(key)
			if lk == "body" || lk == "note_body" || lk == "raw_body" {
				if s, ok := child.(string); ok && s != "" {
					t.Fatalf("%s: envelope leaked body content at key %q = %q", name, key, truncate(s, 80))
				}
			}
			assertNoBodyField(t, name, child)
		}
	case []any:
		for _, child := range val {
			assertNoBodyField(t, name, child)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// TestProofLoopJSONProjections proves every proof loop command renders a valid
// JSON envelope from the shared projection boundary, and that no nested field
// leaks the note body sentinel.
func TestProofLoopJSONProjections(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "add", "Proof Contract", "--body", proofBodySentinel+" extra padding text", "--vault", root, "--json")
	runCLI(t, "index", "sync", "--vault", root, "--json")
	runCLI(t, "version", "snapshot", "--vault", root, "--message", "contract", "--json")

	for _, pc := range proofLoopCommands(root) {
		t.Run(pc.name+"/json", func(t *testing.T) {
			out := runCLI(t, append(pc.args, "--json")...)
			raw := []byte(out)
			var envelope map[string]any
			if err := json.Unmarshal(raw, &envelope); err != nil {
				t.Fatalf("%s --json is not a single JSON envelope: %v\n%s", pc.name, err, out)
			}
			for _, key := range []string{"spec_version", "mode", "command", "status"} {
				v, ok := envelope[key].(string)
				if !ok || v == "" {
					t.Fatalf("%s --json envelope missing %q:\n%s", pc.name, key, out)
				}
			}
			if envelope["mode"] != "json" {
				t.Fatalf("%s --json mode = %q, want json", pc.name, envelope["mode"])
			}
			status := envelope["status"].(string)
			if status != "success" && status != "partial" && status != "failed" {
				t.Fatalf("%s --json status = %q, want success|partial|failed", pc.name, status)
			}
			// Recursively reject any nested body field or sentinel leak.
			assertNoRecursiveBodyLeak(t, pc.name+"/json", raw)
		})
	}
}

// TestProofLoopAgentProjections proves every proof loop command renders stable
// agent key=value lines from the same shared projection, without leaking body.
func TestProofLoopAgentProjections(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "add", "Proof Contract", "--body", proofBodySentinel+" extra padding text", "--vault", root, "--json")
	runCLI(t, "index", "sync", "--vault", root, "--json")

	required := map[string]bool{"spec_version": false, "mode": false, "command": false, "status": false}
	for _, pc := range proofLoopCommands(root) {
		t.Run(pc.name+"/agent", func(t *testing.T) {
			out := runCLI(t, append(pc.args, "--agent")...)
			seen := map[string]bool{}
			for k := range required {
				seen[k] = false
			}
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				key, _, ok := strings.Cut(line, "=")
				if !ok {
					t.Fatalf("%s --agent line missing '=': %q", pc.name, line)
				}
				if _, want := required[key]; want {
					seen[key] = true
				}
			}
			for key, ok := range seen {
				if !ok {
					t.Fatalf("%s --agent missing required key %q:\n%s", pc.name, key, out)
				}
			}
			if !strings.Contains(out, "mode=agent") {
				t.Fatalf("%s --agent missing mode=agent:\n%s", pc.name, out)
			}
			if strings.Contains(out, proofBodySentinel) {
				t.Fatalf("%s --agent leaked body sentinel", pc.name)
			}
		})
	}
}

// TestProofLoopEventsProjections proves proof loop commands render bounded NDJSON
// event streams with start/end markers from the shared projection boundary, and
// that no event leaks the note body sentinel.
func TestProofLoopEventsProjections(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "add", "Proof Contract", "--body", proofBodySentinel+" extra padding text", "--vault", root, "--json")
	runCLI(t, "index", "sync", "--vault", root, "--json")

	commands := []proofLoopCommand{
		{"vault.doctor", []string{"vault", "doctor", "--vault", root}},
		{"repair.plan", []string{"repair", "plan", "--vault", root}},
	}
	for _, pc := range commands {
		t.Run(pc.name+"/events", func(t *testing.T) {
			out := runCLI(t, append(pc.args, "--events")...)
			hasStart := false
			hasEnd := false
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				var event map[string]any
				if err := json.Unmarshal([]byte(line), &event); err != nil {
					t.Fatalf("%s --events line is not JSON: %v\n%q", pc.name, err, line)
				}
				if event["mode"] != "events" {
					t.Fatalf("%s --events event mode = %v, want events", pc.name, event["mode"])
				}
				if event["type"] == "start" {
					hasStart = true
				}
				if event["type"] == "end" {
					hasEnd = true
				}
				// Recursively reject body leaks within each event object.
				raw := []byte(line)
				assertNoRecursiveBodyLeak(t, pc.name+"/events", raw)
			}
			if !hasStart || !hasEnd {
				t.Fatalf("%s --events missing start/end markers:\n%s", pc.name, out)
			}
		})
	}
}

// TestProofLoopDefaultSummaryNoBodyLeak proves the default human-readable summary
// mode also stays bounded: no sentinel body marker in stdout.
func TestProofLoopDefaultSummaryNoBodyLeak(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "add", "Proof Contract", "--body", proofBodySentinel+" extra padding text", "--vault", root, "--json")
	runCLI(t, "index", "sync", "--vault", root, "--json")

	for _, pc := range proofLoopCommands(root) {
		t.Run(pc.name+"/default", func(t *testing.T) {
			out := runCLI(t, pc.args...)
			if strings.Contains(out, proofBodySentinel) {
				t.Fatalf("%s default summary leaked body sentinel:\n%s", pc.name, out)
			}
		})
	}
}

// TestProofLoopRunContractAcrossModes 证明 proof loop run 在 default/json/agent/events
// 四种模式都产出稳定共享投影，携带 proof_loop_run_id，且不泄漏 note body sentinel。
// 它是 proof loop run 作为单一 agent 主入口的契约守卫。
func TestProofLoopRunContractAcrossModes(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "add", "Proof Contract", "--body", proofBodySentinel+" extra padding text", "--vault", root, "--json")
	runCLI(t, "index", "sync", "--vault", root, "--json")

	// json 模式：单一信封 + proof_loop_run_id + 递归无 body 泄漏。
	jsonOut := runCLI(t, "proof", "loop", "run", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("proof loop run --json not single envelope: %v\n%s", err, jsonOut)
	}
	if envelope["command"] != "proof.loop.run" || envelope["status"] != "success" {
		t.Fatalf("proof loop run --json envelope = %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["proof_loop_run_id"] == nil || facts["proof_loop_run_id"] == "" {
		t.Fatalf("proof loop run --json missing proof_loop_run_id: %#v", facts)
	}
	assertNoRecursiveBodyLeak(t, "proof.loop.run/json", []byte(jsonOut))

	// agent 模式：稳定 key=value，含 proof_loop_run_id 与 mode。
	agentOut := runCLI(t, "proof", "loop", "run", "--vault", root, "--agent")
	if !strings.Contains(agentOut, "mode=agent") || !strings.Contains(agentOut, "command=proof.loop.run") {
		t.Fatalf("proof loop run --agent missing stable keys:\n%s", agentOut)
	}
	if !strings.Contains(agentOut, "proof_loop_run_id=") || strings.Contains(agentOut, proofBodySentinel) {
		t.Fatalf("proof loop run --agent missing run id or leaked body:\n%s", agentOut)
	}

	// events 模式：start/end NDJSON，每个 event 递归无 body 泄漏。
	eventsOut := runCLI(t, "proof", "loop", "run", "--vault", root, "--events")
	hasStart, hasEnd := false, false
	for _, line := range strings.Split(eventsOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("proof loop run --events line not JSON: %v\n%q", err, line)
		}
		if event["type"] == "start" {
			hasStart = true
		}
		if event["type"] == "end" {
			hasEnd = true
		}
		assertNoRecursiveBodyLeak(t, "proof.loop.run/events", []byte(line))
	}
	if !hasStart || !hasEnd {
		t.Fatalf("proof loop run --events missing start/end:\n%s", eventsOut)
	}

	// default 模式：人类摘要不泄漏 sentinel。
	defaultOut := runCLI(t, "proof", "loop", "run", "--vault", root)
	if strings.Contains(defaultOut, proofBodySentinel) {
		t.Fatalf("proof loop run default leaked body sentinel:\n%s", defaultOut)
	}
}

// TestVersionRestoreApplyContractAcrossModes 证明 version restore apply 在 json/agent/default
// 模式产出稳定投影且不泄漏 token/Authorization/body。每个模式生成 fresh plan（apply 会改
// vault hash，复用同一 plan 会触发 stale 校验）。
func TestVersionRestoreApplyContractAcrossModes(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	noteRel := "notes/contract.md"
	writeCLIFixture(t, filepath.Join(root, noteRel), "# Contract\n\nbaseline\n")
	if err := gitstore.Snapshot(context.Background(), root, "baseline"); err != nil {
		t.Fatalf("baseline git snapshot: %v", err)
	}

	// 每个 apply 前：损坏文件 → 生成 fresh plan → apply。apply 会恢复内容，下一轮再损坏。
	setupAndApply := func(mode string) string {
		writeCLIFixture(t, filepath.Join(root, noteRel), "# Contract\n\ncorrupted "+mode+"\n")
		planOut := runCLI(t, "version", "restore", noteRel, "--revision", "HEAD", "--plan", "--vault", root, "--json")
		planID := jsonParseFacts(t, planOut)["plan_id"].(string)
		args := []string{"version", "restore", "apply", "--vault", root, "--plan", planID, "--yes"}
		if mode != "" {
			args = append(args, "--"+mode)
		}
		return runCLI(t, args...)
	}

	jsonOut := setupAndApply("json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("restore apply --json not single envelope: %v\n%s", err, jsonOut)
	}
	if envelope["command"] != "version.restore.apply" || envelope["status"] != "success" {
		t.Fatalf("restore apply --json envelope = %#v", envelope)
	}
	for _, secret := range []string{"Authorization", "Bearer", proofBodySentinel} {
		if strings.Contains(jsonOut, secret) {
			t.Fatalf("restore apply --json leaked %q:\n%s", secret, jsonOut)
		}
	}
	agentOut := setupAndApply("agent")
	if !strings.Contains(agentOut, "command=version.restore.apply") || strings.Contains(agentOut, proofBodySentinel) {
		t.Fatalf("restore apply --agent contract broken:\n%s", agentOut)
	}
	defaultOut := setupAndApply("")
	if strings.Contains(defaultOut, proofBodySentinel) {
		t.Fatalf("restore apply default leaked body sentinel:\n%s", defaultOut)
	}
}
