package judgment

import (
	"math"
	"regexp"
	"unicode/utf8"
)

// Primitive answer types supported by contract version 1.0.
const (
	PrimitiveChoice       = "choice"
	PrimitiveOrdinalScore = "ordinal_score"
	PrimitiveBinary       = "binary"
)

// Pin levels for model identity.
const (
	PinRouterModelID      = "router_model_id"
	PinProviderExactModel = "provider_exact_model"
	PinUnderlyingRevision = "underlying_revision"
)

var (
	digestPattern  = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	idPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	extensionName  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,63}$`)
	reasonPattern  = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)
	languageMaxLen = 35
	promptMaxRunes = 2000
	// Hard SDK-side ceilings that bound memory even before capability gates.
	hardMaxQuestions          = 256
	hardMaxCandidates         = 4096
	hardMaxSources            = 256
	hardInlineTextBytes       = 1 << 22
	deadlineMaxMS       int64 = 600000
)

// Request validation reason tokens (shared with TypeScript and vectors).
const (
	ReasonSchemaVersion            = "schema_version"
	ReasonRequestID                = "request_id"
	ReasonAttemptID                = "attempt_id"
	ReasonScopeEmpty               = "scope_empty"
	ReasonScopeInvalid             = "scope_invalid"
	ReasonModelIdentity            = "model_identity"
	ReasonQuestionSetInvalid       = "question_set_invalid"
	ReasonPolicyRefInvalid         = "policy_ref_invalid"
	ReasonDigestFormat             = "digest_format"
	ReasonSourcesInvalid           = "sources_invalid"
	ReasonCandidatesInvalid        = "candidates_invalid"
	ReasonSourceBindingInvalid     = "source_binding_invalid"
	ReasonQuestionInvalid          = "question_invalid"
	ReasonQuestionDuplicate        = "question_duplicate"
	ReasonPrimitiveUnsupported     = "primitive_unsupported"
	ReasonQuestionPromptInvalid    = "question_prompt_invalid"
	ReasonCandidateIDsInvalid      = "candidate_ids_invalid"
	ReasonRequiredFlagMissing      = "required_flag_missing"
	ReasonAnswerDomainInvalid      = "answer_domain_invalid"
	ReasonLimitsInvalid            = "limits_invalid"
	ReasonLimitsExceeded           = "limits_exceeded"
	ReasonExtensionInvalid         = "extension_invalid"
	ReasonUnknownRequiredExtension = "unknown_required_extension"
	ReasonRequestInvalid           = "request_invalid"
)

// Scope identifies the owning context. The server must verify it against its
// trusted authentication context, never against client self-report alone.
type Scope struct {
	OwnerID     *string
	ProjectID   *string
	PrincipalID *string
	Subject     *string
}

// ModelIdentity keeps transport/route identity, upstream reported identity,
// and immutable underlying revision distinct. Unknown revision stays null
// with UnderlyingRevisionVerified false; a router ID never silently becomes
// a direct-provider exact model.
type ModelIdentity struct {
	TransportProvider          string
	ModelProvider              *string
	RequestedModel             string
	ResponseModel              *string
	UnderlyingRevision         *string
	PinLevel                   string
	UnderlyingRevisionVerified bool
}

// VersionedRef references an owner-maintained artifact by id, version and
// canonical digest.
type VersionedRef struct {
	ID      string
	Version string
	Digest  string
}

// Source is bounded, authorized inline material plus provenance. Content is
// untrusted: instructions inside it are never executed or promoted.
type Source struct {
	SourceID   string
	Revision   string
	Digest     string
	InlineText *string
	Language   *string
	LocalRef   *string
}

// SourceBinding binds a candidate to a source by id.
type SourceBinding struct {
	SourceID string
}

// Candidate is one judged unit with explicit source bindings.
type Candidate struct {
	CandidateID    string
	SourceBindings []SourceBinding
}

// ChoiceOption is a stable option of a choice question.
type ChoiceOption struct {
	OptionID string
	Label    string
}

// OrdinalLevel is one explicit level of an ordinal_score question.
type OrdinalLevel struct {
	LevelID      string
	NumericValue float64
	Label        string
}

// Question binds a primitive, prompt, explicit candidate set, explicit
// required flag, and (for choice/ordinal_score) the answer domain.
type Question struct {
	QuestionID    string
	Primitive     string
	Prompt        string
	CandidateIDs  []string
	Required      bool
	ChoiceOptions []ChoiceOption
	OrdinalLevels []OrdinalLevel
}

// Limits are caller-side bounds; they are advisory constraints executed
// together with the executing side, not a hard cost guarantee.
type Limits struct {
	DeadlineMS     *int64
	MaxCandidates  *int
	MaxQuestions   *int
	MaxInputBytes  *int64
	MaxOutputBytes *int64
}

// Extension is a namespaced extension declaration. Unknown optional
// extensions never elevate privileges; unknown required extensions refuse
// execution instead of guessing semantics.
type Extension struct {
	Name     string
	Required bool
	Payload  map[string]any
}

// Request is one explicitly bounded judgment attempt.
type Request struct {
	SchemaVersion string
	RequestID     string
	AttemptID     string
	Scope         Scope
	Model         ModelIdentity
	QuestionSet   VersionedRef
	PolicyRef     VersionedRef
	Sources       []Source
	Candidates    []Candidate
	Questions     []Question
	Limits        *Limits
	Extensions    []Extension

	tree any // canonical tree retained for input digest binding
}

// InputDigest returns the canonical digest of the validated request.
func (r *Request) InputDigest() (string, error) {
	if r.tree == nil {
		return "", validationErr(ReasonRequestInvalid, "request_tree_missing")
	}
	return Digest(r.tree)
}

// RequestOptions configures local request validation.
type RequestOptions struct {
	// KnownExtensions lists extension names this consumer understands.
	KnownExtensions []string
}

func knownExtensionSet(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

// ParseRequest strictly decodes and validates evaluate request bytes.
func ParseRequest(data []byte, opts RequestOptions) (*Request, *ValidationError) {
	tree, err := ParseTree(data)
	if err != nil {
		return nil, validationErr(ReasonRequestInvalid, "request_not_json")
	}
	return parseRequestTree(tree, opts)
}

func parseRequestTree(tree any, opts RequestOptions) (*Request, *ValidationError) {
	obj, ok := tree.(map[string]any)
	if !ok {
		return nil, validationErr(ReasonRequestInvalid, "request_not_object")
	}
	// Version is judged before structural key checks so missing or future
	// versions always classify as schema_version, never as a field error.
	if str, _ := obj["schema_version"].(string); str != "1.0" {
		return nil, validationErr(ReasonSchemaVersion, "schema_version_must_be_1.0")
	}
	missingReasons := map[string]string{
		"request_id":   ReasonRequestID,
		"attempt_id":   ReasonAttemptID,
		"scope":        ReasonScopeInvalid,
		"model":        ReasonModelIdentity,
		"question_set": ReasonQuestionSetInvalid,
		"policy_ref":   ReasonPolicyRefInvalid,
		"sources":      ReasonSourcesInvalid,
		"candidates":   ReasonCandidatesInvalid,
		"questions":    ReasonQuestionInvalid,
		"limits":       ReasonLimitsInvalid,
		"extensions":   ReasonExtensionInvalid,
	}
	for _, key := range []string{
		"request_id", "attempt_id", "scope", "model", "question_set",
		"policy_ref", "sources", "candidates", "questions", "limits",
		"extensions",
	} {
		if _, present := obj[key]; !present {
			return nil, validationErr(missingReasons[key], "missing_field:"+key)
		}
	}
	for key := range obj {
		known := false
		for _, kk := range []string{
			"schema_version", "request_id", "attempt_id", "scope", "model",
			"question_set", "policy_ref", "sources", "candidates",
			"questions", "limits", "extensions",
		} {
			if key == kk {
				known = true
				break
			}
		}
		if !known {
			return nil, validationErr(ReasonRequestInvalid, "unknown_field:"+key)
		}
	}
	req := &Request{SchemaVersion: "1.0", tree: tree}

	var err *ValidationError
	req.RequestID, err = requireID(obj, "request_id", ReasonRequestID)
	if err != nil {
		return nil, err
	}
	req.AttemptID, err = requireID(obj, "attempt_id", ReasonAttemptID)
	if err != nil {
		return nil, err
	}
	req.Scope, err = parseScope(obj["scope"])
	if err != nil {
		return nil, err
	}
	req.Model, err = parseModelIdentity(obj["model"], ReasonModelIdentity)
	if err != nil {
		return nil, err
	}
	req.QuestionSet, err = parseVersionedRef(obj["question_set"], ReasonQuestionSetInvalid)
	if err != nil {
		return nil, err
	}
	req.PolicyRef, err = parseVersionedRef(obj["policy_ref"], ReasonPolicyRefInvalid)
	if err != nil {
		return nil, err
	}
	req.Sources, err = parseSources(obj["sources"])
	if err != nil {
		return nil, err
	}
	req.Candidates, err = parseCandidates(obj["candidates"], req.Sources)
	if err != nil {
		return nil, err
	}
	req.Questions, err = parseQuestions(obj["questions"], req.Candidates)
	if err != nil {
		return nil, err
	}
	req.Limits, err = parseLimits(obj["limits"])
	if err != nil {
		return nil, err
	}
	req.Extensions, err = parseExtensions(obj["extensions"], knownExtensionSet(opts.KnownExtensions))
	if err != nil {
		return nil, err
	}
	if err := checkRequestBounds(req); err != nil {
		return nil, err
	}
	return req, nil
}

func parseScope(v any) (Scope, *ValidationError) {
	obj, ok := v.(map[string]any)
	if !ok {
		return Scope{}, validationErr(ReasonScopeInvalid, "scope_not_object")
	}
	if err := requireExactKeys(obj, []string{"owner_id", "project_id", "principal_id", "subject"}, ReasonScopeInvalid); err != nil {
		return Scope{}, err
	}
	var s Scope
	var err *ValidationError
	if s.OwnerID, err = optID(obj, "owner_id"); err != nil {
		return Scope{}, err
	}
	if s.ProjectID, err = optID(obj, "project_id"); err != nil {
		return Scope{}, err
	}
	if s.PrincipalID, err = optID(obj, "principal_id"); err != nil {
		return Scope{}, err
	}
	if raw, present := obj["subject"]; present {
		if raw != nil {
			str, ok := raw.(string)
			if !ok || str == "" || len(str) > 128 {
				return Scope{}, validationErr(ReasonScopeInvalid, "subject_invalid")
			}
			s.Subject = &str
		}
	}
	for _, p := range []*string{s.OwnerID, s.ProjectID, s.PrincipalID, s.Subject} {
		if p != nil && *p != "" {
			return s, nil
		}
	}
	return Scope{}, validationErr(ReasonScopeEmpty, "scope_has_no_identity")
}

func parseModelIdentity(v any, reason string) (ModelIdentity, *ValidationError) {
	obj, ok := v.(map[string]any)
	if !ok {
		return ModelIdentity{}, validationErr(reason, "model_not_object")
	}
	if err := requireExactKeys(obj, []string{
		"transport_provider", "model_provider", "requested_model",
		"response_model", "underlying_revision", "pin_level",
		"underlying_revision_verified",
	}, reason); err != nil {
		return ModelIdentity{}, err
	}
	var m ModelIdentity
	str, _ := obj["transport_provider"].(string)
	if str == "" || len(str) > 128 {
		return ModelIdentity{}, validationErr(reason, "transport_provider_invalid")
	}
	m.TransportProvider = str
	provider, _ := obj["model_provider"].(string)
	if provider != "" {
		m.ModelProvider = &provider
	}
	requested, _ := obj["requested_model"].(string)
	if requested == "" || len(requested) > 256 {
		return ModelIdentity{}, validationErr(reason, "requested_model_invalid")
	}
	m.RequestedModel = requested
	if raw := obj["response_model"]; raw != nil {
		str, ok := raw.(string)
		if !ok || str == "" || len(str) > 256 {
			return ModelIdentity{}, validationErr(reason, "response_model_invalid")
		}
		m.ResponseModel = &str
	}
	if raw := obj["underlying_revision"]; raw != nil {
		str, ok := raw.(string)
		if !ok || str == "" || len(str) > 256 {
			return ModelIdentity{}, validationErr(reason, "underlying_revision_invalid")
		}
		m.UnderlyingRevision = &str
	}
	pin, _ := obj["pin_level"].(string)
	switch pin {
	case PinRouterModelID, PinProviderExactModel, PinUnderlyingRevision:
		m.PinLevel = pin
	default:
		return ModelIdentity{}, validationErr(reason, "pin_level_invalid")
	}
	verified, ok := obj["underlying_revision_verified"].(bool)
	if !ok {
		return ModelIdentity{}, validationErr(reason, "underlying_revision_verified_not_bool")
	}
	m.UnderlyingRevisionVerified = verified
	return m, nil
}

func parseVersionedRef(v any, reason string) (VersionedRef, *ValidationError) {
	obj, ok := v.(map[string]any)
	if !ok {
		return VersionedRef{}, validationErr(reason, "ref_not_object")
	}
	if err := requireExactKeys(obj, []string{"id", "version", "digest"}, reason); err != nil {
		return VersionedRef{}, err
	}
	id, _ := obj["id"].(string)
	if !idPattern.MatchString(id) {
		return VersionedRef{}, validationErr(reason, "id_invalid")
	}
	version, _ := obj["version"].(string)
	if version == "" || len(version) > 128 {
		return VersionedRef{}, validationErr(reason, "version_invalid")
	}
	digestStr, _ := obj["digest"].(string)
	if !digestPattern.MatchString(digestStr) {
		return VersionedRef{}, validationErr(ReasonDigestFormat, "digest_must_be_sha256_hex64")
	}
	return VersionedRef{ID: id, Version: version, Digest: digestStr}, nil
}

func parseSources(v any) ([]Source, *ValidationError) {
	arr, ok := v.([]any)
	if !ok {
		return nil, validationErr(ReasonSourcesInvalid, "sources_not_array")
	}
	if len(arr) > hardMaxSources {
		return nil, validationErr(ReasonSourcesInvalid, "too_many_sources")
	}
	seen := make(map[string]bool, len(arr))
	out := make([]Source, 0, len(arr))
	for _, raw := range arr {
		obj, ok := raw.(map[string]any)
		if !ok {
			return nil, validationErr(ReasonSourcesInvalid, "source_not_object")
		}
		if err := requireExactKeys(obj, []string{
			"source_id", "revision", "digest", "inline_text", "language", "local_ref",
		}, ReasonSourcesInvalid); err != nil {
			return nil, err
		}
		var s Source
		s.SourceID, ok = obj["source_id"].(string)
		if !ok || !idPattern.MatchString(s.SourceID) {
			return nil, validationErr(ReasonSourcesInvalid, "source_id_invalid")
		}
		if seen[s.SourceID] {
			return nil, validationErr(ReasonSourcesInvalid, "source_id_duplicate")
		}
		seen[s.SourceID] = true
		s.Revision, ok = obj["revision"].(string)
		if !ok || s.Revision == "" || len(s.Revision) > 128 {
			return nil, validationErr(ReasonSourcesInvalid, "source_revision_invalid")
		}
		s.Digest, ok = obj["digest"].(string)
		if !ok || !digestPattern.MatchString(s.Digest) {
			return nil, validationErr(ReasonSourcesInvalid, "source_digest_invalid")
		}
		if raw := obj["inline_text"]; raw != nil {
			str, ok := raw.(string)
			if !ok || str == "" {
				return nil, validationErr(ReasonSourcesInvalid, "inline_text_invalid")
			}
			if len(str) > hardInlineTextBytes {
				return nil, validationErr(ReasonLimitsExceeded, "inline_text_hard_cap")
			}
			s.InlineText = &str
		}
		if raw := obj["language"]; raw != nil {
			str, ok := raw.(string)
			if !ok || str == "" || len(str) > languageMaxLen {
				return nil, validationErr(ReasonSourcesInvalid, "language_invalid")
			}
			s.Language = &str
		}
		if raw := obj["local_ref"]; raw != nil {
			str, ok := raw.(string)
			if !ok || str == "" || len(str) > 1024 {
				return nil, validationErr(ReasonSourcesInvalid, "local_ref_invalid")
			}
			s.LocalRef = &str
		}
		out = append(out, s)
	}
	return out, nil
}

func parseCandidates(v any, sources []Source) ([]Candidate, *ValidationError) {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return nil, validationErr(ReasonCandidatesInvalid, "candidates_must_be_nonempty")
	}
	if len(arr) > hardMaxCandidates {
		return nil, validationErr(ReasonCandidatesInvalid, "too_many_candidates")
	}
	sourceIDs := make(map[string]bool, len(sources))
	for _, s := range sources {
		sourceIDs[s.SourceID] = true
	}
	seen := make(map[string]bool, len(arr))
	out := make([]Candidate, 0, len(arr))
	for _, raw := range arr {
		obj, ok := raw.(map[string]any)
		if !ok {
			return nil, validationErr(ReasonCandidatesInvalid, "candidate_not_object")
		}
		if err := requireExactKeys(obj, []string{"candidate_id", "source_bindings"}, ReasonCandidatesInvalid); err != nil {
			return nil, err
		}
		var c Candidate
		c.CandidateID, ok = obj["candidate_id"].(string)
		if !ok || !idPattern.MatchString(c.CandidateID) {
			return nil, validationErr(ReasonCandidatesInvalid, "candidate_id_invalid")
		}
		if seen[c.CandidateID] {
			return nil, validationErr(ReasonCandidatesInvalid, "candidate_id_duplicate")
		}
		seen[c.CandidateID] = true
		bindings, ok := obj["source_bindings"].([]any)
		if !ok || len(bindings) == 0 {
			return nil, validationErr(ReasonSourceBindingInvalid, "source_bindings_must_be_nonempty")
		}
		bound := make(map[string]bool, len(bindings))
		for _, braw := range bindings {
			bobj, ok := braw.(map[string]any)
			if !ok {
				return nil, validationErr(ReasonSourceBindingInvalid, "binding_not_object")
			}
			if err := requireExactKeys(bobj, []string{"source_id"}, ReasonSourceBindingInvalid); err != nil {
				return nil, err
			}
			sid, ok := bobj["source_id"].(string)
			if !ok || !idPattern.MatchString(sid) {
				return nil, validationErr(ReasonSourceBindingInvalid, "binding_source_id_invalid")
			}
			if !sourceIDs[sid] {
				return nil, validationErr(ReasonSourceBindingInvalid, "binding_source_unknown")
			}
			if bound[sid] {
				return nil, validationErr(ReasonSourceBindingInvalid, "binding_source_duplicate")
			}
			bound[sid] = true
			c.SourceBindings = append(c.SourceBindings, SourceBinding{SourceID: sid})
		}
		out = append(out, c)
	}
	return out, nil
}

func parseQuestions(v any, candidates []Candidate) ([]Question, *ValidationError) {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return nil, validationErr(ReasonQuestionInvalid, "questions_must_be_nonempty")
	}
	if len(arr) > hardMaxQuestions {
		return nil, validationErr(ReasonQuestionInvalid, "too_many_questions")
	}
	candidateIDs := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		candidateIDs[c.CandidateID] = true
	}
	seen := make(map[string]bool, len(arr))
	out := make([]Question, 0, len(arr))
	for _, raw := range arr {
		obj, ok := raw.(map[string]any)
		if !ok {
			return nil, validationErr(ReasonQuestionInvalid, "question_not_object")
		}
		if err := requireExactKeys(obj, []string{
			"question_id", "primitive", "prompt", "candidate_ids", "required", "answer_domain",
		}, ReasonQuestionInvalid); err != nil {
			// Field-specific presences classify before the structural check
			// so removed keys report their own contract section.
			if _, present := obj["prompt"]; !present {
				return nil, validationErr(ReasonQuestionPromptInvalid, "prompt_missing")
			}
			if _, present := obj["required"]; !present {
				return nil, validationErr(ReasonRequiredFlagMissing, "required_flag_missing")
			}
			if _, present := obj["answer_domain"]; !present {
				return nil, validationErr(ReasonAnswerDomainInvalid, "answer_domain_missing")
			}
			return nil, err
		}
		var q Question
		q.QuestionID, ok = obj["question_id"].(string)
		if !ok || !idPattern.MatchString(q.QuestionID) {
			return nil, validationErr(ReasonQuestionInvalid, "question_id_invalid")
		}
		if seen[q.QuestionID] {
			return nil, validationErr(ReasonQuestionDuplicate, "question_id_duplicate")
		}
		seen[q.QuestionID] = true
		q.Primitive, _ = obj["primitive"].(string)
		switch q.Primitive {
		case PrimitiveChoice, PrimitiveOrdinalScore, PrimitiveBinary:
		default:
			return nil, validationErr(ReasonPrimitiveUnsupported, "primitive_not_in_contract")
		}
		prompt, ok := obj["prompt"].(string)
		if !ok || prompt == "" || utf8.RuneCountInString(prompt) > promptMaxRunes {
			return nil, validationErr(ReasonQuestionPromptInvalid, "prompt_invalid")
		}
		q.Prompt = prompt
		ids, ok := obj["candidate_ids"].([]any)
		if !ok || len(ids) == 0 {
			return nil, validationErr(ReasonCandidateIDsInvalid, "candidate_ids_must_be_nonempty")
		}
		dup := make(map[string]bool, len(ids))
		for _, iraw := range ids {
			id, ok := iraw.(string)
			if !ok || !idPattern.MatchString(id) {
				return nil, validationErr(ReasonCandidateIDsInvalid, "candidate_id_invalid")
			}
			if !candidateIDs[id] {
				return nil, validationErr(ReasonCandidateIDsInvalid, "candidate_id_unknown")
			}
			if dup[id] {
				return nil, validationErr(ReasonCandidateIDsInvalid, "candidate_id_duplicate")
			}
			dup[id] = true
			q.CandidateIDs = append(q.CandidateIDs, id)
		}
		requiredRaw, present := obj["required"]
		if !present {
			return nil, validationErr(ReasonRequiredFlagMissing, "required_flag_must_be_explicit")
		}
		required, ok := requiredRaw.(bool)
		if !ok {
			return nil, validationErr(ReasonRequiredFlagMissing, "required_flag_not_boolean")
		}
		q.Required = required
		domain := obj["answer_domain"]
		switch q.Primitive {
		case PrimitiveChoice:
			opts, err := parseChoiceDomain(domain)
			if err != nil {
				return nil, err
			}
			q.ChoiceOptions = opts
		case PrimitiveOrdinalScore:
			levels, err := parseOrdinalDomain(domain)
			if err != nil {
				return nil, err
			}
			q.OrdinalLevels = levels
		case PrimitiveBinary:
			if domain != nil {
				return nil, validationErr(ReasonAnswerDomainInvalid, "binary_has_no_answer_domain")
			}
		}
		out = append(out, q)
	}
	return out, nil
}

func parseChoiceDomain(v any) ([]ChoiceOption, *ValidationError) {
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, validationErr(ReasonAnswerDomainInvalid, "choice_domain_missing")
	}
	if err := requireExactKeys(obj, []string{"options"}, ReasonAnswerDomainInvalid); err != nil {
		return nil, err
	}
	arr, ok := obj["options"].([]any)
	if !ok || len(arr) < 2 || len(arr) > 16 {
		return nil, validationErr(ReasonAnswerDomainInvalid, "options_out_of_range")
	}
	seen := make(map[string]bool, len(arr))
	out := make([]ChoiceOption, 0, len(arr))
	for _, raw := range arr {
		oobj, ok := raw.(map[string]any)
		if !ok {
			return nil, validationErr(ReasonAnswerDomainInvalid, "option_not_object")
		}
		if err := requireExactKeys(oobj, []string{"option_id", "label"}, ReasonAnswerDomainInvalid); err != nil {
			return nil, err
		}
		id, ok := oobj["option_id"].(string)
		if !ok || !idPattern.MatchString(id) {
			return nil, validationErr(ReasonAnswerDomainInvalid, "option_id_invalid")
		}
		if seen[id] {
			return nil, validationErr(ReasonAnswerDomainInvalid, "option_id_duplicate")
		}
		seen[id] = true
		label, ok := oobj["label"].(string)
		if !ok || label == "" || utf8.RuneCountInString(label) > 200 {
			return nil, validationErr(ReasonAnswerDomainInvalid, "option_label_invalid")
		}
		out = append(out, ChoiceOption{OptionID: id, Label: label})
	}
	return out, nil
}

func parseOrdinalDomain(v any) ([]OrdinalLevel, *ValidationError) {
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, validationErr(ReasonAnswerDomainInvalid, "ordinal_domain_missing")
	}
	if err := requireExactKeys(obj, []string{"levels"}, ReasonAnswerDomainInvalid); err != nil {
		return nil, err
	}
	arr, ok := obj["levels"].([]any)
	if !ok || len(arr) < 2 || len(arr) > 16 {
		return nil, validationErr(ReasonAnswerDomainInvalid, "levels_out_of_range")
	}
	seenIDs := make(map[string]bool, len(arr))
	seenValues := make(map[float64]bool, len(arr))
	out := make([]OrdinalLevel, 0, len(arr))
	for _, raw := range arr {
		lobj, ok := raw.(map[string]any)
		if !ok {
			return nil, validationErr(ReasonAnswerDomainInvalid, "level_not_object")
		}
		if err := requireExactKeys(lobj, []string{"level_id", "numeric_value", "label"}, ReasonAnswerDomainInvalid); err != nil {
			return nil, err
		}
		id, ok := lobj["level_id"].(string)
		if !ok || !idPattern.MatchString(id) {
			return nil, validationErr(ReasonAnswerDomainInvalid, "level_id_invalid")
		}
		if seenIDs[id] {
			return nil, validationErr(ReasonAnswerDomainInvalid, "level_id_duplicate")
		}
		seenIDs[id] = true
		num, err := numberValue(lobj["numeric_value"])
		if err != nil || !isIntegral(num) || math.Abs(num) > 1e15 {
			return nil, validationErr(ReasonAnswerDomainInvalid, "level_numeric_value_invalid")
		}
		if seenValues[num] {
			return nil, validationErr(ReasonAnswerDomainInvalid, "level_numeric_value_duplicate")
		}
		seenValues[num] = true
		label, ok := lobj["label"].(string)
		if !ok || label == "" || utf8.RuneCountInString(label) > 200 {
			return nil, validationErr(ReasonAnswerDomainInvalid, "level_label_invalid")
		}
		out = append(out, OrdinalLevel{LevelID: id, NumericValue: num, Label: label})
	}
	return out, nil
}

func parseLimits(v any) (*Limits, *ValidationError) {
	if v == nil {
		return nil, nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, validationErr(ReasonLimitsInvalid, "limits_not_object")
	}
	for key := range obj {
		switch key {
		case "deadline_ms", "max_candidates", "max_questions", "max_input_bytes", "max_output_bytes":
		default:
			return nil, validationErr(ReasonLimitsInvalid, "limits_unknown_field")
		}
	}
	var l Limits
	if raw, present := obj["deadline_ms"]; present {
		n, err := numberValue(raw)
		if err != nil || !isIntegral(n) || n < 1 || n > float64(deadlineMaxMS) {
			return nil, validationErr(ReasonLimitsInvalid, "deadline_ms_invalid")
		}
		v := int64(n)
		l.DeadlineMS = &v
	}
	if raw, present := obj["max_candidates"]; present {
		n, err := numberValue(raw)
		if err != nil || !isIntegral(n) || n < 1 || n > float64(hardMaxCandidates) {
			return nil, validationErr(ReasonLimitsInvalid, "max_candidates_invalid")
		}
		v := int(n)
		l.MaxCandidates = &v
	}
	if raw, present := obj["max_questions"]; present {
		n, err := numberValue(raw)
		if err != nil || !isIntegral(n) || n < 1 || n > float64(hardMaxQuestions) {
			return nil, validationErr(ReasonLimitsInvalid, "max_questions_invalid")
		}
		v := int(n)
		l.MaxQuestions = &v
	}
	if raw, present := obj["max_input_bytes"]; present {
		n, err := numberValue(raw)
		if err != nil || !isIntegral(n) || n < 1 || n > 1<<30 {
			return nil, validationErr(ReasonLimitsInvalid, "max_input_bytes_invalid")
		}
		v := int64(n)
		l.MaxInputBytes = &v
	}
	if raw, present := obj["max_output_bytes"]; present {
		n, err := numberValue(raw)
		if err != nil || !isIntegral(n) || n < 1 || n > 1<<30 {
			return nil, validationErr(ReasonLimitsInvalid, "max_output_bytes_invalid")
		}
		v := int64(n)
		l.MaxOutputBytes = &v
	}
	return &l, nil
}

func parseExtensions(v any, known map[string]bool) ([]Extension, *ValidationError) {
	arr, ok := v.([]any)
	if !ok {
		return nil, validationErr(ReasonExtensionInvalid, "extensions_not_array")
	}
	out := make([]Extension, 0, len(arr))
	for _, raw := range arr {
		obj, ok := raw.(map[string]any)
		if !ok {
			return nil, validationErr(ReasonExtensionInvalid, "extension_not_object")
		}
		if err := requireExactKeys(obj, []string{"name", "required", "payload"}, ReasonExtensionInvalid); err != nil {
			return nil, err
		}
		name, ok := obj["name"].(string)
		if !ok || !extensionName.MatchString(name) {
			return nil, validationErr(ReasonExtensionInvalid, "extension_name_invalid")
		}
		requiredRaw, present := obj["required"]
		if !present {
			return nil, validationErr(ReasonExtensionInvalid, "extension_required_flag_missing")
		}
		required, ok := requiredRaw.(bool)
		if !ok {
			return nil, validationErr(ReasonExtensionInvalid, "extension_required_not_boolean")
		}
		payload, ok := obj["payload"].(map[string]any)
		if !ok {
			return nil, validationErr(ReasonExtensionInvalid, "extension_payload_not_object")
		}
		if required && !known[name] {
			return nil, validationErr(ReasonUnknownRequiredExtension, "unknown_required_extension:"+name)
		}
		out = append(out, Extension{Name: name, Required: required, Payload: payload})
	}
	return out, nil
}

func checkRequestBounds(req *Request) *ValidationError {
	if req.Limits == nil {
		return nil
	}
	if req.Limits.MaxQuestions != nil && len(req.Questions) > *req.Limits.MaxQuestions {
		return validationErr(ReasonLimitsExceeded, "question_count_over_max_questions")
	}
	if req.Limits.MaxCandidates != nil && len(req.Candidates) > *req.Limits.MaxCandidates {
		return validationErr(ReasonLimitsExceeded, "candidate_count_over_max_candidates")
	}
	if req.Limits.MaxInputBytes != nil && req.tree != nil {
		size, err := CanonicalJSON(req.tree)
		if err != nil {
			return validationErr(ReasonRequestInvalid, "request_not_canonicalizable")
		}
		if int64(len(size)) > *req.Limits.MaxInputBytes {
			return validationErr(ReasonLimitsExceeded, "request_bytes_over_max_input_bytes")
		}
	}
	return nil
}

// ExpectedPairs returns every (candidate_id, question_id) pair the contract
// requires in a result, derived from explicit question.candidate_ids.
func (r *Request) ExpectedPairs() [][2]string {
	var pairs [][2]string
	for _, q := range r.Questions {
		for _, cid := range q.CandidateIDs {
			pairs = append(pairs, [2]string{cid, q.QuestionID})
		}
	}
	return pairs
}

// QuestionByIndex builds a lookup of question_id -> Question.
func (r *Request) questionMap() map[string]Question {
	m := make(map[string]Question, len(r.Questions))
	for _, q := range r.Questions {
		m[q.QuestionID] = q
	}
	return m
}

// --- shared strict-decoding helpers -------------------------------------

func requireKeys(obj map[string]any, keys []string, reason string) *ValidationError {
	for _, k := range keys {
		if _, present := obj[k]; !present {
			return validationErr(reason, "missing_field:"+k)
		}
	}
	for k := range obj {
		known := false
		for _, kk := range keys {
			if k == kk {
				known = true
				break
			}
		}
		if !known {
			return validationErr(reason, "unknown_field:"+k)
		}
	}
	return nil
}

func requireExactKeys(obj map[string]any, keys []string, reason string) *ValidationError {
	return requireKeys(obj, keys, reason)
}

func requireID(obj map[string]any, key string, reason string) (string, *ValidationError) {
	str, ok := obj[key].(string)
	if !ok || !idPattern.MatchString(str) {
		return "", validationErr(reason, key+"_invalid")
	}
	return str, nil
}

func optID(obj map[string]any, key string) (*string, *ValidationError) {
	raw, present := obj[key]
	if !present || raw == nil {
		return nil, nil
	}
	str, ok := raw.(string)
	if !ok {
		return nil, validationErr(ReasonScopeInvalid, key+"_invalid")
	}
	if str == "" {
		// Empty strings count as unset so an all-empty scope reports
		// scope_empty rather than a malformed field.
		return nil, nil
	}
	if !idPattern.MatchString(str) {
		return nil, validationErr(ReasonScopeInvalid, key+"_pattern")
	}
	return &str, nil
}
