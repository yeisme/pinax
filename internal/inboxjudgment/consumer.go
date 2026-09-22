package inboxjudgment

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

// inbox judgment consumer：模式（off/shadow/assist，默认 off）、owner 管理
// 的缓存（读取前重授权）、脱敏 evidence、有界链接/重复建议、review handoff
// 与显式采纳门。原确定性流程与人工审阅门保持权威；本文件不合并、不删除
// 笔记、不确认长期记忆、不修改 vault canon、不启动同步或发布。

// 评估结果状态。
const (
	InboxJudgmentStatusOff            = "off"
	InboxJudgmentStatusSuggested      = "suggested"
	InboxJudgmentStatusShadowCompared = "shadow_compared"
	InboxJudgmentStatusUnavailable    = "unavailable"
	InboxJudgmentStatusRateLimited    = "rate_limited"
	InboxJudgmentStatusOutcomeUnknown = "outcome_unknown"
	InboxJudgmentStatusInvalidResp    = "invalid_response"
)

// 建议/evidence/handoff/采纳的 schema 版本。
const (
	InboxJudgmentSuggestionSchema = "pinax.inbox_judgment_suggestion.v1"
	InboxJudgmentEvidenceSchemaV1 = "pinax.inbox_judgment_evidence.v1"
	InboxJudgmentReviewSchemaV1   = "pinax.inbox_judgment_review.v1"
	inboxJudgmentAcceptanceSchema = "pinax.inbox_judgment_acceptance.v1"
)

// InboxJudgmentOptions 配置 consumer。默认模式 off：零发现、零 transport
// 调用、原行为不变。
type InboxJudgmentOptions struct {
	Mode           string
	Model          string // 精确 pin，如 "fixture:local"
	AdapterVersion string
	Transport      JudgmentTransport
	Capabilities   JudgmentCapabilities // 本地快照；仅离线预检
	Cache          *InboxJudgmentCache
	Authorizer     InboxJudgmentAuthorizer
	Limits         InboxJudgmentLimits
	ReviewRefs     []string // 随建议一并返回的既有审阅入口引用
}

// Validate 检查选项形状。启用模式要求注入 transport、authorizer 与精确
// 模型 pin；绝不自行发现 adapter、绝不读取凭据。
func (o InboxJudgmentOptions) Validate() error {
	switch o.Mode {
	case "", InboxJudgmentModeOff:
		return nil
	case InboxJudgmentModeShadow, InboxJudgmentModeAssist:
	default:
		return judgmentInvalidRequest("unknown judgment mode", "Use off, shadow, or assist")
	}
	if o.Transport == nil {
		return judgmentInvalidRequest("enabled judgment mode requires an injected transport", "Configure the explicit judgment transport")
	}
	if o.Authorizer == nil {
		return judgmentInvalidRequest("enabled judgment mode requires a domain authorizer", "Reuse the vault access boundary or the application service permission gate")
	}
	if strings.TrimSpace(o.Model) == "" {
		return judgmentInvalidRequest("enabled judgment mode requires an exact model pin", "Set the exact model identifier")
	}
	if err := o.Capabilities.Validate(); err != nil {
		return judgmentInvalidRequest("enabled judgment mode requires a valid capability snapshot", "Pin the adapter capabilities locally")
	}
	return nil
}

// InboxJudgmentConsumer 对已授权候选评估 inbox 分类、关联与重复线索建议。
type InboxJudgmentConsumer struct {
	options InboxJudgmentOptions
}

// NewInboxJudgmentConsumer 构造 consumer。空/off 模式得到零外部调用的
// consumer。
func NewInboxJudgmentConsumer(options InboxJudgmentOptions) (*InboxJudgmentConsumer, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	return &InboxJudgmentConsumer{options: options}, nil
}

// InboxJudgmentOutcome 是建议性结果。Status 区分 off、建议、shadow 比较、
// 不可用与结果不明；绝不把"没有结果"重新解释为"没有问题"。
type InboxJudgmentOutcome struct {
	Status        string
	Evidence      InboxJudgmentEvidence
	Suggestion    *InboxJudgmentSuggestion
	BaselineOrder []string
	FromCache     bool
	Replay        bool
}

// InboxJudgmentEvidenceItem 是一条脱敏 pair 答案。来源未提供 confidence 时
// 保持 null；不确定性被保留，不被填补。
type InboxJudgmentEvidenceItem struct {
	CandidateID  string                   `json:"candidate_id"`
	QuestionID   string                   `json:"question_id"`
	AnswerStatus string                   `json:"answer_status"`
	Value        *JudgmentWireAnswerValue `json:"value,omitempty"`
	Confidence   *JudgmentWireConfidence  `json:"confidence,omitempty"`
	ReasonCode   string                   `json:"reason_code,omitempty"`
}

// InboxJudgmentEvidence 是持久化的脱敏 evidence 轨迹。它记录版本化绑定、
// 精确模型、attempt、已知用量或 unknown、弃答/错误类别及既有审阅引用。
// 绝不记录 raw prompt、问题文本、inline 摘要、provider payload、secret、
// hidden prompt 或思维链。
type InboxJudgmentEvidence struct {
	SchemaVersion    string                      `json:"schema_version"`
	Mode             string                      `json:"mode"`
	Status           string                      `json:"status"`
	Binding          InboxJudgmentBinding        `json:"binding"`
	VaultDigest      string                      `json:"vault_digest,omitempty"`
	RequestID        string                      `json:"request_id,omitempty"`
	AttemptID        string                      `json:"attempt_id,omitempty"`
	InputDigest      string                      `json:"input_digest,omitempty"`
	ProjectionDigest string                      `json:"projection_digest,omitempty"`
	PermissionDigest string                      `json:"permission_digest,omitempty"`
	ResolvedModel    string                      `json:"resolved_model,omitempty"`
	ExecutionStatus  string                      `json:"execution_status,omitempty"`
	Items            []InboxJudgmentEvidenceItem `json:"items,omitempty"`
	Suggestion       *InboxJudgmentSuggestion    `json:"suggestion,omitempty"`
	UsageKnown       bool                        `json:"usage_known"`
	Usage            *JudgmentWireUsage          `json:"usage,omitempty"`
	LatencyKnown     bool                        `json:"latency_known"`
	LatencyMS        *int64                      `json:"latency_ms,omitempty"`
	ReasonCode       string                      `json:"reason_code,omitempty"`
	SubmissionState  string                      `json:"submission_state,omitempty"`
	RetryClass       string                      `json:"retry_class,omitempty"`
	Summary          string                      `json:"summary"`
	ReviewRefs       []string                    `json:"review_refs,omitempty"`
	BaselineOrder    []string                    `json:"baseline_order,omitempty"`
	Digest           string                      `json:"digest"`
}

// ComputeDigest 计算 evidence digest（不含 digest 字段）。evidence 不携带
// 时间戳，replay 可逐字节复现 digest。
func (e InboxJudgmentEvidence) ComputeDigest() string {
	e.Digest = ""
	return judgmentDigest(e)
}

// Validate 校验持久化 evidence 完整性：schema、digest 与 off 模式不变量。
func (e InboxJudgmentEvidence) Validate() error {
	if e.SchemaVersion != InboxJudgmentEvidenceSchemaV1 {
		return &domain.CommandError{Code: "judgment_invalid_response", Message: "evidence schema version mismatch"}
	}
	if e.Digest == "" || e.Digest != e.ComputeDigest() {
		return &domain.CommandError{Code: "judgment_invalid_response", Message: "evidence digest mismatch"}
	}
	if (e.Mode == "" || e.Mode == InboxJudgmentModeOff) && e.Suggestion != nil {
		return &domain.CommandError{Code: "judgment_invalid_response", Message: "off-mode evidence cannot carry a suggestion"}
	}
	return nil
}

// InboxJudgmentLinkSuggestion 是一条可审阅的关联建议（补充/矛盾关系）。
type InboxJudgmentLinkSuggestion struct {
	NoteID     string `json:"note_id"`
	Relation   string `json:"relation"` // complement | contradiction
	Revision   string `json:"revision"`
	Usefulness string `json:"usefulness"`
}

// InboxJudgmentDuplicateWarning 是一条重复线索（解释关系，绝不合并）。
type InboxJudgmentDuplicateWarning struct {
	NoteID   string `json:"note_id"`
	Revision string `json:"revision"`
	Basis    string `json:"basis"` // model_relation | exact_content_digest
}

// InboxJudgmentMissingPair 是没有可采纳答案的必需 pair。
type InboxJudgmentMissingPair struct {
	CandidateID  string `json:"candidate_id"`
	QuestionID   string `json:"question_id"`
	AnswerStatus string `json:"answer_status"`
}

// InboxJudgmentLanguageCount 报告已作答语言分布（双语校准产物；排序保证
// digest 确定）。
type InboxJudgmentLanguageCount struct {
	Language string `json:"language"`
	Count    int    `json:"count"`
}

// InboxJudgmentSuggestion 是有界的建议性分类/链接/重复线索。它只引用已
// 授权候选、保留基线顺序、列出缺失的必需答案，且仅当全部必需问题作答时
// 才 adoptable。它绝不能替代未检索到的笔记，也绝不执行合并/删除。
type InboxJudgmentSuggestion struct {
	SchemaVersion         string                          `json:"schema_version"`
	Mode                  string                          `json:"mode"`
	BindingDigest         string                          `json:"binding_digest"`
	ProjectionDigest      string                          `json:"projection_digest"`
	InboxNoteID           string                          `json:"inbox_note_id"`
	InboxRevision         string                          `json:"inbox_revision"`
	BaselineOrder         []string                        `json:"baseline_order"`
	Links                 []InboxJudgmentLinkSuggestion   `json:"links,omitempty"`
	Duplicates            []InboxJudgmentDuplicateWarning `json:"duplicates,omitempty"`
	Languages             []InboxJudgmentLanguageCount    `json:"languages,omitempty"`
	Missing               []InboxJudgmentMissingPair      `json:"missing,omitempty"`
	Adoptable             bool                            `json:"adoptable"`
	BoundedCandidatesOnly bool                            `json:"bounded_candidates_only"`
	Reason                string                          `json:"reason"`
	Digest                string                          `json:"digest"`
}

// ComputeDigest 计算建议 digest（不含 digest 字段）。
func (s InboxJudgmentSuggestion) ComputeDigest() string {
	s.Digest = ""
	return judgmentDigest(s)
}

// Validate 校验建议形状与 digest。
func (s InboxJudgmentSuggestion) Validate() error {
	if s.SchemaVersion != InboxJudgmentSuggestionSchema {
		return &domain.CommandError{Code: "judgment_invalid_response", Message: "suggestion schema version mismatch"}
	}
	if s.Digest == "" || s.Digest != s.ComputeDigest() {
		return &domain.CommandError{Code: "judgment_invalid_response", Message: "suggestion digest mismatch"}
	}
	if !s.BoundedCandidatesOnly {
		return &domain.CommandError{Code: "judgment_invalid_response", Message: "suggestion must stay bounded to the authorized candidates"}
	}
	baseline := map[string]bool{}
	for _, id := range s.BaselineOrder {
		if baseline[id] {
			return &domain.CommandError{Code: "judgment_invalid_response", Message: "baseline order must have unique candidates"}
		}
		baseline[id] = true
	}
	for _, link := range s.Links {
		if !baseline[link.NoteID] {
			return &domain.CommandError{Code: "judgment_invalid_response", Message: "link suggestion references a candidate outside the baseline set"}
		}
		if link.Relation != RelationComplement && link.Relation != RelationContradiction {
			return &domain.CommandError{Code: "judgment_invalid_response", Message: "link suggestion relation must be complement or contradiction"}
		}
	}
	for _, duplicate := range s.Duplicates {
		if !baseline[duplicate.NoteID] {
			return &domain.CommandError{Code: "judgment_invalid_response", Message: "duplicate warning references a candidate outside the baseline set"}
		}
		if duplicate.Basis != "model_relation" && duplicate.Basis != "exact_content_digest" {
			return &domain.CommandError{Code: "judgment_invalid_response", Message: "duplicate warning basis is unknown"}
		}
	}
	return nil
}

// InboxJudgmentReviewHandoff 把建议交回原有人工审阅入口。采纳仍留在
// owner 既有权限/审阅门之后。
type InboxJudgmentReviewHandoff struct {
	SchemaVersion         string   `json:"schema_version"`
	SuggestionDigest      string   `json:"suggestion_digest"`
	EvidenceDigest        string   `json:"evidence_digest"`
	Adoptable             bool     `json:"adoptable"`
	NextAction            string   `json:"next_action"` // human_review
	ReviewRefs            []string `json:"review_refs,omitempty"`
	MissingCount          int      `json:"missing_count"`
	BoundedCandidatesOnly bool     `json:"bounded_candidates_only"`
}

// ReviewHandoff 为建议构建指向原 inbox 审阅入口的 handoff。
func (s InboxJudgmentSuggestion) ReviewHandoff(evidence InboxJudgmentEvidence, reviewRefs []string) (InboxJudgmentReviewHandoff, error) {
	if err := s.Validate(); err != nil {
		return InboxJudgmentReviewHandoff{}, err
	}
	if err := evidence.Validate(); err != nil {
		return InboxJudgmentReviewHandoff{}, err
	}
	refs := reviewRefs
	if len(refs) == 0 {
		refs = evidence.ReviewRefs
	}
	return InboxJudgmentReviewHandoff{
		SchemaVersion:         InboxJudgmentReviewSchemaV1,
		SuggestionDigest:      s.Digest,
		EvidenceDigest:        evidence.Digest,
		Adoptable:             s.Adoptable,
		NextAction:            "human_review",
		ReviewRefs:            refs,
		MissingCount:          len(s.Missing),
		BoundedCandidatesOnly: s.BoundedCandidatesOnly,
	}, nil
}

// Projection 把 handoff 渲染进既有 CLI envelope（domain.Projection）：新
// 信息只通过 additive Facts/Actions/Data 呈现，旧 envelope 字段语义与
// canonical 权限不变；Actions 指向原 inbox 审阅命令。
func (h InboxJudgmentReviewHandoff) Projection() (domain.Projection, error) {
	if h.SchemaVersion != InboxJudgmentReviewSchemaV1 {
		return domain.Projection{}, &domain.CommandError{Code: "judgment_invalid_response", Message: "review handoff schema version mismatch"}
	}
	projection := domain.NewProjection("inbox.judgment", "Inbox judgment suggestion recorded; original review flow stays authoritative.")
	projection.Facts["mode"] = "explicit"
	projection.Facts["status"] = "suggested"
	projection.Facts["next_action"] = h.NextAction
	projection.Facts["adoptable"] = fmt.Sprintf("%t", h.Adoptable)
	projection.Facts["missing_count"] = fmt.Sprintf("%d", h.MissingCount)
	projection.Facts["suggestion_digest"] = h.SuggestionDigest
	projection.Facts["evidence_digest"] = h.EvidenceDigest
	projection.Actions = []domain.Action{
		{Name: "Review inbox note", Command: "pinax inbox show <note_ref>"},
		{Name: "Promote inbox note", Command: "pinax inbox promote <note_ref> --yes"},
		{Name: "Discard inbox note", Command: "pinax inbox discard <note_ref> --yes"},
	}
	projection.Data = h
	return projection, nil
}

// InboxJudgmentCacheKey 绑定缓存判断依赖的每个维度：主体、vault、权限
// 版本、inbox/候选 revision、问题集、策略、模型、模式、adapter 与合同
// 版本。任一变化都是不同的 key（shadow evidence 不会 replay 给 assist
// consumer）；权限变化还会使已存条目失效。
type InboxJudgmentCacheKey struct {
	PrincipalDigest         string
	VaultDigest             string
	PermissionDigest        string
	InboxBindingDigest      string
	CandidateBindingsDigest string
	QuestionSetDigest       string
	PolicyDigest            string
	Model                   string
	Mode                    string
	AdapterVersion          string
	ContractVersion         string
}

// Digest 返回缓存 key digest。
func (k InboxJudgmentCacheKey) Digest() string {
	return judgmentDigest(k)
}

// InboxJudgmentCacheKeyFor 为一次投影构建缓存 key。
func InboxJudgmentCacheKeyFor(principalDigest string, projection InboxJudgmentProjection, adapterVersion string) InboxJudgmentCacheKey {
	type inboxBinding struct {
		NoteID   string `json:"note_id"`
		Revision string `json:"revision"`
		Digest   string `json:"digest,omitempty"`
	}
	type candidateBinding struct {
		NoteID   string `json:"note_id"`
		Revision string `json:"revision"`
		Digest   string `json:"digest,omitempty"`
	}
	candidates := make([]candidateBinding, 0, len(projection.Candidates))
	for _, candidate := range projection.Candidates {
		candidates = append(candidates, candidateBinding{NoteID: candidate.NoteID, Revision: candidate.SourceRevision, Digest: candidate.SourceDigest})
	}
	return InboxJudgmentCacheKey{
		PrincipalDigest:         principalDigest,
		VaultDigest:             projection.VaultDigest,
		PermissionDigest:        projection.PermissionDigest,
		InboxBindingDigest:      judgmentDigest(inboxBinding{NoteID: projection.InboxNoteID, Revision: projection.InboxRevision, Digest: projection.InboxDigest}),
		CandidateBindingsDigest: judgmentDigest(candidates),
		QuestionSetDigest:       projection.Binding.QuestionSetDigest,
		PolicyDigest:            projection.Binding.PolicyDigest,
		Model:                   projection.Binding.Model,
		Mode:                    projection.Binding.Mode,
		AdapterVersion:          adapterVersion,
		ContractVersion:         JudgmentWireSchemaVersion,
	}
}

// InboxJudgmentCache 是 owner 管理的 evidence 缓存，可被多个 consumer
// 共享，内部自同步。读取总是先通过领域权限合同重授权：拒绝、权限版本
// 变化或候选离开授权集合都会使条目失效。
type InboxJudgmentCache struct {
	mu      sync.Mutex
	entries map[string]InboxJudgmentEvidence
}

// NewInboxJudgmentCache 构造空缓存。
func NewInboxJudgmentCache() *InboxJudgmentCache {
	return &InboxJudgmentCache{entries: map[string]InboxJudgmentEvidence{}}
}

// Store 在 key 下缓存 evidence。
func (c *InboxJudgmentCache) Store(key InboxJudgmentCacheKey, evidence InboxJudgmentEvidence) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key.Digest()] = evidence
}

// Len 返回缓存条目数。
func (c *InboxJudgmentCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Get 重授权后返回缓存判断。被撤销或收窄的权限报 miss 并删除条目：旧
// 授权绝不能被 replay 进新读取。
func (c *InboxJudgmentCache) Get(ctx context.Context, authorizer InboxJudgmentAuthorizer, key InboxJudgmentCacheKey) (InboxJudgmentEvidence, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	digest := key.Digest()
	evidence, ok := c.entries[digest]
	if !ok {
		return InboxJudgmentEvidence{}, false
	}
	if authorizer == nil {
		delete(c.entries, digest)
		return InboxJudgmentEvidence{}, false
	}
	authorization, err := authorizer.Authorize(ctx)
	if err != nil || authorization.Denied || authorization.Digest() != key.PermissionDigest || (evidence.VaultDigest != "" && authorization.VaultDigest != evidence.VaultDigest) {
		delete(c.entries, digest)
		return InboxJudgmentEvidence{}, false
	}
	for _, id := range evidence.BaselineOrder {
		if !authorization.Allows(id) {
			delete(c.entries, digest)
			return InboxJudgmentEvidence{}, false
		}
	}
	if evidence.Suggestion != nil && !authorization.Allows(evidence.Suggestion.InboxNoteID) {
		delete(c.entries, digest)
		return InboxJudgmentEvidence{}, false
	}
	return evidence, true
}

// Evaluate 执行一次显式判断 attempt。off 模式立即返回、零 transport 调用。
// 失败保留原流程并给出显式状态；结果不明绝不自动重发。
func (c *InboxJudgmentConsumer) Evaluate(ctx context.Context, input InboxJudgmentInput) (InboxJudgmentOutcome, error) {
	mode := c.options.Mode
	if mode == "" {
		mode = InboxJudgmentModeOff
	}
	baselineOrder := make([]string, 0, len(input.Candidates))
	for _, candidate := range input.Candidates {
		baselineOrder = append(baselineOrder, candidate.NoteID)
	}
	if mode == InboxJudgmentModeOff {
		evidence := InboxJudgmentEvidence{
			SchemaVersion: InboxJudgmentEvidenceSchemaV1,
			Mode:          InboxJudgmentModeOff,
			Status:        InboxJudgmentStatusOff,
			Summary:       "judgment disabled; original inbox review flow preserved; zero external calls",
			BaselineOrder: baselineOrder,
		}
		evidence.Digest = evidence.ComputeDigest()
		return InboxJudgmentOutcome{
			Status:        InboxJudgmentStatusOff,
			Evidence:      evidence,
			BaselineOrder: baselineOrder,
		}, nil
	}
	binding := InboxJudgmentBinding{
		Mode:               mode,
		Model:              c.options.Model,
		AdapterVersion:     c.options.AdapterVersion,
		QuestionSetID:      InboxJudgmentQuestionSetID,
		QuestionSetVersion: InboxJudgmentQuestionSetVersion,
		QuestionSetDigest:  InboxJudgmentQuestionSetDigest(),
		PolicyID:           InboxJudgmentPolicyID,
		PolicyVersion:      InboxJudgmentPolicyVersion,
		PolicyDigest:       InboxJudgmentPolicyDigest(),
	}
	limits := c.options.Limits
	projection, err := BuildInboxJudgmentProjection(ctx, binding, c.options.Authorizer, input, limits)
	if err != nil {
		return InboxJudgmentOutcome{}, err
	}
	cacheKey := InboxJudgmentCacheKeyFor(projection.PrincipalDigest, projection, c.options.AdapterVersion)
	if c.options.Cache != nil {
		if evidence, ok := c.options.Cache.Get(ctx, c.options.Authorizer, cacheKey); ok {
			// 已保存 evidence 的零网络 replay：不新建 attempt、不触 provider。
			return InboxJudgmentOutcome{
				Status:        evidence.Status,
				Evidence:      evidence,
				Suggestion:    evidence.Suggestion,
				BaselineOrder: evidence.BaselineOrder,
				FromCache:     true,
			}, nil
		}
	}
	request, err := buildInboxJudgmentWireRequest(binding, projection)
	if err != nil {
		return InboxJudgmentOutcome{}, err
	}
	client := &JudgmentClient{Transport: c.options.Transport, Capabilities: c.options.Capabilities}
	if limits.DeadlineMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(limits.DeadlineMS)*time.Millisecond)
		defer cancel()
	}
	result, err := client.Evaluate(ctx, request)
	if err != nil {
		transportError, ok := asJudgmentTransportError(err)
		if !ok {
			return InboxJudgmentOutcome{}, err
		}
		status := InboxJudgmentStatusInvalidResp
		switch transportError.Code {
		case JudgmentCodeUnavailable:
			status = InboxJudgmentStatusUnavailable
		case JudgmentCodeRateLimited:
			status = InboxJudgmentStatusRateLimited
		case JudgmentCodeOutcomeUnknown, JudgmentCodeDeadlineExceeded:
			status = InboxJudgmentStatusOutcomeUnknown
		}
		evidence := InboxJudgmentEvidence{
			SchemaVersion:    InboxJudgmentEvidenceSchemaV1,
			Mode:             mode,
			Status:           status,
			Binding:          binding,
			VaultDigest:      projection.VaultDigest,
			RequestID:        request.RequestID,
			AttemptID:        request.AttemptID,
			ProjectionDigest: projection.Digest,
			PermissionDigest: projection.PermissionDigest,
			ReasonCode:       transportError.Code,
			SubmissionState:  transportError.SubmissionState,
			RetryClass:       transportError.RetryClass,
			Summary:          "judgment transport failed; original inbox review flow preserved; not resubmitted automatically",
			ReviewRefs:       c.options.ReviewRefs,
			BaselineOrder:    baselineOrder,
		}
		evidence.Digest = evidence.ComputeDigest()
		return InboxJudgmentOutcome{Status: status, Evidence: evidence, BaselineOrder: baselineOrder}, nil
	}
	suggestion, evidenceItems := buildInboxJudgmentSuggestion(mode, binding, projection, result)
	status := InboxJudgmentStatusSuggested
	if mode == InboxJudgmentModeShadow {
		status = InboxJudgmentStatusShadowCompared
	}
	evidence := InboxJudgmentEvidence{
		SchemaVersion:    InboxJudgmentEvidenceSchemaV1,
		Mode:             mode,
		Status:           status,
		Binding:          binding,
		VaultDigest:      projection.VaultDigest,
		RequestID:        request.RequestID,
		AttemptID:        request.AttemptID,
		InputDigest:      result.InputDigest,
		ProjectionDigest: projection.Digest,
		PermissionDigest: projection.PermissionDigest,
		ResolvedModel:    result.ResolvedModel.Provider + ":" + result.ResolvedModel.Model,
		ExecutionStatus:  result.ExecutionStatus,
		Items:            evidenceItems,
		Suggestion:       &suggestion,
		UsageKnown:       result.Usage != nil,
		Usage:            result.Usage,
		LatencyKnown:     result.LatencyMS != nil,
		LatencyMS:        result.LatencyMS,
		ReasonCode:       result.ReasonCode,
		Summary:          "advisory inbox judgment recorded; original review flow and canonical state stay authoritative",
		ReviewRefs:       c.options.ReviewRefs,
		BaselineOrder:    suggestion.BaselineOrder,
	}
	evidence.Digest = evidence.ComputeDigest()
	if c.options.Cache != nil {
		c.options.Cache.Store(cacheKey, evidence)
	}
	return InboxJudgmentOutcome{
		Status:        status,
		Evidence:      evidence,
		Suggestion:    &suggestion,
		BaselineOrder: suggestion.BaselineOrder,
	}, nil
}

// ReplayInboxJudgmentEvidence 零网络重放已保存的判断 evidence：本函数不
// 持有 transport、绝不重新评估。新的 live 评估必须显式新建 attempt。
func ReplayInboxJudgmentEvidence(evidence InboxJudgmentEvidence) (InboxJudgmentOutcome, error) {
	if err := evidence.Validate(); err != nil {
		return InboxJudgmentOutcome{}, err
	}
	return InboxJudgmentOutcome{
		Status:        evidence.Status,
		Evidence:      evidence,
		Suggestion:    evidence.Suggestion,
		BaselineOrder: evidence.BaselineOrder,
		Replay:        true,
	}, nil
}

// InboxJudgmentAcceptance 是显式、重授权后的采纳结果。它只枚举应通过原
// 有入口执行的动作，自身不执行任何业务写入。
type InboxJudgmentAcceptance struct {
	SchemaVersion    string                        `json:"schema_version"`
	SuggestionDigest string                        `json:"suggestion_digest"`
	EvidenceDigest   string                        `json:"evidence_digest"`
	InboxNoteID      string                        `json:"inbox_note_id"`
	AcceptedLinks    []InboxJudgmentLinkSuggestion `json:"accepted_links,omitempty"`
	NextActions      []string                      `json:"next_actions"`
}

// AcceptInboxJudgmentSuggestion 执行显式采纳门。它重验 digest、复核没有
// 必需 pair 缺答、核对 source revision 未变（过期建议不可采纳）、并按当前
// 领域权限合同重授权主体。shadow 建议绝不可采纳；本函数不执行任何隐式
// 业务动作——所有后续操作通过原有命令入口显式进行。
func AcceptInboxJudgmentSuggestion(ctx context.Context, suggestion InboxJudgmentSuggestion, evidence InboxJudgmentEvidence, authorizer InboxJudgmentAuthorizer, currentRevisions map[string]string, projectionCandidates []InboxJudgmentCandidate) (InboxJudgmentAcceptance, error) {
	notAdoptable := func(message, hint string) error {
		return &domain.CommandError{Code: "judgment_not_adoptable", Message: message, Hint: hint}
	}
	if err := suggestion.Validate(); err != nil {
		return InboxJudgmentAcceptance{}, err
	}
	if err := evidence.Validate(); err != nil {
		return InboxJudgmentAcceptance{}, err
	}
	if evidence.Suggestion == nil || evidence.Suggestion.Digest != suggestion.Digest {
		return InboxJudgmentAcceptance{}, notAdoptable("suggestion is not bound to the evidence", "Re-evaluate explicitly")
	}
	if suggestion.Mode == InboxJudgmentModeShadow {
		return InboxJudgmentAcceptance{}, notAdoptable("shadow suggestions are comparison-only", "Switch to assist mode and re-evaluate explicitly")
	}
	if !suggestion.Adoptable || len(suggestion.Missing) > 0 {
		return InboxJudgmentAcceptance{}, notAdoptable("required answers are missing, abstained, or errored", "Review the missing items and re-evaluate explicitly")
	}
	if evidence.ExecutionStatus == "unknown" || evidence.Status == InboxJudgmentStatusOutcomeUnknown {
		return InboxJudgmentAcceptance{}, notAdoptable("execution outcome is unknown", "Resolve the attempt outcome before adoption")
	}
	// 过期检查：每条绑定的 source revision 必须仍是当前值（含 inbox 笔记）。
	revisions := map[string]string{}
	for _, candidate := range projectionCandidates {
		revisions[candidate.CandidateID] = candidate.SourceRevision
	}
	revisions[suggestion.InboxNoteID] = suggestion.InboxRevision
	stale := func(id string) error {
		bound, ok := revisions[id]
		if !ok {
			return notAdoptable("suggestion references an unbound candidate", "Re-evaluate explicitly")
		}
		current, ok := currentRevisions[id]
		if !ok {
			// revision 未知 ≠ 未变化：缺当前 revision 一律 fail closed，
			// 不把“查询不到”静默解释成“未变化”。
			return &domain.CommandError{Code: "judgment_stale_source", Message: "current revision is unknown for a bound source", Hint: "Resolve the note's current revision and re-run acceptance; unknown never counts as unchanged"}
		}
		if current != bound {
			return &domain.CommandError{Code: "judgment_stale_source", Message: "source revision changed after evaluation", Hint: "Historical evidence stays read-only; re-evaluate explicitly for the new version"}
		}
		return nil
	}
	if err := stale(suggestion.InboxNoteID); err != nil {
		return InboxJudgmentAcceptance{}, err
	}
	referenced := map[string]bool{}
	for _, link := range suggestion.Links {
		referenced[link.NoteID] = true
	}
	for _, duplicate := range suggestion.Duplicates {
		referenced[duplicate.NoteID] = true
	}
	for id := range referenced {
		if err := stale(id); err != nil {
			return InboxJudgmentAcceptance{}, err
		}
	}
	if authorizer != nil {
		authorization, err := authorizer.Authorize(ctx)
		if err != nil {
			return InboxJudgmentAcceptance{}, err
		}
		if authorization.Denied {
			// 错误永不点名候选：被撤销的存在性同样不泄露。
			return InboxJudgmentAcceptance{}, &domain.CommandError{Code: "judgment_unauthorized", Message: "principal is no longer authorized for this suggestion", Hint: "Use the original inbox review workflow"}
		}
		for id := range referenced {
			if !authorization.Allows(id) {
				return InboxJudgmentAcceptance{}, &domain.CommandError{Code: "judgment_unauthorized", Message: "suggestion is no longer fully authorized", Hint: "Use the original inbox review workflow"}
			}
		}
		if !authorization.Allows(suggestion.InboxNoteID) {
			return InboxJudgmentAcceptance{}, &domain.CommandError{Code: "judgment_unauthorized", Message: "inbox note is no longer authorized", Hint: "Use the original inbox review workflow"}
		}
		if evidence.PermissionDigest != "" && authorization.Digest() != evidence.PermissionDigest {
			return InboxJudgmentAcceptance{}, notAdoptable("permission version changed after evaluation", "Re-evaluate explicitly")
		}
		if evidence.VaultDigest != "" && authorization.VaultDigest != evidence.VaultDigest {
			return InboxJudgmentAcceptance{}, notAdoptable("vault scope changed after evaluation", "Re-evaluate explicitly within the current vault")
		}
	}
	return InboxJudgmentAcceptance{
		SchemaVersion:    inboxJudgmentAcceptanceSchema,
		SuggestionDigest: suggestion.Digest,
		EvidenceDigest:   evidence.Digest,
		InboxNoteID:      suggestion.InboxNoteID,
		AcceptedLinks:    append([]InboxJudgmentLinkSuggestion(nil), suggestion.Links...),
		NextActions: []string{
			"pinax inbox show " + suggestion.InboxNoteID,
			"pinax inbox promote " + suggestion.InboxNoteID + " --yes",
		},
	}, nil
}

// buildInboxJudgmentWireRequest 把授权投影映射到冻结 wire 合同。inbox 笔记
// 作为上下文 source 进入；每个问题显式绑定全部候选笔记。
func buildInboxJudgmentWireRequest(binding InboxJudgmentBinding, projection InboxJudgmentProjection) (JudgmentWireRequest, error) {
	request := JudgmentWireRequest{
		SchemaVersion: JudgmentWireSchemaVersion,
		RequestID:     JudgmentNewID("judgment-req"),
		AttemptID:     JudgmentNewID("judgment-attempt"),
		Scope: JudgmentWireScope{
			Owner:     "pinax",
			Project:   "inbox-judgment",
			Principal: projection.PrincipalDigest,
		},
		Model:       JudgmentWireModel{Provider: "judgment", Model: binding.Model, Version: binding.AdapterVersion},
		QuestionSet: JudgmentWireVersionedRef{ID: binding.QuestionSetID, Version: binding.QuestionSetVersion, Digest: binding.QuestionSetDigest},
		PolicyRef:   JudgmentWireVersionedRef{ID: binding.PolicyID, Version: binding.PolicyVersion, Digest: binding.PolicyDigest},
		Limits: JudgmentWireLimits{
			DeadlineMS:    projection.Limits.DeadlineMS,
			MaxCandidates: projection.Limits.MaxCandidates,
			MaxQuestions:  len(projection.Questions),
			// 输入上界覆盖完整规范 wire 请求（绑定、提示、答案域），
			// 不只是文本字节。
			MaxInputBytes:  64 * 1024,
			MaxOutputBytes: 64 * 1024,
		},
	}
	// 上下文 source：明确选中的 inbox 文本（有界、脱敏）。
	request.Sources = append(request.Sources, JudgmentWireSource{
		SourceID:   "inbox:" + projection.InboxNoteID,
		Revision:   projection.InboxRevision,
		Digest:     projection.InboxDigest,
		InlineText: projection.InboxText,
	})
	candidateIDs := make([]string, 0, len(projection.Candidates))
	for _, candidate := range projection.Candidates {
		request.Sources = append(request.Sources, JudgmentWireSource{
			SourceID:   "note:" + candidate.NoteID,
			Revision:   candidate.SourceRevision,
			Digest:     candidate.SourceDigest,
			InlineText: candidate.InlineText,
			Language:   candidate.Language,
		})
		request.Candidates = append(request.Candidates, JudgmentWireCandidate{CandidateID: candidate.CandidateID, SourceID: "note:" + candidate.NoteID})
		candidateIDs = append(candidateIDs, candidate.CandidateID)
	}
	for _, question := range projection.Questions {
		wire := JudgmentWireQuestion{
			QuestionID:   question.ID,
			Primitive:    question.Primitive,
			Text:         question.Text,
			CandidateIDs: candidateIDs, // 每个问题都是 candidate 作用域
			Required:     question.Required,
			OptionIDs:    question.OptionIDs,
		}
		for index, level := range question.LevelIDs {
			numeric := float64(index)
			wire.Levels = append(wire.Levels, JudgmentWireOrdinalLevel{LevelID: level, NumericValue: numeric})
		}
		request.Questions = append(request.Questions, wire)
	}
	if err := request.Validate(); err != nil {
		return JudgmentWireRequest{}, err
	}
	return request, nil
}

// buildInboxJudgmentSuggestion 从已校验答案派生建议性链接与重复线索。
// 任一必需问题未作答的候选保持其基线相对顺序且不可采纳（不确定性绝不
// 提升排名）；确定性 digest 全等结论无论模型是否作答都会呈现。
func buildInboxJudgmentSuggestion(mode string, binding InboxJudgmentBinding, projection InboxJudgmentProjection, result JudgmentWireResult) (InboxJudgmentSuggestion, []InboxJudgmentEvidenceItem) {
	questions := map[string]InboxJudgmentQuestion{}
	for _, question := range projection.Questions {
		questions[question.ID] = question
	}
	answers := map[string]map[string]JudgmentWireItem{}
	evidenceItems := make([]InboxJudgmentEvidenceItem, 0, len(result.Items))
	for _, item := range result.Items {
		if answers[item.CandidateID] == nil {
			answers[item.CandidateID] = map[string]JudgmentWireItem{}
		}
		answers[item.CandidateID][item.QuestionID] = item
		evidenceItems = append(evidenceItems, InboxJudgmentEvidenceItem{
			CandidateID:  item.CandidateID,
			QuestionID:   item.QuestionID,
			AnswerStatus: item.AnswerStatus,
			Value:        item.Value,
			Confidence:   item.Confidence,
			ReasonCode:   item.ReasonCode,
		})
	}
	baseline := make([]string, 0, len(projection.Candidates))
	for _, candidate := range projection.Candidates {
		baseline = append(baseline, candidate.CandidateID)
	}
	var links []InboxJudgmentLinkSuggestion
	var missing []InboxJudgmentMissingPair
	languageCounts := map[string]int{}
	duplicatesByNote := map[string]InboxJudgmentDuplicateWarning{}
	// 确定性 digest 全等：先于模型、无需概率。
	for _, finding := range projection.Deterministic {
		duplicatesByNote[finding.NoteID] = InboxJudgmentDuplicateWarning{
			NoteID: finding.NoteID, Revision: finding.Revision, Basis: "exact_content_digest",
		}
	}
	for _, candidate := range projection.Candidates {
		// 完整性由 missing 聚合表达；排序键使用逐问答案。
		for _, question := range projection.Questions {
			if !question.Required {
				continue
			}
			item, ok := answers[candidate.CandidateID][question.ID]
			if !ok || item.AnswerStatus != "answered" {
				if ok {
					missing = append(missing, InboxJudgmentMissingPair{CandidateID: candidate.CandidateID, QuestionID: question.ID, AnswerStatus: item.AnswerStatus})
				}
			}
		}
		if relation, ok := answers[candidate.CandidateID]["relation"]; ok && relation.AnswerStatus == "answered" && relation.Value != nil {
			switch relation.Value.Choice {
			case RelationDuplicate:
				if _, exists := duplicatesByNote[candidate.NoteID]; !exists {
					duplicatesByNote[candidate.NoteID] = InboxJudgmentDuplicateWarning{
						NoteID: candidate.NoteID, Revision: candidate.SourceRevision, Basis: "model_relation",
					}
				}
			case RelationComplement, RelationContradiction:
				usefulness := ""
				if item, ok := answers[candidate.CandidateID]["link_usefulness"]; ok && item.AnswerStatus == "answered" && item.Value != nil {
					usefulness = item.Value.OrdinalLevel
				}
				links = append(links, InboxJudgmentLinkSuggestion{
					NoteID: candidate.NoteID, Relation: relation.Value.Choice, Revision: candidate.SourceRevision, Usefulness: usefulness,
				})
			}
		}
		if item, ok := answers[candidate.CandidateID]["language"]; ok && item.AnswerStatus == "answered" && item.Value != nil && item.Value.Choice != "" {
			languageCounts[item.Value.Choice]++
		}
	}
	// 链接排序：clearly_useful > maybe_useful > 未答，同级保持基线顺序
	// （不确定性绝不提升排名）。
	rank := map[string]int{"clearly_useful": 0, "maybe_useful": 1}
	baselineIndex := map[string]int{}
	for index, id := range baseline {
		baselineIndex[id] = index
	}
	sort.SliceStable(links, func(i, j int) bool {
		ri, rj := 2, 2
		if value, ok := rank[links[i].Usefulness]; ok {
			ri = value
		}
		if value, ok := rank[links[j].Usefulness]; ok {
			rj = value
		}
		if ri != rj {
			return ri < rj
		}
		return baselineIndex[links[i].NoteID] < baselineIndex[links[j].NoteID]
	})
	var duplicates []InboxJudgmentDuplicateWarning
	for _, warning := range duplicatesByNote {
		duplicates = append(duplicates, warning)
	}
	sort.Slice(duplicates, func(i, j int) bool { return duplicates[i].NoteID < duplicates[j].NoteID })
	sort.Slice(missing, func(i, j int) bool {
		if missing[i].CandidateID != missing[j].CandidateID {
			return missing[i].CandidateID < missing[j].CandidateID
		}
		return missing[i].QuestionID < missing[j].QuestionID
	})
	languages := make([]InboxJudgmentLanguageCount, 0, len(languageCounts))
	for language, count := range languageCounts {
		languages = append(languages, InboxJudgmentLanguageCount{Language: language, Count: count})
	}
	sort.Slice(languages, func(i, j int) bool { return languages[i].Language < languages[j].Language })
	reason := "bounded advisory link and duplicate hints over authorized current-vault candidates"
	if mode == InboxJudgmentModeShadow {
		reason = "shadow comparison only; adoption requires assist mode and human review"
	}
	suggestion := InboxJudgmentSuggestion{
		SchemaVersion:         InboxJudgmentSuggestionSchema,
		Mode:                  mode,
		BindingDigest:         InboxJudgmentBindingDigest(binding),
		ProjectionDigest:      projection.Digest,
		InboxNoteID:           projection.InboxNoteID,
		InboxRevision:         projection.InboxRevision,
		BaselineOrder:         baseline,
		Links:                 links,
		Duplicates:            duplicates,
		Languages:             languages,
		Missing:               missing,
		Adoptable:             mode == InboxJudgmentModeAssist && len(missing) == 0 && (result.ExecutionStatus == "succeeded" || result.ExecutionStatus == "partial"),
		BoundedCandidatesOnly: true,
		Reason:                reason,
	}
	suggestion.Digest = suggestion.ComputeDigest()
	return suggestion, evidenceItems
}
