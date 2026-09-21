package judgment

import (
	"fmt"
	"math"
	"reflect"
)

// Execution status values; unknown values fail closed as protocol errors.
const (
	ExecutionSucceeded = "succeeded"
	ExecutionPartial   = "partial"
	ExecutionFailed    = "failed"
	ExecutionUnknown   = "unknown"
)

// Answer status values.
const (
	AnswerAnswered  = "answered"
	AnswerAbstained = "abstained"
	AnswerError     = "error"
)

// Confidence provenance values keep provider confidence, choice probability
// and domain evidence confidence from filling each other.
const (
	ProvenanceProviderSelfReport = "provider_self_report"
	ProvenanceCalibrated         = "calibrated"
	ProvenanceDerived            = "derived"
)

// Result validation reason tokens (shared with TypeScript and vectors).
const (
	ReasonIdentityMismatch         = "identity_mismatch"
	ReasonInputDigestMismatch      = "input_digest_mismatch"
	ReasonResolvedModelMismatch    = "resolved_model_mismatch"
	ReasonExecutionStatusInvalid   = "execution_status_invalid"
	ReasonItemsMissing             = "items_missing"
	ReasonItemsDuplicate           = "items_duplicate"
	ReasonItemsUnknownPair         = "items_unknown_pair"
	ReasonItemInvalid              = "item_invalid"
	ReasonAnswerStatusInvalid      = "answer_status_invalid"
	ReasonValueMismatch            = "value_mismatch"
	ReasonValuePresentOnUnanswered = "value_present_on_unanswered"
	ReasonDistributionKeys         = "distribution_keys"
	ReasonDistributionNegative     = "distribution_negative"
	ReasonDistributionSum          = "distribution_sum"
	ReasonDistributionForbidden    = "distribution_forbidden"
	ReasonConfidenceInvalid        = "confidence_invalid"
	ReasonProbabilityTrueInvalid   = "probability_true_invalid"
	ReasonNormalizationMissing     = "normalization_missing"
	ReasonNormalizationInvalid     = "normalization_invalid"
	ReasonUsageInvalid             = "usage_invalid"
	ReasonLatencyInvalid           = "latency_invalid"
	ReasonSourceRefsInvalid        = "source_refs_invalid"
	ReasonStatusItemsConflict      = "status_items_conflict"
	ReasonResultInvalid            = "result_invalid"
)

// PrecisionPolicy is the versioned tolerance policy the caller applies when
// validating distributions. The SDK ships a default but never hardcodes a
// provider-specific tolerance.
type PrecisionPolicy struct {
	Version         string
	AbsSumTolerance float64
}

// DefaultPrecisionPolicy is the generic v1 policy (1e-6 absolute sum
// tolerance). Provider-specific precision belongs to adapter-declared
// normalization evidence, not to this SDK.
func DefaultPrecisionPolicy() PrecisionPolicy {
	return PrecisionPolicy{Version: "yeisme.judgment.precision.v1", AbsSumTolerance: 1e-6}
}

// Confidence carries an explicit provenance; it stays null when the provider
// has no calibrated confidence.
type Confidence struct {
	Value      float64
	Provenance string
}

// Normalization documents the adapter-versioned rule reconciling
// provider_value and derived_value when they differ.
type Normalization struct {
	Version string
	Rule    string
}

// AnswerValue is the adoptable answer; exactly one arm matches the primitive.
type AnswerValue struct {
	OptionID *string
	LevelID  *string
	Binary   *bool
}

// Usage reports known usage facts; unknown stays null and never becomes zero.
type Usage struct {
	InputTokens  *int64
	OutputTokens *int64
	Cost         *float64
}

// SourceRef traces an answer back to request sources.
type SourceRef struct {
	SourceID string
	Revision string
}

// Item is one (candidate_id, question_id) judgment.
type Item struct {
	CandidateID     string
	QuestionID      string
	AnswerStatus    string
	Value           *AnswerValue
	Distribution    map[string]float64
	Confidence      *Confidence
	ProbabilityTrue *float64
	ReasonCode      *string
	SourceRefs      []SourceRef
	ProviderValue   map[string]any
	DerivedValue    map[string]any
	Normalization   *Normalization
}

// Result is the validated outcome of one attempt.
type Result struct {
	SchemaVersion     string
	RequestID         string
	AttemptID         string
	InputDigest       string
	ExecutionStatus   string
	ResolvedModel     ModelIdentity
	Items             []Item
	Usage             *Usage
	LatencyMS         *int64
	ProviderRequestID *string
	Extensions        map[string]any

	tree any // retained wire tree for evidence persistence
}

// WireJSON re-encodes the validated request tree for transport or evidence.
func (r *Request) WireJSON() ([]byte, error) {
	if r.tree == nil {
		return nil, validationErr(ReasonRequestInvalid, "request_tree_missing")
	}
	return MarshalTree(r.tree)
}

// ItemByPair looks an item up by (candidate_id, question_id).
func (r *Result) ItemByPair(candidateID, questionID string) *Item {
	for i := range r.Items {
		if r.Items[i].CandidateID == candidateID && r.Items[i].QuestionID == questionID {
			return &r.Items[i]
		}
	}
	return nil
}

// ValidateOptions carries validation inputs for results.
type ValidateOptions struct {
	Policy PrecisionPolicy
}

// ParseResult strictly decodes and validates result bytes against the
// originating request and precision policy.
func ParseResult(data []byte, req *Request, policy PrecisionPolicy) (*Result, *ValidationError) {
	tree, err := ParseTree(data)
	if err != nil {
		return nil, validationErr(ReasonResultInvalid, "result_not_json")
	}
	return parseResultTree(tree, req, policy)
}

func parseResultTree(tree any, req *Request, policy PrecisionPolicy) (*Result, *ValidationError) {
	obj, ok := tree.(map[string]any)
	if !ok {
		return nil, validationErr(ReasonResultInvalid, "result_not_object")
	}
	if err := requireExactKeys(obj, []string{
		"schema_version", "request_id", "attempt_id", "input_digest",
		"execution_status", "resolved_model", "items", "usage", "latency_ms",
		"provider_request_id", "extensions",
	}, ReasonResultInvalid); err != nil {
		return nil, err
	}
	if str, _ := obj["schema_version"].(string); str != "1.0" {
		return nil, validationErr(ReasonSchemaVersion, "schema_version_must_be_1.0")
	}
	res := &Result{SchemaVersion: "1.0", tree: tree}

	res.RequestID, ok = obj["request_id"].(string)
	if !ok || res.RequestID != req.RequestID {
		return nil, validationErr(ReasonIdentityMismatch, "request_id_mismatch")
	}
	res.AttemptID, ok = obj["attempt_id"].(string)
	if !ok || res.AttemptID != req.AttemptID {
		return nil, validationErr(ReasonIdentityMismatch, "attempt_id_mismatch")
	}
	res.InputDigest, ok = obj["input_digest"].(string)
	if !ok || !digestPattern.MatchString(res.InputDigest) {
		return nil, validationErr(ReasonInputDigestMismatch, "input_digest_malformed")
	}
	wantDigest, err := req.InputDigest()
	if err != nil {
		return nil, validationErr(ReasonRequestInvalid, "request_digest_unavailable")
	}
	if res.InputDigest != wantDigest {
		return nil, validationErr(ReasonInputDigestMismatch, "input_digest_not_bound_to_request")
	}
	res.ExecutionStatus, ok = obj["execution_status"].(string)
	if !ok {
		return nil, validationErr(ReasonExecutionStatusInvalid, "execution_status_not_string")
	}
	switch res.ExecutionStatus {
	case ExecutionSucceeded, ExecutionPartial, ExecutionFailed, ExecutionUnknown:
	default:
		return nil, validationErr(ReasonExecutionStatusInvalid, "execution_status_unknown_value")
	}
	model, verr := parseModelIdentity(obj["resolved_model"], ReasonCapabilitiesInvalid)
	if verr != nil {
		return nil, verr
	}
	if model.RequestedModel != req.Model.RequestedModel {
		return nil, validationErr(ReasonResolvedModelMismatch, "requested_model_rewritten")
	}
	res.ResolvedModel = model

	itemsRaw, ok := obj["items"].([]any)
	if !ok {
		return nil, validationErr(ReasonItemInvalid, "items_not_array")
	}
	questions := req.questionMap()
	sourceRevisions := make(map[string]string, len(req.Sources))
	for _, s := range req.Sources {
		sourceRevisions[s.SourceID] = s.Revision
	}
	expected := make(map[[2]string]bool)
	for _, pair := range req.ExpectedPairs() {
		expected[pair] = true
	}
	seen := make(map[[2]string]bool, len(itemsRaw))
	res.Items = make([]Item, 0, len(itemsRaw))
	for _, raw := range itemsRaw {
		iobj, ok := raw.(map[string]any)
		if !ok {
			return nil, validationErr(ReasonItemInvalid, "item_not_object")
		}
		if err := requireExactKeys(iobj, []string{
			"candidate_id", "question_id", "answer_status", "value",
			"distribution", "confidence", "probability_true", "reason_code",
			"source_refs", "provider_value", "derived_value", "normalization",
		}, ReasonItemInvalid); err != nil {
			// A dropped normalization key on a divergent provider/derived
			// pair is a normalization problem, not a structural one.
			if _, present := iobj["normalization"]; !present {
				pv, pvOK := iobj["provider_value"].(map[string]any)
				dv, dvOK := iobj["derived_value"].(map[string]any)
				if pvOK && dvOK && !reflect.DeepEqual(pv, dv) {
					return nil, validationErr(ReasonNormalizationMissing, "provider_derived_divergence_requires_normalization")
				}
			}
			return nil, err
		}
		var item Item
		item.CandidateID, ok = iobj["candidate_id"].(string)
		if !ok || !idPattern.MatchString(item.CandidateID) {
			return nil, validationErr(ReasonItemInvalid, "item_candidate_id_invalid")
		}
		item.QuestionID, ok = iobj["question_id"].(string)
		if !ok || !idPattern.MatchString(item.QuestionID) {
			return nil, validationErr(ReasonItemInvalid, "item_question_id_invalid")
		}
		pair := [2]string{item.CandidateID, item.QuestionID}
		if !expected[pair] {
			return nil, validationErr(ReasonItemsUnknownPair, fmt.Sprintf("unexpected_pair:%s/%s", pair[0], pair[1]))
		}
		if seen[pair] {
			return nil, validationErr(ReasonItemsDuplicate, fmt.Sprintf("duplicate_pair:%s/%s", pair[0], pair[1]))
		}
		seen[pair] = true
		question, known := questions[item.QuestionID]
		if !known {
			return nil, validationErr(ReasonItemsUnknownPair, "question_not_in_request")
		}
		item.AnswerStatus, ok = iobj["answer_status"].(string)
		if !ok {
			return nil, validationErr(ReasonAnswerStatusInvalid, "answer_status_not_string")
		}
		switch item.AnswerStatus {
		case AnswerAnswered, AnswerAbstained, AnswerError:
		default:
			return nil, validationErr(ReasonAnswerStatusInvalid, "answer_status_unknown_value")
		}
		if item.AnswerStatus != AnswerAnswered {
			// Unanswered items must carry no adoptable value at all; shape
			// problems are secondary to the presence rule.
			if iobj["value"] != nil {
				return nil, validationErr(ReasonValuePresentOnUnanswered, "value_must_be_null_unless_answered")
			}
		} else {
			value, verr := parseAnswerValue(iobj["value"], question)
			if verr != nil {
				return nil, verr
			}
			if value == nil {
				return nil, validationErr(ReasonValueMismatch, "answered_item_requires_value")
			}
			item.Value = value
		}
		if verr := parseDistribution(iobj["distribution"], question, item.AnswerStatus, policy, &item); verr != nil {
			return nil, verr
		}
		if raw := iobj["confidence"]; raw != nil {
			cobj, ok := raw.(map[string]any)
			if !ok {
				return nil, validationErr(ReasonConfidenceInvalid, "confidence_not_object")
			}
			if err := requireExactKeys(cobj, []string{"value", "provenance"}, ReasonConfidenceInvalid); err != nil {
				return nil, err
			}
			cv, nerr := numberValue(cobj["value"])
			if nerr != nil || cv < 0 || cv > 1 {
				return nil, validationErr(ReasonConfidenceInvalid, "confidence_value_out_of_range")
			}
			provenance, _ := cobj["provenance"].(string)
			switch provenance {
			case ProvenanceProviderSelfReport, ProvenanceCalibrated, ProvenanceDerived:
			default:
				return nil, validationErr(ReasonConfidenceInvalid, "confidence_provenance_missing_or_unknown")
			}
			if item.AnswerStatus == AnswerError {
				return nil, validationErr(ReasonConfidenceInvalid, "confidence_not_allowed_on_error_item")
			}
			item.Confidence = &Confidence{Value: cv, Provenance: provenance}
		}
		if raw := iobj["probability_true"]; raw != nil {
			if question.Primitive != PrimitiveBinary {
				return nil, validationErr(ReasonProbabilityTrueInvalid, "probability_true_only_for_binary")
			}
			pv, nerr := numberValue(raw)
			if nerr != nil || pv < 0 || pv > 1 {
				return nil, validationErr(ReasonProbabilityTrueInvalid, "probability_true_out_of_range")
			}
			if item.AnswerStatus != AnswerAnswered {
				return nil, validationErr(ReasonProbabilityTrueInvalid, "probability_true_requires_answered")
			}
			item.ProbabilityTrue = &pv
		}
		if raw := iobj["reason_code"]; raw != nil {
			str, ok := raw.(string)
			if !ok || !reasonPattern.MatchString(str) {
				return nil, validationErr(ReasonItemInvalid, "reason_code_invalid")
			}
			item.ReasonCode = &str
		}
		refs, ok := iobj["source_refs"].([]any)
		if !ok {
			return nil, validationErr(ReasonSourceRefsInvalid, "source_refs_not_array")
		}
		for _, rraw := range refs {
			robj, ok := rraw.(map[string]any)
			if !ok {
				return nil, validationErr(ReasonSourceRefsInvalid, "source_ref_not_object")
			}
			if err := requireExactKeys(robj, []string{"source_id", "revision"}, ReasonSourceRefsInvalid); err != nil {
				return nil, err
			}
			sid, ok := robj["source_id"].(string)
			rev, ok2 := robj["revision"].(string)
			if !ok || !ok2 || sid == "" || rev == "" {
				return nil, validationErr(ReasonSourceRefsInvalid, "source_ref_fields_invalid")
			}
			wantRev, knownSource := sourceRevisions[sid]
			if !knownSource || wantRev != rev {
				return nil, validationErr(ReasonSourceRefsInvalid, "source_ref_not_in_request")
			}
			item.SourceRefs = append(item.SourceRefs, SourceRef{SourceID: sid, Revision: rev})
		}
		if raw := iobj["provider_value"]; raw != nil {
			m, ok := raw.(map[string]any)
			if !ok {
				return nil, validationErr(ReasonItemInvalid, "provider_value_not_object")
			}
			item.ProviderValue = m
		}
		if raw := iobj["derived_value"]; raw != nil {
			m, ok := raw.(map[string]any)
			if !ok {
				return nil, validationErr(ReasonItemInvalid, "derived_value_not_object")
			}
			item.DerivedValue = m
		}
		if raw := iobj["normalization"]; raw != nil {
			nobj, ok := raw.(map[string]any)
			if !ok {
				return nil, validationErr(ReasonNormalizationInvalid, "normalization_not_object")
			}
			if err := requireExactKeys(nobj, []string{"version", "rule"}, ReasonNormalizationInvalid); err != nil {
				return nil, err
			}
			version, _ := nobj["version"].(string)
			rule, _ := nobj["rule"].(string)
			if version == "" || len(version) > 128 || rule == "" || len(rule) > 128 {
				return nil, validationErr(ReasonNormalizationInvalid, "normalization_fields_invalid")
			}
			item.Normalization = &Normalization{Version: version, Rule: rule}
		}
		if item.ProviderValue != nil && item.DerivedValue != nil && !reflect.DeepEqual(item.ProviderValue, item.DerivedValue) {
			if item.Normalization == nil {
				return nil, validationErr(ReasonNormalizationMissing, "provider_derived_divergence_requires_normalization")
			}
		}
		res.Items = append(res.Items, item)
	}
	if res.ExecutionStatus != ExecutionUnknown {
		for pair := range expected {
			if !seen[pair] {
				return nil, validationErr(ReasonItemsMissing, fmt.Sprintf("missing_pair:%s/%s", pair[0], pair[1]))
			}
		}
	}
	if verr := checkStatusItems(res); verr != nil {
		return nil, verr
	}
	if raw := obj["usage"]; raw != nil {
		uobj, ok := raw.(map[string]any)
		if !ok {
			return nil, validationErr(ReasonUsageInvalid, "usage_not_object")
		}
		if err := requireExactKeys(uobj, []string{"input_tokens", "output_tokens", "cost"}, ReasonUsageInvalid); err != nil {
			return nil, err
		}
		usage := &Usage{}
		for _, tok := range []struct {
			key string
			dst **int64
		}{{"input_tokens", &usage.InputTokens}, {"output_tokens", &usage.OutputTokens}} {
			if v := uobj[tok.key]; v != nil {
				n, nerr := numberValue(v)
				if nerr != nil || !isIntegral(n) || n < 0 || n > 1e15 {
					return nil, validationErr(ReasonUsageInvalid, tok.key+"_invalid")
				}
				iv := int64(n)
				*tok.dst = &iv
			}
		}
		if v := uobj["cost"]; v != nil {
			cv, nerr := numberValue(v)
			if nerr != nil || cv < 0 {
				return nil, validationErr(ReasonUsageInvalid, "cost_invalid")
			}
			usage.Cost = &cv
		}
		res.Usage = usage
	}
	if raw := obj["latency_ms"]; raw != nil {
		n, nerr := numberValue(raw)
		if nerr != nil || !isIntegral(n) || n < 0 || n > 36e9 {
			return nil, validationErr(ReasonLatencyInvalid, "latency_ms_invalid")
		}
		lv := int64(n)
		res.LatencyMS = &lv
	}
	if raw := obj["provider_request_id"]; raw != nil {
		str, ok := raw.(string)
		if !ok || str == "" || len(str) > 256 {
			return nil, validationErr(ReasonResultInvalid, "provider_request_id_invalid")
		}
		res.ProviderRequestID = &str
	}
	ext, ok := obj["extensions"].(map[string]any)
	if !ok {
		return nil, validationErr(ReasonResultInvalid, "extensions_not_object")
	}
	res.Extensions = ext
	return res, nil
}

func parseAnswerValue(v any, question Question) (*AnswerValue, *ValidationError) {
	if v == nil {
		return nil, nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, validationErr(ReasonValueMismatch, "value_not_object")
	}
	switch question.Primitive {
	case PrimitiveChoice:
		if err := requireExactKeys(obj, []string{"option_id"}, ReasonValueMismatch); err != nil {
			return nil, err
		}
		id, ok := obj["option_id"].(string)
		if !ok {
			return nil, validationErr(ReasonValueMismatch, "option_id_not_string")
		}
		for _, opt := range question.ChoiceOptions {
			if opt.OptionID == id {
				return &AnswerValue{OptionID: &id}, nil
			}
		}
		return nil, validationErr(ReasonValueMismatch, "option_id_not_in_domain")
	case PrimitiveOrdinalScore:
		if err := requireExactKeys(obj, []string{"level_id"}, ReasonValueMismatch); err != nil {
			return nil, err
		}
		id, ok := obj["level_id"].(string)
		if !ok {
			return nil, validationErr(ReasonValueMismatch, "level_id_not_string")
		}
		for _, lvl := range question.OrdinalLevels {
			if lvl.LevelID == id {
				return &AnswerValue{LevelID: &id}, nil
			}
		}
		return nil, validationErr(ReasonValueMismatch, "level_id_not_in_domain")
	case PrimitiveBinary:
		if err := requireExactKeys(obj, []string{"binary"}, ReasonValueMismatch); err != nil {
			return nil, err
		}
		b, ok := obj["binary"].(bool)
		if !ok {
			return nil, validationErr(ReasonValueMismatch, "binary_not_boolean")
		}
		return &AnswerValue{Binary: &b}, nil
	}
	return nil, validationErr(ReasonValueMismatch, "primitive_unknown")
}

func parseDistribution(v any, question Question, answerStatus string, policy PrecisionPolicy, item *Item) *ValidationError {
	if v == nil {
		return nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return validationErr(ReasonDistributionKeys, "distribution_not_object")
	}
	if question.Primitive == PrimitiveBinary {
		return validationErr(ReasonDistributionForbidden, "binary_has_no_distribution")
	}
	if answerStatus != AnswerAnswered {
		return validationErr(ReasonDistributionForbidden, "distribution_requires_answered")
	}
	var domainKeys []string
	if question.Primitive == PrimitiveChoice {
		for _, opt := range question.ChoiceOptions {
			domainKeys = append(domainKeys, opt.OptionID)
		}
	} else {
		for _, lvl := range question.OrdinalLevels {
			domainKeys = append(domainKeys, lvl.LevelID)
		}
	}
	if len(obj) != len(domainKeys) {
		return validationErr(ReasonDistributionKeys, "distribution_key_set_mismatch")
	}
	dist := make(map[string]float64, len(obj))
	sum := 0.0
	for _, key := range domainKeys {
		raw, present := obj[key]
		if !present {
			return validationErr(ReasonDistributionKeys, "distribution_key_missing:"+key)
		}
		fv, err := numberValue(raw)
		if err != nil {
			return validationErr(ReasonDistributionKeys, "distribution_value_not_number")
		}
		if fv < 0 {
			return validationErr(ReasonDistributionNegative, "distribution_value_negative:"+key)
		}
		dist[key] = fv
		sum += fv
	}
	for key := range obj {
		known := false
		for _, dk := range domainKeys {
			if key == dk {
				known = true
				break
			}
		}
		if !known {
			return validationErr(ReasonDistributionKeys, "distribution_key_unknown:"+key)
		}
	}
	if policy.AbsSumTolerance <= 0 || math.Abs(sum-1) > policy.AbsSumTolerance {
		return validationErr(ReasonDistributionSum, "distribution_sum_out_of_tolerance")
	}
	item.Distribution = dist
	return nil
}

func checkStatusItems(res *Result) *ValidationError {
	answered, abstained, errored := 0, 0, 0
	for _, item := range res.Items {
		switch item.AnswerStatus {
		case AnswerAnswered:
			answered++
		case AnswerAbstained:
			abstained++
		case AnswerError:
			errored++
		}
	}
	nonError := answered + abstained
	switch res.ExecutionStatus {
	case ExecutionSucceeded:
		if errored != 0 {
			return validationErr(ReasonStatusItemsConflict, "succeeded_with_error_items")
		}
	case ExecutionPartial:
		if errored == 0 || nonError == 0 {
			return validationErr(ReasonStatusItemsConflict, "partial_requires_mixed_statuses")
		}
	case ExecutionFailed:
		if nonError != 0 {
			return validationErr(ReasonStatusItemsConflict, "failed_with_non_error_items")
		}
	case ExecutionUnknown:
		if len(res.Items) != 0 {
			return validationErr(ReasonStatusItemsConflict, "unknown_execution_with_items")
		}
	}
	return nil
}

// MarshalResult serializes a Result back to wire JSON by re-encoding the
// retained validated tree; it never re-derives fields from the typed view.
func (r *Result) MarshalResult() ([]byte, error) {
	if r.tree == nil {
		return nil, validationErr(ReasonResultInvalid, "result_tree_missing")
	}
	return MarshalTree(r.tree)
}
