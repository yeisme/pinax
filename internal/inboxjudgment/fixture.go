package inboxjudgment

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// FixtureTransport 是合同与场景测试使用的离线确定性 transport。它不触碰
// 网络、不伪造不确定性：只有 fixture 摘要携带显式 verdict tag 时才有答案，
// 否则该 pair 弃答（abstained）。答案按 (candidate_id, question_id) pair
// 对齐，绝不按数组顺序。
type FixtureTransport struct {
	Name     string
	Behavior string // ""（tag 答案）| abstain_all | unavailable | timeout_after_submit | invalid_response | partial | outcome_unknown

	mu            sync.Mutex
	describeCalls int
	evaluateCalls int
	lastRequest   *JudgmentWireRequest
}

// NewFixtureTransport 构造 fixture transport。
func NewFixtureTransport(name string) *FixtureTransport {
	return &FixtureTransport{Name: name}
}

// DescribeCalls 返回已发生的 capability 调用数。
func (f *FixtureTransport) DescribeCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.describeCalls
}

// EvaluateCalls 返回已发生的 evaluate 调用数。
func (f *FixtureTransport) EvaluateCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.evaluateCalls
}

// TotalCalls 返回 describe + evaluate 调用总数（off 模式断言零调用用）。
func (f *FixtureTransport) TotalCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.describeCalls + f.evaluateCalls
}

// LastRequest 返回最近一次收到的请求（仅测试断言用）。
func (f *FixtureTransport) LastRequest() JudgmentWireRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastRequest == nil {
		return JudgmentWireRequest{}
	}
	return *f.lastRequest
}

// FixtureJudgmentCapabilities 返回 fixture 能力快照：纯文本模态、三种原语
// 齐备，probability/confidence 显式不可用（字段保持 null，不合成数值）。
func FixtureJudgmentCapabilities(name string) JudgmentCapabilities {
	return JudgmentCapabilities{
		SchemaVersion:        JudgmentWireSchemaVersion,
		ContractVersion:      JudgmentWireSchemaVersion,
		Adapter:              "fixture",
		AdapterVersion:       "fixture-v1",
		Model:                "fixture:" + name,
		Modalities:           []string{"text"},
		Primitives:           []string{JudgmentPrimitiveBinary, JudgmentPrimitiveOrdinalScore, JudgmentPrimitiveChoice},
		MaxBatchCandidates:   InboxJudgmentDefaultMaxCandidates,
		MaxBatchQuestions:    InboxJudgmentDefaultMaxCandidates * 4,
		MaxInputBytes:        64 * 1024,
		MaxOutputBytes:       64 * 1024,
		LanguageNote:         "fixture: tag-driven answers; zh and en treated equally",
		ProbabilityAvailable: false,
		ConfidenceAvailable:  false,
		ReconcileSupported:   false,
		CancelSupported:      false,
	}
}

// DescribeCapabilities 返回 fixture 能力快照。
func (f *FixtureTransport) DescribeCapabilities(context.Context) (JudgmentCapabilities, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.describeCalls++
	return FixtureJudgmentCapabilities(f.Name), nil
}

// Evaluate 从投影内容确定性作答。verdict tag 形如
// "fixture:relation=duplicate"、"fixture:topical_match=yes"、
// "fixture:link_usefulness=clearly_useful"、"fixture:language=zh"、
// "fixture:abstain=relation"。
func (f *FixtureTransport) Evaluate(_ context.Context, request JudgmentWireRequest) (JudgmentWireResult, error) {
	f.mu.Lock()
	f.evaluateCalls++
	requestCopy := request
	f.lastRequest = &requestCopy
	f.mu.Unlock()
	if err := request.Validate(); err != nil {
		return JudgmentWireResult{}, err
	}
	switch f.Behavior {
	case "unavailable":
		return JudgmentWireResult{}, &JudgmentTransportError{Code: JudgmentCodeUnavailable, Message: "fixture adapter is unavailable", DiagnosticRef: "fixture:unavailable"}
	case "timeout_after_submit":
		return JudgmentWireResult{}, &JudgmentTransportError{Code: JudgmentCodeDeadlineExceeded, Message: "fixture deadline exceeded after submission", SubmissionState: JudgmentSubmissionUnknown, DiagnosticRef: "fixture:timeout_after_submit"}
	case "outcome_unknown":
		return JudgmentWireResult{}, &JudgmentTransportError{Code: JudgmentCodeOutcomeUnknown, Message: "fixture outcome is unknown", SubmissionState: JudgmentSubmissionUnknown, DiagnosticRef: "fixture:outcome_unknown"}
	}
	sources := map[string]JudgmentWireSource{}
	for _, source := range request.Sources {
		sources[source.SourceID] = source
	}
	items := make([]JudgmentWireItem, 0, len(request.ExpectedPairs()))
	errorItems := 0
	for _, pair := range request.ExpectedPairs() {
		candidateID, questionID := pair[0], pair[1]
		var question JudgmentWireQuestion
		for _, entry := range request.Questions {
			if entry.QuestionID == questionID {
				question = entry
				break
			}
		}
		var candidate JudgmentWireCandidate
		for _, entry := range request.Candidates {
			if entry.CandidateID == candidateID {
				candidate = entry
				break
			}
		}
		source := sources[candidate.SourceID]
		tags := fixtureVerdictTags(source.InlineText)
		item := JudgmentWireItem{CandidateID: candidateID, QuestionID: questionID}
		if f.Behavior == "abstain_all" || tags.abstain(questionID) {
			item.AnswerStatus = "abstained"
			item.ReasonCode = "fixture_abstained"
			items = append(items, item)
			continue
		}
		value, ok := fixtureAnswer(question, tags)
		if !ok {
			// 无显式 verdict tag：保留不确定性，绝不猜测。
			item.AnswerStatus = "abstained"
			item.ReasonCode = "fixture_no_verdict_tag"
			items = append(items, item)
			continue
		}
		if f.Behavior == "partial" && questionID == "language" && candidateID == request.Candidates[len(request.Candidates)-1].CandidateID {
			item.AnswerStatus = "error"
			item.ReasonCode = "fixture_partial_error"
			errorItems++
			items = append(items, item)
			continue
		}
		item.AnswerStatus = "answered"
		item.Value = value
		items = append(items, item)
	}
	result := JudgmentWireResult{
		SchemaVersion:     JudgmentWireSchemaVersion,
		RequestID:         request.RequestID,
		AttemptID:         request.AttemptID,
		InputDigest:       request.CanonicalDigest(),
		ResolvedModel:     request.Model, // fixture 回显精确请求 pin
		ExecutionStatus:   "succeeded",
		Items:             items,
		Usage:             nil, // unknown，绝不伪造为零
		ProviderRequestID: "fixture-" + request.AttemptID,
		ReasonCode:        "",
		SourceRefs:        fixtureSourceRefs(request),
	}
	if f.Behavior == "partial" {
		result.ExecutionStatus = "partial"
		result.ReasonCode = "fixture_partial"
	}
	if f.Behavior == "invalid_response" {
		// 故意畸形：重复 pair 答案。
		result.Items = append(result.Items, result.Items[0])
	}
	return result, nil
}

func fixtureSourceRefs(request JudgmentWireRequest) []string {
	refs := make([]string, 0, len(request.Sources))
	for _, source := range request.Sources {
		refs = append(refs, source.SourceID+"@"+source.Revision)
	}
	sort.Strings(refs)
	return refs
}

// fixtureVerdictTags 从 fixture 摘要解析显式 verdict tag。
type fixtureTags struct {
	values map[string]string
}

func (t fixtureTags) abstain(questionID string) bool {
	return t.values["abstain"] == "all" || t.values["abstain"] == questionID
}

func fixtureVerdictTags(text string) fixtureTags {
	values := map[string]string{}
	for _, field := range strings.Fields(text) {
		if !strings.HasPrefix(field, "fixture:") {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(field, "fixture:"), "=", 2)
		if len(parts) == 2 && parts[1] != "" {
			values[parts[0]] = parts[1]
		}
	}
	return fixtureTags{values: values}
}

func fixtureAnswer(question JudgmentWireQuestion, tags fixtureTags) (*JudgmentWireAnswerValue, bool) {
	yesNo := func(key string) (*bool, bool) {
		switch tags.values[key] {
		case "yes":
			value := true
			return &value, true
		case "no":
			value := false
			return &value, true
		}
		return nil, false
	}
	switch question.Primitive {
	case JudgmentPrimitiveBinary:
		if binary, ok := yesNo(question.QuestionID); ok {
			return &JudgmentWireAnswerValue{Binary: binary}, true
		}
		return nil, false
	case JudgmentPrimitiveChoice:
		if choice, ok := tags.values[question.QuestionID]; ok && judgmentContainsString(question.OptionIDs, choice) {
			return &JudgmentWireAnswerValue{Choice: choice}, true
		}
		return nil, false
	case JudgmentPrimitiveOrdinalScore:
		if level, ok := tags.values[question.QuestionID]; ok {
			for _, declared := range question.Levels {
				if declared.LevelID == level {
					numeric := declared.NumericValue
					return &JudgmentWireAnswerValue{OrdinalLevel: level, OrdinalValue: &numeric}, true
				}
			}
		}
		return nil, false
	}
	return nil, false
}

var _ JudgmentTransport = (*FixtureTransport)(nil)
