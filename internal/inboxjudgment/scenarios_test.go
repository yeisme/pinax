package inboxjudgment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/search"
	"github.com/yeisme/pinax/tools/testkit/evidence"
)

// scenarios_test.go 以离线 fixture transport 运行 design.md 场景矩阵
// （inbox-link、duplicate-warning、vault-isolation）加一组失败注入。每个
// 场景通过本项目 evidence runner（tools/testkit/evidence）包装：runner 执行
// 重入的 `go test -run` 子命令，退出码必须来自真正匹配并通过的子测试，
// 证据落在 temp/integration-test-runs/<run-id>/ 的六个标准类
// （summary.json、command.txt、stdout.log、stderr.log、env.json、artifacts/）。
// 无网络、无凭据、无付费调用；场景日志/输出是脱敏英文摘要，场景设计见
// 中文设计文档。

const (
	scenarioWorkerEnv   = "PINAX_INBOX_JUDGMENT_WORKER"
	scenarioArtifactEnv = "PINAX_INBOX_JUDGMENT_ARTIFACT"
	scenarioFailEnv     = "PINAX_INBOX_JUDGMENT_SCENARIO_FAIL"
)

// TestInboxJudgmentScenarioMatrix 是场景矩阵驱动/worker 入口。
func TestInboxJudgmentScenarioMatrix(t *testing.T) {
	scenarios := []string{"inbox-link", "duplicate-warning", "vault-isolation", "failure-injection"}
	for _, scenarioID := range scenarios {
		scenarioID := scenarioID
		t.Run(scenarioID, func(t *testing.T) {
			if os.Getenv(scenarioWorkerEnv) == "1" {
				// Worker 模式：真实断言在此执行；退出码由匹配的子测试产生。
				if os.Getenv(scenarioFailEnv) == "1" {
					t.Fatal("forced scenario failure: evidence must keep the original exit code")
				}
				runInboxJudgmentScenarioWorker(t, scenarioID)
				return
			}
			// Driver 模式：通过既有 evidence runner 包装重入子命令。
			parentDir := filepath.Join("..", "..", "temp", "integration-test-runs")
			runID := fmt.Sprintf("inbox-judgment-%s-%d", scenarioID, time.Now().UnixNano())
			artifactPath := filepath.Join(os.TempDir(), runID+"-artifact.json")
			t.Setenv(scenarioWorkerEnv, "1")
			t.Setenv(scenarioArtifactEnv, artifactPath)
			command := []string{
				"go", "test", ".", "-v", "-count=1",
				"-run", "TestInboxJudgmentScenarioMatrix/^" + scenarioID + "$",
			}
			result, err := evidence.Run(evidence.Config{
				RunID:      runID,
				ParentDir:  parentDir,
				Layer:      "integration",
				PassStatus: "passed",
				Command:    command,
				ExtraChecks: map[string]any{
					"scenario_id":     scenarioID,
					"transport":       "offline_fixture",
					"network":         "none",
					"mode":            "explicit_opt_in",
					"matched_subtest": "TestInboxJudgmentScenarioMatrix/" + scenarioID,
				},
			})
			if err != nil {
				t.Fatalf("evidence run for %s: %v", scenarioID, err)
			}
			if result.ExitCode != 0 {
				t.Fatalf("scenario %s failed (exit %d); evidence kept at %s", scenarioID, result.ExitCode, result.RunDir)
			}
			assertScenarioEvidenceClasses(t, result.RunDir, scenarioID, true, 0)
			// 退出码必须来自真正匹配的子测试，而非"no tests to run"。
			stdout := readScenarioFile(t, filepath.Join(result.RunDir, "stdout.log"))
			if !strings.Contains(stdout, "=== RUN   TestInboxJudgmentScenarioMatrix/"+scenarioID) {
				t.Fatalf("scenario %s: stdout must prove the matched subtest ran:\n%s", scenarioID, stdout)
			}
			if strings.Contains(stdout, "no tests to run") {
				t.Fatalf("scenario %s: unmatched test run cannot pass the evidence gate", scenarioID)
			}
			// 场景产物进入 artifacts/，并通过脱敏幂等扫描。
			if err := copyScenarioArtifact(t, artifactPath, filepath.Join(result.RunDir, "artifacts", "scenario-"+scenarioID+".json")); err != nil {
				t.Fatalf("scenario %s: artifact copy: %v", scenarioID, err)
			}
			assertScenarioArtifactRedacted(t, filepath.Join(result.RunDir, "artifacts", "scenario-"+scenarioID+".json"))
		})
	}
}

// TestInboxJudgmentScenarioFailureEvidence 证明失败的 evidence 运行同样落全
// 六类证据并保留原始非零退出码（失败注入的另一半）。
func TestInboxJudgmentScenarioFailureEvidence(t *testing.T) {
	if os.Getenv(scenarioWorkerEnv) == "1" {
		// 由其他场景 worker 复用；本测试不作为 worker 运行。
		t.Skip("worker mode reserved for scenario matrix")
	}
	parentDir := filepath.Join("..", "..", "temp", "integration-test-runs")
	runID := fmt.Sprintf("inbox-judgment-failure-evidence-%d", time.Now().UnixNano())
	t.Setenv(scenarioWorkerEnv, "1")
	t.Setenv(scenarioFailEnv, "1")
	command := []string{
		"go", "test", ".", "-v", "-count=1",
		"-run", "TestInboxJudgmentScenarioMatrix/^inbox-link$",
	}
	result, err := evidence.Run(evidence.Config{
		RunID:      runID,
		ParentDir:  parentDir,
		Layer:      "integration",
		PassStatus: "passed",
		Command:    command,
		ExtraChecks: map[string]any{
			"scenario_id": "inbox-link",
			"purpose":     "prove failed runs keep six evidence classes and the original exit code",
			"transport":   "offline_fixture",
			"network":     "none",
		},
	})
	if err != nil {
		t.Fatalf("evidence run: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatalf("forced failure must preserve a non-zero exit code, got %d", result.ExitCode)
	}
	if result.Summary.Status != "failed" {
		t.Fatalf("status = %s, want failed", result.Summary.Status)
	}
	assertScenarioEvidenceClasses(t, result.RunDir, "failure-evidence", false, result.ExitCode)
}

// runInboxJudgmentScenarioWorker 执行单个场景的真实断言并写场景产物。
func runInboxJudgmentScenarioWorker(t *testing.T, scenarioID string) {
	switch scenarioID {
	case "inbox-link":
		inboxJudgmentScenarioInboxLink(t)
	case "duplicate-warning":
		inboxJudgmentScenarioDuplicateWarning(t)
	case "vault-isolation":
		inboxJudgmentScenarioVaultIsolation(t)
	case "failure-injection":
		inboxJudgmentScenarioFailureInjection(t)
	default:
		t.Fatalf("unknown scenario %s", scenarioID)
	}
}

// scenarioVaultNotes 构造 fixture vault 笔记（既有领域模型 + 既有搜索入口）。
func scenarioVaultNotes() []domain.Note {
	return []domain.Note{
		{ID: "note-kyoto-itinerary", Title: "Kyoto temple itinerary", Path: "notes/travel/kyoto-itinerary.md", Status: "active",
			Body: "Day-by-day Kyoto temple itinerary draft. fixture:relation=complement fixture:topical_match=yes fixture:link_usefulness=clearly_useful fixture:language=en"},
		{ID: "note-kyoto-budget", Title: "Kyoto budget sheet", Path: "notes/travel/kyoto-budget.md", Status: "active",
			Body: "Kyoto budget notes complementing the itinerary. fixture:relation=complement fixture:topical_match=yes fixture:link_usefulness=maybe_useful fixture:language=en"},
		{ID: "note-ramen", Title: "Tokyo ramen list", Path: "notes/food/ramen.md", Status: "active",
			Body: "Tokyo ramen shop list. fixture:relation=unrelated fixture:topical_match=no fixture:link_usefulness=not_useful fixture:language=zh"},
	}
}

func scenarioInboxNote() domain.Note {
	return domain.Note{
		ID: "note-inbox-trip", Title: "Kyoto trip plan", Path: "inbox/kyoto-trip-plan.md", Status: "inbox",
		Body: "Plan for the Kyoto visit with temple notes and budget questions.",
	}
}

// scenarioBuildCandidates 复用既有搜索入口做确定性候选预选，再用本包适配
// 为有界授权投影输入（挂接 internal/search 的现有入口）。
func scenarioBuildCandidates(t *testing.T, notes []domain.Note, query string) []InboxJudgmentCandidate {
	t.Helper()
	result := search.Notes(context.Background(), "", query, notes)
	candidates := make([]InboxJudgmentCandidate, 0, len(result.Notes))
	for index, note := range result.Notes {
		summary, truncated := SummarizeNote(note, InboxJudgmentDefaultMaxInlineBytes)
		candidates = append(candidates, InboxJudgmentCandidate{
			CandidateID:    note.ID,
			NoteID:         note.ID,
			SourceRevision: NoteRevision(note),
			InlineText:     summary,
			Truncated:      truncated,
			BaselineRank:   index + 1,
		})
	}
	return candidates
}

func scenarioCandidatesFromNotes(t *testing.T, notes []domain.Note) []InboxJudgmentCandidate {
	t.Helper()
	candidates := make([]InboxJudgmentCandidate, 0, len(notes))
	for index, note := range notes {
		summary, truncated := SummarizeNote(note, InboxJudgmentDefaultMaxInlineBytes)
		candidates = append(candidates, InboxJudgmentCandidate{
			CandidateID: note.ID, NoteID: note.ID, SourceRevision: NoteRevision(note),
			InlineText: summary, Truncated: truncated, BaselineRank: index + 1,
		})
	}
	return candidates
}

func scenarioEvaluate(t *testing.T, mode string, behavior string, input InboxJudgmentInput, allowed []string) (InboxJudgmentOutcome, *FixtureTransport) {
	t.Helper()
	transport := &FixtureTransport{Name: "scenario-" + mode, Behavior: behavior}
	authorizer := StaticJudgmentAuthorizer{Authorization: InboxJudgmentAuthorization{
		VaultDigest: VaultDigestFor(input.VaultRoot), AllowedNoteIDs: allowed,
	}}
	consumer, err := NewInboxJudgmentConsumer(InboxJudgmentOptions{
		Mode: mode, Model: "fixture:local", AdapterVersion: "fixture-v1",
		Transport: transport, Capabilities: FixtureJudgmentCapabilities("scenario"),
		Authorizer: authorizer, Limits: DefaultInboxJudgmentLimits(),
		ReviewRefs: []string{"pinax inbox show " + input.InboxNoteID},
	})
	if err != nil {
		t.Fatalf("consumer: %v", err)
	}
	outcome, err := consumer.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	return outcome, transport
}

// inboxJudgmentScenarioInboxLink：笔记作者整理收件箱——对选中 inbox 文本
// 给出可审阅的关联建议；用户接受/拒绝；交回原 inbox review。
func inboxJudgmentScenarioInboxLink(t *testing.T) {
	notes := scenarioVaultNotes()
	inbox := scenarioInboxNote()
	// 既有搜索入口预选当前 vault 候选（确定性基线）。
	candidates := scenarioBuildCandidates(t, notes, "kyoto")
	if len(candidates) == 0 {
		t.Fatal("baseline candidate preselection via internal/search must return candidates")
	}
	input := InboxJudgmentInput{
		VaultRoot: "/vault/scenario-main", Principal: "owner-local",
		InboxNoteID: inbox.ID, InboxRevision: NoteRevision(inbox),
		InboxText:  summarizeOrFatal(t, inbox),
		Candidates: candidates,
	}
	allowed := append([]string{inbox.ID}, candidateNoteIDs(candidates)...)
	outcome, transport := scenarioEvaluate(t, InboxJudgmentModeAssist, "", input, allowed)
	if outcome.Status != InboxJudgmentStatusSuggested {
		t.Fatalf("status = %s, want suggested", outcome.Status)
	}
	suggestion := outcome.Suggestion
	if !suggestion.Adoptable || len(suggestion.Missing) != 0 {
		t.Fatalf("fully answered scenario must be adoptable (missing=%d)", len(suggestion.Missing))
	}
	if len(suggestion.Links) == 0 || suggestion.Links[0].NoteID != "note-kyoto-itinerary" {
		t.Fatalf("clearly_useful complement must lead the link suggestions: %+v", suggestion.Links)
	}
	// 无关候选绝不进入建议。
	for _, link := range suggestion.Links {
		if link.NoteID == "note-ramen" {
			t.Fatal("unrelated candidate must not win a link suggestion")
		}
	}
	// 显式采纳门：revision 未变、权限有效 → 通过；只枚举原入口命令。
	authorizer := StaticJudgmentAuthorizer{Authorization: InboxJudgmentAuthorization{
		VaultDigest: VaultDigestFor(input.VaultRoot), AllowedNoteIDs: allowed,
	}}
	acceptance, err := AcceptInboxJudgmentSuggestion(context.Background(), *suggestion, outcome.Evidence, authorizer, scenarioCurrentRevisions(input), judgmentCurrentCandidates(input))
	if err != nil {
		t.Fatalf("acceptance: %v", err)
	}
	for _, action := range acceptance.NextActions {
		if !strings.HasPrefix(action, "pinax inbox ") {
			t.Fatalf("acceptance must only enumerate original review commands, got %q", action)
		}
	}
	// 交回原 inbox review（handoff + 旧 envelope）。
	handoff, err := suggestion.ReviewHandoff(outcome.Evidence, nil)
	if err != nil {
		t.Fatalf("handoff: %v", err)
	}
	if handoff.NextAction != "human_review" {
		t.Fatalf("handoff next action = %s", handoff.NextAction)
	}
	writeScenarioArtifact(t, map[string]any{
		"scenario_id": "inbox-link", "status": outcome.Status,
		"adoptable": suggestion.Adoptable, "links": suggestion.Links,
		"baseline_order":           suggestion.BaselineOrder,
		"transport_evaluate_calls": transport.EvaluateCalls(),
		"review_refs":              handoff.ReviewRefs, "next_action": handoff.NextAction,
		"acceptance_next_actions": acceptance.NextActions,
		"suggestion_digest":       suggestion.Digest, "evidence_digest": outcome.Evidence.Digest,
	})
}

// inboxJudgmentScenarioDuplicateWarning：作者发现近似笔记——解释重复/补充
// 关系但不合并；无自动删除；交回原 merge/edit 入口。
func inboxJudgmentScenarioDuplicateWarning(t *testing.T) {
	inbox := scenarioInboxNote()
	duplicate := domain.Note{
		ID: "note-inbox-duplicate", Title: inbox.Title, Path: "inbox/kyoto-trip-plan-copy.md", Status: "inbox",
		Body: inbox.Body, // 内容全等：digest 全等 → 确定性重复。
	}
	related := scenarioVaultNotes()[0]
	notes := []domain.Note{duplicate, related}
	candidates := scenarioCandidatesFromNotes(t, notes)
	input := InboxJudgmentInput{
		VaultRoot: "/vault/scenario-main", Principal: "owner-local",
		InboxNoteID: inbox.ID, InboxRevision: NoteRevision(inbox),
		InboxText:  summarizeOrFatal(t, inbox),
		Candidates: candidates,
	}
	allowed := append([]string{inbox.ID}, candidateNoteIDs(candidates)...)
	// 模型全部弃答：确定性 digest 全等结论仍必须呈现，且采纳被缺答阻止。
	outcome, _ := scenarioEvaluate(t, InboxJudgmentModeAssist, "abstain_all", input, allowed)
	suggestion := outcome.Suggestion
	if len(suggestion.Duplicates) != 1 || suggestion.Duplicates[0].NoteID != "note-inbox-duplicate" || suggestion.Duplicates[0].Basis != "exact_content_digest" {
		t.Fatalf("deterministic duplicate warning must surface without model answers: %+v", suggestion.Duplicates)
	}
	if suggestion.Adoptable {
		t.Fatal("abstained required answers must block adoption")
	}
	// 关系解释到位但绝不合并：建议不携带任何 merge/delete/discard 动作面。
	encoded := mustMarshal(t, suggestion)
	for _, forbidden := range []string{"merge", "delete", "discard"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("duplicate-warning suggestion must not carry %q actions", forbidden)
		}
	}
	writeScenarioArtifact(t, map[string]any{
		"scenario_id": "duplicate-warning", "status": outcome.Status,
		"duplicates": suggestion.Duplicates, "missing_pairs": len(suggestion.Missing),
		"adoptable":         suggestion.Adoptable,
		"guarantee":         "explains relation only; merge/delete stay in the original edit entries",
		"suggestion_digest": suggestion.Digest, "evidence_digest": outcome.Evidence.Digest,
	})
}

// inboxJudgmentScenarioVaultIsolation：多 vault 用户——阻止跨 vault cache/
// 候选泄露；权限隔离；交回原访问边界。
func inboxJudgmentScenarioVaultIsolation(t *testing.T) {
	notes := scenarioVaultNotes()
	inbox := scenarioInboxNote()
	candidates := scenarioCandidatesFromNotes(t, notes)
	input := InboxJudgmentInput{
		VaultRoot: "/vault/scenario-main", Principal: "owner-local",
		InboxNoteID: inbox.ID, InboxRevision: NoteRevision(inbox),
		InboxText:  summarizeOrFatal(t, inbox),
		Candidates: candidates,
	}
	allowedMain := append([]string{inbox.ID}, candidateNoteIDs(candidates)...)
	// vault A 正常评估并缓存。
	cache := NewInboxJudgmentCache()
	transportMain := &FixtureTransport{Name: "vault-main"}
	authorizerMain := StaticJudgmentAuthorizer{Authorization: InboxJudgmentAuthorization{
		VaultDigest: VaultDigestFor("/vault/scenario-main"), AllowedNoteIDs: allowedMain,
	}}
	consumerMain, err := NewInboxJudgmentConsumer(InboxJudgmentOptions{
		Mode: InboxJudgmentModeAssist, Model: "fixture:local", AdapterVersion: "fixture-v1",
		Transport: transportMain, Capabilities: FixtureJudgmentCapabilities("scenario"),
		Cache: cache, Authorizer: authorizerMain, Limits: DefaultInboxJudgmentLimits(),
	})
	if err != nil {
		t.Fatalf("consumer: %v", err)
	}
	outcomeMain, err := consumerMain.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("vault A evaluate: %v", err)
	}
	if cache.Len() != 1 {
		t.Fatalf("vault A evidence must be cached, len=%d", cache.Len())
	}
	// vault B 的授权对同一输入 fail closed（跨 vault 默认不取材）。
	authorizerB := StaticJudgmentAuthorizer{Authorization: InboxJudgmentAuthorization{
		VaultDigest: VaultDigestFor("/vault/scenario-other"), AllowedNoteIDs: allowedMain,
	}}
	transportB := &FixtureTransport{Name: "vault-other"}
	consumerB, err := NewInboxJudgmentConsumer(InboxJudgmentOptions{
		Mode: InboxJudgmentModeAssist, Model: "fixture:local", AdapterVersion: "fixture-v1",
		Transport: transportB, Capabilities: FixtureJudgmentCapabilities("scenario"),
		Cache: cache, Authorizer: authorizerB, Limits: DefaultInboxJudgmentLimits(),
	})
	if err != nil {
		t.Fatalf("consumer B: %v", err)
	}
	_, crossVaultErr := consumerB.Evaluate(context.Background(), input)
	if crossVaultErr == nil {
		t.Fatal("cross-vault authorization must fail closed")
	}
	if strings.Contains(crossVaultErr.Error(), "note-kyoto") {
		t.Fatal("cross-vault failure must not leak vault A candidate ids")
	}
	if transportB.TotalCalls() != 0 {
		t.Fatalf("cross-vault attempt must make zero transport calls, got %d", transportB.TotalCalls())
	}
	// 跨 vault 采纳同样被 vault scope 检查阻止。
	if _, err := AcceptInboxJudgmentSuggestion(context.Background(), *outcomeMain.Suggestion, outcomeMain.Evidence, authorizerB, scenarioCurrentRevisions(input), judgmentCurrentCandidates(input)); err == nil {
		t.Fatal("vault B authorization must not adopt vault A suggestions")
	}
	// 缓存条目仍可被 vault A 自身读取（零网络）。
	replayed, err := consumerMain.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("vault A re-evaluate: %v", err)
	}
	if !replayed.FromCache || transportMain.EvaluateCalls() != 1 {
		t.Fatalf("vault A cache read must stay zero-network, from_cache=%t calls=%d", replayed.FromCache, transportMain.EvaluateCalls())
	}
	writeScenarioArtifact(t, map[string]any{
		"scenario_id":              "vault-isolation",
		"vault_a_status":           outcomeMain.Status,
		"cross_vault_send_blocked": true, "cross_vault_transport_calls": transportB.TotalCalls(),
		"cross_vault_adoption_blocked": true,
		"vault_a_cache_replay":         replayed.FromCache,
		"cache_entries":                cache.Len(),
		"suggestion_digest":            outcomeMain.Suggestion.Digest, "evidence_digest": outcomeMain.Evidence.Digest,
	})
}

// inboxJudgmentScenarioFailureInjection：注入 transport 失败——状态保留、
// 基线顺序保留、outcome unknown 不自动重发、evidence 完整。
func inboxJudgmentScenarioFailureInjection(t *testing.T) {
	notes := scenarioVaultNotes()
	inbox := scenarioInboxNote()
	candidates := scenarioCandidatesFromNotes(t, notes)
	input := InboxJudgmentInput{
		VaultRoot: "/vault/scenario-main", Principal: "owner-local",
		InboxNoteID: inbox.ID, InboxRevision: NoteRevision(inbox),
		InboxText:  summarizeOrFatal(t, inbox),
		Candidates: candidates,
	}
	allowed := append([]string{inbox.ID}, candidateNoteIDs(candidates)...)
	unavailable, _ := scenarioEvaluate(t, InboxJudgmentModeAssist, "unavailable", input, allowed)
	if unavailable.Status != InboxJudgmentStatusUnavailable {
		t.Fatalf("unavailable injection status = %s", unavailable.Status)
	}
	unknown, unknownTransport := scenarioEvaluate(t, InboxJudgmentModeAssist, "timeout_after_submit", input, allowed)
	if unknown.Status != InboxJudgmentStatusOutcomeUnknown {
		t.Fatalf("timeout injection status = %s", unknown.Status)
	}
	if unknownTransport.EvaluateCalls() != 1 {
		t.Fatalf("outcome unknown must not auto-resend, calls=%d", unknownTransport.EvaluateCalls())
	}
	if unknown.Evidence.SubmissionState != JudgmentSubmissionUnknown || unknown.Evidence.RetryClass != JudgmentRetryReconcileFirst {
		t.Fatalf("unknown outcome evidence must carry unknown/reconcile_first, got %s/%s", unknown.Evidence.SubmissionState, unknown.Evidence.RetryClass)
	}
	if len(unavailable.BaselineOrder) != len(input.Candidates) || len(unknown.BaselineOrder) != len(input.Candidates) {
		t.Fatal("failures must preserve the baseline candidate order")
	}
	if err := unavailable.Evidence.Validate(); err != nil {
		t.Fatalf("failure evidence must validate: %v", err)
	}
	// 不可用绝不伪装成"没有问题"：摘要明示失败与不重发。
	if !strings.Contains(unavailable.Evidence.Summary, "failed") || !strings.Contains(unavailable.Evidence.Summary, "not resubmitted") {
		t.Fatalf("failure summary must state failure and no-resend: %s", unavailable.Evidence.Summary)
	}
	writeScenarioArtifact(t, map[string]any{
		"scenario_id":                     "failure-injection",
		"unavailable_status":              unavailable.Status,
		"outcome_unknown_status":          unknown.Status,
		"outcome_unknown_transport_calls": unknownTransport.EvaluateCalls(),
		"submission_state":                unknown.Evidence.SubmissionState, "retry_class": unknown.Evidence.RetryClass,
		"baseline_order_preserved":    len(unknown.BaselineOrder) == len(input.Candidates),
		"unavailable_evidence_digest": unavailable.Evidence.Digest,
		"unknown_evidence_digest":     unknown.Evidence.Digest,
	})
}

// assertScenarioEvidenceClasses 校验六个标准证据类与 summary 状态。
func assertScenarioEvidenceClasses(t *testing.T, dir, scenarioID string, expectPassed bool, expectedExit int) {
	t.Helper()
	required := []string{"summary.json", "command.txt", "stdout.log", "stderr.log", "env.json"}
	for _, name := range required {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s: evidence class %s missing: %v", scenarioID, name, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(dir, "artifacts"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("%s: evidence artifacts class missing or empty (err=%v)", scenarioID, err)
	}
	summary := readScenarioFile(t, filepath.Join(dir, "summary.json"))
	var parsed struct {
		Status   string         `json:"status"`
		ExitCode int            `json:"exit_code"`
		Layer    string         `json:"layer"`
		RunID    string         `json:"run_id"`
		Checks   map[string]any `json:"checks"`
	}
	if err := json.Unmarshal([]byte(summary), &parsed); err != nil {
		t.Fatalf("%s: summary invalid: %v", scenarioID, err)
	}
	wantStatus := "failed"
	if expectPassed {
		wantStatus = "passed"
	}
	if parsed.Status != wantStatus || parsed.ExitCode != expectedExit {
		t.Fatalf("%s: summary status/exit mismatch: %s/%d want %s/%d", scenarioID, parsed.Status, parsed.ExitCode, wantStatus, expectedExit)
	}
	if parsed.Layer != "integration" {
		t.Fatalf("%s: evidence layer mismatch: %s", scenarioID, parsed.Layer)
	}
}

func assertScenarioArtifactRedacted(t *testing.T, path string) {
	t.Helper()
	raw := readScenarioFile(t, path)
	if evidence.Redact(raw) != raw {
		t.Fatalf("scenario artifact must be redaction-clean:\n%s", evidence.Redact(raw))
	}
}

func copyScenarioArtifact(t *testing.T, from, to string) error {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("read worker artifact: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("worker artifact is empty")
	}
	if err := os.WriteFile(to, data, 0o644); err != nil {
		return fmt.Errorf("write artifact: %w", err)
	}
	_ = os.Remove(from)
	return nil
}

func readScenarioFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func writeScenarioArtifact(t *testing.T, artifact map[string]any) {
	t.Helper()
	artifactPath := os.Getenv(scenarioArtifactEnv)
	if artifactPath == "" {
		return
	}
	encoded, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		t.Fatalf("artifact marshal: %v", err)
	}
	if err := os.WriteFile(artifactPath, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("artifact write: %v", err)
	}
}

func candidateNoteIDs(candidates []InboxJudgmentCandidate) []string {
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.NoteID)
	}
	return ids
}

func scenarioCurrentRevisions(input InboxJudgmentInput) map[string]string {
	revisions := map[string]string{input.InboxNoteID: input.InboxRevision}
	for _, candidate := range input.Candidates {
		revisions[candidate.NoteID] = candidate.SourceRevision
	}
	return revisions
}

func summarizeOrFatal(t *testing.T, note domain.Note) string {
	t.Helper()
	summary, _ := SummarizeNote(note, InboxJudgmentDefaultMaxInlineBytes)
	return summary
}
