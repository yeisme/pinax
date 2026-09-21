package judgment

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
)

// Capability gate reason tokens (shared with TypeScript and vectors).
const (
	ReasonContractVersion          = "contract_version"
	ReasonMaxQuestionsCap          = "max_questions"
	ReasonMaxCandidatesCap         = "max_candidates"
	ReasonMaxSourcesCap            = "max_sources"
	ReasonInlineTextLimit          = "inline_text_limit"
	ReasonLanguageUnsupported      = "language_unsupported"
	ReasonUnderlyingRevisionNeeded = "underlying_revision_required"
	ReasonProbabilityNeeded        = "probability_required"
	ReasonConfidenceNeeded         = "confidence_required"
	ReasonExtensionUnsupported     = "extension_unsupported"
	ReasonCapabilitiesInvalid      = "capabilities_invalid"
)

// Adapter identifies the transport adapter serving the contract.
type Adapter struct {
	Name    string
	Version string
}

// DistributionTolerance is the versioned precision policy for distribution
// sums. The public SDK never hardcodes a provider-specific tolerance.
type DistributionTolerance struct {
	Version         string
	AbsSumTolerance float64
}

// Capabilities is the negotiated capability envelope.
type Capabilities struct {
	SchemaVersion         string
	ContractVersion       string
	Adapter               Adapter
	Model                 ModelIdentity
	Modalities            []string
	Primitives            []string
	MaxCandidates         int
	MaxQuestions          int
	MaxSources            int
	MaxInlineTextBytes    int64
	Languages             []string
	ProbabilityAvailable  bool
	ConfidenceAvailable   bool
	ConfidenceProvenance  *string
	DistributionTolerance DistributionTolerance
	SupportsIdempotency   bool
	SupportsReconcile     bool
	SupportsCancel        bool
	Extensions            []string
}

// ParseCapabilities strictly decodes a capabilities payload.
func ParseCapabilities(data []byte) (*Capabilities, *ValidationError) {
	tree, err := ParseTree(data)
	if err != nil {
		return nil, validationErr(ReasonCapabilitiesInvalid, "capabilities_not_json")
	}
	return parseCapabilitiesTree(tree)
}

func parseCapabilitiesTree(tree any) (*Capabilities, *ValidationError) {
	obj, ok := tree.(map[string]any)
	if !ok {
		return nil, validationErr(ReasonCapabilitiesInvalid, "capabilities_not_object")
	}
	if err := requireExactKeys(obj, []string{
		"schema_version", "contract_version", "adapter", "model", "modalities",
		"primitives", "max_candidates", "max_questions", "max_sources",
		"max_inline_text_bytes", "languages", "probability_available",
		"confidence_available", "confidence_provenance",
		"distribution_tolerance", "supports_idempotency", "supports_reconcile",
		"supports_cancel", "extensions",
	}, ReasonCapabilitiesInvalid); err != nil {
		return nil, err
	}
	if str, _ := obj["schema_version"].(string); str != "1.0" {
		return nil, validationErr(ReasonSchemaVersion, "schema_version_must_be_1.0")
	}
	var c Capabilities
	c.SchemaVersion = "1.0"
	c.ContractVersion, _ = obj["contract_version"].(string)
	if c.ContractVersion == "" || len(c.ContractVersion) > 32 {
		return nil, validationErr(ReasonCapabilitiesInvalid, "contract_version_invalid")
	}
	// Major-version compatibility is negotiated by the gate, not by the
	// parser: an incompatible peer is an unsupported capability, not a
	// malformed payload.
	adapter, ok := obj["adapter"].(map[string]any)
	if !ok {
		return nil, validationErr(ReasonCapabilitiesInvalid, "adapter_not_object")
	}
	if err := requireExactKeys(adapter, []string{"name", "version"}, ReasonCapabilitiesInvalid); err != nil {
		return nil, err
	}
	c.Adapter.Name, _ = adapter["name"].(string)
	c.Adapter.Version, _ = adapter["version"].(string)
	if c.Adapter.Name == "" || c.Adapter.Version == "" {
		return nil, validationErr(ReasonCapabilitiesInvalid, "adapter_identity_invalid")
	}
	model, err := parseModelIdentity(obj["model"], ReasonCapabilitiesInvalid)
	if err != nil {
		return nil, err
	}
	c.Model = model
	for _, listKey := range []string{"modalities", "primitives", "languages", "extensions"} {
		arr, ok := obj[listKey].([]any)
		if !ok {
			return nil, validationErr(ReasonCapabilitiesInvalid, listKey+"_not_array")
		}
		out := make([]string, 0, len(arr))
		for _, raw := range arr {
			s, ok := raw.(string)
			if !ok || s == "" || len(s) > 128 {
				return nil, validationErr(ReasonCapabilitiesInvalid, listKey+"_entry_invalid")
			}
			out = append(out, s)
		}
		switch listKey {
		case "modalities":
			if len(out) == 0 {
				return nil, validationErr(ReasonCapabilitiesInvalid, "modalities_empty")
			}
			c.Modalities = out
		case "primitives":
			if len(out) == 0 {
				return nil, validationErr(ReasonCapabilitiesInvalid, "primitives_empty")
			}
			for _, p := range out {
				switch p {
				case PrimitiveChoice, PrimitiveOrdinalScore, PrimitiveBinary:
				default:
					return nil, validationErr(ReasonCapabilitiesInvalid, "primitive_unknown:"+p)
				}
			}
			c.Primitives = out
		case "languages":
			if len(out) == 0 {
				return nil, validationErr(ReasonCapabilitiesInvalid, "languages_empty")
			}
			c.Languages = out
		case "extensions":
			c.Extensions = out
		}
	}
	positiveInt := func(key string, hardMax float64) (int, *ValidationError) {
		n, nerr := numberValue(obj[key])
		if nerr != nil || !isIntegral(n) || n < 1 || n > hardMax {
			return 0, validationErr(ReasonCapabilitiesInvalid, key+"_invalid")
		}
		return int(n), nil
	}
	if c.MaxCandidates, err = positiveInt("max_candidates", 1<<20); err != nil {
		return nil, err
	}
	if c.MaxQuestions, err = positiveInt("max_questions", float64(hardMaxQuestions)); err != nil {
		return nil, err
	}
	if c.MaxSources, err = positiveInt("max_sources", float64(hardMaxSources)); err != nil {
		return nil, err
	}
	inlineN, nerr := numberValue(obj["max_inline_text_bytes"])
	if nerr != nil || !isIntegral(inlineN) || inlineN < 1 || inlineN > 1<<30 {
		return nil, validationErr(ReasonCapabilitiesInvalid, "max_inline_text_bytes_invalid")
	}
	c.MaxInlineTextBytes = int64(inlineN)
	flag := func(key string) (bool, *ValidationError) {
		b, ok := obj[key].(bool)
		if !ok {
			return false, validationErr(ReasonCapabilitiesInvalid, key+"_not_boolean")
		}
		return b, nil
	}
	if c.ProbabilityAvailable, err = flag("probability_available"); err != nil {
		return nil, err
	}
	if c.ConfidenceAvailable, err = flag("confidence_available"); err != nil {
		return nil, err
	}
	if raw := obj["confidence_provenance"]; raw != nil {
		str, ok := raw.(string)
		if !ok {
			return nil, validationErr(ReasonCapabilitiesInvalid, "confidence_provenance_invalid")
		}
		switch str {
		case "provider_self_report", "calibrated", "derived":
		default:
			return nil, validationErr(ReasonCapabilitiesInvalid, "confidence_provenance_invalid")
		}
		c.ConfidenceProvenance = &str
	}
	tol, ok := obj["distribution_tolerance"].(map[string]any)
	if !ok {
		return nil, validationErr(ReasonCapabilitiesInvalid, "distribution_tolerance_not_object")
	}
	if err := requireExactKeys(tol, []string{"version", "abs_sum_tolerance"}, ReasonCapabilitiesInvalid); err != nil {
		return nil, err
	}
	c.DistributionTolerance.Version, _ = tol["version"].(string)
	if c.DistributionTolerance.Version == "" || len(c.DistributionTolerance.Version) > 128 {
		return nil, validationErr(ReasonCapabilitiesInvalid, "distribution_tolerance_version_invalid")
	}
	absTol, terr := numberValue(tol["abs_sum_tolerance"])
	if terr != nil || !(absTol > 0 && absTol < 1) {
		return nil, validationErr(ReasonCapabilitiesInvalid, "abs_sum_tolerance_invalid")
	}
	c.DistributionTolerance.AbsSumTolerance = absTol
	if c.SupportsIdempotency, err = flag("supports_idempotency"); err != nil {
		return nil, err
	}
	if c.SupportsReconcile, err = flag("supports_reconcile"); err != nil {
		return nil, err
	}
	if c.SupportsCancel, err = flag("supports_cancel"); err != nil {
		return nil, err
	}
	return &c, nil
}

// contractVersionCompatible accepts the same major version with any minor.
func contractVersionCompatible(v string) bool {
	major, _, ok := strings.Cut(v, ".")
	if !ok || major != "1" {
		return false
	}
	return true
}

// GateOptions expresses caller-side policy on top of provider capabilities.
type GateOptions struct {
	// RequireVerifiedUnderlyingRevision rejects routers that cannot prove an
	// immutable underlying weights revision.
	RequireVerifiedUnderlyingRevision bool
	// RequireProbability rejects capabilities without real probabilities.
	RequireProbability bool
	// RequireConfidence rejects capabilities without calibrated or provider
	// self-reported confidence.
	RequireConfidence bool
}

// CheckCapabilities verifies a request against negotiated capabilities
// before anything is submitted. It returns nil when the request may proceed.
func CheckCapabilities(caps *Capabilities, req *Request, opts GateOptions) *ValidationError {
	if caps == nil {
		return validationErr(ReasonCapabilitiesInvalid, "capabilities_missing")
	}
	if !contractVersionCompatible(caps.ContractVersion) {
		return validationErr(ReasonContractVersion, "contract_version_incompatible")
	}
	supported := make(map[string]bool, len(caps.Primitives))
	for _, p := range caps.Primitives {
		supported[p] = true
	}
	for _, q := range req.Questions {
		if !supported[q.Primitive] {
			return validationErr(ReasonPrimitiveUnsupported, "primitive_not_advertised:"+q.Primitive)
		}
	}
	if len(req.Questions) > caps.MaxQuestions {
		return validationErr(ReasonMaxQuestionsCap, "question_count_over_capability")
	}
	if len(req.Candidates) > caps.MaxCandidates {
		return validationErr(ReasonMaxCandidatesCap, "candidate_count_over_capability")
	}
	if len(req.Sources) > caps.MaxSources {
		return validationErr(ReasonMaxSourcesCap, "source_count_over_capability")
	}
	for _, s := range req.Sources {
		if s.InlineText != nil && int64(len(*s.InlineText)) > caps.MaxInlineTextBytes {
			return validationErr(ReasonInlineTextLimit, "inline_text_over_capability")
		}
		if s.Language != nil && !containsString(caps.Languages, *s.Language) {
			return validationErr(ReasonLanguageUnsupported, "language_not_advertised:"+*s.Language)
		}
	}
	if opts.RequireVerifiedUnderlyingRevision {
		if caps.Model.UnderlyingRevision == nil || !caps.Model.UnderlyingRevisionVerified {
			return validationErr(ReasonUnderlyingRevisionNeeded, "underlying_revision_not_verified")
		}
	}
	if opts.RequireProbability && !caps.ProbabilityAvailable {
		return validationErr(ReasonProbabilityNeeded, "probability_not_available")
	}
	if opts.RequireConfidence && !caps.ConfidenceAvailable {
		return validationErr(ReasonConfidenceNeeded, "confidence_not_available")
	}
	advertised := make(map[string]bool, len(caps.Extensions))
	for _, name := range caps.Extensions {
		advertised[name] = true
	}
	for _, ext := range req.Extensions {
		if ext.Required && !advertised[ext.Name] {
			return validationErr(ReasonExtensionUnsupported, "required_extension_not_advertised:"+ext.Name)
		}
	}
	return nil
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// --- number helpers -------------------------------------------------------

func numberValue(v any) (float64, error) {
	switch t := v.(type) {
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return 0, err
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, errors.New("nonfinite_number")
		}
		return f, nil
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return 0, errors.New("nonfinite_number")
		}
		return t, nil
	default:
		return 0, errors.New("not_a_number")
	}
}

func isIntegral(f float64) bool {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return false
	}
	return f == math.Trunc(f)
}
