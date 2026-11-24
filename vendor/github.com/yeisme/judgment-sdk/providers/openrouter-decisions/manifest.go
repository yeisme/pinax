package openrouterdecisions

import (
	judgment "github.com/yeisme/judgment-sdk"
)

// Fixed adapter identity and capability truth. The manifest is the single
// source of capability truth.
const (
	// AdapterName is also the consumer identity registered in
	// credentialctl's structured-judgment preset.
	AdapterName = "aigora-openrouter-decisions"
	// AdapterVersion carries the alpha lifecycle marker: the Decisions
	// endpoint and this adapter are pre-stable and may change contract
	// shapes; consumers gate on this version explicitly.
	AdapterVersion = "0.1.0-alpha"

	// TransportProvider is OpenRouter; the model itself comes from TypeSafe.
	TransportProvider = "openrouter"
	// ModelProvider is the upstream model provider behind the route.
	ModelProvider = "typesafe"
	// RouteModel is the OpenRouter route id. It is a versioned-family route
	// id, NOT a verified exact provider model and NOT an immutable
	// underlying revision.
	RouteModel = "typesafe/jev-1.13"
	// PinLevel is the strongest identity this adapter can honor: the router
	// model id. Requests demanding more must fail, not be spoofed.
	PinLevel = judgment.PinRouterModelID

	// ConsumerIdentity / GrantCapability form the credentialctl grant
	// pairing checked before every execution.
	ConsumerIdentity = "aigora-openrouter-decisions"
	GrantCapability  = "structured_judgment"

	// UpstreamEndpoint is fixed: the Decisions alpha endpoint, never
	// chat/completions, and never overridable by request input.
	UpstreamEndpoint = "https://openrouter.ai/api/alpha/decisions"

	// SecretEnvName is the environment variable credentialctl exec injects.
	SecretEnvName = "OPENROUTER_API_KEY"

	// DefaultCredentialRef mirrors the user's existing OpenRouter reference.
	// It is a reference example only; the adapter never stores the secret.
	DefaultCredentialRef = "openrouter/personal-default"

	// ProviderOnly pins the route to the TypeSafe provider; fallbacks are
	// disabled so a failure can never silently become another paid path.
	ProviderOnly = "typesafe"
)

// Declared bounds. The local validated prototype client observed the
// OpenRouter Decisions request contract (1..24 questions, bounded criteria
// and instructions, 24000-byte input); those bounds are enforced as
// prechecks and declared here, never claimed as unlimited.
const (
	// MaxPairs is the OpenRouter Decisions question-count bound.
	MaxPairs = 24
	// MaxScoreLevels / MaxChoiceOptions mirror the alpha criteria bounds.
	MaxScoreLevels  = 16
	MaxChoiceOption = 16
	// MaxPayloadBytes bounds the upstream request body.
	MaxPayloadBytes = 24000
	// MaxInstructionRunes bounds one generated provider instruction.
	MaxInstructionRunes = 2000
	// MaxUpstreamResponseBytes bounds the upstream response read.
	MaxUpstreamResponseBytes = 128 << 10
	// DefaultDeadlineMS applies when the request declares no deadline.
	DefaultDeadlineMS = 20000
)

// NormalizationVersion versions the wire-to-contract normalization rules.
// The rounding relationship between the provider score and its distribution
// is RECORDED here but explicitly NOT certified until versioned conformance
// passes; consumers must not read the rule name as a tolerance guarantee.
const NormalizationVersion = "aigora-adapters-openrouter-decisions.v0-alpha"

// ResponseModelFamily is the set of upstream-reported model strings observed
// and accepted for this route family. A value outside the family is a
// corrupt or drifted response, never a silent identity rewrite. Even when
// the family contains "jev-1.13.0", it is recorded verbatim as
// response_model with underlying_revision=null and verified=false.
var ResponseModelFamily = map[string]bool{
	"typesafe/jev-1.13":          true,
	"jev-1.13.0":                 true,
	"typesafe/jev-1.13-20260917": true,
}

// Capabilities returns the versioned capability manifest as the typed,
// contract-validated capability envelope.
func Capabilities() *judgment.Capabilities {
	caps, verr := judgment.ParseCapabilities(CapabilitiesJSON())
	if verr != nil {
		panic("openrouterdecisions: embedded manifest violates the contract: " + verr.Reason + ": " + verr.Detail)
	}
	return caps
}

// CapabilitiesJSON returns the versioned capability manifest document.
//
// Honesty notes: probability_available=true means real probabilities are
// mapped when present and never fabricated — per-answer statistics stay null
// when OpenRouter omits them (the official SDK declares them optional).
// confidence_available=false because the alpha route does not guarantee
// calibrated or self-reported confidence.
func CapabilitiesJSON() []byte {
	doc := map[string]any{
		"schema_version":   "1.0",
		"contract_version": "1.0",
		"adapter": map[string]any{
			"name":    AdapterName,
			"version": AdapterVersion,
		},
		"model": map[string]any{
			"transport_provider":           TransportProvider,
			"model_provider":               ModelProvider,
			"requested_model":              RouteModel,
			"response_model":               nil,
			"underlying_revision":          nil,
			"pin_level":                    PinLevel,
			"underlying_revision_verified": false,
		},
		"modalities":             []string{"text"},
		"primitives":             []string{judgment.PrimitiveChoice, judgment.PrimitiveOrdinalScore, judgment.PrimitiveBinary},
		"max_candidates":         16,
		"max_questions":          24,
		"max_sources":            8,
		"max_inline_text_bytes":  65536,
		"languages":              []string{"zh", "en"},
		"probability_available":  true,
		"confidence_available":   false,
		"confidence_provenance":  nil,
		"distribution_tolerance": map[string]any{"version": "yeisme.judgment.precision.v1", "abs_sum_tolerance": 0.000001},
		"supports_idempotency":   false,
		"supports_reconcile":     false,
		"supports_cancel":        false,
		"extensions":             []string{},
	}
	return mustMarshal(doc)
}
