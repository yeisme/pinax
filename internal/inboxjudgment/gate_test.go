package inboxjudgment

import (
	"context"
	"strings"
	"testing"
)

// gate_test.go 覆盖任务 2.1 实验性开关四态：默认/未启用与 enabled+off 均
// 完全休眠（零装配、零调用、采纳门一律拒绝并提示未启用）；enabled+shadow
// 与 enabled+assist 走已落地 consumer/采纳门语义；非法组合 fail-fast；
// 关闭即恢复原 inbox 流程。

func judgmentGateFixtureOptions(transport JudgmentTransport, authorizer InboxJudgmentAuthorizer) InboxJudgmentOptions {
	return InboxJudgmentOptions{
		Model:          "fixture:gate",
		AdapterVersion: "fixture-v1",
		Transport:      transport,
		Capabilities:   FixtureJudgmentCapabilities("gate"),
		Authorizer:     authorizer,
		Limits:         DefaultInboxJudgmentLimits(),
	}
}

// judgmentGateEvaluatedSuggestion 用启用 consumer 生成一条完整作答的
// assist 建议（供休眠开关证明"即使建议本身合法也一律拒绝"）。
func judgmentGateEvaluatedSuggestion(t *testing.T, gate ExperimentalJudgmentGate, transport *FixtureTransport, input InboxJudgmentInput) (InboxJudgmentSuggestion, InboxJudgmentEvidence) {
	t.Helper()
	consumer, err := gate.Assemble(judgmentGateFixtureOptions(transport, judgmentFixtureAuthorizer(input)))
	if err != nil {
		t.Fatalf("assemble enabled gate: %v", err)
	}
	if consumer == nil {
		t.Fatal("enabled gate must assemble a consumer")
	}
	outcome, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if outcome.Suggestion == nil {
		t.Fatal("enabled assist evaluation must produce a suggestion")
	}
	return *outcome.Suggestion, outcome.Evidence
}

func judgmentGateCurrentRevisions(input InboxJudgmentInput) map[string]string {
	return map[string]string{
		input.InboxNoteID: input.InboxRevision,
		"note-a":          input.Candidates[0].SourceRevision,
		"note-b":          input.Candidates[1].SourceRevision,
	}
}

func TestJudgmentGateDormantStatesAssembleNothing(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	transport := NewFixtureTransport("dormant")
	suggestion, evidence := judgmentGateEvaluatedSuggestion(t, ExperimentalJudgmentGate{Enabled: true, Mode: InboxJudgmentModeAssist}, NewFixtureTransport("seed"), input)
	authorizer := judgmentFixtureAuthorizer(input)
	candidates := judgmentCurrentCandidates(input)

	// 休眠三态：零值（默认未启用）、未启用+显式 off、已启用+off。
	for name, gate := range map[string]ExperimentalJudgmentGate{
		"zero value":      {},
		"disabled off":    {Enabled: false, Mode: InboxJudgmentModeOff},
		"enabled off":     {Enabled: true, Mode: InboxJudgmentModeOff},
		"empty mode":      {Enabled: false, Mode: ""},
		"enabled no mode": {Enabled: true, Mode: ""},
	} {
		if err := gate.Validate(); err != nil {
			t.Fatalf("%s: dormant config must validate: %v", name, err)
		}
		if !gate.Dormant() {
			t.Fatalf("%s: config must be dormant", name)
		}
		consumer, err := gate.Assemble(judgmentGateFixtureOptions(transport, authorizer))
		if err != nil {
			t.Fatalf("%s: dormant assemble: %v", name, err)
		}
		if consumer != nil {
			t.Fatalf("%s: dormant config must assemble nothing, got consumer", name)
		}
		// 即使建议/证据本身合法且新鲜，未启用的采纳门也一律拒绝并提示未启用。
		_, err = gate.AcceptSuggestion(context.Background(), suggestion, evidence, authorizer, judgmentGateCurrentRevisions(input), candidates)
		if err == nil || !isJudgmentErrorCode(err, "judgment_not_enabled") {
			t.Fatalf("%s: dormant acceptance must fail with judgment_not_enabled, got %v", name, err)
		}
		if !strings.Contains(err.Error(), "not enabled") || !strings.Contains(err.Error(), "experimental") {
			t.Fatalf("%s: rejection must mention the experimental switch: %v", name, err)
		}
	}
	if transport.TotalCalls() != 0 {
		t.Fatalf("dormant states must make zero external calls, got %d", transport.TotalCalls())
	}
}

func TestJudgmentGateShadowFollowsExistingSemantics(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	authorizer := judgmentFixtureAuthorizer(input)
	transport := NewFixtureTransport("gate-shadow")
	gate := ExperimentalJudgmentGate{Enabled: true, Mode: InboxJudgmentModeShadow}
	// 开关优先：即使注入 options 声明 assist，也按开关的 shadow 装配。
	options := judgmentGateFixtureOptions(transport, authorizer)
	options.Mode = InboxJudgmentModeAssist
	consumer, err := gate.Assemble(options)
	if err != nil {
		t.Fatalf("assemble shadow gate: %v", err)
	}
	outcome, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("shadow evaluate: %v", err)
	}
	if outcome.Status != InboxJudgmentStatusShadowCompared {
		t.Fatalf("status = %s, want shadow_compared", outcome.Status)
	}
	if outcome.Suggestion == nil || outcome.Suggestion.Adoptable {
		t.Fatal("shadow suggestions must never be adoptable")
	}
	if transport.EvaluateCalls() != 1 {
		t.Fatalf("shadow evaluate must map to one transport call, got %d", transport.EvaluateCalls())
	}
	// 启用开关 + shadow：采纳沿用既有语义（shadow comparison-only）。
	_, err = gate.AcceptSuggestion(context.Background(), *outcome.Suggestion, outcome.Evidence, authorizer, judgmentGateCurrentRevisions(input), judgmentCurrentCandidates(input))
	if err == nil || !isJudgmentErrorCode(err, "judgment_not_adoptable") {
		t.Fatalf("shadow acceptance must stay comparison-only, got %v", err)
	}
	if !strings.Contains(err.Error(), "comparison-only") {
		t.Fatalf("shadow rejection must explain comparison-only: %v", err)
	}
}

func TestJudgmentGateAssistFollowsExistingSemantics(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	authorizer := judgmentFixtureAuthorizer(input)
	transport := NewFixtureTransport("gate-assist")
	gate := ExperimentalJudgmentGate{Enabled: true, Mode: InboxJudgmentModeAssist}
	consumer, err := gate.Assemble(judgmentGateFixtureOptions(transport, authorizer))
	if err != nil {
		t.Fatalf("assemble assist gate: %v", err)
	}
	outcome, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("assist evaluate: %v", err)
	}
	if outcome.Status != InboxJudgmentStatusSuggested || outcome.Suggestion == nil || !outcome.Suggestion.Adoptable {
		t.Fatalf("assist outcome = %+v", outcome)
	}
	acceptance, err := gate.AcceptSuggestion(context.Background(), *outcome.Suggestion, outcome.Evidence, authorizer, judgmentGateCurrentRevisions(input), judgmentCurrentCandidates(input))
	if err != nil {
		t.Fatalf("assist acceptance: %v", err)
	}
	for _, action := range acceptance.NextActions {
		if !strings.HasPrefix(action, "pinax inbox ") {
			t.Fatalf("acceptance must only enumerate original inbox entry commands, got %q", action)
		}
	}
	// stale 源在启用态依旧阻断（既有语义不被开关放松）。
	stale := judgmentGateCurrentRevisions(input)
	stale[input.InboxNoteID] = "sha256:inbox-rev-2"
	if _, err := gate.AcceptSuggestion(context.Background(), *outcome.Suggestion, outcome.Evidence, authorizer, stale, judgmentCurrentCandidates(input)); err == nil || !isJudgmentErrorCode(err, "judgment_stale_source") {
		t.Fatalf("stale source must still block adoption, got %v", err)
	}
}

func TestJudgmentGateRejectsInvalidCombinations(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	authorizer := judgmentFixtureAuthorizer(input)
	candidates := judgmentCurrentCandidates(input)
	invalid := map[string]ExperimentalJudgmentGate{
		"shadow without enabled": {Enabled: false, Mode: InboxJudgmentModeShadow},
		"assist without enabled": {Enabled: false, Mode: InboxJudgmentModeAssist},
		"unknown mode":           {Enabled: true, Mode: "auto"},
		"unknown mode disabled":  {Enabled: false, Mode: "live"},
	}
	for name, gate := range invalid {
		if err := gate.Validate(); err == nil {
			t.Fatalf("%s: must be rejected", name)
		}
		if _, err := gate.Assemble(judgmentGateFixtureOptions(NewFixtureTransport("invalid"), authorizer)); err == nil {
			t.Fatalf("%s: assemble must fail fast", name)
		}
		// 非法配置的采纳门同样 fail-fast，不给休眠兜底。
		if _, err := gate.AcceptSuggestion(context.Background(), InboxJudgmentSuggestion{}, InboxJudgmentEvidence{}, authorizer, nil, candidates); err == nil {
			t.Fatalf("%s: acceptance must fail fast", name)
		}
	}
}

func TestJudgmentGateDisableRestoresOriginalFlow(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	authorizer := judgmentFixtureAuthorizer(input)
	transport := NewFixtureTransport("gate-off-again")
	enabled := ExperimentalJudgmentGate{Enabled: true, Mode: InboxJudgmentModeAssist}
	suggestion, evidence := judgmentGateEvaluatedSuggestion(t, enabled, transport, input)
	callsAfterEnabled := transport.EvaluateCalls()
	if callsAfterEnabled != 1 {
		t.Fatalf("enabled evaluation must make exactly one call, got %d", callsAfterEnabled)
	}

	// 关闭：mode 回 off 即完全休眠，零装配、零新增调用、采纳门拒绝。
	disabled := ExperimentalJudgmentGate{Enabled: true, Mode: InboxJudgmentModeOff}
	consumer, err := disabled.Assemble(judgmentGateFixtureOptions(transport, authorizer))
	if err != nil || consumer != nil {
		t.Fatalf("disabled gate must assemble nothing: consumer=%v err=%v", consumer, err)
	}
	if transport.TotalCalls() != callsAfterEnabled {
		t.Fatalf("disabling must not add external calls: %d -> %d", callsAfterEnabled, transport.TotalCalls())
	}
	_, err = disabled.AcceptSuggestion(context.Background(), suggestion, evidence, authorizer, judgmentGateCurrentRevisions(input), judgmentCurrentCandidates(input))
	if err == nil || !isJudgmentErrorCode(err, "judgment_not_enabled") {
		t.Fatalf("acceptance after disable must fail with judgment_not_enabled, got %v", err)
	}
	if !strings.Contains(err.Error(), "original inbox review flow") {
		t.Fatalf("rejection must point back to the original flow: %v", err)
	}
}
