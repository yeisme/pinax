package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestContinueWorkbenchCommandContract 覆盖 typed facade 命令契约：
// json envelope（合同标识/错误码透传/恢复 action）、agent facts、
// packet digest 与 envelope 一致、错误态不合成 resume card。
func TestContinueWorkbenchCommandContract(t *testing.T) {
	repoRoot, _ := setupContinueFixture(t, "pinax")

	bindOut := runCLI(t, "continue", "bind", "--repo", repoRoot, "--vault", "test-vault", "--scope", "project:pinax", "--json")
	var bindEnvelope map[string]any
	if err := json.Unmarshal([]byte(bindOut), &bindEnvelope); err != nil {
		t.Fatalf("bind envelope invalid: %v\n%s", err, bindOut)
	}
	bindData, _ := bindEnvelope["data"].(map[string]any)
	projectRef, _ := bindData["binding_id"].(string)
	if projectRef == "" {
		t.Fatalf("binding_id missing from data: %#v", bindData)
	}

	// ready 投影：machine envelope 合同标识 + bounded resume card。
	out := runCLI(t, "continue", "workbench", projectRef, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("workbench envelope invalid: %v\n%s", err, out)
	}
	if envelope["command"] != "continue.workbench" || envelope["status"] != "success" {
		t.Fatalf("envelope header: %#v", envelope)
	}
	envelopeFacts, _ := envelope["facts"].(map[string]any)
	if envelopeFacts["binding_status"] != "ready" || envelopeFacts["ready"] != "true" {
		t.Fatalf("ready facts: %#v", envelopeFacts)
	}
	digest, _ := envelopeFacts["digest"].(string)
	if !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("digest fact = %q", digest)
	}
	if strings.Contains(out, repoRoot) {
		t.Fatalf("envelope leaked absolute repo path:\n%s", out)
	}

	// agent 输出 facts。
	agent := runCLI(t, "continue", "workbench", projectRef, "--agent")
	for _, want := range []string{"command=continue.workbench", "fact.binding_status=ready", "fact.digest="} {
		if !strings.Contains(agent, want) {
			t.Fatalf("agent output missing %q:\n%s", want, agent)
		}
	}
	assertMachineOutputClean(t, agent)

	// 未知 projectRef：错误态 envelope + 唯一恢复 action，不猜 scope。
	missingOut := runCLI(t, "continue", "workbench", "bind_unknown", "--json")
	var missingEnvelope map[string]any
	if err := json.Unmarshal([]byte(missingOut), &missingEnvelope); err != nil {
		t.Fatalf("missing envelope invalid: %v\n%s", err, missingOut)
	}
	missingFacts, _ := missingEnvelope["facts"].(map[string]any)
	if missingFacts["binding_status"] != "not_found" || missingFacts["recovery_code"] != "binding_not_found" {
		t.Fatalf("missing facts: %#v", missingFacts)
	}
	if strings.Contains(missingOut, "resume_card\":{") {
		t.Fatalf("error state must not synthesize a resume card:\n%s", missingOut)
	}

	// --packet：digest 与 ready envelope 一致。
	packetOut := runCLI(t, "continue", "workbench", "--packet", "--json")
	if !strings.Contains(packetOut, digest) {
		t.Fatalf("packet digest must equal envelope digest %q:\n%s", digest, packetOut)
	}
	if !strings.Contains(packetOut, "pinax.provider_packet.v1") || !strings.Contains(packetOut, "checkpoint.propose") {
		t.Fatalf("packet content incomplete:\n%s", packetOut)
	}
}

// TestContinueWorkbenchRequiresProjectRef：缺 projectRef（非 --packet）fail-closed。
func TestContinueWorkbenchRequiresProjectRef(t *testing.T) {
	setupContinueFixture(t, "pinax")
	out, err := runCLIExpectError("continue", "workbench", "--json")
	if err == nil || !strings.Contains(out, "validation_failed") {
		t.Fatalf("missing projectRef must fail closed: err=%v out=%s", err, out)
	}
}
