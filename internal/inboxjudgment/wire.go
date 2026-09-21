package inboxjudgment

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

// 结构化判断 wire 合同 seam（schema_version "1.0"）。
//
// 公共 SDK（github.com/yeisme/judgment-sdk，Aigora owner）尚未发布；本文件
// 以零外部依赖钉住冻结合同形状：DescribeCapabilities/Evaluate、snake_case
// wire、稳定错误码集合与 choice/ordinal_score/binary 原语。SDK 发布后由
// build-tag 隔离的 bridge（judgment_sdk_bridge.go）替换为真实引用，不改动
// 领域投影、evidence 与 review handoff。本 seam 不读取密钥；credentialctl
// grant 由显式 transport 指向的 adapter 持有。

// JudgmentWireSchemaVersion 是冻结合同版本。
const JudgmentWireSchemaVersion = "1.0"

// 答案原语（合同 v1）。
const (
	JudgmentPrimitiveBinary       = "binary"
	JudgmentPrimitiveOrdinalScore = "ordinal_score"
	JudgmentPrimitiveChoice       = "choice"
)

// 稳定错误码（合同 v1，共 8 个）。
const (
	JudgmentCodeUnsupportedCapability = "unsupported_capability"
	JudgmentCodeInvalidRequest        = "invalid_request"
	JudgmentCodeUnauthorized          = "unauthorized"
	JudgmentCodeRateLimited           = "rate_limited"
	JudgmentCodeUnavailable           = "unavailable"
	JudgmentCodeDeadlineExceeded      = "deadline_exceeded"
	JudgmentCodeInvalidResponse       = "invalid_response"
	JudgmentCodeOutcomeUnknown        = "outcome_unknown"
)

// submission state 与 retry class（transport error 携带）。
const (
	JudgmentSubmissionNotSubmitted = "not_submitted"
	JudgmentSubmissionSubmitted    = "submitted"
	JudgmentSubmissionUnknown      = "unknown"

	JudgmentRetrySafeBeforeSubmit = "safe_before_submit"
	JudgmentRetryReconcileFirst   = "reconcile_first"
	JudgmentRetryNever            = "never"
)

// JudgmentTransportError 是结构化 transport 错误。SDK 侧只给出建议分类、
// 从不自行重试；提交后 outcome_unknown 不得自动重发或切换付费模型。
type JudgmentTransportError struct {
	Code            string
	Message         string
	SubmissionState string
	RetryClass      string
	DiagnosticRef   string
}

func (e *JudgmentTransportError) Error() string {
	code := e.Code
	if code == "" {
		code = JudgmentCodeUnavailable
	}
	if strings.TrimSpace(e.Message) == "" {
		return "judgment transport error: " + code
	}
	return "judgment transport error: " + code + ": " + e.Message
}

// IsOutcomeUnknown 报告错误是否表示执行结果不明（绝不静默当作"没有问题"）。
func (e *JudgmentTransportError) IsOutcomeUnknown() bool {
	return e.Code == JudgmentCodeOutcomeUnknown ||
		e.Code == JudgmentCodeDeadlineExceeded && e.SubmissionState != JudgmentSubmissionNotSubmitted
}

// JudgmentErrorClass 返回稳定错误码的默认 submission state 与 retry class。
// 提交后的 503/unavailable 不能假定为未执行，因此 submission state 为 unknown。
func JudgmentErrorClass(code string) (submissionState, retryClass string) {
	switch code {
	case JudgmentCodeUnsupportedCapability, JudgmentCodeInvalidRequest, JudgmentCodeUnauthorized:
		return JudgmentSubmissionNotSubmitted, JudgmentRetryNever
	case JudgmentCodeRateLimited:
		return JudgmentSubmissionNotSubmitted, JudgmentRetrySafeBeforeSubmit
	case JudgmentCodeUnavailable, JudgmentCodeDeadlineExceeded, JudgmentCodeOutcomeUnknown:
		return JudgmentSubmissionUnknown, JudgmentRetryReconcileFirst
	case JudgmentCodeInvalidResponse:
		return JudgmentSubmissionSubmitted, JudgmentRetryNever
	default:
		return JudgmentSubmissionUnknown, JudgmentRetryReconcileFirst
	}
}

// JudgmentCapabilities 是 DescribeCapabilities 结果的本地快照：合同版本、
// adapter 身份、精确模型、模态、原语、批量与文本上限、语言说明、
// probability/confidence 可用性（显式 provenance）与 reconcile/cancel 支持。
type JudgmentCapabilities struct {
	SchemaVersion         string   `json:"schema_version"`
	ContractVersion       string   `json:"contract_version"`
	Adapter               string   `json:"adapter"`
	AdapterVersion        string   `json:"adapter_version"`
	Model                 string   `json:"model"`
	Modalities            []string `json:"modalities"`
	Primitives            []string `json:"primitives"`
	MaxBatchCandidates    int      `json:"max_batch_candidates"`
	MaxBatchQuestions     int      `json:"max_batch_questions"`
	MaxInputBytes         int      `json:"max_input_bytes"`
	MaxOutputBytes        int      `json:"max_output_bytes"`
	LanguageNote          string   `json:"language_note,omitempty"`
	ProbabilityAvailable  bool     `json:"probability_available"`
	ProbabilityProvenance string   `json:"probability_provenance,omitempty"`
	ConfidenceAvailable   bool     `json:"confidence_available"`
	ConfidenceProvenance  string   `json:"confidence_provenance,omitempty"`
	ReconcileSupported    bool     `json:"reconcile_supported"`
	CancelSupported       bool     `json:"cancel_supported"`
}

// SupportsPrimitive 报告原语是否被声明。
func (c JudgmentCapabilities) SupportsPrimitive(primitive string) bool {
	return judgmentContainsString(c.Primitives, primitive)
}

// SupportedPrimitives 返回离线预检用的原语支持表（不涉及网络）。
func (c JudgmentCapabilities) SupportedPrimitives() map[string]bool {
	supported := map[string]bool{}
	for _, primitive := range c.Primitives {
		supported[primitive] = true
	}
	return supported
}

// Validate 检查能力快照形状。
func (c JudgmentCapabilities) Validate() error {
	if c.SchemaVersion != JudgmentWireSchemaVersion {
		return &JudgmentTransportError{Code: JudgmentCodeInvalidResponse, Message: "capabilities schema version mismatch"}
	}
	if strings.TrimSpace(c.Model) == "" || strings.TrimSpace(c.Adapter) == "" {
		return &JudgmentTransportError{Code: JudgmentCodeInvalidResponse, Message: "capabilities must pin adapter and exact model"}
	}
	if !judgmentContainsString(c.Modalities, "text") {
		return &JudgmentTransportError{Code: JudgmentCodeUnsupportedCapability, Message: "text modality is required for inbox judgment"}
	}
	return nil
}

// JudgmentWireScope 是 owner/project/principal 范围。服务端以可信认证上下文
// 校验；客户端自报永不授予权限。
type JudgmentWireScope struct {
	Owner     string `json:"owner"`
	Project   string `json:"project"`
	Principal string `json:"principal"`
}

// JudgmentWireModel 钉住精确模型身份。
type JudgmentWireModel struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Version  string `json:"version,omitempty"`
}

// JudgmentWireVersionedRef 是版本化 id/version/digest 三元组。
type JudgmentWireVersionedRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

// JudgmentWireSource 是一个已授权、大小有界的 inline source。本地路径或
// URL 只是 owner 侧 provenance；adapter 永不抓取。
type JudgmentWireSource struct {
	SourceID   string `json:"source_id"`
	Revision   string `json:"revision"`
	Digest     string `json:"digest,omitempty"`
	InlineText string `json:"inline_text"`
	Language   string `json:"language,omitempty"`
}

// JudgmentWireCandidate 把 candidate 绑定到其精确 source。
type JudgmentWireCandidate struct {
	CandidateID string `json:"candidate_id"`
	SourceID    string `json:"source_id"`
}

// JudgmentWireOrdinalLevel 是一个显式 ordinal level。
type JudgmentWireOrdinalLevel struct {
	LevelID      string  `json:"level_id"`
	NumericValue float64 `json:"numeric_value"`
}

// JudgmentWireQuestion 是一个原子问题，携带显式 candidate 绑定。答案按
// (candidate_id, question_id) pair 对齐，绝不按数组顺序猜测。
type JudgmentWireQuestion struct {
	QuestionID   string                     `json:"question_id"`
	Primitive    string                     `json:"primitive"`
	Text         string                     `json:"text"`
	CandidateIDs []string                   `json:"candidate_ids"`
	Required     bool                       `json:"required"`
	OptionIDs    []string                   `json:"option_ids,omitempty"`
	Levels       []JudgmentWireOrdinalLevel `json:"levels,omitempty"`
}

// JudgmentWireLimits 约束一次显式请求。
type JudgmentWireLimits struct {
	DeadlineMS     int64 `json:"deadline_ms,omitempty"`
	MaxCandidates  int   `json:"max_candidates,omitempty"`
	MaxQuestions   int   `json:"max_questions,omitempty"`
	MaxInputBytes  int   `json:"max_input_bytes,omitempty"`
	MaxOutputBytes int   `json:"max_output_bytes,omitempty"`
}

// JudgmentWireExtension 声明一个命名空间扩展。未知可选扩展永不提权；
// 未知必需扩展被拒绝。
type JudgmentWireExtension struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

// JudgmentWireRequest 是一次显式评估请求。
type JudgmentWireRequest struct {
	SchemaVersion string                   `json:"schema_version"`
	RequestID     string                   `json:"request_id"`
	AttemptID     string                   `json:"attempt_id"`
	Scope         JudgmentWireScope        `json:"scope"`
	Model         JudgmentWireModel        `json:"model"`
	QuestionSet   JudgmentWireVersionedRef `json:"question_set"`
	PolicyRef     JudgmentWireVersionedRef `json:"policy_ref"`
	Sources       []JudgmentWireSource     `json:"sources"`
	Candidates    []JudgmentWireCandidate  `json:"candidates"`
	Questions     []JudgmentWireQuestion   `json:"questions"`
	Limits        JudgmentWireLimits       `json:"limits"`
	Extensions    []JudgmentWireExtension  `json:"extensions,omitempty"`
}

// CanonicalDigest 确定性地 digest 请求。digest shadow struct 不含 map，
// 编码顺序稳定（本 seam 的显式 RFC-8785 等价规则；NaN/Infinity 上游已拒绝）。
func (r JudgmentWireRequest) CanonicalDigest() string {
	type digestShadow struct {
		SchemaVersion string                   `json:"schema_version"`
		RequestID     string                   `json:"request_id"`
		AttemptID     string                   `json:"attempt_id"`
		Scope         JudgmentWireScope        `json:"scope"`
		Model         JudgmentWireModel        `json:"model"`
		QuestionSet   JudgmentWireVersionedRef `json:"question_set"`
		PolicyRef     JudgmentWireVersionedRef `json:"policy_ref"`
		Sources       []JudgmentWireSource     `json:"sources"`
		Candidates    []JudgmentWireCandidate  `json:"candidates"`
		Questions     []JudgmentWireQuestion   `json:"questions"`
		Limits        JudgmentWireLimits       `json:"limits"`
		Extensions    []JudgmentWireExtension  `json:"extensions,omitempty"`
	}
	return judgmentDigest(digestShadow(r))
}

// ExpectedPairs 返回结果必须逐对回答且仅回答一次的 (candidate_id,
// question_id) pair 列表（确定性排序）。
func (r JudgmentWireRequest) ExpectedPairs() [][2]string {
	pairs := make([][2]string, 0, len(r.Candidates)*len(r.Questions))
	for _, candidate := range r.Candidates {
		for _, question := range r.Questions {
			if judgmentContainsString(question.CandidateIDs, candidate.CandidateID) {
				pairs = append(pairs, [2]string{candidate.CandidateID, question.QuestionID})
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] != pairs[j][0] {
			return pairs[i][0] < pairs[j][0]
		}
		return pairs[i][1] < pairs[j][1]
	})
	return pairs
}

// Validate 在请求可被发送前检查其形状。
func (r JudgmentWireRequest) Validate() error {
	if r.SchemaVersion != JudgmentWireSchemaVersion {
		return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "wire schema version must be 1.0"}
	}
	if strings.TrimSpace(r.RequestID) == "" || strings.TrimSpace(r.AttemptID) == "" {
		return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "request_id and attempt_id are owner-generated and required"}
	}
	if strings.TrimSpace(r.Model.Provider) == "" || strings.TrimSpace(r.Model.Model) == "" {
		return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "exact model pin is required"}
	}
	if strings.TrimSpace(r.QuestionSet.ID) == "" || strings.TrimSpace(r.QuestionSet.Version) == "" || strings.TrimSpace(r.QuestionSet.Digest) == "" {
		return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "versioned question_set ref is required"}
	}
	if strings.TrimSpace(r.PolicyRef.ID) == "" || strings.TrimSpace(r.PolicyRef.Version) == "" || strings.TrimSpace(r.PolicyRef.Digest) == "" {
		return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "versioned policy_ref is required"}
	}
	sources := map[string]bool{}
	for _, source := range r.Sources {
		if strings.TrimSpace(source.SourceID) == "" || strings.TrimSpace(source.Revision) == "" || strings.TrimSpace(source.InlineText) == "" {
			return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "every source needs id, revision, and bounded inline text"}
		}
		sources[source.SourceID] = true
	}
	seenCandidates := map[string]bool{}
	for _, candidate := range r.Candidates {
		if strings.TrimSpace(candidate.CandidateID) == "" {
			return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "candidate_id is required"}
		}
		if seenCandidates[candidate.CandidateID] {
			return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "candidate ids must be unique"}
		}
		seenCandidates[candidate.CandidateID] = true
		if !sources[candidate.SourceID] {
			return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "every candidate needs an exact source binding"}
		}
	}
	seenQuestions := map[string]bool{}
	for _, question := range r.Questions {
		if strings.TrimSpace(question.QuestionID) == "" {
			return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "question_id is required"}
		}
		if seenQuestions[question.QuestionID] {
			return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "question ids must be unique"}
		}
		seenQuestions[question.QuestionID] = true
		switch question.Primitive {
		case JudgmentPrimitiveBinary:
		case JudgmentPrimitiveChoice:
			if len(question.OptionIDs) == 0 {
				return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "choice question requires explicit option ids"}
			}
		case JudgmentPrimitiveOrdinalScore:
			if len(question.Levels) < 2 {
				return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "ordinal question requires explicit ordered levels"}
			}
		default:
			return &JudgmentTransportError{Code: JudgmentCodeUnsupportedCapability, Message: "unknown primitive"}
		}
		if len(question.CandidateIDs) == 0 {
			return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "questions need explicit candidate bindings"}
		}
	}
	if len(r.ExpectedPairs()) == 0 {
		return &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "request has no (candidate, question) pairs"}
	}
	return nil
}

// JudgmentWireAnswerValue 是类型化答案值；恰好设置一个 arm 且必须与问题
// 原语一致。
type JudgmentWireAnswerValue struct {
	Binary       *bool    `json:"binary,omitempty"`
	OrdinalLevel string   `json:"ordinal_level,omitempty"`
	OrdinalValue *float64 `json:"ordinal_value,omitempty"`
	Choice       string   `json:"choice,omitempty"`
}

// JudgmentWireDistributionEntry 是一条答案概率。
type JudgmentWireDistributionEntry struct {
	AnswerID    string  `json:"answer_id"`
	Probability float64 `json:"probability"`
}

// JudgmentWireConfidence 是显式标注来源的 confidence。provider confidence、
// choice probability 与领域证据 confidence 分开，永不互相填补。
type JudgmentWireConfidence struct {
	Value      float64 `json:"value"`
	Provenance string  `json:"provenance"`
}

// JudgmentWireItem 回答恰好一个 (candidate_id, question_id) pair。
// abstained 或 error 的 item 不携带可采纳值。
type JudgmentWireItem struct {
	CandidateID     string                          `json:"candidate_id"`
	QuestionID      string                          `json:"question_id"`
	AnswerStatus    string                          `json:"answer_status"` // answered | abstained | error
	Value           *JudgmentWireAnswerValue        `json:"value,omitempty"`
	Distribution    []JudgmentWireDistributionEntry `json:"distribution,omitempty"`     // nullable
	Confidence      *JudgmentWireConfidence         `json:"confidence,omitempty"`       // nullable
	ProbabilityTrue *float64                        `json:"probability_true,omitempty"` // 仅 binary，nullable
	ReasonCode      string                          `json:"reason_code,omitempty"`
}

// JudgmentWireUsage 只记录已知用量；nil 表示未知（绝不伪造为零）。
type JudgmentWireUsage struct {
	Unit        string   `json:"unit,omitempty"`
	InputUnits  *float64 `json:"input_units,omitempty"`
	OutputUnits *float64 `json:"output_units,omitempty"`
}

// JudgmentWireResult 是类型化评估结果。
type JudgmentWireResult struct {
	SchemaVersion     string             `json:"schema_version"`
	RequestID         string             `json:"request_id"`
	AttemptID         string             `json:"attempt_id"`
	InputDigest       string             `json:"input_digest"`
	ResolvedModel     JudgmentWireModel  `json:"resolved_model"`
	ExecutionStatus   string             `json:"execution_status"` // succeeded | partial | failed | unknown
	Items             []JudgmentWireItem `json:"items,omitempty"`
	Usage             *JudgmentWireUsage `json:"usage,omitempty"` // nil = unknown
	LatencyMS         *int64             `json:"latency_ms,omitempty"`
	ProviderRequestID string             `json:"provider_request_id,omitempty"`
	ReasonCode        string             `json:"reason_code,omitempty"`
	SourceRefs        []string           `json:"source_refs,omitempty"`
}

// judgmentDistributionTolerance 是本 seam 的版本化分布求和精度策略
// （跨语言数值由公共 SDK 统一拥有；本包不硬编码 adapter 特定容差语义）。
const judgmentDistributionTolerance = 1e-6

// ValidateJudgmentWireResult 在任何领域阈值之前检查结构：身份绑定、pair
// 对齐（缺失/重复/未知 pair 是协议错误，绝不靠猜测补答案）、原语一致的值、
// 弃答卫生、分布形状与 fail-closed 的 unknown 状态。
func ValidateJudgmentWireResult(request JudgmentWireRequest, result JudgmentWireResult) error {
	invalidResponse := func(message string) error {
		return &JudgmentTransportError{Code: JudgmentCodeInvalidResponse, Message: message, SubmissionState: JudgmentSubmissionSubmitted, RetryClass: JudgmentRetryNever}
	}
	if result.SchemaVersion != JudgmentWireSchemaVersion {
		return invalidResponse("result schema version mismatch")
	}
	if result.RequestID != request.RequestID || result.AttemptID != request.AttemptID {
		return invalidResponse("result is not bound to the request attempt")
	}
	if result.InputDigest != request.CanonicalDigest() {
		return invalidResponse("result input digest does not match the request")
	}
	if result.ResolvedModel.Model != request.Model.Model || result.ResolvedModel.Provider != request.Model.Provider {
		return invalidResponse("resolved model does not match the exact pin")
	}
	switch result.ExecutionStatus {
	case "succeeded", "partial":
	case "failed":
		return nil
	case "unknown":
		return &JudgmentTransportError{Code: JudgmentCodeOutcomeUnknown, Message: "execution status unknown", SubmissionState: JudgmentSubmissionUnknown, RetryClass: JudgmentRetryReconcileFirst}
	default:
		return invalidResponse("unknown execution status value")
	}
	questions := map[string]JudgmentWireQuestion{}
	for _, question := range request.Questions {
		questions[question.QuestionID] = question
	}
	seen := map[[2]string]int{}
	for _, item := range result.Items {
		pair := [2]string{item.CandidateID, item.QuestionID}
		seen[pair]++
		if seen[pair] > 1 {
			return invalidResponse("duplicate pair answer")
		}
		question, known := questions[item.QuestionID]
		if !known || !judgmentContainsString(question.CandidateIDs, item.CandidateID) {
			return invalidResponse("answer references an unexpected pair")
		}
		switch item.AnswerStatus {
		case "answered":
		case "abstained", "error":
			if item.Value != nil || len(item.Distribution) > 0 || item.ProbabilityTrue != nil {
				return invalidResponse("abstained or errored items carry no adoptable value")
			}
			continue
		default:
			return invalidResponse("unknown answer status")
		}
		if item.Value == nil {
			return invalidResponse("answered item lacks a value")
		}
		setArms := 0
		for _, ok := range []bool{item.Value.Binary != nil, item.Value.OrdinalLevel != "", item.Value.Choice != ""} {
			if ok {
				setArms++
			}
		}
		if setArms != 1 {
			return invalidResponse("value must set exactly one primitive arm")
		}
		switch question.Primitive {
		case JudgmentPrimitiveBinary:
			if item.Value.Binary == nil {
				return invalidResponse("binary question requires a binary value")
			}
		case JudgmentPrimitiveOrdinalScore:
			if item.Value.OrdinalLevel == "" {
				return invalidResponse("ordinal question requires a level id")
			}
			found := false
			for _, level := range question.Levels {
				if level.LevelID == item.Value.OrdinalLevel {
					found = true
				}
			}
			if !found {
				return invalidResponse("ordinal level is outside the declared domain")
			}
		case JudgmentPrimitiveChoice:
			if item.Value.Choice == "" {
				return invalidResponse("choice question requires an option id")
			}
			if !judgmentContainsString(question.OptionIDs, item.Value.Choice) {
				return invalidResponse("choice is outside the declared domain")
			}
		}
		if item.ProbabilityTrue != nil && question.Primitive != JudgmentPrimitiveBinary {
			return invalidResponse("probability_true is only valid for binary questions")
		}
		if item.Confidence != nil && strings.TrimSpace(item.Confidence.Provenance) == "" {
			return invalidResponse("confidence requires explicit provenance")
		}
		if len(item.Distribution) > 0 {
			answerDomain := make([]string, 0, len(question.OptionIDs)+len(question.Levels))
			answerDomain = append(answerDomain, question.OptionIDs...)
			for _, level := range question.Levels {
				answerDomain = append(answerDomain, level.LevelID)
			}
			if len(item.Distribution) != len(answerDomain) {
				return invalidResponse("distribution must cover the full answer domain")
			}
			seenAnswers := map[string]bool{}
			sum := 0.0
			for _, entry := range item.Distribution {
				if !judgmentContainsString(answerDomain, entry.AnswerID) {
					return invalidResponse("distribution entry outside the answer domain")
				}
				if seenAnswers[entry.AnswerID] {
					return invalidResponse("distribution entry duplicated")
				}
				seenAnswers[entry.AnswerID] = true
				if math.IsNaN(entry.Probability) || math.IsInf(entry.Probability, 0) || entry.Probability < 0 {
					return invalidResponse("distribution probabilities must be finite and non-negative")
				}
				sum += entry.Probability
			}
			if math.Abs(sum-1) > judgmentDistributionTolerance {
				return invalidResponse("distribution must sum to one within the precision policy")
			}
		}
	}
	for _, pair := range request.ExpectedPairs() {
		if seen[pair] == 0 {
			return invalidResponse("expected pair is missing an answer")
		}
	}
	return nil
}

// JudgmentTransport 是注入的 transport port（HTTP adapter、本地 stdio 或
// fixture）。SDK 不持密钥、不做隐式付费调用。
type JudgmentTransport interface {
	DescribeCapabilities(ctx context.Context) (JudgmentCapabilities, error)
	Evaluate(ctx context.Context, request JudgmentWireRequest) (JudgmentWireResult, error)
}

// JudgmentClient 围绕一次显式 attempt 校验 wire 流量。它从不重试、从不发起
// 隐式调用：一次 client Evaluate 精确映射一次 transport Evaluate。
type JudgmentClient struct {
	Transport    JudgmentTransport
	Capabilities JudgmentCapabilities // 离线预检用的本地能力快照
}

// Evaluate 先执行预检（针对本地能力快照的原语支持检查——无网络），再执行
// 单次 transport 调用，然后校验结果结构。transport 错误只分类、绝不在此重试。
func (c *JudgmentClient) Evaluate(ctx context.Context, request JudgmentWireRequest) (JudgmentWireResult, error) {
	if c.Transport == nil {
		return JudgmentWireResult{}, &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "judgment client has no transport", SubmissionState: JudgmentSubmissionNotSubmitted, RetryClass: JudgmentRetryNever}
	}
	if err := request.Validate(); err != nil {
		return JudgmentWireResult{}, err
	}
	for _, question := range request.Questions {
		if !c.Capabilities.SupportsPrimitive(question.Primitive) {
			return JudgmentWireResult{}, &JudgmentTransportError{Code: JudgmentCodeUnsupportedCapability, Message: "question primitive is not supported by the adapter capabilities", SubmissionState: JudgmentSubmissionNotSubmitted, RetryClass: JudgmentRetryNever}
		}
	}
	result, err := c.Transport.Evaluate(ctx, request)
	if err != nil {
		return JudgmentWireResult{}, classifyJudgmentTransportError(err)
	}
	if err := ValidateJudgmentWireResult(request, result); err != nil {
		return JudgmentWireResult{}, err
	}
	return result, nil
}

// classifyJudgmentTransportError 在 adapter 省略默认 submission state 与
// retry class 时补齐，不改变错误码。
func classifyJudgmentTransportError(err error) error {
	transportError, ok := err.(*JudgmentTransportError)
	if !ok {
		return &JudgmentTransportError{Code: JudgmentCodeUnavailable, Message: "unclassified transport failure", SubmissionState: JudgmentSubmissionUnknown, RetryClass: JudgmentRetryReconcileFirst, DiagnosticRef: judgmentDiagnosticRef(err)}
	}
	if transportError.SubmissionState == "" || transportError.RetryClass == "" {
		submissionState, retryClass := JudgmentErrorClass(transportError.Code)
		if transportError.SubmissionState == "" {
			transportError.SubmissionState = submissionState
		}
		if transportError.RetryClass == "" {
			transportError.RetryClass = retryClass
		}
	}
	return transportError
}

func judgmentDiagnosticRef(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 120 {
		message = message[:120]
	}
	return "diagnostic:" + judgmentDigest(message)
}

// JudgmentNewID 生成 owner 侧 request/attempt id。
func JudgmentNewID(prefix string) string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return prefix + "-unknown"
	}
	return prefix + "-" + hex.EncodeToString(raw[:])
}

// judgmentDigest 计算 JSON 值的 sha256 digest（与 app 层 hashString 同格式）。
func judgmentDigest(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return judgmentDigestBytes(raw)
}

func judgmentDigestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// judgmentInvalidRequest 构造领域侧 CommandError（预检失败）。
func judgmentInvalidRequest(message, hint string) error {
	return &domain.CommandError{Code: "judgment_invalid_request", Message: message, Hint: hint}
}

func judgmentContainsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// asJudgmentTransportError 提取 transport 错误；非 transport 错误原样返回。
func asJudgmentTransportError(err error) (*JudgmentTransportError, bool) {
	var transportError *JudgmentTransportError
	if errors.As(err, &transportError) {
		return transportError, true
	}
	return nil, false
}
