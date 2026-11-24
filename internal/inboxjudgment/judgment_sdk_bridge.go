package inboxjudgment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	judgment "github.com/yeisme/judgment-sdk"
	"github.com/yeisme/judgment-sdk/providers"
	"github.com/yeisme/pinax/internal/sqlitedsn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var strongSHA256Pattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// The public SDK bridge is available in ordinary builds. Construction is
// offline; execution requires an explicitly injected transport.

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
	pinLevel := judgment.PinProviderExactModel
	if request.Model.Provider == "openrouter" {
		pinLevel = judgment.PinRouterModelID
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
			"pin_level":                    pinLevel,
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
		TransportProvider:    caps.Model.TransportProvider,
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

// ReceiptStore 是本 owner 的持久执行回执存储，实现公共 SDK 的
// providers.ReceiptStore。它只保存可恢复的执行事实（owner/project/request/
// attempt 身份、输入与 profile digest、状态与已校验的 neutral result），
// 不保存输入正文、凭据或 provider 原始 payload。
//
// 数据放在 vault 自己的 `.pinax/api/judgment.sqlite`，与 operation ledger
// 同级、彼此独立：judgment 回执不会写进 operation 表，operation 也不读取
// judgment 状态。SQLite 一律经 internal/sqlitedsn 打开。
//
// 语义约束（与 providers.Runtime 对齐）：
//   - Claim 是原子的 insert-if-absent；同一身份再次 Claim 返回已存回执，
//     由调用方比较 input/profile digest，不一致即冲突。
//   - Finish 保留原身份绑定，只更新状态、错误码与结果。
//   - 未完成的 claim 永远粘滞：重启无法证明 provider 是否已接收，因此
//     只能通过显式新 attempt 重新发起。
type ReceiptStore struct {
	db *gorm.DB
}

// JudgmentReceiptRow 是 judgment 回执行。主键是 owner/project/request/
// attempt 四元组；State 取值 dispatching/completed/failed/unknown。
type JudgmentReceiptRow struct {
	Owner           string    `gorm:"primaryKey;size:64" json:"owner"`
	Project         string    `gorm:"primaryKey;size:128" json:"project"`
	RequestID       string    `gorm:"primaryKey;size:128" json:"request_id"`
	AttemptID       string    `gorm:"primaryKey;size:128" json:"attempt_id"`
	InputDigest     string    `gorm:"size:128" json:"input_digest"`
	ProfileDigest   string    `gorm:"size:128" json:"profile_digest"`
	State           string    `gorm:"size:32" json:"state"`
	ErrorCode       string    `gorm:"size:64" json:"error_code,omitempty"`
	SubmissionState string    `gorm:"size:32" json:"submission_state,omitempty"`
	Result          []byte    `json:"-"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TableName 固定回执表名，避免 GORM 复数化漂移。
func (JudgmentReceiptRow) TableName() string { return "judgment_receipts" }

// OpenReceiptStore 打开（或创建）vault 内的 judgment 回执数据库并迁移
// 加性表。它不触发任何 provider 调用，也不读取凭据。
func OpenReceiptStore(root string) (*ReceiptStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("judgment_receipt_store_root_required")
	}
	dir := filepath.Join(root, ".pinax", "api")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("judgment_receipt_store_unavailable: %w", err)
	}
	path := filepath.Join(dir, "judgment.sqlite")
	db, err := sqlitedsn.Open(path)
	if err != nil {
		return nil, fmt.Errorf("judgment_receipt_store_unavailable: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		return nil, fmt.Errorf("judgment_receipt_store_unavailable: %w", err)
	}
	store, err := OpenReceiptStoreDB(db)
	if err != nil {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		return nil, err
	}
	return store, nil
}

// OpenReceiptStoreDB 在既有连接上迁移并构造回执存储（测试与共享连接用）。
func OpenReceiptStoreDB(db *gorm.DB) (*ReceiptStore, error) {
	if db == nil {
		return nil, errors.New("judgment_receipt_store_unavailable")
	}
	if err := db.AutoMigrate(&JudgmentReceiptRow{}); err != nil {
		return nil, fmt.Errorf("judgment_receipt_store_migration_failed: %w", err)
	}
	return &ReceiptStore{db: db}, nil
}

// Close 关闭底层连接池。
func (s *ReceiptStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Claim 实现 providers.ReceiptStore：原子 insert-if-absent。已存在的回执
// 原样返回且 claimed=false，调用方负责 digest/身份冲突判定。
func (s *ReceiptStore) Claim(ctx context.Context, receipt providers.Receipt) (providers.Receipt, bool, error) {
	if err := validateReceiptKey(receipt); err != nil {
		return providers.Receipt{}, false, err
	}
	row := receiptRowFromProvider(receipt)
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return providers.Receipt{}, false, fmt.Errorf("judgment_receipt_claim_failed: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return receipt, true, nil
	}
	existing, found, err := s.Get(ctx, receipt.Owner, receipt.Project, receipt.RequestID, receipt.AttemptID)
	if err != nil {
		return providers.Receipt{}, false, err
	}
	if !found {
		return providers.Receipt{}, false, errors.New("judgment_receipt_claim_lost")
	}
	return existing, false, nil
}

// Finish 实现 providers.ReceiptStore：保留原身份绑定，只更新终态。
// 不存在的回执不静默插入（那会让未经 claim 的执行看起来已记录）。
func (s *ReceiptStore) Finish(ctx context.Context, receipt providers.Receipt) error {
	if err := validateReceiptKey(receipt); err != nil {
		return err
	}
	updates := map[string]any{
		"state":            receipt.State,
		"error_code":       receipt.ErrorCode,
		"submission_state": receipt.SubmissionState,
		"updated_at":       receipt.UpdatedAt,
	}
	if len(receipt.Result) > 0 {
		updates["result"] = []byte(receipt.Result)
	}
	result := s.db.WithContext(ctx).Model(&JudgmentReceiptRow{}).
		Where("owner = ? AND project = ? AND request_id = ? AND attempt_id = ?",
			receipt.Owner, receipt.Project, receipt.RequestID, receipt.AttemptID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("judgment_receipt_finish_failed: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		// 相同值的 UPDATE 可能报告 0 行变化；先确认回执是否存在，再决定是否失败。
		if _, found, err := s.Get(ctx, receipt.Owner, receipt.Project, receipt.RequestID, receipt.AttemptID); err != nil {
			return err
		} else if found {
			return nil
		}
		return errors.New("judgment_receipt_not_found")
	}
	return nil
}

// Get 读取一条回执；不存在时返回 found=false。
func (s *ReceiptStore) Get(ctx context.Context, owner, project, requestID, attemptID string) (providers.Receipt, bool, error) {
	var row JudgmentReceiptRow
	err := s.db.WithContext(ctx).
		Where("owner = ? AND project = ? AND request_id = ? AND attempt_id = ?", owner, project, requestID, attemptID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return providers.Receipt{}, false, nil
	}
	if err != nil {
		return providers.Receipt{}, false, fmt.Errorf("judgment_receipt_get_failed: %w", err)
	}
	return row.providerReceipt(), true, nil
}

func validateReceiptKey(receipt providers.Receipt) error {
	if strings.TrimSpace(receipt.Owner) == "" || strings.TrimSpace(receipt.Project) == "" ||
		strings.TrimSpace(receipt.RequestID) == "" || strings.TrimSpace(receipt.AttemptID) == "" {
		return errors.New("judgment_receipt_identity_required")
	}
	return nil
}

func receiptRowFromProvider(receipt providers.Receipt) JudgmentReceiptRow {
	row := JudgmentReceiptRow{
		Owner:           receipt.Owner,
		Project:         receipt.Project,
		RequestID:       receipt.RequestID,
		AttemptID:       receipt.AttemptID,
		InputDigest:     receipt.InputDigest,
		ProfileDigest:   receipt.ProfileDigest,
		State:           receipt.State,
		ErrorCode:       receipt.ErrorCode,
		SubmissionState: receipt.SubmissionState,
		CreatedAt:       receipt.CreatedAt,
		UpdatedAt:       receipt.UpdatedAt,
	}
	if len(receipt.Result) > 0 {
		row.Result = []byte(receipt.Result)
	}
	return row
}

func (row JudgmentReceiptRow) providerReceipt() providers.Receipt {
	receipt := providers.Receipt{
		SchemaVersion:   "1.0",
		Owner:           row.Owner,
		Project:         row.Project,
		RequestID:       row.RequestID,
		AttemptID:       row.AttemptID,
		InputDigest:     row.InputDigest,
		ProfileDigest:   row.ProfileDigest,
		State:           row.State,
		ErrorCode:       row.ErrorCode,
		SubmissionState: row.SubmissionState,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
	if len(row.Result) > 0 {
		receipt.Result = append([]byte(nil), row.Result...)
	}
	return receipt
}
