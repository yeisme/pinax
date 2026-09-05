package promptrepo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

const (
	RepositorySetSchemaVersion        = "promptrepo.repository-set.v0.1"
	PolicyDecisionSchemaVersion       = "promptrepo.policy-decision.v0.1"
	ManagementProjectionSchemaVersion = "promptrepo.management-projection.v0.1"

	RepositoryScopeUser         = "user"
	RepositoryScopeOrganization = "organization"
	RepositoryScopeProject      = "project"
	RepositoryScopeSession      = "session"

	RepositoryModeEmbedded = "embedded"
	RepositoryModeService  = "service"

	SelectionSessionExact        = "session_exact"
	SelectionProjectPin          = "project_pin"
	SelectionUserPreference      = "user_preference"
	SelectionOrganizationDefault = "organization_default"
	SelectionOfficialFallback    = "official_fallback"

	ReadinessReady    = "ready"
	ReadinessNotReady = "not_ready"
	ReadinessBlocked  = "blocked"

	PolicyResultAllowed       = "allowed"
	PolicyResultBlocked       = "blocked"
	PolicyResultNotApplicable = "not_applicable"

	ReasonRepositoryDisabled   = "REPOSITORY_DISABLED"
	ReasonSourceNotReady       = "SOURCE_NOT_READY"
	ReasonSourceDegraded       = "SOURCE_DEGRADED"
	ReasonSourceQuarantined    = "SOURCE_QUARANTINED"
	ReasonRepositoryDenied     = "REPOSITORY_DENIED"
	ReasonRepositoryNotAllowed = "REPOSITORY_NOT_ALLOWED"
	ReasonOperationDenied      = "OPERATION_DENIED"
	ReasonOperationNotAllowed  = "OPERATION_NOT_ALLOWED"
	ReasonTrustBelowMinimum    = "TRUST_BELOW_MINIMUM"
	ReasonRightsBlocked        = "RIGHTS_BLOCKED"
	ReasonRightsNotAllowed     = "RIGHTS_NOT_ALLOWED"
	ReasonCapabilityMissing    = "CAPABILITY_MISSING"

	OperationRepositoryList    = "promptrepo.repository.list"
	OperationRepositoryAdd     = "promptrepo.repository.add"
	OperationRepositoryRemove  = "promptrepo.repository.remove"
	OperationRepositoryEnable  = "promptrepo.repository.enable"
	OperationRepositoryDisable = "promptrepo.repository.disable"
	OperationRepositorySync    = "promptrepo.repository.sync"
	OperationRepositoryDoctor  = "promptrepo.repository.doctor"
	OperationCatalogSearch     = "promptrepo.catalog.search"
	OperationCatalogShow       = "promptrepo.catalog.show"
	OperationCatalogResolve    = "promptrepo.catalog.resolve"
	OperationTemplateInspect   = "promptrepo.template.inspect"
	OperationTemplateValidate  = "promptrepo.template.validate"
	OperationTemplatePreview   = "promptrepo.template.preview"
	OperationPolicyReview      = "promptrepo.policy.review"
)

// RepositorySetReader is an optional read-only capability. Client remains
// unchanged so v0.1-v0.3 consumers do not need to implement management APIs.
type RepositorySetReader interface {
	EffectiveRepositorySet(context.Context, EffectiveRepositorySetRequest) (RepositorySet, error)
}

// PolicyEvaluator is an optional, side-effect-free admission capability.
type PolicyEvaluator interface {
	EvaluatePolicy(context.Context, EvaluatePolicyRequest) (PolicyDecision, error)
}

// RepositoryScopeBinding describes caller-owned ordering and policy lineage.
// ScopeRef is accepted as input but is never copied to RepositorySet output.
type RepositoryScopeBinding struct {
	Scope         string   `json:"scope" yaml:"scope"`
	ScopeRef      string   `json:"scope_ref,omitempty" yaml:"scope_ref,omitempty"`
	RepositoryIDs []string `json:"repository_ids" yaml:"repository_ids"`
	PolicyRef     string   `json:"policy_ref,omitempty" yaml:"policy_ref,omitempty"`
	PolicyDigest  string   `json:"policy_digest,omitempty" yaml:"policy_digest,omitempty"`
	Priority      int      `json:"priority,omitempty" yaml:"priority,omitempty"`
	Enabled       bool     `json:"enabled" yaml:"enabled"`
}

// RepositorySetCandidate is safe repository metadata supplied by the owner of
// a scope. It deliberately excludes source locations and credential refs.
type RepositorySetCandidate struct {
	RepositoryID string `json:"repository_id" yaml:"repository_id"`
	Scope        string `json:"scope" yaml:"scope"`
	Trust        string `json:"trust" yaml:"trust"`
	Health       string `json:"health" yaml:"health"`
	Enabled      bool   `json:"enabled" yaml:"enabled"`
}

type EffectiveRepositorySetRequest struct {
	Consumer                      string                   `json:"consumer" yaml:"consumer"`
	Mode                          string                   `json:"mode,omitempty" yaml:"mode,omitempty"`
	Bindings                      []RepositoryScopeBinding `json:"bindings,omitempty" yaml:"bindings,omitempty"`
	Repositories                  []RepositorySetCandidate `json:"repositories,omitempty" yaml:"repositories,omitempty"`
	ExactRepositoryID             string                   `json:"exact_repository_id,omitempty" yaml:"exact_repository_id,omitempty"`
	OfficialFallbackRepositoryIDs []string                 `json:"official_fallback_repository_ids,omitempty" yaml:"official_fallback_repository_ids,omitempty"`
}

type RepositoryScopeLayer struct {
	Scope          string   `json:"scope" yaml:"scope"`
	ScopeRefDigest string   `json:"scope_ref_digest,omitempty" yaml:"scope_ref_digest,omitempty"`
	RepositoryIDs  []string `json:"repository_ids" yaml:"repository_ids"`
	PolicyDigest   string   `json:"policy_digest,omitempty" yaml:"policy_digest,omitempty"`
	Priority       int      `json:"priority,omitempty" yaml:"priority,omitempty"`
}

type RepositorySetEntry struct {
	RepositoryID    string `json:"repository_id" yaml:"repository_id"`
	Scope           string `json:"scope" yaml:"scope"`
	RepositoryScope string `json:"repository_scope" yaml:"repository_scope"`
	SelectionSource string `json:"selection_source" yaml:"selection_source"`
	Rank            int    `json:"rank" yaml:"rank"`
	ScopeRefDigest  string `json:"scope_ref_digest,omitempty" yaml:"scope_ref_digest,omitempty"`
	PolicyDigest    string `json:"policy_digest,omitempty" yaml:"policy_digest,omitempty"`
	Trust           string `json:"trust" yaml:"trust"`
	Health          string `json:"health" yaml:"health"`
	Enabled         bool   `json:"enabled" yaml:"enabled"`
}

type RepositorySetHealthSummary struct {
	Total       int `json:"total" yaml:"total"`
	Ready       int `json:"ready" yaml:"ready"`
	Configured  int `json:"configured" yaml:"configured"`
	Degraded    int `json:"degraded" yaml:"degraded"`
	Quarantined int `json:"quarantined" yaml:"quarantined"`
	Disabled    int `json:"disabled" yaml:"disabled"`
	Unknown     int `json:"unknown" yaml:"unknown"`
}

type RepositorySet struct {
	SchemaVersion         string                     `json:"schema_version" yaml:"schema_version"`
	Consumer              string                     `json:"consumer" yaml:"consumer"`
	Mode                  string                     `json:"mode" yaml:"mode"`
	Layers                []RepositoryScopeLayer     `json:"layers" yaml:"layers"`
	EffectiveRepositories []RepositorySetEntry       `json:"effective_repositories" yaml:"effective_repositories"`
	PolicyDigest          string                     `json:"policy_digest,omitempty" yaml:"policy_digest,omitempty"`
	HealthSummary         RepositorySetHealthSummary `json:"health_summary" yaml:"health_summary"`
}

type PolicyConstraint struct {
	Scope                string   `json:"scope" yaml:"scope"`
	Ref                  string   `json:"ref,omitempty" yaml:"ref,omitempty"`
	Digest               string   `json:"digest,omitempty" yaml:"digest,omitempty"`
	AllowedRepositoryIDs []string `json:"allowed_repository_ids,omitempty" yaml:"allowed_repository_ids,omitempty"`
	DeniedRepositoryIDs  []string `json:"denied_repository_ids,omitempty" yaml:"denied_repository_ids,omitempty"`
	AllowedOperations    []string `json:"allowed_operations,omitempty" yaml:"allowed_operations,omitempty"`
	DeniedOperations     []string `json:"denied_operations,omitempty" yaml:"denied_operations,omitempty"`
	MinimumTrust         string   `json:"minimum_trust,omitempty" yaml:"minimum_trust,omitempty"`
	AllowedRights        []string `json:"allowed_rights,omitempty" yaml:"allowed_rights,omitempty"`
	DeniedRights         []string `json:"denied_rights,omitempty" yaml:"denied_rights,omitempty"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty" yaml:"required_capabilities,omitempty"`
}

type EvaluatePolicyRequest struct {
	Repository           RepositorySetEntry `json:"repository" yaml:"repository"`
	OperationID          string             `json:"operation_id" yaml:"operation_id"`
	ResolvedLocale       string             `json:"resolved_locale,omitempty" yaml:"resolved_locale,omitempty"`
	Rights               string             `json:"rights,omitempty" yaml:"rights,omitempty"`
	Capabilities         []string           `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	RequiredCapabilities []string           `json:"required_capabilities,omitempty" yaml:"required_capabilities,omitempty"`
	Policies             []PolicyConstraint `json:"policies,omitempty" yaml:"policies,omitempty"`
}

type PolicyDecision struct {
	SchemaVersion       string   `json:"schema_version" yaml:"schema_version"`
	Allowed             bool     `json:"allowed" yaml:"allowed"`
	Readiness           string   `json:"readiness" yaml:"readiness"`
	PolicyDigest        string   `json:"policy_digest,omitempty" yaml:"policy_digest,omitempty"`
	ReasonCodes         []string `json:"reason_codes,omitempty" yaml:"reason_codes,omitempty"`
	RequiredActions     []string `json:"required_actions,omitempty" yaml:"required_actions,omitempty"`
	MatchedPolicyRefs   []string `json:"matched_policy_refs,omitempty" yaml:"matched_policy_refs,omitempty"`
	ResolvedLocale      string   `json:"resolved_locale,omitempty" yaml:"resolved_locale,omitempty"`
	MinimumTrust        string   `json:"minimum_trust,omitempty" yaml:"minimum_trust,omitempty"`
	RightsResult        string   `json:"rights_result" yaml:"rights_result"`
	CapabilityResult    string   `json:"capability_result" yaml:"capability_result"`
	MissingCapabilities []string `json:"missing_capabilities,omitempty" yaml:"missing_capabilities,omitempty"`
}

type ManagementAction struct {
	ActionID      string `json:"action_id" yaml:"action_id"`
	Owner         string `json:"owner" yaml:"owner"`
	Risk          string `json:"risk" yaml:"risk"`
	Writes        string `json:"writes" yaml:"writes"`
	ProviderCalls bool   `json:"provider_calls" yaml:"provider_calls"`
}

type ManagementProjectionRequest struct {
	OperationID         string
	Command             string
	Status              string
	Mode                string
	Repository          RepositorySetEntry
	Decision            PolicyDecision
	PolicyDigest        string
	ExactRevisionDigest string
	CatalogDigest       string
	SnapshotDigest      string
	CacheState          string
	NextActions         []ManagementAction
	ProviderCalls       bool
	DurableWrites       bool
}

type ManagementProjection struct {
	SchemaVersion       string             `json:"schema_version" yaml:"schema_version"`
	OperationID         string             `json:"operation_id" yaml:"operation_id"`
	Command             string             `json:"command,omitempty" yaml:"command,omitempty"`
	Status              string             `json:"status" yaml:"status"`
	Scope               string             `json:"scope" yaml:"scope"`
	ScopeRefDigest      string             `json:"scope_ref_digest,omitempty" yaml:"scope_ref_digest,omitempty"`
	Mode                string             `json:"mode" yaml:"mode"`
	RepositoryID        string             `json:"repository_id" yaml:"repository_id"`
	Health              string             `json:"health" yaml:"health"`
	Trust               string             `json:"trust" yaml:"trust"`
	Readiness           string             `json:"readiness" yaml:"readiness"`
	ReasonCodes         []string           `json:"reason_codes,omitempty" yaml:"reason_codes,omitempty"`
	PolicyDigest        string             `json:"policy_digest,omitempty" yaml:"policy_digest,omitempty"`
	ExactRevisionDigest string             `json:"exact_revision_digest,omitempty" yaml:"exact_revision_digest,omitempty"`
	CatalogDigest       string             `json:"catalog_digest,omitempty" yaml:"catalog_digest,omitempty"`
	SnapshotDigest      string             `json:"snapshot_digest,omitempty" yaml:"snapshot_digest,omitempty"`
	CacheState          string             `json:"cache_state,omitempty" yaml:"cache_state,omitempty"`
	NextActions         []ManagementAction `json:"next_actions,omitempty" yaml:"next_actions,omitempty"`
	ProviderCalls       bool               `json:"provider_calls" yaml:"provider_calls"`
	DurableWrites       bool               `json:"durable_writes" yaml:"durable_writes"`
}

type indexedBinding struct {
	binding RepositoryScopeBinding
	index   int
}

func ComposeRepositorySet(request EffectiveRepositorySetRequest) (RepositorySet, error) {
	consumer := strings.TrimSpace(request.Consumer)
	if consumer == "" {
		return RepositorySet{}, NewError(CodeInvalidRequest, "repository set consumer is required", false, nil)
	}
	mode := strings.TrimSpace(request.Mode)
	if mode == "" {
		mode = RepositoryModeEmbedded
	}
	if mode != RepositoryModeEmbedded && mode != RepositoryModeService {
		return RepositorySet{}, NewError(CodeInvalidRequest, "repository set mode is invalid", false, nil)
	}

	candidates := make(map[string]RepositorySetCandidate, len(request.Repositories))
	for _, candidate := range request.Repositories {
		candidate.RepositoryID = strings.TrimSpace(candidate.RepositoryID)
		candidate.Scope = strings.TrimSpace(candidate.Scope)
		if candidate.RepositoryID == "" || !validRepositoryScope(candidate.Scope) {
			return RepositorySet{}, NewError(CodeInvalidRequest, "repository set candidate is invalid", false, nil)
		}
		if _, exists := candidates[candidate.RepositoryID]; exists {
			return RepositorySet{}, NewError(CodeInvalidRequest, "repository set candidate is duplicated", false, nil)
		}
		candidate.Trust = normalizeOrDefault(candidate.Trust, "untrusted")
		candidate.Health = normalizeOrDefault(candidate.Health, "unknown")
		candidates[candidate.RepositoryID] = candidate
	}

	bindings := make([]indexedBinding, 0, len(request.Bindings))
	for index, binding := range request.Bindings {
		binding.Scope = strings.TrimSpace(binding.Scope)
		if !binding.Enabled {
			continue
		}
		if !validRepositoryScope(binding.Scope) {
			return RepositorySet{}, NewError(CodeInvalidRequest, "repository scope binding is invalid", false, nil)
		}
		binding.RepositoryIDs = compactStrings(binding.RepositoryIDs)
		bindings = append(bindings, indexedBinding{binding: binding, index: index})
	}
	sort.SliceStable(bindings, func(left, right int) bool {
		leftRank := repositoryScopeRank(bindings[left].binding.Scope)
		rightRank := repositoryScopeRank(bindings[right].binding.Scope)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if bindings[left].binding.Priority != bindings[right].binding.Priority {
			return bindings[left].binding.Priority > bindings[right].binding.Priority
		}
		return bindings[left].index < bindings[right].index
	})

	result := RepositorySet{SchemaVersion: RepositorySetSchemaVersion, Consumer: consumer, Mode: mode}
	seen := make(map[string]struct{}, len(candidates))
	policyDigests := make([]string, 0, len(bindings))
	appendEntry := func(repositoryID, scope, source, scopeRefDigest, policyDigest string) error {
		candidate, ok := candidates[repositoryID]
		if !ok {
			return NewError(CodeNotFound, "repository scope binding references an unknown repository", false, nil)
		}
		if _, exists := seen[repositoryID]; exists {
			return nil
		}
		seen[repositoryID] = struct{}{}
		result.EffectiveRepositories = append(result.EffectiveRepositories, RepositorySetEntry{
			RepositoryID: repositoryID, Scope: scope, RepositoryScope: candidate.Scope,
			SelectionSource: source, Rank: len(result.EffectiveRepositories), ScopeRefDigest: scopeRefDigest,
			PolicyDigest: policyDigest, Trust: candidate.Trust, Health: candidate.Health, Enabled: candidate.Enabled,
		})
		return nil
	}

	if exact := strings.TrimSpace(request.ExactRepositoryID); exact != "" {
		if err := appendEntry(exact, RepositoryScopeSession, SelectionSessionExact, "", ""); err != nil {
			return RepositorySet{}, err
		}
		result.Layers = append(result.Layers, RepositoryScopeLayer{Scope: RepositoryScopeSession, RepositoryIDs: []string{exact}})
	}
	for _, indexed := range bindings {
		binding := indexed.binding
		scopeDigest := digestScopeRef(binding.Scope, binding.ScopeRef)
		layer := RepositoryScopeLayer{
			Scope: binding.Scope, ScopeRefDigest: scopeDigest, RepositoryIDs: append([]string(nil), binding.RepositoryIDs...),
			PolicyDigest: strings.TrimSpace(binding.PolicyDigest), Priority: binding.Priority,
		}
		result.Layers = append(result.Layers, layer)
		if layer.PolicyDigest != "" {
			policyDigests = append(policyDigests, layer.PolicyDigest)
		}
		for _, repositoryID := range binding.RepositoryIDs {
			if err := appendEntry(repositoryID, binding.Scope, selectionSourceForScope(binding.Scope), scopeDigest, layer.PolicyDigest); err != nil {
				return RepositorySet{}, err
			}
		}
	}
	for _, repositoryID := range compactStrings(request.OfficialFallbackRepositoryIDs) {
		candidate, ok := candidates[repositoryID]
		if !ok {
			return RepositorySet{}, NewError(CodeNotFound, "official fallback references an unknown repository", false, nil)
		}
		if err := appendEntry(repositoryID, candidate.Scope, SelectionOfficialFallback, "", ""); err != nil {
			return RepositorySet{}, err
		}
	}
	result.PolicyDigest = combinedDigest(policyDigests)
	result.HealthSummary = summarizeRepositoryHealth(result.EffectiveRepositories)
	return result, nil
}

func EvaluateRepositoryPolicy(request EvaluatePolicyRequest) (PolicyDecision, error) {
	if strings.TrimSpace(request.Repository.RepositoryID) == "" || strings.TrimSpace(request.OperationID) == "" {
		return PolicyDecision{}, NewError(CodeInvalidRequest, "policy evaluation requires repository and operation id", false, nil)
	}
	decision := PolicyDecision{
		SchemaVersion: PolicyDecisionSchemaVersion, ResolvedLocale: strings.TrimSpace(request.ResolvedLocale),
		RightsResult: PolicyResultNotApplicable, CapabilityResult: PolicyResultAllowed,
	}
	reasons := map[string]struct{}{}
	actions := map[string]struct{}{}
	addReason := func(reason, action string) {
		reasons[reason] = struct{}{}
		if action != "" {
			actions[action] = struct{}{}
		}
	}

	health := strings.ToLower(strings.TrimSpace(request.Repository.Health))
	if !request.Repository.Enabled || health == "disabled" {
		addReason(ReasonRepositoryDisabled, OperationRepositoryEnable)
	} else {
		switch health {
		case "ready":
		case "quarantined":
			addReason(ReasonSourceQuarantined, OperationRepositoryDoctor)
		case "degraded":
			addReason(ReasonSourceDegraded, OperationRepositoryDoctor)
		default:
			addReason(ReasonSourceNotReady, OperationRepositorySync)
		}
	}

	minimumTrust := ""
	minimumRank := 0
	policyRefs := map[string]struct{}{}
	policyDigests := make([]string, 0, len(request.Policies))
	requiredCapabilities := append([]string(nil), request.RequiredCapabilities...)
	rights := strings.ToLower(strings.TrimSpace(request.Rights))
	if rights != "" {
		decision.RightsResult = PolicyResultAllowed
		if containsFold([]string{"blocked", "prohibited", "forbidden", "denied"}, rights) {
			decision.RightsResult = PolicyResultBlocked
			addReason(ReasonRightsBlocked, OperationPolicyReview)
		}
	}
	for _, policy := range request.Policies {
		if ref := strings.TrimSpace(policy.Ref); ref != "" {
			policyRefs[ref] = struct{}{}
		}
		if digest := strings.TrimSpace(policy.Digest); digest != "" {
			policyDigests = append(policyDigests, digest)
		}
		if containsFold(policy.DeniedRepositoryIDs, request.Repository.RepositoryID) {
			addReason(ReasonRepositoryDenied, OperationPolicyReview)
		}
		if len(compactStrings(policy.AllowedRepositoryIDs)) > 0 && !containsFold(policy.AllowedRepositoryIDs, request.Repository.RepositoryID) {
			addReason(ReasonRepositoryNotAllowed, OperationPolicyReview)
		}
		if containsFold(policy.DeniedOperations, request.OperationID) {
			addReason(ReasonOperationDenied, OperationPolicyReview)
		}
		if len(compactStrings(policy.AllowedOperations)) > 0 && !containsFold(policy.AllowedOperations, request.OperationID) {
			addReason(ReasonOperationNotAllowed, OperationPolicyReview)
		}
		if rights != "" {
			if containsFold(policy.DeniedRights, rights) {
				decision.RightsResult = PolicyResultBlocked
				addReason(ReasonRightsBlocked, OperationPolicyReview)
			}
			if len(compactStrings(policy.AllowedRights)) > 0 && !containsFold(policy.AllowedRights, rights) {
				decision.RightsResult = PolicyResultBlocked
				addReason(ReasonRightsNotAllowed, OperationPolicyReview)
			}
		}
		if trust := strings.ToLower(strings.TrimSpace(policy.MinimumTrust)); trust != "" {
			rank := managementTrustRank(trust)
			if rank == 0 {
				return PolicyDecision{}, NewError(CodeInvalidRequest, "policy minimum trust is invalid", false, nil)
			}
			if rank > minimumRank {
				minimumRank, minimumTrust = rank, trust
			}
		}
		requiredCapabilities = append(requiredCapabilities, policy.RequiredCapabilities...)
	}
	if minimumRank > 0 && managementTrustRank(request.Repository.Trust) < minimumRank {
		addReason(ReasonTrustBelowMinimum, OperationPolicyReview)
	}
	decision.MinimumTrust = minimumTrust
	decision.PolicyDigest = combinedDigest(policyDigests)
	decision.MissingCapabilities = missingManagementCapabilities(request.Capabilities, requiredCapabilities)
	if len(decision.MissingCapabilities) > 0 {
		decision.CapabilityResult = PolicyResultBlocked
		addReason(ReasonCapabilityMissing, OperationTemplateInspect)
	}
	decision.MatchedPolicyRefs = sortedKeys(policyRefs)
	decision.ReasonCodes = sortedKeys(reasons)
	decision.RequiredActions = sortedKeys(actions)
	decision.Allowed = len(decision.ReasonCodes) == 0
	if decision.Allowed {
		decision.Readiness = ReadinessReady
	} else if len(decision.ReasonCodes) == 1 && decision.ReasonCodes[0] == ReasonSourceNotReady {
		decision.Readiness = ReadinessNotReady
	} else {
		decision.Readiness = ReadinessBlocked
	}
	return decision, nil
}

func BuildManagementProjection(request ManagementProjectionRequest) (ManagementProjection, error) {
	operationID := strings.TrimSpace(request.OperationID)
	if operationID == "" || !strings.HasPrefix(operationID, "promptrepo.") {
		return ManagementProjection{}, NewError(CodeInvalidRequest, "management operation id is invalid", false, nil)
	}
	mode := strings.TrimSpace(request.Mode)
	if mode == "" {
		mode = RepositoryModeEmbedded
	}
	if mode != RepositoryModeEmbedded && mode != RepositoryModeService {
		return ManagementProjection{}, NewError(CodeInvalidRequest, "management projection mode is invalid", false, nil)
	}
	status := strings.TrimSpace(request.Status)
	if status == "" {
		switch request.Decision.Readiness {
		case ReadinessReady:
			status = "ok"
		case ReadinessNotReady:
			status = "not_ready"
		default:
			status = "blocked"
		}
	}
	return ManagementProjection{
		SchemaVersion: ManagementProjectionSchemaVersion, OperationID: operationID, Command: strings.TrimSpace(request.Command), Status: status,
		Scope: request.Repository.Scope, ScopeRefDigest: request.Repository.ScopeRefDigest, Mode: mode,
		RepositoryID: request.Repository.RepositoryID, Health: request.Repository.Health, Trust: request.Repository.Trust,
		Readiness: request.Decision.Readiness, ReasonCodes: append([]string(nil), request.Decision.ReasonCodes...),
		PolicyDigest: firstNonEmpty(strings.TrimSpace(request.PolicyDigest), request.Decision.PolicyDigest), ExactRevisionDigest: strings.TrimSpace(request.ExactRevisionDigest),
		CatalogDigest: strings.TrimSpace(request.CatalogDigest), SnapshotDigest: strings.TrimSpace(request.SnapshotDigest), CacheState: strings.TrimSpace(request.CacheState),
		NextActions: append([]ManagementAction(nil), request.NextActions...), ProviderCalls: request.ProviderCalls, DurableWrites: request.DurableWrites,
	}, nil
}

func validRepositoryScope(scope string) bool {
	switch scope {
	case RepositoryScopeUser, RepositoryScopeOrganization, RepositoryScopeProject, RepositoryScopeSession:
		return true
	default:
		return false
	}
}

func repositoryScopeRank(scope string) int {
	switch scope {
	case RepositoryScopeSession:
		return 0
	case RepositoryScopeProject:
		return 1
	case RepositoryScopeUser:
		return 2
	case RepositoryScopeOrganization:
		return 3
	default:
		return 4
	}
}

func selectionSourceForScope(scope string) string {
	switch scope {
	case RepositoryScopeSession:
		return SelectionSessionExact
	case RepositoryScopeProject:
		return SelectionProjectPin
	case RepositoryScopeUser:
		return SelectionUserPreference
	default:
		return SelectionOrganizationDefault
	}
}

func digestScopeRef(scope, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(scope) + "\x00" + ref))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func combinedDigest(values []string) string {
	values = compactStrings(values)
	if len(values) == 0 {
		return ""
	}
	sort.Strings(values)
	values = uniqueStrings(values)
	if len(values) == 1 {
		return values[0]
	}
	payload, _ := json.Marshal(values)
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func summarizeRepositoryHealth(entries []RepositorySetEntry) RepositorySetHealthSummary {
	summary := RepositorySetHealthSummary{Total: len(entries)}
	for _, entry := range entries {
		if !entry.Enabled || strings.EqualFold(entry.Health, "disabled") {
			summary.Disabled++
			continue
		}
		switch strings.ToLower(strings.TrimSpace(entry.Health)) {
		case "ready":
			summary.Ready++
		case "configured":
			summary.Configured++
		case "degraded":
			summary.Degraded++
		case "quarantined":
			summary.Quarantined++
		default:
			summary.Unknown++
		}
	}
	return summary
}

func missingManagementCapabilities(actual, required []string) []string {
	actualSet := map[string]struct{}{}
	for _, value := range compactStrings(actual) {
		actualSet[strings.ToLower(value)] = struct{}{}
	}
	missing := map[string]struct{}{}
	for _, value := range compactStrings(required) {
		value = strings.ToLower(value)
		if _, ok := actualSet[value]; !ok {
			missing[value] = struct{}{}
		}
	}
	return sortedKeys(missing)
}

func managementTrustRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "official":
		return 4
	case "verified":
		return 3
	case "user_trusted":
		return 2
	case "untrusted":
		return 1
	default:
		return 0
	}
}

func normalizeOrDefault(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func containsFold(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == target {
			return true
		}
	}
	return false
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
