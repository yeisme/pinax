package output

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/domain"
)

// continueCardProjection 构造一个 ready 状态的 continue projection（Data 是
// map 视图，与 CLI 的 agentcontinuity.ContinuityPack JSON 形状一致）。
func continueCardProjection() domain.Projection {
	p := domain.NewProjection("continue", "Continuity pack compiled.")
	p.Data = map[string]any{
		"objective":      "ship the continuity dogfood v1",
		"current_state":  "binding resolved, checkpoint pending",
		"handoff_status": "consumed",
		"sections": []map[string]any{
			{"kind": "decision", "title": "Key decisions", "items": []string{"reuse canonical handoff service", "keep packs bounded"}},
		},
		"source_coverage":  map[string]any{"total": 2, "resolved": 2},
		"evidence_status":  "resolved",
		"freshness_status": "fresh",
		"pack_status":      "ready",
		"recommended_next_action": map[string]any{
			"name": "View handoff details", "command": "pinax agent handoff show h_1",
		},
	}
	return p
}

func TestResumeCardClampPreservesUTF8(t *testing.T) {
	t.Parallel()
	got := resumeCardClamp(strings.Repeat("连续性验收", 40))
	if !utf8.ValidString(got) {
		t.Fatalf("clamped resume card line is invalid UTF-8: %q", got)
	}
	if len([]rune(got)) != 120 {
		t.Fatalf("clamped rune length = %d, want 120", len([]rune(got)))
	}
}

// TestContinueRenderResumeCardSixBlocks 覆盖六个固定区块的 human golden：
// Objective/Last state/Key decisions/Blockers/Recommended next action/Evidence status。
func TestContinueRenderResumeCardSixBlocks(t *testing.T) {
	t.Parallel()
	p := continueCardProjection()

	var summary bytes.Buffer
	if err := RenderWithOptions(&summary, ModeSummary, p, RenderOptions{ColorMode: "never", IsTerminal: false}); err != nil {
		t.Fatalf("render resume card: %v", err)
	}
	got := summary.String()
	for _, want := range []string{
		"Objective", "ship the continuity dogfood v1",
		"Last state", "binding resolved, checkpoint pending",
		"Key decisions", "reuse canonical handoff service",
		"Blockers / conflicts", "-",
		"Recommended next action", "View handoff details",
		"Evidence status", "sources: 2/2 resolved", "evidence=resolved freshness=fresh",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("resume card missing %q:\n%s", want, got)
		}
	}
	// bounded：不能出现完整 section body 字段名或 warning 泄漏到正文。
	if strings.Contains(got, "current_state") {
		t.Fatalf("resume card leaked machine field name:\n%s", got)
	}
}

// TestContinueRenderResumeCardMissingHandoff 覆盖无 handoff：
// context-only、Handoff: missing、checkpoint next action、不编造 last state。
func TestContinueRenderResumeCardMissingHandoff(t *testing.T) {
	t.Parallel()
	p := continueCardProjection()
	p.Data = map[string]any{
		"objective":        "no handoff yet",
		"handoff_status":   "missing",
		"source_coverage":  map[string]any{"total": 0},
		"evidence_status":  "not_measured",
		"freshness_status": "not_measured",
		"pack_status":      "partial",
		"warning_codes":    []string{"handoff_missing"},
		"recommended_next_action": map[string]any{
			"name": "Create a continuity checkpoint", "command": "pinax continue checkpoint --scope project:pinax --objective \"<bounded objective>\"",
		},
	}

	var summary bytes.Buffer
	if err := RenderWithOptions(&summary, ModeSummary, p, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := summary.String()
	for _, want := range []string{"partial", "Handoff: missing (context-only)", "Create a continuity checkpoint", "warnings: handoff_missing"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing-handoff card missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "binding resolved, checkpoint pending") {
		t.Fatalf("missing-handoff card must not fabricate last state:\n%s", got)
	}
}

// TestContinueRenderResumeCardPartialConflict 覆盖 conflict/stale：
// conflict warning 与 source 漂移可见，不被 budget 丢弃。
func TestContinueRenderResumeCardPartialConflict(t *testing.T) {
	t.Parallel()
	p := continueCardProjection()
	p.Data = map[string]any{
		"objective":      "conflicted work",
		"handoff_status": "consumed",
		"sections": []map[string]any{
			{"kind": "blocker", "title": "Blockers", "items": []string{"waiting for review"}},
		},
		"conflicts":        []map[string]any{{"memory_ids": []string{"m1", "m2"}, "reason": "incompatible decisions"}},
		"source_coverage":  map[string]any{"total": 3, "resolved": 1, "stale": 1, "missing": 1},
		"evidence_status":  "partial",
		"freshness_status": "stale",
		"pack_status":      "partial",
		"warning_codes":    []string{"decision_conflict", "source_missing", "source_stale"},
		"recommended_next_action": map[string]any{
			"name": "Resolve memory conflicts", "command": "pinax review --scope project:pinax",
		},
	}

	var summary bytes.Buffer
	if err := RenderWithOptions(&summary, ModeSummary, p, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render: %v", err)
	}
	got := summary.String()
	for _, want := range []string{
		"partial", "conflict: incompatible decisions",
		"stale=1 missing=1", "warnings: decision_conflict, source_missing, source_stale",
		"Resolve memory conflicts",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("conflict card missing %q:\n%s", want, got)
		}
	}
}

// TestContinueRenderJSONMachineContractUnchanged 覆盖 Resume Card 是
// human-only refinement：--json envelope 的 English keys 与 top-level 结构不变。
func TestContinueRenderJSONMachineContractUnchanged(t *testing.T) {
	t.Parallel()
	p := continueCardProjection()

	var jsonOut bytes.Buffer
	if err := RenderWithOptions(&jsonOut, ModeJSON, p, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render json: %v", err)
	}
	got := jsonOut.String()
	// JSON envelope 必须保留 machine fields，不被 human card 干扰。
	for _, want := range []string{`"command":"continue"`, `"handoff_status":"consumed"`, `"recommended_next_action"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("json machine contract missing %q:\n%s", want, got)
		}
	}

	var agentOut bytes.Buffer
	if err := RenderWithOptions(&agentOut, ModeAgent, p, RenderOptions{ColorMode: "never"}); err != nil {
		t.Fatalf("render agent: %v", err)
	}
	if !strings.Contains(agentOut.String(), "command=continue") && !strings.Contains(agentOut.String(), "continue") {
		t.Fatalf("agent output must keep continue command: %s", agentOut.String())
	}
}
