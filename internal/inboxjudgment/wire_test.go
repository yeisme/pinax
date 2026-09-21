package inboxjudgment

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// wire_test.go 覆盖冻结合同 seam：错误分类、请求/结果校验、能力快照与
// fixture transport 的离线行为。

func TestJudgmentErrorClassCoversAllStableCodes(t *testing.T) {
	cases := []struct {
		code            string
		submissionState string
		retryClass      string
	}{
		{JudgmentCodeUnsupportedCapability, JudgmentSubmissionNotSubmitted, JudgmentRetryNever},
		{JudgmentCodeInvalidRequest, JudgmentSubmissionNotSubmitted, JudgmentRetryNever},
		{JudgmentCodeUnauthorized, JudgmentSubmissionNotSubmitted, JudgmentRetryNever},
		{JudgmentCodeRateLimited, JudgmentSubmissionNotSubmitted, JudgmentRetrySafeBeforeSubmit},
		{JudgmentCodeUnavailable, JudgmentSubmissionUnknown, JudgmentRetryReconcileFirst},
		{JudgmentCodeDeadlineExceeded, JudgmentSubmissionUnknown, JudgmentRetryReconcileFirst},
		{JudgmentCodeInvalidResponse, JudgmentSubmissionSubmitted, JudgmentRetryNever},
		{JudgmentCodeOutcomeUnknown, JudgmentSubmissionUnknown, JudgmentRetryReconcileFirst},
	}
	if len(cases) != 8 {
		t.Fatalf("contract v1 must classify exactly 8 stable codes, got %d", len(cases))
	}
	for _, tc := range cases {
		submissionState, retryClass := JudgmentErrorClass(tc.code)
		if submissionState != tc.submissionState || retryClass != tc.retryClass {
			t.Errorf("JudgmentErrorClass(%s) = %s/%s, want %s/%s", tc.code, submissionState, retryClass, tc.submissionState, tc.retryClass)
		}
	}
	// 未知码 fail closed 到 reconcile_first。
	submissionState, retryClass := JudgmentErrorClass("mystery")
	if submissionState != JudgmentSubmissionUnknown || retryClass != JudgmentRetryReconcileFirst {
		t.Errorf("unknown code must fail closed to unknown/reconcile_first, got %s/%s", submissionState, retryClass)
	}
}

func TestJudgmentTransportErrorIsOutcomeUnknown(t *testing.T) {
	unknown := &JudgmentTransportError{Code: JudgmentCodeOutcomeUnknown, SubmissionState: JudgmentSubmissionUnknown}
	if !unknown.IsOutcomeUnknown() {
		t.Error("outcome_unknown must report IsOutcomeUnknown")
	}
	timeoutSubmitted := &JudgmentTransportError{Code: JudgmentCodeDeadlineExceeded, SubmissionState: JudgmentSubmissionUnknown}
	if !timeoutSubmitted.IsOutcomeUnknown() {
		t.Error("deadline_exceeded after submission must report IsOutcomeUnknown")
	}
	timeoutBefore := &JudgmentTransportError{Code: JudgmentCodeDeadlineExceeded, SubmissionState: JudgmentSubmissionNotSubmitted}
	if timeoutBefore.IsOutcomeUnknown() {
		t.Error("deadline_exceeded before submission must not report IsOutcomeUnknown")
	}
}

func TestJudgmentCapabilitiesValidate(t *testing.T) {
	caps := FixtureJudgmentCapabilities("validate")
	if err := caps.Validate(); err != nil {
		t.Fatalf("fixture capabilities must validate: %v", err)
	}
	if !caps.SupportsPrimitive(JudgmentPrimitiveBinary) || !caps.SupportsPrimitive(JudgmentPrimitiveOrdinalScore) || !caps.SupportsPrimitive(JudgmentPrimitiveChoice) {
		t.Error("fixture capabilities must declare all three primitives")
	}
	if caps.ProbabilityAvailable || caps.ConfidenceAvailable {
		t.Error("fixture capabilities must keep probability/confidence explicitly unavailable")
	}
	wrongVersion := caps
	wrongVersion.SchemaVersion = "2.0"
	if err := wrongVersion.Validate(); err == nil {
		t.Error("schema version mismatch must fail validation")
	}
	noModel := caps
	noModel.Model = ""
	if err := noModel.Validate(); err == nil {
		t.Error("missing exact model pin must fail validation")
	}
	noText := caps
	noText.Modalities = []string{"image"}
	if err := noText.Validate(); err == nil {
		t.Error("missing text modality must fail validation")
	}
}

// judgmentWireFixtureRequest 构造一个最小合法 wire 请求。
func judgmentWireFixtureRequest() JudgmentWireRequest {
	request := JudgmentWireRequest{
		SchemaVersion: JudgmentWireSchemaVersion,
		RequestID:     "judgment-req-fixture",
		AttemptID:     "judgment-attempt-fixture",
		Scope:         JudgmentWireScope{Owner: "pinax", Project: "inbox-judgment", Principal: "sha256:principal"},
		Model:         JudgmentWireModel{Provider: "judgment", Model: "fixture:wire"},
		QuestionSet:   JudgmentWireVersionedRef{ID: InboxJudgmentQuestionSetID, Version: InboxJudgmentQuestionSetVersion, Digest: InboxJudgmentQuestionSetDigest()},
		PolicyRef:     JudgmentWireVersionedRef{ID: InboxJudgmentPolicyID, Version: InboxJudgmentPolicyVersion, Digest: InboxJudgmentPolicyDigest()},
		Sources: []JudgmentWireSource{
			{SourceID: "inbox:n1", Revision: "rev-1", InlineText: "inbox excerpt"},
			{SourceID: "note:n2", Revision: "rev-2", InlineText: "candidate excerpt"},
		},
		Candidates: []JudgmentWireCandidate{{CandidateID: "n2", SourceID: "note:n2"}},
		Questions: []JudgmentWireQuestion{
			{QuestionID: "relation", Primitive: JudgmentPrimitiveChoice, Text: "relation?", CandidateIDs: []string{"n2"}, Required: true, OptionIDs: []string{RelationDuplicate, RelationComplement, RelationContradiction, RelationUnrelated}},
		},
		Limits: JudgmentWireLimits{DeadlineMS: 5000, MaxCandidates: 8},
	}
	if err := request.Validate(); err != nil {
		panic(err)
	}
	return request
}

func TestJudgmentWireRequestValidate(t *testing.T) {
	base := judgmentWireFixtureRequest()
	if err := base.Validate(); err != nil {
		t.Fatalf("base request must validate: %v", err)
	}
	pairs := base.ExpectedPairs()
	if len(pairs) != 1 || pairs[0] != [2]string{"n2", "relation"} {
		t.Fatalf("expected pairs mismatch: %v", pairs)
	}
	// snake_case wire：JSON key 必须是 snake_case（冻结合同形状）。
	encoded, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{`"schema_version"`, `"request_id"`, `"attempt_id"`, `"question_set"`, `"policy_ref"`, `"inline_text"`, `"candidate_id"`, `"question_id"`, `"option_ids"`, `"deadline_ms"`} {
		if !strings.Contains(string(encoded), key) {
			t.Errorf("wire JSON must carry snake_case key %s", key)
		}
	}
	if strings.Contains(string(encoded), `"schemaVersion"`) {
		t.Error("wire JSON must not carry camelCase keys")
	}

	bad := base
	bad.SchemaVersion = "0.9"
	if err := bad.Validate(); err == nil {
		t.Error("schema version must be 1.0")
	}
	bad = base
	bad.RequestID = ""
	if err := bad.Validate(); err == nil {
		t.Error("request_id is required")
	}
	bad = base
	bad.QuestionSet.Digest = ""
	if err := bad.Validate(); err == nil {
		t.Error("question_set digest is required")
	}
	bad = base
	bad.Sources[1].Revision = ""
	if err := bad.Validate(); err == nil {
		t.Error("source revision is required")
	}
	bad = base
	bad.Candidates[0].SourceID = "note:missing"
	if err := bad.Validate(); err == nil {
		t.Error("candidate must bind to an existing source")
	}
	bad = base
	bad.Questions[0].CandidateIDs = nil
	if err := bad.Validate(); err == nil {
		t.Error("questions need explicit candidate bindings")
	}
}

func TestValidateJudgmentWireResult(t *testing.T) {
	request := judgmentWireFixtureRequest()
	valid := JudgmentWireResult{
		SchemaVersion:   JudgmentWireSchemaVersion,
		RequestID:       request.RequestID,
		AttemptID:       request.AttemptID,
		InputDigest:     request.CanonicalDigest(),
		ResolvedModel:   request.Model,
		ExecutionStatus: "succeeded",
		Items: []JudgmentWireItem{{
			CandidateID: "n2", QuestionID: "relation", AnswerStatus: "answered",
			Value: &JudgmentWireAnswerValue{Choice: RelationComplement},
		}},
	}
	if err := ValidateJudgmentWireResult(request, valid); err != nil {
		t.Fatalf("valid result rejected: %v", err)
	}
	// duplicate pair。
	dup := valid
	dup.Items = append(dup.Items, valid.Items[0])
	if err := ValidateJudgmentWireResult(request, dup); err == nil {
		t.Error("duplicate pair answer must be rejected")
	}
	// 超出答案域的 choice。
	outOfDomain := valid
	outOfDomain.Items = []JudgmentWireItem{{
		CandidateID: "n2", QuestionID: "relation", AnswerStatus: "answered",
		Value: &JudgmentWireAnswerValue{Choice: "maybe"},
	}}
	if err := ValidateJudgmentWireResult(request, outOfDomain); err == nil {
		t.Error("choice outside the declared domain must be rejected")
	}
	// 未在请求中声明的 pair。
	unexpected := valid
	unexpected.Items = append(unexpected.Items, JudgmentWireItem{
		CandidateID: "n3", QuestionID: "relation", AnswerStatus: "answered",
		Value: &JudgmentWireAnswerValue{Choice: RelationUnrelated},
	})
	if err := ValidateJudgmentWireResult(request, unexpected); err == nil {
		t.Error("answer for an unexpected pair must be rejected")
	}
	// 弃答携带值。
	abstainedWithValue := valid
	abstainedWithValue.Items = []JudgmentWireItem{{
		CandidateID: "n2", QuestionID: "relation", AnswerStatus: "abstained",
		Value: &JudgmentWireAnswerValue{Choice: RelationUnrelated},
	}}
	if err := ValidateJudgmentWireResult(request, abstainedWithValue); err == nil {
		t.Error("abstained item with a value must be rejected")
	}
	// 缺失 pair。
	missingPair := valid
	missingPair.Items = nil
	if err := ValidateJudgmentWireResult(request, missingPair); err == nil {
		t.Error("missing expected pair must be rejected")
	}
	// execution unknown → outcome_unknown 分类。
	unknown := valid
	unknown.ExecutionStatus = "unknown"
	err := ValidateJudgmentWireResult(request, unknown)
	if err == nil {
		t.Fatal("unknown execution status must fail")
	}
	transportError := &JudgmentTransportError{}
	if !errors.As(err, &transportError) || transportError.Code != JudgmentCodeOutcomeUnknown {
		t.Errorf("unknown execution status must map to outcome_unknown, got %v", err)
	}
	// digest 不绑定 attempt。
	mismatch := valid
	mismatch.AttemptID = "other-attempt"
	if err := ValidateJudgmentWireResult(request, mismatch); err == nil {
		t.Error("result not bound to the request attempt must be rejected")
	}
}

func TestJudgmentClientSingleTransportCall(t *testing.T) {
	transport := NewFixtureTransport("client")
	client := &JudgmentClient{Transport: transport, Capabilities: FixtureJudgmentCapabilities("client")}
	request := judgmentWireFixtureRequest()
	result, err := client.Evaluate(context.Background(), request)
	if err != nil {
		t.Fatalf("client evaluate: %v", err)
	}
	if transport.EvaluateCalls() != 1 || transport.DescribeCalls() != 0 {
		t.Errorf("one client evaluate must map to exactly one transport evaluate, got evaluate=%d describe=%d", transport.EvaluateCalls(), transport.DescribeCalls())
	}
	if result.ExecutionStatus != "succeeded" {
		t.Errorf("fixture result execution status = %s", result.ExecutionStatus)
	}
	// 不支持的原语：预检拒绝、零 transport 调用。
	unsupported := FixtureJudgmentCapabilities("client")
	unsupported.Primitives = []string{JudgmentPrimitiveBinary}
	unsupportedClient := &JudgmentClient{Transport: transport, Capabilities: unsupported}
	if _, err := unsupportedClient.Evaluate(context.Background(), request); err == nil {
		t.Error("unsupported primitive must be rejected before the transport call")
	}
	if transport.EvaluateCalls() != 1 {
		t.Errorf("unsupported primitive must not consume a transport call, evaluate=%d", transport.EvaluateCalls())
	}
	// 无 transport：显式错误。
	if _, err := (&JudgmentClient{}).Evaluate(context.Background(), request); err == nil {
		t.Error("missing transport must fail explicitly")
	}
}

func TestFixtureTransportBehaviors(t *testing.T) {
	for _, behavior := range []string{"unavailable", "timeout_after_submit", "outcome_unknown", "invalid_response"} {
		transport := &FixtureTransport{Name: behavior, Behavior: behavior}
		request := judgmentWireFixtureRequest()
		result, err := transport.Evaluate(context.Background(), request)
		switch behavior {
		case "unavailable", "timeout_after_submit", "outcome_unknown":
			if err == nil {
				t.Errorf("%s: expected transport error", behavior)
				continue
			}
			var transportError *JudgmentTransportError
			if !errors.As(err, &transportError) {
				t.Errorf("%s: error must be a JudgmentTransportError, got %T", behavior, err)
			}
		case "invalid_response":
			if err != nil {
				t.Errorf("%s: fixture returns a malformed result, not an error: %v", behavior, err)
				continue
			}
			if verr := ValidateJudgmentWireResult(request, result); verr == nil {
				t.Errorf("%s: malformed result must fail validation", behavior)
			}
		}
	}
}

func TestClassifyJudgmentTransportErrorFillsDefaults(t *testing.T) {
	bare := &JudgmentTransportError{Code: JudgmentCodeRateLimited}
	classified := classifyJudgmentTransportError(bare)
	transportError, ok := classified.(*JudgmentTransportError)
	if !ok {
		t.Fatalf("classification must keep the transport error type")
	}
	if transportError.SubmissionState != JudgmentSubmissionNotSubmitted || transportError.RetryClass != JudgmentRetrySafeBeforeSubmit {
		t.Errorf("defaults must be filled from the code, got %s/%s", transportError.SubmissionState, transportError.RetryClass)
	}
	// 非 transport 错误归类为 unknown/reconcile_first，不吞原始信息。
	wrapped := classifyJudgmentTransportError(errors.New("socket exploded"))
	transportError, ok = wrapped.(*JudgmentTransportError)
	if !ok || transportError.Code != JudgmentCodeUnavailable || transportError.SubmissionState != JudgmentSubmissionUnknown {
		t.Errorf("unclassified error must fail closed to unavailable/unknown, got %+v", wrapped)
	}
	if transportError.DiagnosticRef == "" {
		t.Error("unclassified error must carry a diagnostic ref")
	}
}
