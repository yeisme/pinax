package inboxjudgment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

// consumer_test.go 覆盖任务 1.2/1.3：off/shadow/assist 模式、注入
// transport、unknown outcome 不自动重发、零网络 replay、缓存权限撤销、
// stale/权限撤销不可采纳、review handoff 复用旧 CLI envelope。

func judgmentFixtureConsumer(t *testing.T, mode string, transport JudgmentTransport, authorizer InboxJudgmentAuthorizer, cache *InboxJudgmentCache, reviewRefs []string) *InboxJudgmentConsumer {
	t.Helper()
	consumer, err := NewInboxJudgmentConsumer(InboxJudgmentOptions{
		Mode:           mode,
		Model:          "fixture:local",
		AdapterVersion: "fixture-v1",
		Transport:      transport,
		Capabilities:   FixtureJudgmentCapabilities("consumer"),
		Cache:          cache,
		Authorizer:     authorizer,
		Limits:         DefaultInboxJudgmentLimits(),
		ReviewRefs:     reviewRefs,
	})
	if err != nil {
		t.Fatalf("consumer for mode %s: %v", mode, err)
	}
	return consumer
}

func TestConsumerOffModeZeroCalls(t *testing.T) {
	transport := NewFixtureTransport("off")
	input := judgmentProjectionFixtureInput()
	// off 模式不要求 transport/authorizer；空 Mode 同样按 off 处理。
	consumer, err := NewInboxJudgmentConsumer(InboxJudgmentOptions{})
	if err != nil {
		t.Fatalf("off consumer: %v", err)
	}
	outcome, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("off evaluate: %v", err)
	}
	if outcome.Status != InboxJudgmentStatusOff {
		t.Errorf("status = %s, want off", outcome.Status)
	}
	if outcome.Suggestion != nil {
		t.Error("off outcome must not carry a suggestion")
	}
	if outcome.Evidence.Mode != InboxJudgmentModeOff || outcome.Evidence.Suggestion != nil {
		t.Errorf("off evidence must stay suggestion-free: %+v", outcome.Evidence)
	}
	if err := outcome.Evidence.Validate(); err != nil {
		t.Errorf("off evidence must validate: %v", err)
	}
	if transport.TotalCalls() != 0 {
		t.Errorf("off mode must make zero external calls, got %d", transport.TotalCalls())
	}
	if len(outcome.BaselineOrder) != len(input.Candidates) {
		t.Error("off outcome must still expose the baseline order")
	}
}

func TestConsumerOptionsValidation(t *testing.T) {
	if err := (InboxJudgmentOptions{Mode: "auto"}).Validate(); err == nil {
		t.Error("unknown mode must be rejected")
	}
	if err := (InboxJudgmentOptions{Mode: InboxJudgmentModeAssist}).Validate(); err == nil {
		t.Error("enabled mode without transport must be rejected")
	}
	transport := NewFixtureTransport("validate")
	if err := (InboxJudgmentOptions{Mode: InboxJudgmentModeAssist, Transport: transport}).Validate(); err == nil {
		t.Error("enabled mode without authorizer must be rejected")
	}
	if err := (InboxJudgmentOptions{Mode: InboxJudgmentModeAssist, Transport: transport, Authorizer: StaticJudgmentAuthorizer{}}).Validate(); err == nil {
		t.Error("enabled mode without exact model pin must be rejected")
	}
	if err := (InboxJudgmentOptions{Mode: InboxJudgmentModeAssist, Transport: transport, Authorizer: StaticJudgmentAuthorizer{}, Model: "fixture:local"}).Validate(); err == nil {
		t.Error("enabled mode without capability snapshot must be rejected")
	}
}

func TestConsumerAssistSuggestion(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	transport := NewFixtureTransport("assist")
	consumer := judgmentFixtureConsumer(t, InboxJudgmentModeAssist, transport, judgmentFixtureAuthorizer(input), nil, []string{"pinax inbox show note-inbox-1"})
	outcome, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("assist evaluate: %v", err)
	}
	if outcome.Status != InboxJudgmentStatusSuggested {
		t.Fatalf("status = %s, want suggested", outcome.Status)
	}
	suggestion := outcome.Suggestion
	if suggestion == nil {
		t.Fatal("assist outcome must carry a suggestion")
	}
	if err := suggestion.Validate(); err != nil {
		t.Fatalf("suggestion must validate: %v", err)
	}
	if !suggestion.Adoptable || len(suggestion.Missing) != 0 {
		t.Errorf("fully answered suggestion must be adoptable (missing=%d)", len(suggestion.Missing))
	}
	// note-a 是 complement 且 clearly_useful → 链接建议。
	if len(suggestion.Links) != 1 || suggestion.Links[0].NoteID != "note-a" || suggestion.Links[0].Relation != RelationComplement || suggestion.Links[0].Usefulness != "clearly_useful" {
		t.Errorf("link suggestions mismatch: %+v", suggestion.Links)
	}
	if len(suggestion.Duplicates) != 0 {
		t.Errorf("no duplicate expected: %+v", suggestion.Duplicates)
	}
	// 语言分布（双语校准产物）。
	if len(suggestion.Languages) != 2 {
		t.Errorf("language distribution must record zh and en, got %+v", suggestion.Languages)
	}
	// wire 请求只携带已授权候选与 inbox 上下文 source。
	lastRequest := transport.LastRequest()
	if len(lastRequest.Candidates) != 2 || len(lastRequest.Questions) != 4 {
		t.Errorf("wire shape mismatch: %d candidates, %d questions", len(lastRequest.Candidates), len(lastRequest.Questions))
	}
	hasInboxSource := false
	for _, source := range lastRequest.Sources {
		if strings.HasPrefix(source.SourceID, "inbox:") {
			hasInboxSource = true
		}
	}
	if !hasInboxSource {
		t.Error("wire request must carry the inbox context source")
	}
	if transport.EvaluateCalls() != 1 {
		t.Errorf("one evaluate must map to one transport evaluate, got %d", transport.EvaluateCalls())
	}
	// evidence 校验与脱敏：不含问题文本/inline 摘要。
	if err := outcome.Evidence.Validate(); err != nil {
		t.Fatalf("evidence must validate: %v", err)
	}
	encoded := mustMarshal(t, outcome.Evidence)
	if strings.Contains(encoded, "How useful would linking") {
		t.Error("evidence must not carry question prompt text")
	}
	if strings.Contains(encoded, "Kyoto temple itinerary draft") {
		t.Error("evidence must not carry inline excerpts")
	}
	if len(outcome.Evidence.ReviewRefs) != 1 || outcome.Evidence.ReviewRefs[0] != "pinax inbox show note-inbox-1" {
		t.Errorf("evidence must carry the original review refs: %+v", outcome.Evidence.ReviewRefs)
	}
	if outcome.Evidence.UsageKnown {
		t.Error("fixture usage is unknown and must stay unknown")
	}
}

func TestConsumerShadowNotAdoptable(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	consumer := judgmentFixtureConsumer(t, InboxJudgmentModeShadow, NewFixtureTransport("shadow"), judgmentFixtureAuthorizer(input), nil, nil)
	outcome, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("shadow evaluate: %v", err)
	}
	if outcome.Status != InboxJudgmentStatusShadowCompared {
		t.Fatalf("status = %s, want shadow_compared", outcome.Status)
	}
	if outcome.Suggestion == nil || outcome.Suggestion.Adoptable {
		t.Error("shadow suggestions must never be adoptable")
	}
	if !strings.Contains(outcome.Suggestion.Reason, "shadow comparison only") {
		t.Errorf("shadow reason must be explicit: %s", outcome.Suggestion.Reason)
	}
}

func TestConsumerMissingRequiredBlocksAdoption(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	// note-b 不带 verdict tag → 全部问题弃答 → 必需缺答阻止采纳。
	input.Candidates[1].InlineText = "Tokyo ramen list without verdict tags."
	consumer := judgmentFixtureConsumer(t, InboxJudgmentModeAssist, NewFixtureTransport("missing"), judgmentFixtureAuthorizer(input), nil, nil)
	outcome, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	suggestion := outcome.Suggestion
	if suggestion == nil {
		t.Fatal("suggestion expected")
	}
	if suggestion.Adoptable {
		t.Error("suggestion with missing required answers must not be adoptable")
	}
	if len(suggestion.Missing) < 4 {
		t.Errorf("missing pairs must be recorded per required question, got %d", len(suggestion.Missing))
	}
	// 缺答候选仍保留在 baseline，但绝不进入链接建议。
	for _, link := range suggestion.Links {
		if link.NoteID == "note-b" {
			t.Error("unanswered candidate must not win a link suggestion")
		}
	}
}

func TestConsumerDeterministicDuplicateSurfacesWithoutModelAnswer(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	// 内容 digest 全等：确定性重复线索即使模型全部弃答也必须呈现。
	input.Candidates[0].InlineText = input.InboxText
	consumer := judgmentFixtureConsumer(t, InboxJudgmentModeAssist, &FixtureTransport{Name: "det", Behavior: "abstain_all"}, judgmentFixtureAuthorizer(input), nil, nil)
	outcome, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	duplicates := outcome.Suggestion.Duplicates
	if len(duplicates) != 1 || duplicates[0].NoteID != "note-a" || duplicates[0].Basis != "exact_content_digest" {
		t.Fatalf("deterministic duplicate must surface regardless of model abstention: %+v", duplicates)
	}
	if outcome.Suggestion.Adoptable {
		t.Error("abstained required answers must still block adoption")
	}
}

func TestConsumerFailureStatusesNoAutoResend(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	cases := []struct {
		behavior string
		status   string
	}{
		{"unavailable", InboxJudgmentStatusUnavailable},
		{"timeout_after_submit", InboxJudgmentStatusOutcomeUnknown},
		{"outcome_unknown", InboxJudgmentStatusOutcomeUnknown},
		{"invalid_response", InboxJudgmentStatusInvalidResp},
	}
	for _, tc := range cases {
		transport := &FixtureTransport{Name: tc.behavior, Behavior: tc.behavior}
		consumer := judgmentFixtureConsumer(t, InboxJudgmentModeAssist, transport, judgmentFixtureAuthorizer(input), nil, nil)
		outcome, err := consumer.Evaluate(context.Background(), input)
		if err != nil {
			t.Fatalf("%s: evaluate returned error: %v", tc.behavior, err)
		}
		if outcome.Status != tc.status {
			t.Errorf("%s: status = %s, want %s", tc.behavior, outcome.Status, tc.status)
		}
		if outcome.Suggestion != nil {
			t.Errorf("%s: failed attempt must not carry a suggestion", tc.behavior)
		}
		if outcome.Evidence.SubmissionState == "" || outcome.Evidence.RetryClass == "" {
			t.Errorf("%s: evidence must record submission state and retry class", tc.behavior)
		}
		if tc.status == InboxJudgmentStatusOutcomeUnknown && outcome.Evidence.ReasonCode != JudgmentCodeOutcomeUnknown && outcome.Evidence.ReasonCode != JudgmentCodeDeadlineExceeded {
			t.Errorf("%s: reason code must preserve the unknown-outcome class, got %s", tc.behavior, outcome.Evidence.ReasonCode)
		}
		// 不自动重发：单次 Evaluate 只消耗一次 transport 调用。
		if transport.EvaluateCalls() != 1 {
			t.Errorf("%s: outcome unknown must not auto-resend, evaluate calls = %d", tc.behavior, transport.EvaluateCalls())
		}
		if err := outcome.Evidence.Validate(); err != nil {
			t.Errorf("%s: failure evidence must validate: %v", tc.behavior, err)
		}
	}
}

func TestConsumerCacheReplayAndRevocation(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	transport := NewFixtureTransport("cache")
	cache := NewInboxJudgmentCache()
	authorizer := judgmentFixtureAuthorizer(input)
	consumer := judgmentFixtureConsumer(t, InboxJudgmentModeAssist, transport, authorizer, cache, nil)
	first, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("first evaluate: %v", err)
	}
	if first.FromCache {
		t.Error("first evaluate must not come from cache")
	}
	if cache.Len() != 1 {
		t.Fatalf("evidence must be cached, len=%d", cache.Len())
	}
	second, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("second evaluate: %v", err)
	}
	if !second.FromCache {
		t.Error("second evaluate must replay from cache")
	}
	if transport.EvaluateCalls() != 1 {
		t.Errorf("cache replay must not touch the transport, evaluate calls = %d", transport.EvaluateCalls())
	}
	if second.Evidence.Digest != first.Evidence.Digest {
		t.Error("cached evidence digest must match the original")
	}
	// 权限撤销：评估路径 fail closed（零发送），缓存读取路径重授权后
	// miss 且条目被删除。
	revoked := StaticJudgmentAuthorizer{Authorization: InboxJudgmentAuthorization{Denied: true, Reason: "revoked"}}
	revokedConsumer := judgmentFixtureConsumer(t, InboxJudgmentModeAssist, transport, revoked, cache, nil)
	if _, err := revokedConsumer.Evaluate(context.Background(), input); err == nil {
		t.Fatal("revoked authorization must fail closed before any send")
	}
	if transport.EvaluateCalls() != 1 {
		t.Errorf("revoked evaluation must not reach the transport, evaluate calls = %d", transport.EvaluateCalls())
	}
	// 投影确定性可重建：用同一绑定重算缓存 key，对已存条目执行重授权读取。
	storedProjection, err := BuildInboxJudgmentProjection(context.Background(), judgmentEnabledBinding(), judgmentFixtureAuthorizer(input), input, DefaultInboxJudgmentLimits())
	if err != nil {
		t.Fatalf("rebuild projection: %v", err)
	}
	storedKey := InboxJudgmentCacheKeyFor(storedProjection.PrincipalDigest, storedProjection, "fixture-v1")
	if _, ok := cache.Get(context.Background(), revoked, storedKey); ok {
		t.Fatal("revoked permission must turn the cached entry into a miss")
	}
	if cache.Len() != 0 {
		t.Errorf("revoked permission must delete the cached entry, len=%d", cache.Len())
	}
	// 缩小授权范围：候选离开授权集合同样失效。
	narrowed := StaticJudgmentAuthorizer{Authorization: InboxJudgmentAuthorization{
		VaultDigest:    VaultDigestFor(input.VaultRoot),
		AllowedNoteIDs: []string{input.InboxNoteID, "note-a"},
	}}
	narrowedConsumer := judgmentFixtureConsumer(t, InboxJudgmentModeAssist, transport, narrowed, cache, nil)
	// note-b 已不在授权集合 → 投影在候选检查处 fail closed（不点名）。
	if _, err := narrowedConsumer.Evaluate(context.Background(), input); err == nil {
		t.Fatal("candidate outside the narrowed authorization must fail closed")
	}
}

func TestShadowEvidenceNotReplayedAcrossModes(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	transport := NewFixtureTransport("cross-mode")
	cache := NewInboxJudgmentCache()
	shadow := judgmentFixtureConsumer(t, InboxJudgmentModeShadow, transport, judgmentFixtureAuthorizer(input), cache, nil)
	shadowOutcome, err := shadow.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("shadow evaluate: %v", err)
	}
	if shadowOutcome.FromCache || transport.EvaluateCalls() != 1 {
		t.Fatalf("shadow evaluate must be one transport attempt, from_cache=%v calls=%d", shadowOutcome.FromCache, transport.EvaluateCalls())
	}
	// 同输入的 assist consumer：缓存 key 绑定 mode，shadow evidence 不得
	// replay 成 assist 结果；assist 必须跑自己的 attempt。
	assist := judgmentFixtureConsumer(t, InboxJudgmentModeAssist, transport, judgmentFixtureAuthorizer(input), cache, nil)
	assistOutcome, err := assist.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("assist evaluate: %v", err)
	}
	if assistOutcome.FromCache {
		t.Fatal("shadow evidence must not replay to an assist consumer")
	}
	if transport.EvaluateCalls() != 2 {
		t.Fatalf("assist must run its own transport attempt, calls=%d", transport.EvaluateCalls())
	}
	if assistOutcome.Evidence.Mode != InboxJudgmentModeAssist {
		t.Errorf("assist evidence mode = %s", assistOutcome.Evidence.Mode)
	}
}

func TestInboxJudgmentCacheConcurrentAccess(t *testing.T) {
	cache := NewInboxJudgmentCache()
	evidence := InboxJudgmentEvidence{SchemaVersion: InboxJudgmentEvidenceSchemaV1, Mode: InboxJudgmentModeAssist}
	// 缓存可被多个 consumer 并发共享：Store/Get/Len 并发访问必须无数据
	// 竞态（go test -race 验证）。
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := InboxJudgmentCacheKey{PrincipalDigest: fmt.Sprintf("principal-%d", i%4), Mode: InboxJudgmentModeAssist}
			cache.Store(key, evidence)
			cache.Get(context.Background(), nil, key)
			cache.Len()
		}(i)
	}
	wg.Wait()
}

func TestReplayInboxJudgmentEvidenceZeroNetwork(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	transport := NewFixtureTransport("replay")
	consumer := judgmentFixtureConsumer(t, InboxJudgmentModeAssist, transport, judgmentFixtureAuthorizer(input), nil, nil)
	outcome, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	calls := transport.EvaluateCalls()
	replayed, err := ReplayInboxJudgmentEvidence(outcome.Evidence)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !replayed.Replay || replayed.FromCache {
		t.Error("replay outcome must be marked replay-only")
	}
	if replayed.Evidence.Digest != outcome.Evidence.Digest {
		t.Error("replay must reproduce the evidence digest byte-for-byte")
	}
	if replayed.Suggestion == nil || replayed.Suggestion.Digest != outcome.Suggestion.Digest {
		t.Error("replay must reproduce the suggestion")
	}
	if transport.EvaluateCalls() != calls {
		t.Errorf("replay must be zero-network, evaluate calls %d -> %d", calls, transport.EvaluateCalls())
	}
	// 篡改 evidence：digest 失配拒绝。
	tampered := outcome.Evidence
	tampered.Summary = "tampered"
	if _, err := ReplayInboxJudgmentEvidence(tampered); err == nil {
		t.Error("tampered evidence must fail validation")
	}
}

func TestAcceptInboxJudgmentSuggestionGates(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	authorizer := judgmentFixtureAuthorizer(input)
	consumer := judgmentFixtureConsumer(t, InboxJudgmentModeAssist, NewFixtureTransport("accept"), authorizer, nil, nil)
	outcome, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	suggestion := *outcome.Suggestion
	evidence := outcome.Evidence
	projectionCandidates := judgmentCurrentCandidates(input)
	currentRevisions := map[string]string{
		input.InboxNoteID: input.InboxRevision,
		"note-a":          input.Candidates[0].SourceRevision,
		"note-b":          input.Candidates[1].SourceRevision,
	}
	acceptance, err := AcceptInboxJudgmentSuggestion(context.Background(), suggestion, evidence, authorizer, currentRevisions, projectionCandidates)
	if err != nil {
		t.Fatalf("happy-path acceptance failed: %v", err)
	}
	if acceptance.SchemaVersion != inboxJudgmentAcceptanceSchema {
		t.Errorf("acceptance schema = %s", acceptance.SchemaVersion)
	}
	for _, action := range acceptance.NextActions {
		if !strings.HasPrefix(action, "pinax inbox ") {
			t.Errorf("acceptance must only enumerate original entry commands, got %q", action)
		}
	}
	// shadow 建议绝不可采纳。
	shadowConsumer := judgmentFixtureConsumer(t, InboxJudgmentModeShadow, NewFixtureTransport("accept-shadow"), authorizer, nil, nil)
	shadowOutcome, err := shadowConsumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("shadow evaluate: %v", err)
	}
	if _, err := AcceptInboxJudgmentSuggestion(context.Background(), *shadowOutcome.Suggestion, shadowOutcome.Evidence, authorizer, currentRevisions, projectionCandidates); err == nil {
		t.Error("shadow suggestions must not be adoptable")
	}
	// stale source：inbox revision 变化后不可采纳。
	staleRevisions := map[string]string{
		input.InboxNoteID: "sha256:inbox-rev-2",
		"note-a":          input.Candidates[0].SourceRevision,
		"note-b":          input.Candidates[1].SourceRevision,
	}
	_, err = AcceptInboxJudgmentSuggestion(context.Background(), suggestion, evidence, authorizer, staleRevisions, projectionCandidates)
	if err == nil {
		t.Fatal("stale inbox revision must block adoption")
	}
	if !isJudgmentErrorCode(err, "judgment_stale_source") {
		t.Errorf("stale adoption must carry judgment_stale_source, got %v", err)
	}
	// 候选 revision 变化同样阻止采纳。
	staleCandidate := map[string]string{
		input.InboxNoteID: input.InboxRevision,
		"note-a":          "sha256:rev-a-new",
		"note-b":          input.Candidates[1].SourceRevision,
	}
	if _, err := AcceptInboxJudgmentSuggestion(context.Background(), suggestion, evidence, authorizer, staleCandidate, projectionCandidates); err == nil {
		t.Error("stale candidate revision must block adoption")
	}
	// 缺失当前 revision（笔记已删除或调用方映射不完整）：未知 ≠ 未变化，
	// 一律 fail closed，不得静默放行。inbox 笔记被引用且缺当前 revision。
	missingRevision := map[string]string{
		"note-a": input.Candidates[0].SourceRevision,
		"note-b": input.Candidates[1].SourceRevision,
	}
	err = AcceptInboxJudgmentSuggestionErr(context.Background(), suggestion, evidence, authorizer, missingRevision, projectionCandidates)
	if err == nil || !isJudgmentErrorCode(err, "judgment_stale_source") {
		t.Fatalf("unknown current revision must block adoption with judgment_stale_source, got %v", err)
	}
	// 权限撤销：不可采纳且不泄露候选存在性。
	revoked := StaticJudgmentAuthorizer{Authorization: InboxJudgmentAuthorization{Denied: true}}
	err = AcceptInboxJudgmentSuggestionErr(context.Background(), suggestion, evidence, revoked, currentRevisions, projectionCandidates)
	if err == nil || !isJudgmentErrorCode(err, "judgment_unauthorized") {
		t.Fatalf("revoked permission must block adoption with judgment_unauthorized, got %v", err)
	}
	if strings.Contains(err.Error(), "note-a") {
		t.Error("revocation error must not name candidates")
	}
	// 权限版本变化：不可采纳。
	changedPerm := StaticJudgmentAuthorizer{Authorization: InboxJudgmentAuthorization{
		VaultDigest:    VaultDigestFor(input.VaultRoot),
		AllowedNoteIDs: []string{input.InboxNoteID, "note-a", "note-b", "note-new"},
	}}
	if _, err := AcceptInboxJudgmentSuggestion(context.Background(), suggestion, evidence, changedPerm, currentRevisions, projectionCandidates); err == nil {
		t.Error("permission version change must block adoption")
	}
	// evidence 与建议未绑定。
	unbound := suggestion
	unbound.Digest = "sha256:unbound"
	if _, err := AcceptInboxJudgmentSuggestion(context.Background(), unbound, evidence, authorizer, currentRevisions, projectionCandidates); err == nil {
		t.Error("suggestion not bound to the evidence must be rejected")
	}
}

func TestReviewHandoffProjectionEnvelope(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	consumer := judgmentFixtureConsumer(t, InboxJudgmentModeAssist, NewFixtureTransport("handoff"), judgmentFixtureAuthorizer(input), nil, []string{"pinax inbox show note-inbox-1"})
	outcome, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	handoff, err := outcome.Suggestion.ReviewHandoff(outcome.Evidence, nil)
	if err != nil {
		t.Fatalf("review handoff: %v", err)
	}
	if handoff.NextAction != "human_review" {
		t.Errorf("next action = %s, want human_review", handoff.NextAction)
	}
	if len(handoff.ReviewRefs) != 1 || handoff.ReviewRefs[0] != "pinax inbox show note-inbox-1" {
		t.Errorf("handoff must fall back to the evidence review refs: %+v", handoff.ReviewRefs)
	}
	projection, err := handoff.Projection()
	if err != nil {
		t.Fatalf("projection: %v", err)
	}
	// 旧 envelope：SpecVersion/Command/Status 字段语义保持；新信息走
	// additive Facts/Actions/Data。
	if projection.SpecVersion != "1.0" || projection.Command != "inbox.judgment" || projection.Status != "success" {
		t.Errorf("projection envelope mismatch: %+v", projection)
	}
	if projection.Facts["next_action"] != "human_review" || projection.Facts["adoptable"] != "true" {
		t.Errorf("projection facts must surface the advisory state: %+v", projection.Facts)
	}
	if len(projection.Actions) == 0 || !strings.HasPrefix(projection.Actions[0].Command, "pinax inbox show") {
		t.Errorf("projection actions must point to the original review commands: %+v", projection.Actions)
	}
	if _, ok := projection.Data.(InboxJudgmentReviewHandoff); !ok {
		t.Errorf("projection data must carry the handoff additively: %T", projection.Data)
	}
}

// judgmentCurrentCandidates 从输入构建当前候选绑定（采纳门用）。
func judgmentCurrentCandidates(input InboxJudgmentInput) []InboxJudgmentCandidate {
	candidates := make([]InboxJudgmentCandidate, 0, len(input.Candidates))
	for index, candidate := range input.Candidates {
		candidates = append(candidates, InboxJudgmentCandidate{
			CandidateID:    candidate.NoteID,
			NoteID:         candidate.NoteID,
			SourceRevision: candidate.SourceRevision,
			InlineText:     candidate.InlineText,
			BaselineRank:   index + 1,
		})
	}
	return candidates
}

// AcceptInboxJudgmentSuggestionErr 显式返回错误，便于断言具体错误码。
func AcceptInboxJudgmentSuggestionErr(ctx context.Context, suggestion InboxJudgmentSuggestion, evidence InboxJudgmentEvidence, authorizer InboxJudgmentAuthorizer, currentRevisions map[string]string, projectionCandidates []InboxJudgmentCandidate) error {
	_, err := AcceptInboxJudgmentSuggestion(ctx, suggestion, evidence, authorizer, currentRevisions, projectionCandidates)
	return err
}

func isJudgmentErrorCode(err error, code string) bool {
	commandErr, ok := err.(*domain.CommandError)
	if !ok {
		return false
	}
	return commandErr.Code == code
}

func mustMarshal(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(data)
}
