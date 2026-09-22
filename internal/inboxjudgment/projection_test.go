package inboxjudgment

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/domain"
)

// projection_test.go 覆盖任务 1.1：领域最小投影与问题集——版本绑定、
// 确定性前置规则、权限/vault 隔离与"不夹带未授权文本"。

func TestInboxJudgmentQuestionSetStable(t *testing.T) {
	questions := InboxJudgmentQuestions()
	if len(questions) != 4 {
		t.Fatalf("v1 question set must stay atomic and bounded, got %d questions", len(questions))
	}
	seen := map[string]bool{}
	for _, question := range questions {
		if seen[question.ID] {
			t.Errorf("duplicate question id %s", question.ID)
		}
		seen[question.ID] = true
		if !question.Required {
			t.Errorf("v1 question %s must be required so missing answers block adoption", question.ID)
		}
		if strings.TrimSpace(question.Text) == "" {
			t.Errorf("question %s needs controlled prompt text", question.ID)
		}
		switch question.Primitive {
		case JudgmentPrimitiveChoice:
			if len(question.OptionIDs) == 0 {
				t.Errorf("choice question %s needs an explicit answer domain", question.ID)
			}
		case JudgmentPrimitiveOrdinalScore:
			if len(question.LevelIDs) < 2 {
				t.Errorf("ordinal question %s needs ordered levels", question.ID)
			}
		case JudgmentPrimitiveBinary:
		default:
			t.Errorf("question %s uses an unknown primitive %s", question.ID, question.Primitive)
		}
	}
	questionSetDigest, questionSetDigestAgain := InboxJudgmentQuestionSetDigest(), InboxJudgmentQuestionSetDigest()
	if questionSetDigest != questionSetDigestAgain {
		t.Error("question set digest must be deterministic")
	}
	if len(InboxJudgmentQuestionSetDigest()) == 0 || InboxJudgmentPolicyDigest() == "" {
		t.Error("digests must be populated")
	}
}

func TestInboxJudgmentBindingValidate(t *testing.T) {
	enabled := InboxJudgmentBinding{
		Mode:               InboxJudgmentModeAssist,
		Model:              "fixture:local",
		AdapterVersion:     "fixture-v1",
		QuestionSetID:      InboxJudgmentQuestionSetID,
		QuestionSetVersion: InboxJudgmentQuestionSetVersion,
		QuestionSetDigest:  InboxJudgmentQuestionSetDigest(),
		PolicyID:           InboxJudgmentPolicyID,
		PolicyVersion:      InboxJudgmentPolicyVersion,
		PolicyDigest:       InboxJudgmentPolicyDigest(),
	}
	if err := enabled.Validate(); err != nil {
		t.Fatalf("enabled binding must validate: %v", err)
	}
	if !enabled.Enabled() {
		t.Error("assist binding must report Enabled")
	}
	// off 绑定必须保持空。
	off := InboxJudgmentBinding{Mode: InboxJudgmentModeOff}
	if err := off.Validate(); err != nil {
		t.Fatalf("off binding must validate: %v", err)
	}
	offWithFields := enabled
	offWithFields.Mode = InboxJudgmentModeOff
	if err := offWithFields.Validate(); err == nil {
		t.Error("off binding carrying fields must be rejected")
	}
	// 缺模型 pin。
	noModel := enabled
	noModel.Model = ""
	if err := noModel.Validate(); err == nil {
		t.Error("enabled binding requires an exact model pin")
	}
	// 问题集 digest 不匹配。
	staleQuestionSet := enabled
	staleQuestionSet.QuestionSetDigest = "sha256:deadbeef"
	if err := staleQuestionSet.Validate(); err == nil {
		t.Error("stale question set digest must be rejected")
	}
	// 策略 digest 不匹配。
	stalePolicy := enabled
	stalePolicy.PolicyDigest = "sha256:deadbeef"
	if err := stalePolicy.Validate(); err == nil {
		t.Error("stale policy digest must be rejected")
	}
	// 未知模式。
	unknown := enabled
	unknown.Mode = "auto"
	if err := unknown.Validate(); err == nil {
		t.Error("unknown mode must be rejected")
	}
}

// judgmentProjectionFixtureInput 构造一组合法投影输入（候选文本带 fixture
// verdict tag，供 fixture transport 离线作答）。
func judgmentProjectionFixtureInput() InboxJudgmentInput {
	return InboxJudgmentInput{
		VaultRoot:     "/vault/main",
		Principal:     "owner-local",
		InboxNoteID:   "note-inbox-1",
		InboxRevision: "sha256:inbox-rev-1",
		InboxText:     "Trip plan for the Kyoto visit with temple notes. fixture:relation=unrelated fixture:topical_match=no fixture:link_usefulness=not_useful fixture:language=en",
		Candidates: []InboxJudgmentCandidate{
			{CandidateID: "note-a", NoteID: "note-a", SourceRevision: "sha256:rev-a", InlineText: "Kyoto temple itinerary draft. fixture:relation=complement fixture:topical_match=yes fixture:link_usefulness=clearly_useful fixture:language=en", BaselineRank: 1},
			{CandidateID: "note-b", NoteID: "note-b", SourceRevision: "sha256:rev-b", InlineText: "Tokyo ramen list. fixture:relation=unrelated fixture:topical_match=no fixture:link_usefulness=not_useful fixture:language=zh", BaselineRank: 2},
		},
	}
}

func judgmentFixtureAuthorizer(input InboxJudgmentInput) StaticJudgmentAuthorizer {
	return StaticJudgmentAuthorizer{Authorization: InboxJudgmentAuthorization{
		VaultDigest:    VaultDigestFor(input.VaultRoot),
		AllowedNoteIDs: []string{input.InboxNoteID, "note-a", "note-b"},
	}}
}

func judgmentEnabledBinding() InboxJudgmentBinding {
	return InboxJudgmentBinding{
		Mode:               InboxJudgmentModeAssist,
		Model:              "fixture:local",
		AdapterVersion:     "fixture-v1",
		QuestionSetID:      InboxJudgmentQuestionSetID,
		QuestionSetVersion: InboxJudgmentQuestionSetVersion,
		QuestionSetDigest:  InboxJudgmentQuestionSetDigest(),
		PolicyID:           InboxJudgmentPolicyID,
		PolicyVersion:      InboxJudgmentPolicyVersion,
		PolicyDigest:       InboxJudgmentPolicyDigest(),
	}
}

func TestBuildInboxJudgmentProjection(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	binding := judgmentEnabledBinding()
	projection, err := BuildInboxJudgmentProjection(context.Background(), binding, judgmentFixtureAuthorizer(input), input, DefaultInboxJudgmentLimits())
	if err != nil {
		t.Fatalf("build projection: %v", err)
	}
	if projection.SchemaVersion != InboxJudgmentProjectionSchema {
		t.Errorf("projection schema = %s", projection.SchemaVersion)
	}
	if projection.VaultDigest != VaultDigestFor(input.VaultRoot) {
		t.Error("projection must bind the current vault digest")
	}
	if projection.PermissionDigest == "" || projection.Digest == "" {
		t.Error("projection must carry permission and projection digests")
	}
	if len(projection.Questions) != len(InboxJudgmentQuestions()) {
		t.Error("projection must pin the versioned question set")
	}
	// 候选 ID 归一为笔记 ID；基线排名保留。
	for index, candidate := range projection.Candidates {
		if candidate.CandidateID != candidate.NoteID {
			t.Errorf("candidate id must align to the note id, got %s vs %s", candidate.CandidateID, candidate.NoteID)
		}
		if candidate.BaselineRank != index+1 {
			t.Errorf("baseline rank must preserve deterministic order, candidate %s rank %d", candidate.NoteID, candidate.BaselineRank)
		}
	}
	// 输入不夹带未授权文本：授权集合外的候选直接 fail closed。
	leaking := input
	leaking.Candidates = append(leaking.Candidates, InboxJudgmentCandidate{
		NoteID: "note-private", SourceRevision: "sha256:rev-private",
		InlineText: "Existence of this note must not leak. fixture:relation=duplicate",
	})
	_, err = BuildInboxJudgmentProjection(context.Background(), binding, judgmentFixtureAuthorizer(input), leaking, DefaultInboxJudgmentLimits())
	if err == nil {
		t.Fatal("unauthorized candidate must fail closed")
	}
	if strings.Contains(err.Error(), "note-private") {
		t.Error("failure must not leak the unauthorized candidate id")
	}
}

func TestBuildProjectionVaultIsolation(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	binding := judgmentEnabledBinding()
	// 授权绑定到另一个 vault：跨 vault 取材 fail closed。
	foreign := StaticJudgmentAuthorizer{Authorization: InboxJudgmentAuthorization{
		VaultDigest:    VaultDigestFor("/vault/other"),
		AllowedNoteIDs: []string{input.InboxNoteID, "note-a", "note-b"},
	}}
	_, err := BuildInboxJudgmentProjection(context.Background(), binding, foreign, input, DefaultInboxJudgmentLimits())
	if err == nil {
		t.Fatal("cross-vault sourcing must fail closed")
	}
	if strings.Contains(err.Error(), "note-a") || strings.Contains(err.Error(), "/vault/other") {
		t.Error("cross-vault failure must not leak candidate ids or the foreign vault path")
	}
	// 整体拒绝：denied 授权 fail closed。
	denied := StaticJudgmentAuthorizer{Authorization: InboxJudgmentAuthorization{Denied: true, Reason: "revoked"}}
	if _, err := BuildInboxJudgmentProjection(context.Background(), binding, denied, input, DefaultInboxJudgmentLimits()); err == nil {
		t.Fatal("denied authorization must fail closed")
	}
	// 无 authorizer：显式错误。
	if _, err := BuildInboxJudgmentProjection(context.Background(), binding, nil, input, DefaultInboxJudgmentLimits()); err == nil {
		t.Fatal("missing authorizer must fail explicitly")
	}
}

func TestBuildProjectionInboxNoteAuthorization(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	binding := judgmentEnabledBinding()
	// inbox 笔记自身不在授权集合：投影阶段即拒绝，inbox 正文绝不进入
	// 可外发投影（不留“已外发、采纳门才拒绝”的窗口）。
	unauthorized := StaticJudgmentAuthorizer{Authorization: InboxJudgmentAuthorization{
		VaultDigest:    VaultDigestFor(input.VaultRoot),
		AllowedNoteIDs: []string{"note-a", "note-b"},
	}}
	_, err := BuildInboxJudgmentProjection(context.Background(), binding, unauthorized, input, DefaultInboxJudgmentLimits())
	if err == nil {
		t.Fatal("inbox note outside the authorized set must fail closed at projection time")
	}
	var commandError *domain.CommandError
	if !errors.As(err, &commandError) || commandError.Code != "judgment_unauthorized" {
		t.Fatalf("want judgment_unauthorized, got %v", err)
	}
	if strings.Contains(err.Error(), input.InboxNoteID) {
		t.Error("failure must not leak the inbox note id")
	}
	// 空授权集合 = deny-all：与缓存重授权语义一致，不静默放行任何候选。
	empty := StaticJudgmentAuthorizer{Authorization: InboxJudgmentAuthorization{
		VaultDigest: VaultDigestFor(input.VaultRoot),
	}}
	if _, err := BuildInboxJudgmentProjection(context.Background(), binding, empty, input, DefaultInboxJudgmentLimits()); err == nil {
		t.Fatal("empty allowed set must fail closed")
	}
}

func TestBuildProjectionDeterministicExactDuplicate(t *testing.T) {
	input := judgmentProjectionFixtureInput()
	// 候选正文与 inbox 文本完全一致：digest 全等即可计算重复，无需模型。
	input.Candidates[0].InlineText = input.InboxText
	projection, err := BuildInboxJudgmentProjection(context.Background(), judgmentEnabledBinding(), judgmentFixtureAuthorizer(input), input, DefaultInboxJudgmentLimits())
	if err != nil {
		t.Fatalf("build projection: %v", err)
	}
	if len(projection.Deterministic) != 1 {
		t.Fatalf("exact content digest must produce one deterministic finding, got %d", len(projection.Deterministic))
	}
	finding := projection.Deterministic[0]
	if finding.Kind != "exact_content_digest" || finding.NoteID != "note-a" {
		t.Errorf("deterministic finding mismatch: %+v", finding)
	}
}

func TestInboxJudgmentPrecheckRules(t *testing.T) {
	questions := InboxJudgmentQuestions()
	limits := DefaultInboxJudgmentLimits()
	base := judgmentProjectionFixtureInput()
	baseCandidate := func() InboxJudgmentCandidate { return base.Candidates[0] }
	emptyInbox := base
	emptyInbox.InboxText = "   "
	if err := InboxJudgmentPrecheck(emptyInbox.InboxText, emptyInbox.InboxNoteID, emptyInbox.InboxRevision, emptyInbox.Candidates, questions, limits, nil); err == nil {
		t.Error("empty inbox text must fail")
	}
	noInboxRevision := base
	noInboxRevision.InboxRevision = " "
	if err := InboxJudgmentPrecheck(noInboxRevision.InboxText, noInboxRevision.InboxNoteID, noInboxRevision.InboxRevision, noInboxRevision.Candidates, questions, limits, nil); err == nil {
		t.Error("missing inbox revision must fail")
	}
	noCandidates := base
	noCandidates.Candidates = nil
	if err := InboxJudgmentPrecheck(noCandidates.InboxText, noCandidates.InboxNoteID, noCandidates.InboxRevision, nil, questions, limits, nil); err == nil {
		t.Error("empty candidate set must fail")
	}
	duplicateIDs := base
	duplicateIDs.Candidates = []InboxJudgmentCandidate{baseCandidate(), baseCandidate()}
	if err := InboxJudgmentPrecheck(duplicateIDs.InboxText, duplicateIDs.InboxNoteID, duplicateIDs.InboxRevision, duplicateIDs.Candidates, questions, limits, nil); err == nil {
		t.Error("duplicate candidate ids must fail")
	}
	missingRevision := baseCandidate()
	missingRevision.SourceRevision = ""
	if err := InboxJudgmentPrecheck(base.InboxText, base.InboxNoteID, base.InboxRevision, []InboxJudgmentCandidate{missingRevision}, questions, limits, nil); err == nil {
		t.Error("missing candidate revision must fail")
	}
	selfPair := baseCandidate()
	selfPair.NoteID = base.InboxNoteID
	selfPair.CandidateID = base.InboxNoteID
	if err := InboxJudgmentPrecheck(base.InboxText, base.InboxNoteID, base.InboxRevision, []InboxJudgmentCandidate{selfPair}, questions, limits, nil); err == nil {
		t.Error("self-pair must fail")
	}
	oversized := base
	oversized.InboxText = strings.Repeat("a", limits.MaxInboxBytes+1)
	if err := InboxJudgmentPrecheck(oversized.InboxText, oversized.InboxNoteID, oversized.InboxRevision, oversized.Candidates, questions, limits, nil); err == nil {
		t.Error("oversized inbox text must fail")
	}
	tooMany := base
	for i := 0; i < limits.MaxCandidates; i++ {
		tooMany.Candidates = append(tooMany.Candidates, InboxJudgmentCandidate{NoteID: "note-extra", SourceRevision: "sha256:rev", InlineText: "excerpt"})
	}
	if err := InboxJudgmentPrecheck(tooMany.InboxText, tooMany.InboxNoteID, tooMany.InboxRevision, tooMany.Candidates, questions, limits, nil); err == nil {
		t.Error("candidate count over the bound must fail")
	}
	// 敏感形态：fail closed 且不回显文本。
	secretCandidate := baseCandidate()
	secretCandidate.InlineText = "note with api_key=live-secret-value inside"
	err := InboxJudgmentPrecheck(base.InboxText, base.InboxNoteID, base.InboxRevision, []InboxJudgmentCandidate{secretCandidate}, questions, limits, nil)
	if err == nil {
		t.Fatal("secret-shaped candidate text must fail closed")
	}
	if strings.Contains(err.Error(), "live-secret-value") {
		t.Error("precheck error must never echo the sensitive text")
	}
	secretInbox := base
	secretInbox.InboxText = "inbox with Authorization: Bearer abc123 token"
	if err := InboxJudgmentPrecheck(secretInbox.InboxText, secretInbox.InboxNoteID, secretInbox.InboxRevision, secretInbox.Candidates, questions, limits, nil); err == nil {
		t.Error("secret-shaped inbox text must fail closed")
	}
	// 原语支持预检（本地能力快照；无网络）。
	binaryOnly := map[string]bool{JudgmentPrimitiveBinary: true}
	err = InboxJudgmentPrecheck(base.InboxText, base.InboxNoteID, base.InboxRevision, base.Candidates, questions, limits, binaryOnly)
	if err == nil {
		t.Fatal("unsupported primitive must fail precheck")
	}
	if !strings.Contains(err.Error(), JudgmentCodeUnsupportedCapability) {
		t.Errorf("unsupported primitive must use the capability error code, got %v", err)
	}
}

func TestSummarizeNoteAndNoteRevision(t *testing.T) {
	note := domain.Note{ID: "note-1", Title: "Kyoto temples", Body: strings.Repeat("body ", 2000)}
	summary, truncated := SummarizeNote(note, InboxJudgmentDefaultMaxInlineBytes)
	if !truncated {
		t.Error("long note summary must record truncation")
	}
	if len(summary) > InboxJudgmentDefaultMaxInlineBytes {
		t.Errorf("summary must stay within the byte bound, got %d", len(summary))
	}
	if !utf8.ValidString(summary) {
		t.Error("summary must truncate at a rune boundary")
	}
	// CJK 截断同样在 rune 边界收尾。
	cjk := domain.Note{ID: "note-2", Title: "京都寺庙", Body: strings.Repeat("禅", 4000)}
	cjkSummary, cjkTruncated := SummarizeNote(cjk, 30)
	if cjkTruncated && !utf8.ValidString(cjkSummary) {
		t.Error("CJK truncation must stay rune-safe")
	}
	revision := NoteRevision(note)
	if !strings.HasPrefix(revision, "sha256:") {
		t.Fatalf("note revision must be a sha256 content digest, got %s", revision)
	}
	if revision != NoteRevision(domain.Note{ID: "note-1", Title: "Kyoto temples", Body: strings.Repeat("body ", 2000)}) {
		t.Error("note revision must be stable for identical content")
	}
	changed := note
	changed.Body = "edited"
	if NoteRevision(changed) == revision {
		t.Error("note revision must change when content changes")
	}
}

func TestVaultJudgmentAuthorizer(t *testing.T) {
	if _, err := (VaultJudgmentAuthorizer{}).Authorize(context.Background()); err == nil {
		t.Error("missing vault root must fail explicitly")
	}
	authorizer := VaultJudgmentAuthorizer{VaultRoot: "/vault/main", AllowedNoteIDs: []string{"n1"}}
	authorization, err := authorizer.Authorize(context.Background())
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if authorization.VaultDigest != VaultDigestFor("/vault/main") || !authorization.Allows("n1") || authorization.Allows("n2") {
		t.Errorf("authorization mismatch: %+v", authorization)
	}
	authDigest, authDigestAgain := authorization.Digest(), authorization.Digest()
	if authDigest != authDigestAgain {
		t.Error("authorization digest must be deterministic")
	}
}
