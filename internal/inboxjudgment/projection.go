package inboxjudgment

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/redaction"
)

// 笔记 inbox 分类、关联与重复线索的最小投影与问题集（exploratory）。
//
// 输入只包含：明确选中的 inbox 文本、当前 vault 内获授权的候选笔记摘要
// 及 revision。跨 vault 默认不取材（vault scope digest 绑定）。模型只接收
// 有界的 inline_text，不自行抓取 source URL、不读取任意文件、不解释权限。
// 确定性前置规则（权限、必需字段、类型、可计算约束）先于任何模型调用，
// 且永不被概率替代。见 openspec/changes/pinax-inbox-judgment-v1。

// 判断模式。默认 off：零发现、零远程调用、原流程不变。
const (
	InboxJudgmentModeOff    = "off"
	InboxJudgmentModeShadow = "shadow"
	InboxJudgmentModeAssist = "assist"
)

// 问题集与策略的 owner 版本绑定。
const (
	InboxJudgmentQuestionSetID      = "pinax.inbox_judgment_questions"
	InboxJudgmentQuestionSetVersion = "v1"
	InboxJudgmentPolicyID           = "pinax.inbox_judgment_policy"
	InboxJudgmentPolicyVersion      = "exploratory-v1"
	// 策略文本是 owner 维护的阈值/规则声明。刻意不设全局 0.8 默认值：
	// 采纳要求全部必需问题显式作答；建议只做分类/链接/重复线索，
	// 永不合并或删除；跨 vault 不取材；超时保留原流程；outcome unknown
	// 不自动重发；调用与缓存读取前都重新核验权限。
	InboxJudgmentPolicyText = "exploratory-v1:no-global-threshold;adoptable-requires-all-required-answered;suggestions-never-merge-or-delete;adoption-stays-behind-original-inbox-review-gate;same-vault-scope-only;timeout-keeps-original-flow;outcome-unknown-no-auto-resend;permission-recheck-before-call-and-cache-read"

	InboxJudgmentProjectionSchema = "pinax.inbox_judgment_projection.v1"

	InboxJudgmentDefaultMaxCandidates  = 8
	InboxJudgmentDefaultMaxInboxBytes  = 2000
	InboxJudgmentDefaultMaxInlineBytes = 4000
	InboxJudgmentDefaultDeadlineMS     = 5000
)

// 关系答案域（question "relation" 的 choice 域；与设计问题
// "两条笔记是重复、补充还是矛盾？"对齐，另加 unrelated 以保持完备）。
const (
	RelationDuplicate     = "duplicate"
	RelationComplement    = "complement"
	RelationContradiction = "contradiction"
	RelationUnrelated     = "unrelated"
)

// InboxJudgmentQuestion 是一个原子、candidate 作用域的问题，带显式答案域。
// 输出按显式 (candidate_id, question_id) pair 对齐，绝不按数组顺序。
type InboxJudgmentQuestion struct {
	ID        string   `json:"id"`
	Primitive string   `json:"primitive"`            // binary | ordinal_score | choice
	Text      string   `json:"text"`                 // 受控问题提示（英文、有界）
	Required  bool     `json:"required"`             // 必需问题缺答/弃答阻止完整建议采纳
	OptionIDs []string `json:"option_ids,omitempty"` // choice 答案域
	LevelIDs  []string `json:"level_ids,omitempty"`  // ordinal 答案域，由低到高
}

// InboxJudgmentQuestions 返回版本化 v1 问题集，用于 inbox 分类、关联与
// 重复线索建议。每个问题保持原子化与显式答案域；不确定性用弃答表达，
// 不发明概率。
func InboxJudgmentQuestions() []InboxJudgmentQuestion {
	return []InboxJudgmentQuestion{
		{
			ID:        "topical_match",
			Primitive: JudgmentPrimitiveBinary,
			Required:  true,
			Text:      "Does this candidate note address the same subject as the selected inbox note?",
		},
		{
			ID:        "relation",
			Primitive: JudgmentPrimitiveChoice,
			Required:  true,
			OptionIDs: []string{RelationDuplicate, RelationComplement, RelationContradiction, RelationUnrelated},
			Text:      "Compared with the selected inbox note, is this candidate note a duplicate, a complement, a contradiction, or unrelated?",
		},
		{
			ID:        "link_usefulness",
			Primitive: JudgmentPrimitiveOrdinalScore,
			Required:  true,
			LevelIDs:  []string{"not_useful", "maybe_useful", "clearly_useful"},
			Text:      "How useful would linking the selected inbox note to this candidate note be for a human reviewer?",
		},
		{
			ID:        "language",
			Primitive: JudgmentPrimitiveChoice,
			Required:  true,
			OptionIDs: []string{"zh", "en", "mixed", "other"},
			Text:      "Which language does this candidate note use?",
		},
	}
}

// InboxJudgmentQuestionSetDigest 是版本化问题集的规范 digest。任何问题、
// 原语或答案域变化都会改变它，使旧绑定与缓存失效。
func InboxJudgmentQuestionSetDigest() string {
	payload, _ := json.Marshal(struct {
		ID        string                  `json:"id"`
		Version   string                  `json:"version"`
		Questions []InboxJudgmentQuestion `json:"questions"`
	}{InboxJudgmentQuestionSetID, InboxJudgmentQuestionSetVersion, InboxJudgmentQuestions()})
	return judgmentDigestBytes(payload)
}

// InboxJudgmentPolicyDigest 是每条判断绑定钉住的 owner 策略声明 digest。
func InboxJudgmentPolicyDigest() string {
	payload, _ := json.Marshal(struct {
		ID      string `json:"id"`
		Version string `json:"version"`
		Policy  string `json:"policy"`
	}{InboxJudgmentPolicyID, InboxJudgmentPolicyVersion, InboxJudgmentPolicyText})
	return judgmentDigestBytes(payload)
}

// InboxJudgmentBinding 钉住一次评估使用的精确问题集、策略、模型与
// adapter 版本。evidence 与建议携带该绑定，旧结果不能对新版本采纳。
type InboxJudgmentBinding struct {
	Mode               string `json:"mode"`
	Model              string `json:"model,omitempty"`
	AdapterVersion     string `json:"adapter_version,omitempty"`
	QuestionSetID      string `json:"question_set_id,omitempty"`
	QuestionSetVersion string `json:"question_set_version,omitempty"`
	QuestionSetDigest  string `json:"question_set_digest,omitempty"`
	PolicyID           string `json:"policy_id,omitempty"`
	PolicyVersion      string `json:"policy_version,omitempty"`
	PolicyDigest       string `json:"policy_digest,omitempty"`
}

// Enabled 报告绑定是否激活判断（shadow 或 assist）。
func (b InboxJudgmentBinding) Enabled() bool {
	return b.Mode == InboxJudgmentModeShadow || b.Mode == InboxJudgmentModeAssist
}

// Validate 把绑定钉到当前版本化问题集与策略。
func (b InboxJudgmentBinding) Validate() error {
	switch b.Mode {
	case "", InboxJudgmentModeOff:
		if b.Model != "" || b.AdapterVersion != "" || b.QuestionSetID != "" || b.QuestionSetVersion != "" ||
			b.QuestionSetDigest != "" || b.PolicyID != "" || b.PolicyVersion != "" || b.PolicyDigest != "" {
			return judgmentInvalidRequest("off binding must stay empty", "Clear the judgment binding fields for off mode")
		}
		return nil
	case InboxJudgmentModeShadow, InboxJudgmentModeAssist:
	default:
		return judgmentInvalidRequest("unknown judgment mode", "Use off, shadow, or assist")
	}
	if strings.TrimSpace(b.Model) == "" {
		return judgmentInvalidRequest("enabled binding requires an exact model pin", "Set the exact model identifier; aliases are resolved before evaluation")
	}
	if b.QuestionSetID != InboxJudgmentQuestionSetID || b.QuestionSetVersion != InboxJudgmentQuestionSetVersion || b.QuestionSetDigest != InboxJudgmentQuestionSetDigest() {
		return judgmentInvalidRequest("question set binding does not match the versioned set", "Rebuild the binding from the current question set")
	}
	if b.PolicyID != InboxJudgmentPolicyID || b.PolicyVersion != InboxJudgmentPolicyVersion || b.PolicyDigest != InboxJudgmentPolicyDigest() {
		return judgmentInvalidRequest("policy binding does not match the versioned policy", "Rebuild the binding from the current policy")
	}
	return nil
}

// InboxJudgmentBindingDigest 计算绑定的 digest（evidence 记录用）。
func InboxJudgmentBindingDigest(b InboxJudgmentBinding) string {
	return judgmentDigest(b)
}

// InboxJudgmentLimits 约束判断输入。限制基于字节数与条目数，不做 token
// 猜测；调用方保留原上下文预算。
type InboxJudgmentLimits struct {
	MaxCandidates  int   `json:"max_candidates"`
	MaxInboxBytes  int   `json:"max_inbox_bytes"`
	MaxInlineBytes int   `json:"max_inline_bytes"`
	DeadlineMS     int64 `json:"deadline_ms,omitempty"`
}

// DefaultInboxJudgmentLimits 返回有界默认限制。
func DefaultInboxJudgmentLimits() InboxJudgmentLimits {
	return InboxJudgmentLimits{
		MaxCandidates:  InboxJudgmentDefaultMaxCandidates,
		MaxInboxBytes:  InboxJudgmentDefaultMaxInboxBytes,
		MaxInlineBytes: InboxJudgmentDefaultMaxInlineBytes,
		DeadlineMS:     InboxJudgmentDefaultDeadlineMS,
	}
}

// InboxJudgmentCandidate 是一条已授权候选笔记的最小投影。InlineText 在
// 离开领域边界前已脱敏并有界。
type InboxJudgmentCandidate struct {
	CandidateID    string `json:"candidate_id"`
	NoteID         string `json:"note_id"`
	SourceRevision string `json:"source_revision"`
	SourceDigest   string `json:"source_digest,omitempty"`
	InlineText     string `json:"inline_text"`
	Language       string `json:"language,omitempty"`
	Truncated      bool   `json:"truncated,omitempty"`
	BaselineRank   int    `json:"baseline_rank"` // 原确定性候选顺序（如既有搜索排名），1 起
}

// InboxJudgmentDeterministicFinding 是模型介入前即可计算的确定性结论。
// 可计算约束（如内容 digest 全等）不能用概率替代，直接进入建议。
type InboxJudgmentDeterministicFinding struct {
	Kind     string `json:"kind"` // exact_content_digest
	NoteID   string `json:"note_id"`
	Revision string `json:"revision"`
}

// InboxJudgmentProjection 是交给判断 transport 的有界、版本化问题集投影。
// 只含已授权候选与选中 inbox 文本；绝不夹带未授权条目——其存在性同样
// 不得泄露。
type InboxJudgmentProjection struct {
	SchemaVersion    string                              `json:"schema_version"`
	Binding          InboxJudgmentBinding                `json:"binding"`
	VaultDigest      string                              `json:"vault_digest"`
	PrincipalDigest  string                              `json:"principal_digest"`
	PermissionDigest string                              `json:"permission_digest"`
	InboxNoteID      string                              `json:"inbox_note_id"`
	InboxRevision    string                              `json:"inbox_revision"`
	InboxDigest      string                              `json:"inbox_digest"`
	InboxText        string                              `json:"inbox_text"`
	Candidates       []InboxJudgmentCandidate            `json:"candidates"`
	Deterministic    []InboxJudgmentDeterministicFinding `json:"deterministic_findings,omitempty"`
	Questions        []InboxJudgmentQuestion             `json:"questions"`
	Limits           InboxJudgmentLimits                 `json:"limits"`
	Digest           string                              `json:"digest"`
}

// ComputeDigest 计算投影 digest（不含 digest 字段自身）。
func (p InboxJudgmentProjection) ComputeDigest() string {
	p.Digest = ""
	return judgmentDigest(p)
}

// InboxJudgmentAuthorization 是领域权限合同的可复核结果。Digest 绑定权限
// 版本：授权集合变化即改变，使缓存判断失效。
type InboxJudgmentAuthorization struct {
	Denied         bool
	Reason         string
	VaultDigest    string // 当前 vault scope digest（跨 vault 隔离）
	AllowedNoteIDs []string
}

// Allows 报告笔记当前是否被授权。
func (a InboxJudgmentAuthorization) Allows(noteID string) bool {
	if a.Denied {
		return false
	}
	return judgmentContainsString(a.AllowedNoteIDs, noteID)
}

// Digest 返回授权的权限版本 digest。
func (a InboxJudgmentAuthorization) Digest() string {
	ids := append([]string(nil), a.AllowedNoteIDs...)
	sort.Strings(ids)
	payload, _ := json.Marshal(struct {
		Denied         bool     `json:"denied"`
		VaultDigest    string   `json:"vault_digest,omitempty"`
		AllowedNoteIDs []string `json:"allowed_note_ids,omitempty"`
		Reason         string   `json:"reason,omitempty"`
	}{a.Denied, a.VaultDigest, ids, a.Reason})
	return judgmentDigestBytes(payload)
}

// InboxJudgmentAuthorizer 是每次判断调用前与每次缓存读取前调用的领域
// 权限 seam。实现必须复用既有领域访问边界，不把授权移进 transport。
type InboxJudgmentAuthorizer interface {
	Authorize(ctx context.Context) (InboxJudgmentAuthorization, error)
}

// StaticJudgmentAuthorizer 是 fixture 与离线测试使用的固定授权；生产路径
// 使用 vault/应用服务 adapter。
type StaticJudgmentAuthorizer struct {
	Authorization InboxJudgmentAuthorization
}

// Authorize 返回静态授权。
func (a StaticJudgmentAuthorizer) Authorize(context.Context) (InboxJudgmentAuthorization, error) {
	return a.Authorization, nil
}

// VaultDigestFor 计算 vault 根路径的 scope digest（原访问边界即 vault
// 本身：本地优先、单 vault 授权）。
func VaultDigestFor(vaultRoot string) string {
	return judgmentDigestBytes([]byte("pinax-vault:" + strings.TrimSpace(vaultRoot)))
}

// VaultJudgmentAuthorizer 把既有 vault 访问边界适配为判断授权 seam：
// vault digest + 调用方（应用服务）解析的已授权笔记集合。
type VaultJudgmentAuthorizer struct {
	VaultRoot      string
	AllowedNoteIDs []string
}

// Authorize 按当前 vault 与授权笔记集合解析授权。
func (a VaultJudgmentAuthorizer) Authorize(context.Context) (InboxJudgmentAuthorization, error) {
	if strings.TrimSpace(a.VaultRoot) == "" {
		return InboxJudgmentAuthorization{}, judgmentInvalidRequest("vault judgment authorizer has no vault root", "Set VaultJudgmentAuthorizer.VaultRoot")
	}
	return InboxJudgmentAuthorization{
		VaultDigest:    VaultDigestFor(a.VaultRoot),
		AllowedNoteIDs: append([]string(nil), a.AllowedNoteIDs...),
	}, nil
}

// InboxJudgmentInput 是判断评估的输入。候选笔记来自既有 vault 应用服务
// （索引/搜索/列表）的输出；此处只做再授权与最小投影，绝不复制业务状态机。
type InboxJudgmentInput struct {
	VaultRoot     string
	Principal     string // 主体标识（owner-local 用户/agent）
	InboxNoteID   string
	InboxRevision string
	InboxText     string // 明确选中的 inbox 文本（服务侧已选定的正文摘要）
	Candidates    []InboxJudgmentCandidate
}

// BuildInboxJudgmentProjection 执行确定性前置规则并构建最小授权投影。
// 它零 transport 调用：这里的每条规则（权限、必需字段、类型、可计算约束）
// 都先于任何模型介入，永不被概率替代。
func BuildInboxJudgmentProjection(ctx context.Context, binding InboxJudgmentBinding, authorizer InboxJudgmentAuthorizer, input InboxJudgmentInput, limits InboxJudgmentLimits) (InboxJudgmentProjection, error) {
	if err := binding.Validate(); err != nil {
		return InboxJudgmentProjection{}, err
	}
	if authorizer == nil {
		return InboxJudgmentProjection{}, judgmentInvalidRequest("judgment projection requires an authorizer", "Reuse the vault access boundary or the application service permission gate")
	}
	authorization, err := authorizer.Authorize(ctx)
	if err != nil {
		return InboxJudgmentProjection{}, err
	}
	if authorization.Denied {
		// Fail closed；错误永不点名候选，未授权存在性同样不泄露。
		return InboxJudgmentProjection{}, &domain.CommandError{Code: "judgment_unauthorized", Message: "principal is not authorized for inbox judgment", Hint: "Use the original inbox review workflow"}
	}
	if strings.TrimSpace(input.VaultRoot) == "" {
		return InboxJudgmentProjection{}, judgmentInvalidRequest("vault root is required", "Judgment only sources candidates from the current vault")
	}
	vaultDigest := VaultDigestFor(input.VaultRoot)
	if authorization.VaultDigest != "" && authorization.VaultDigest != vaultDigest {
		// 跨 vault 默认不取材：授权绑定到另一个 vault 时 fail closed。
		return InboxJudgmentProjection{}, &domain.CommandError{Code: "judgment_unauthorized", Message: "authorization is bound to a different vault", Hint: "Cross-vault sourcing is disabled; authorize within the current vault"}
	}
	if limits.MaxCandidates <= 0 {
		limits.MaxCandidates = InboxJudgmentDefaultMaxCandidates
	}
	if limits.MaxInboxBytes <= 0 {
		limits.MaxInboxBytes = InboxJudgmentDefaultMaxInboxBytes
	}
	if limits.MaxInlineBytes <= 0 {
		limits.MaxInlineBytes = InboxJudgmentDefaultMaxInlineBytes
	}
	if limits.DeadlineMS <= 0 {
		limits.DeadlineMS = InboxJudgmentDefaultDeadlineMS
	}
	questions := InboxJudgmentQuestions()
	candidates := make([]InboxJudgmentCandidate, 0, len(input.Candidates))
	seen := map[string]bool{}
	for _, candidate := range input.Candidates {
		// 每条候选必须位于当前授权集合内；集合外的候选 fail closed 且不回显 ID。
		if len(authorization.AllowedNoteIDs) > 0 && !authorization.Allows(candidate.NoteID) {
			return InboxJudgmentProjection{}, &domain.CommandError{Code: "judgment_unauthorized", Message: "candidate input is outside the authorized vault scope", Hint: "Re-select candidates through the original note service before judgment"}
		}
		if seen[candidate.NoteID] {
			return InboxJudgmentProjection{}, judgmentInvalidRequest("candidate note ids must be unique", "Duplicate candidates cannot align pair answers")
		}
		seen[candidate.NoteID] = true
		inline, truncated := judgmentBoundedText(candidate.InlineText, limits.MaxInlineBytes)
		updated := candidate
		updated.CandidateID = candidate.NoteID
		updated.InlineText = inline
		updated.Truncated = truncated
		candidates = append(candidates, updated)
	}
	inboxText, _ := judgmentBoundedText(input.InboxText, limits.MaxInboxBytes)
	inboxDigest := judgmentDigestBytes([]byte(inboxText))
	// 可计算约束先于模型：内容 digest 全等即确定性重复，无需概率。
	var findings []InboxJudgmentDeterministicFinding
	for _, candidate := range candidates {
		digest := candidate.SourceDigest
		if digest == "" {
			digest = judgmentDigestBytes([]byte(candidate.InlineText))
		}
		if digest != "" && digest == inboxDigest {
			findings = append(findings, InboxJudgmentDeterministicFinding{
				Kind: "exact_content_digest", NoteID: candidate.NoteID, Revision: candidate.SourceRevision,
			})
		}
	}
	projection := InboxJudgmentProjection{
		SchemaVersion:    InboxJudgmentProjectionSchema,
		Binding:          binding,
		VaultDigest:      vaultDigest,
		PrincipalDigest:  judgmentDigestBytes([]byte(strings.TrimSpace(input.Principal))),
		PermissionDigest: authorization.Digest(),
		InboxNoteID:      strings.TrimSpace(input.InboxNoteID),
		InboxRevision:    strings.TrimSpace(input.InboxRevision),
		InboxDigest:      inboxDigest,
		InboxText:        inboxText,
		Candidates:       candidates,
		Deterministic:    findings,
		Questions:        questions,
		Limits:           limits,
	}
	if err := InboxJudgmentPrecheck(projection.InboxText, input.InboxNoteID, projection.InboxRevision, candidates, questions, limits, nil); err != nil {
		return InboxJudgmentProjection{}, err
	}
	projection.Digest = projection.ComputeDigest()
	return projection, nil
}

// InboxJudgmentPrecheck 应用模型前置的确定性规则。导出以便调用方与测试
// 单独运行同一道门。supported 为 nil 时跳过原语能力检查；否则它是本地
// 能力快照（绝不涉及网络）。
func InboxJudgmentPrecheck(inboxText, inboxNoteID, inboxRevision string, candidates []InboxJudgmentCandidate, questions []InboxJudgmentQuestion, limits InboxJudgmentLimits, supported map[string]bool) error {
	if strings.TrimSpace(inboxText) == "" {
		return judgmentInvalidRequest("selected inbox text is required", "Provide the explicitly selected inbox excerpt")
	}
	if len(inboxText) > limits.MaxInboxBytes {
		return judgmentInvalidRequest("inbox text exceeds the byte limit", "Shorten the excerpt or raise MaxInboxBytes explicitly")
	}
	if strings.TrimSpace(inboxNoteID) == "" {
		return judgmentInvalidRequest("inbox note id is required", "Bind the judgment to the inbox note identity")
	}
	if strings.TrimSpace(inboxRevision) == "" {
		return judgmentInvalidRequest("inbox note revision is required", "Bind the judgment to the inbox note revision for staleness checks")
	}
	if len(candidates) == 0 {
		return judgmentInvalidRequest("at least one authorized candidate is required", "Judgment never runs over an empty candidate set")
	}
	if len(candidates) > limits.MaxCandidates {
		return judgmentInvalidRequest("candidate count exceeds the bounded top-N", "Judgment considers only the authorized bounded candidates")
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.CandidateID) == "" {
			return judgmentInvalidRequest("candidate id is required", "Every judgment candidate needs a stable id")
		}
		if seen[candidate.CandidateID] {
			return judgmentInvalidRequest("candidate ids must be unique", "Duplicate candidates cannot align pair answers")
		}
		seen[candidate.CandidateID] = true
		if strings.TrimSpace(candidate.SourceRevision) == "" {
			return judgmentInvalidRequest("candidate source revision is required", "Bind every candidate to a source revision for staleness checks")
		}
		if strings.TrimSpace(candidate.InlineText) == "" {
			return judgmentInvalidRequest("candidate inline text is required", "Provide the bounded redacted excerpt")
		}
		if candidate.CandidateID == inboxNoteID {
			return judgmentInvalidRequest("candidate must differ from the inbox note", "Self-pairs cannot judge duplication or linkage")
		}
		if judgmentCarriesForbiddenPattern(candidate.InlineText) {
			// 错误永不回显文本：疑似敏感内容 fail closed，不转发也不记录。
			return judgmentInvalidRequest("candidate projection carries a forbidden pattern", "Redact the excerpt before judgment")
		}
	}
	if judgmentCarriesForbiddenPattern(inboxText) {
		return judgmentInvalidRequest("inbox projection carries a forbidden pattern", "Redact the excerpt before judgment")
	}
	if len(questions) == 0 {
		return judgmentInvalidRequest("question set is required", "Pin the versioned question set")
	}
	questionIDs := map[string]bool{}
	for _, question := range questions {
		if strings.TrimSpace(question.ID) == "" {
			return judgmentInvalidRequest("question id is required", "Every question needs a stable id")
		}
		if questionIDs[question.ID] {
			return judgmentInvalidRequest("question ids must be unique", "Duplicate questions cannot align pair answers")
		}
		questionIDs[question.ID] = true
		switch question.Primitive {
		case JudgmentPrimitiveBinary:
		case JudgmentPrimitiveChoice:
			if len(question.OptionIDs) == 0 {
				return judgmentInvalidRequest("choice question requires explicit option ids", "Declare the stable answer domain")
			}
		case JudgmentPrimitiveOrdinalScore:
			if len(question.LevelIDs) < 2 {
				return judgmentInvalidRequest("ordinal question requires explicit ordered levels", "Declare the finite level domain low to high")
			}
		default:
			return &JudgmentTransportError{Code: JudgmentCodeUnsupportedCapability, Message: "question uses an unknown primitive", SubmissionState: JudgmentSubmissionNotSubmitted, RetryClass: JudgmentRetryNever}
		}
		if supported != nil && !supported[question.Primitive] {
			return &JudgmentTransportError{Code: JudgmentCodeUnsupportedCapability, Message: "question primitive is not supported by the judgment adapter", SubmissionState: JudgmentSubmissionNotSubmitted, RetryClass: JudgmentRetryNever}
		}
	}
	return nil
}

// judgmentBoundedText 先脱敏再确定性截断到最多 maxBytes（rune 边界）。
// 截断是 owner 在最小化投影；是否截断被记录，evidence 保持诚实。
func judgmentBoundedText(text string, maxBytes int) (string, bool) {
	text = strings.TrimSpace(text)
	if len(text) <= maxBytes {
		return text, false
	}
	truncated := text[:maxBytes]
	for len(truncated) > 0 && !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated, true
}

// judgmentCarriesForbiddenPattern 用既有 internal/redaction 合同检测敏感形态。
func judgmentCarriesForbiddenPattern(text string) bool {
	classes := redaction.ScanSensitiveClasses(text)
	// 判断输入只允许文本语义；authorization/cookie/webhook/provider/
	// secret/private-body 形态一律 fail closed（绝对路径与 .pinax 引用
	// 由投影构造排除，这里对正文残留同样防御）。
	return len(classes) > 0
}

// SummarizeNote 把既有应用服务返回的 domain.Note 投影为有界摘要
// （title + 正文头部）。这是"既有 vault application service 产出 → 判断
// 输入"的唯一适配点；不读取文件、不解析 frontmatter 之外的字段。
func SummarizeNote(note domain.Note, maxInlineBytes int) (string, bool) {
	parts := []string{}
	if title := strings.TrimSpace(note.Title); title != "" {
		parts = append(parts, title)
	}
	if body := strings.TrimSpace(note.Body); body != "" {
		parts = append(parts, body)
	}
	summary := strings.Join(parts, "\n")
	if maxInlineBytes <= 0 {
		maxInlineBytes = InboxJudgmentDefaultMaxInlineBytes
	}
	return judgmentBoundedText(summary, maxInlineBytes)
}

// NoteRevision 把笔记内容投影为 revision 绑定（与 app 层内容 digest 同
// 格式），供 candidate 与 inbox 绑定使用。
func NoteRevision(note domain.Note) string {
	payload := fmt.Sprintf("pinax-note:%s\n%s\n%s", note.ID, note.Title, note.Body)
	return judgmentDigestBytes([]byte(payload))
}
