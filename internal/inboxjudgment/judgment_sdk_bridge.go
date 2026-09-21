//go:build judgment_sdk

package inboxjudgment

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"

	judgment "github.com/yeisme/judgment-sdk"
)

// 真实公共 SDK 集成 bridge（build tag: judgment_sdk）。
//
// Aigora owner 的 SDK 模块（github.com/yeisme/judgment-sdk，合同 "1.0"）已
// 在 workspace 内提交但未发布；本仓默认构建停在本地 seam（wire.go），零
// 外部耦合。本 bridge 验证真实引用兼容性：把 seam 请求转换为 SDK 规范
// wire JSON，经真实 SDK client 校验并执行（单次 transport 调用、不重试），
// 再把已校验的 SDK 结果转换回 seam 结果。
//
// 运行：go test -tags judgment_sdk ./internal/inboxjudgment/
// （vendor 快照已提交，tagged 构建离线可用；-mod=mod 则直接解析 workspace
// 内的 SDK 树。）默认构建不编译本文件；`go mod tidy` 会因为也评估
// build-tag 门控文件而保留可选 require/replace 对。

// strongSHA256Pattern 是合同要求的强 digest 形态。
var strongSHA256Pattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// JudgmentSDKBridge 把真实 SDK transport 适配为 seam JudgmentTransport
// port。能力是钉住的本地快照：本 bridge 不做任何发现调用。
type JudgmentSDKBridge struct {
	client *judgment.Client
	caps   judgment.Capabilities
}

// NewJudgmentSDKBridge 基于注入的 SDK transport 与钉住的能力快照构建
// bridge（离线门，无网络）。
func NewJudgmentSDKBridge(transport judgment.Transport, caps judgment.Capabilities) *JudgmentSDKBridge {
	client := judgment.NewClient(transport, judgment.ClientOptions{StaticCapabilities: &caps})
	return &JudgmentSDKBridge{client: client, caps: caps}
}

// DescribeCapabilities 从钉住的快照实现 JudgmentTransport。
func (b *JudgmentSDKBridge) DescribeCapabilities(context.Context) (JudgmentCapabilities, error) {
	return judgmentSDKCapsToSeam(b.caps), nil
}

// Evaluate 实现 JudgmentTransport：seam 请求 → SDK 规范 wire → 真实 SDK
// 校验、能力门与恰好一次 transport 调用 → seam 结果。SDK 错误 1:1 映射到
// seam 错误 envelope。
func (b *JudgmentSDKBridge) Evaluate(ctx context.Context, request JudgmentWireRequest) (JudgmentWireResult, error) {
	tree, err := judgmentSDKRequestTree(request)
	if err != nil {
		return JudgmentWireResult{}, err
	}
	wire, err := json.Marshal(tree)
	if err != nil {
		return JudgmentWireResult{}, &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "bridge request encode failed", SubmissionState: JudgmentSubmissionNotSubmitted, RetryClass: JudgmentRetryNever}
	}
	sdkRequest, verr := judgment.ParseRequest(wire, judgment.RequestOptions{})
	if verr != nil {
		return JudgmentWireResult{}, judgmentSDKErrorToSeam(verr.AsRequestError())
	}
	result, aerr := b.client.Evaluate(ctx, sdkRequest)
	if aerr != nil {
		return JudgmentWireResult{}, judgmentSDKErrorToSeam(aerr)
	}
	return judgmentSDKResultToSeam(result, request)
}

// judgmentSDKRequestTree 把 seam 请求转换为 SDK 规范 wire 树（精确 key、
// snake_case、显式 null）。owner 未钉 source digest 时默认使用确定性的
// 内容 digest。inbox 上下文 source 不绑定任何 candidate。
func judgmentSDKRequestTree(request JudgmentWireRequest) (map[string]any, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	candidateSources := map[string]bool{}
	for _, candidate := range request.Candidates {
		candidateSources[candidate.SourceID] = true
	}
	sources := make([]any, 0, len(request.Sources))
	for _, source := range request.Sources {
		digest := source.Digest
		if digest == "" {
			digest = judgmentDigestBytes([]byte(source.InlineText))
		}
		if !strongSHA256Pattern.MatchString(digest) {
			return nil, &JudgmentTransportError{Code: JudgmentCodeInvalidRequest, Message: "source digest must be a sha256 digest", SubmissionState: JudgmentSubmissionNotSubmitted, RetryClass: JudgmentRetryNever}
		}
		language := any(nil)
		if source.Language != "" {
			language = source.Language
		}
		sources = append(sources, map[string]any{
			"source_id":   source.SourceID,
			"revision":    source.Revision,
			"digest":      digest,
			"inline_text": source.InlineText,
			"language":    language,
			"local_ref":   nil,
		})
	}
	candidates := make([]any, 0, len(request.Candidates))
	for _, candidate := range request.Candidates {
		candidates = append(candidates, map[string]any{
			"candidate_id": candidate.CandidateID,
			"source_bindings": []any{
				map[string]any{"source_id": candidate.SourceID},
			},
		})
	}
	questions := make([]any, 0, len(request.Questions))
	for _, question := range request.Questions {
		answerDomain := any(nil)
		switch question.Primitive {
		case JudgmentPrimitiveChoice:
			options := make([]any, 0, len(question.OptionIDs))
			for _, optionID := range question.OptionIDs {
				options = append(options, map[string]any{"option_id": optionID, "label": optionID})
			}
			answerDomain = map[string]any{"options": options}
		case JudgmentPrimitiveOrdinalScore:
			levels := make([]any, 0, len(question.Levels))
			for _, level := range question.Levels {
				levels = append(levels, map[string]any{"level_id": level.LevelID, "numeric_value": level.NumericValue, "label": level.LevelID})
			}
			answerDomain = map[string]any{"levels": levels}
		}
		questions = append(questions, map[string]any{
			"question_id":   question.QuestionID,
			"primitive":     question.Primitive,
			"prompt":        question.Text,
			"candidate_ids": append([]string(nil), question.CandidateIDs...),
			"required":      question.Required,
			"answer_domain": answerDomain,
		})
	}
	limits := map[string]any{}
	if request.Limits.DeadlineMS > 0 {
		limits["deadline_ms"] = request.Limits.DeadlineMS
	}
	if request.Limits.MaxCandidates > 0 {
		limits["max_candidates"] = request.Limits.MaxCandidates
	}
	if request.Limits.MaxQuestions > 0 {
		limits["max_questions"] = request.Limits.MaxQuestions
	}
	if request.Limits.MaxInputBytes > 0 {
		limits["max_input_bytes"] = request.Limits.MaxInputBytes
	}
	if request.Limits.MaxOutputBytes > 0 {
		limits["max_output_bytes"] = request.Limits.MaxOutputBytes
	}
	return map[string]any{
		"schema_version": request.SchemaVersion,
		"request_id":     request.RequestID,
		"attempt_id":     request.AttemptID,
		"scope": map[string]any{
			"owner_id":     request.Scope.Owner,
			"project_id":   request.Scope.Project,
			"principal_id": request.Scope.Principal,
			"subject":      nil,
		},
		"model": map[string]any{
			"transport_provider":           request.Model.Provider,
			"model_provider":               nil,
			"requested_model":              request.Model.Model,
			"response_model":               nil,
			"underlying_revision":          nil,
			"pin_level":                    judgment.PinProviderExactModel,
			"underlying_revision_verified": false,
		},
		"question_set": map[string]any{"id": request.QuestionSet.ID, "version": request.QuestionSet.Version, "digest": request.QuestionSet.Digest},
		"policy_ref":   map[string]any{"id": request.PolicyRef.ID, "version": request.PolicyRef.Version, "digest": request.PolicyRef.Digest},
		"sources":      sources,
		"candidates":   candidates,
		"questions":    questions,
		"limits":       limits,
		"extensions":   []any{},
	}, nil
}

// judgmentSDKCapsToSeam 把 SDK 能力映射到 seam 快照。
func judgmentSDKCapsToSeam(caps judgment.Capabilities) JudgmentCapabilities {
	seam := JudgmentCapabilities{
		SchemaVersion:        JudgmentWireSchemaVersion,
		ContractVersion:      caps.ContractVersion,
		Adapter:              caps.Adapter.Name,
		AdapterVersion:       caps.Adapter.Version,
		Model:                caps.Model.RequestedModel,
		Modalities:           append([]string(nil), caps.Modalities...),
		Primitives:           append([]string(nil), caps.Primitives...),
		MaxBatchCandidates:   caps.MaxCandidates,
		MaxBatchQuestions:    caps.MaxQuestions,
		MaxInputBytes:        int(caps.MaxInlineTextBytes),
		ProbabilityAvailable: caps.ProbabilityAvailable,
		ConfidenceAvailable:  caps.ConfidenceAvailable,
		ReconcileSupported:   caps.SupportsReconcile,
		CancelSupported:      caps.SupportsCancel,
	}
	if caps.ConfidenceProvenance != nil {
		seam.ConfidenceProvenance = *caps.ConfidenceProvenance
	}
	return seam
}

// judgmentSDKErrorToSeam 1:1 映射结构化 SDK 错误。
func judgmentSDKErrorToSeam(err *judgment.Error) *JudgmentTransportError {
	if err == nil {
		return nil
	}
	return &JudgmentTransportError{
		Code:            err.Code,
		Message:         err.Detail,
		SubmissionState: err.SubmissionState,
		RetryClass:      err.RetryClass,
		DiagnosticRef:   err.DiagnosticRef,
	}
}

// judgmentSDKResultToSeam 把已校验的 SDK 结果转换为 seam 结果。SDK 已在
// client 内强制 input-digest 与模型绑定；seam 侧再把身份绑定回 seam 请求
// digest，由 ValidateJudgmentWireResult 复核。
func judgmentSDKResultToSeam(result *judgment.Result, request JudgmentWireRequest) (JudgmentWireResult, error) {
	if result == nil {
		return JudgmentWireResult{}, &JudgmentTransportError{Code: JudgmentCodeInvalidResponse, Message: "sdk returned no result", SubmissionState: JudgmentSubmissionSubmitted, RetryClass: JudgmentRetryNever}
	}
	items := make([]JudgmentWireItem, 0, len(result.Items))
	for _, item := range result.Items {
		seamItem := JudgmentWireItem{
			CandidateID:  item.CandidateID,
			QuestionID:   item.QuestionID,
			AnswerStatus: item.AnswerStatus,
		}
		if item.Value != nil {
			value := &JudgmentWireAnswerValue{}
			if item.Value.Binary != nil {
				value.Binary = item.Value.Binary
			}
			if item.Value.LevelID != nil {
				value.OrdinalLevel = *item.Value.LevelID
			}
			if item.Value.OptionID != nil {
				value.Choice = *item.Value.OptionID
			}
			seamItem.Value = value
		}
		if item.Distribution != nil {
			entries := make([]JudgmentWireDistributionEntry, 0, len(item.Distribution))
			for answerID, probability := range item.Distribution {
				entries = append(entries, JudgmentWireDistributionEntry{AnswerID: answerID, Probability: probability})
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].AnswerID < entries[j].AnswerID })
			seamItem.Distribution = entries
		}
		if item.Confidence != nil {
			seamItem.Confidence = &JudgmentWireConfidence{Value: item.Confidence.Value, Provenance: item.Confidence.Provenance}
		}
		if item.ProbabilityTrue != nil {
			probability := *item.ProbabilityTrue
			seamItem.ProbabilityTrue = &probability
		}
		if item.ReasonCode != nil {
			seamItem.ReasonCode = *item.ReasonCode
		}
		items = append(items, seamItem)
	}
	seam := JudgmentWireResult{
		SchemaVersion:   JudgmentWireSchemaVersion,
		RequestID:       result.RequestID,
		AttemptID:       result.AttemptID,
		InputDigest:     request.CanonicalDigest(), // seam 身份绑定；SDK 绑定已在 client 内强制
		ResolvedModel:   JudgmentWireModel{Provider: result.ResolvedModel.TransportProvider, Model: result.ResolvedModel.RequestedModel},
		ExecutionStatus: result.ExecutionStatus,
		Items:           items,
	}
	if result.Usage != nil {
		usage := &JudgmentWireUsage{Unit: "tokens"}
		if result.Usage.InputTokens != nil {
			input := float64(*result.Usage.InputTokens)
			usage.InputUnits = &input
		}
		if result.Usage.OutputTokens != nil {
			output := float64(*result.Usage.OutputTokens)
			usage.OutputUnits = &output
		}
		seam.Usage = usage
	}
	if result.LatencyMS != nil {
		latency := *result.LatencyMS
		seam.LatencyMS = &latency
	}
	if result.ProviderRequestID != nil {
		seam.ProviderRequestID = *result.ProviderRequestID
	}
	return seam, nil
}

// JudgmentSDKFixtureResponder 构造一个确定性的 SDK 侧 responder，复用
// seam fixture verdict tag，使 bridge 一致性运行与 seam-only 测试共享同一
// 离线答案模型。只从显式 tag 作答，否则弃答（绝不伪造不确定性）。
func JudgmentSDKFixtureResponder() func(context.Context, *judgment.Request) (*judgment.Result, error) {
	return func(_ context.Context, req *judgment.Request) (*judgment.Result, error) {
		sourceText := map[string]string{}
		sourceRevision := map[string]string{}
		for _, source := range req.Sources {
			if source.InlineText != nil {
				sourceText[source.SourceID] = *source.InlineText
			}
			sourceRevision[source.SourceID] = source.Revision
		}
		candidateSource := map[string]string{}
		for _, candidate := range req.Candidates {
			for _, binding := range candidate.SourceBindings {
				candidateSource[candidate.CandidateID] = binding.SourceID
			}
		}
		items := make([]any, 0, len(req.ExpectedPairs()))
		for _, pair := range req.ExpectedPairs() {
			candidateID, questionID := pair[0], pair[1]
			var question judgment.Question
			for _, entry := range req.Questions {
				if entry.QuestionID == questionID {
					question = entry
					break
				}
			}
			tags := fixtureVerdictTags(sourceText[candidateSource[candidateID]])
			item := map[string]any{
				"candidate_id":     candidateID,
				"question_id":      questionID,
				"value":            nil,
				"distribution":     nil,
				"confidence":       nil,
				"probability_true": nil,
				"reason_code":      nil,
				"source_refs":      []any{},
				"provider_value":   nil,
				"derived_value":    nil,
				"normalization":    nil,
			}
			sourceID := candidateSource[candidateID]
			if sourceID != "" {
				item["source_refs"] = []any{map[string]any{"source_id": sourceID, "revision": sourceRevision[sourceID]}}
			}
			if tags.abstain(questionID) {
				item["answer_status"] = "abstained"
				reason := "fixture_abstained"
				item["reason_code"] = reason
				items = append(items, item)
				continue
			}
			seamQuestion := JudgmentWireQuestion{QuestionID: question.QuestionID, Primitive: question.Primitive, OptionIDs: nil}
			for _, option := range question.ChoiceOptions {
				seamQuestion.OptionIDs = append(seamQuestion.OptionIDs, option.OptionID)
			}
			for _, level := range question.OrdinalLevels {
				numeric := level.NumericValue
				seamQuestion.Levels = append(seamQuestion.Levels, JudgmentWireOrdinalLevel{LevelID: level.LevelID, NumericValue: numeric})
			}
			value, ok := fixtureAnswer(seamQuestion, tags)
			if !ok {
				item["answer_status"] = "abstained"
				reason := "fixture_no_verdict_tag"
				item["reason_code"] = reason
				items = append(items, item)
				continue
			}
			item["answer_status"] = "answered"
			switch question.Primitive {
			case judgment.PrimitiveBinary:
				item["value"] = map[string]any{"binary": *value.Binary}
			case judgment.PrimitiveChoice:
				item["value"] = map[string]any{"option_id": value.Choice}
			case judgment.PrimitiveOrdinalScore:
				item["value"] = map[string]any{"level_id": value.OrdinalLevel}
			}
			items = append(items, item)
		}
		digest, err := req.InputDigest()
		if err != nil {
			return nil, judgment.NewError(judgment.CodeInvalidResponse, judgment.SubmissionNotSubmitted, judgment.RetryNever, "", "request_digest_unavailable")
		}
		resultTree := map[string]any{
			"schema_version":   "1.0",
			"request_id":       req.RequestID,
			"attempt_id":       req.AttemptID,
			"input_digest":     digest,
			"execution_status": "succeeded",
			"resolved_model": map[string]any{
				"transport_provider":           req.Model.TransportProvider,
				"model_provider":               nil,
				"requested_model":              req.Model.RequestedModel,
				"response_model":               nil,
				"underlying_revision":          nil,
				"pin_level":                    req.Model.PinLevel,
				"underlying_revision_verified": false,
			},
			"items":               items,
			"usage":               nil,
			"latency_ms":          nil,
			"provider_request_id": "bridge-" + req.AttemptID,
			"extensions":          map[string]any{},
		}
		wire, err := json.Marshal(resultTree)
		if err != nil {
			return nil, judgment.NewError(judgment.CodeInvalidResponse, judgment.SubmissionNotSubmitted, judgment.RetryNever, "", "fixture_result_encode_failed")
		}
		parsed, verr := judgment.ParseResult(wire, req, judgment.DefaultPrecisionPolicy())
		if verr != nil {
			return nil, verr.AsResponseError()
		}
		return parsed, nil
	}
}

// JudgmentSDKFixtureCapabilities 返回 bridge 一致性运行使用的钉住 SDK
// 能力快照。
func JudgmentSDKFixtureCapabilities(model string) judgment.Capabilities {
	return judgment.Capabilities{
		SchemaVersion:   JudgmentWireSchemaVersion,
		ContractVersion: JudgmentWireSchemaVersion,
		Adapter:         judgment.Adapter{Name: "bridge-fixture", Version: "fixture-v1"},
		Model: judgment.ModelIdentity{
			TransportProvider: "judgment",
			RequestedModel:    model,
			PinLevel:          judgment.PinProviderExactModel,
		},
		Modalities:           []string{"text"},
		Primitives:           []string{judgment.PrimitiveBinary, judgment.PrimitiveOrdinalScore, judgment.PrimitiveChoice},
		MaxCandidates:        InboxJudgmentDefaultMaxCandidates,
		MaxQuestions:         InboxJudgmentDefaultMaxCandidates * 4,
		MaxSources:           InboxJudgmentDefaultMaxCandidates + 1,
		MaxInlineTextBytes:   64 * 1024,
		Languages:            []string{"zh", "en", "mixed", "other"},
		ProbabilityAvailable: false,
		ConfidenceAvailable:  false,
		DistributionTolerance: judgment.DistributionTolerance{
			Version:         judgment.DefaultPrecisionPolicy().Version,
			AbsSumTolerance: judgment.DefaultPrecisionPolicy().AbsSumTolerance,
		},
	}
}

var _ JudgmentTransport = (*JudgmentSDKBridge)(nil)
