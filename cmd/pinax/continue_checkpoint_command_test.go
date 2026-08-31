package main

import (
	"strings"
	"testing"
)

// TestContinueCheckpointCommandContract 覆盖 checkpoint 命令契约：
// handoff 创建、proposal-only durable candidate、confirmed_created=0。
func TestContinueCheckpointCommandContract(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")

	out := runCLI(t, "continue", "checkpoint",
		"--vault", vaultRoot,
		"--scope", "project:pinax",
		"--objective", "finish the continuity dogfood contract",
		"--current-state", "checkpoint command wired",
		"--decisions", "reuse canonical handoff service",
		"--blockers", "none",
		"--verification", "go test ./cmd/pinax",
		"--follow-ups", "run cross-runtime e2e",
		"--sources", "repository:openspec/changes/x/proposal.md",
		"--durable", "decision:checkpoint caps enforced=caps fail before write",
		"--json")
	facts := jsonParseFacts(t, out)
	if facts["handoff_id"] == "" || facts["handoff_id"] == nil {
		t.Fatalf("checkpoint must return handoff_id: %#v", facts)
	}
	if facts["confirmed_created"] != "0" {
		t.Fatalf("checkpoint must never create confirmed memory: %#v", facts)
	}
	if facts["proposal_1"] == nil {
		t.Fatalf("durable candidate must produce proposal: %#v", facts)
	}
}

// TestContinueCheckpointCapsReject 覆盖 CLI 层超限与 transcript dump 防护。
func TestContinueCheckpointCapsReject(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")
	huge := strings.Repeat("x", 201)
	if _, err := runCLIExpectError("continue", "checkpoint",
		"--vault", vaultRoot, "--scope", "project:pinax",
		"--objective", "cap check", "--blockers", huge, "--json"); err == nil {
		t.Fatal("oversize section must fail validation")
	}
	if _, err := runCLIExpectError("continue", "checkpoint",
		"--vault", vaultRoot, "--scope", "project:pinax", "--json"); err == nil {
		t.Fatal("missing objective must fail validation")
	}
}

// TestContinueCheckpointRejectsMalformedRepeatedFlags 固定 CLI 不得静默丢弃
// 用户提交的 malformed source/durable item；失败必须发生在 handoff 写入前。
func TestContinueCheckpointRejectsMalformedRepeatedFlags(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")
	for _, args := range [][]string{
		{"--sources", "missing-separator"},
		{"--durable", "missing-separators"},
	} {
		base := []string{"continue", "checkpoint", "--vault", vaultRoot, "--scope", "project:pinax", "--objective", "reject malformed flags"}
		base = append(base, args...)
		base = append(base, "--json")
		if _, err := runCLIExpectError(base...); err == nil {
			t.Fatalf("malformed checkpoint flag must fail: %v", args)
		}
	}
	listOut := runCLI(t, "agent", "handoff", "list", "--vault", vaultRoot, "--scope", "project:pinax", "--json")
	if facts := jsonParseFacts(t, listOut); facts["count"] != "0" {
		t.Fatalf("malformed flags must not write handoffs: %#v", facts)
	}
}

// TestContinueCheckpointRepeatFlags 覆盖 repeated flag 与 comma 兼容映射。
func TestContinueCheckpointRepeatFlags(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")
	out := runCLI(t, "continue", "checkpoint",
		"--vault", vaultRoot, "--scope", "project:pinax",
		"--objective", "repeat flags",
		"--decisions", "a", "--decisions", "b,c",
		"--json")
	facts := jsonParseFacts(t, out)
	if facts["handoff_id"] == nil {
		t.Fatalf("repeat flags checkpoint failed: %#v", facts)
	}
}

// TestHandoffCompatibilityAfterCheckpoint 证明 checkpoint 是 facade：
// 既有 `pinax agent handoff create/list/show` 命令行为不变，且能消费
// checkpoint 创建的 handoff。
func TestHandoffCompatibilityAfterCheckpoint(t *testing.T) {
	_, vaultRoot := setupContinueFixture(t, "pinax")

	createOut := runCLI(t, "agent", "handoff", "create",
		"--vault", vaultRoot, "--scope", "project:pinax",
		"--objective", "legacy handoff path", "--json")
	legacyFacts := jsonParseFacts(t, createOut)
	legacyID, _ := legacyFacts["handoff_id"].(string)
	if legacyID == "" {
		t.Fatalf("legacy handoff create failed: %#v", legacyFacts)
	}

	checkpointOut := runCLI(t, "continue", "checkpoint",
		"--vault", vaultRoot, "--scope", "project:pinax",
		"--objective", "checkpoint handoff", "--json")
	cpFacts := jsonParseFacts(t, checkpointOut)
	cpID, _ := cpFacts["handoff_id"].(string)

	showOut := runCLI(t, "agent", "handoff", "show", cpID, "--vault", vaultRoot, "--json")
	showFacts := jsonParseFacts(t, showOut)
	if showFacts["handoff_id"] != cpID {
		t.Fatalf("handoff show must consume checkpoint handoff: %#v", showFacts)
	}
	listOut := runCLI(t, "agent", "handoff", "list", "--vault", vaultRoot, "--scope", "project:pinax", "--json")
	listFacts := jsonParseFacts(t, listOut)
	if listFacts["count"] != "2" {
		t.Fatalf("handoff list must include both legacy and checkpoint handoffs: %#v", listFacts)
	}
}
