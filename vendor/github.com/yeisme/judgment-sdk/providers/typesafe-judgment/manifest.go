package typesafejudgment

import (
	judgment "github.com/yeisme/judgment-sdk"
)

// Fixed adapter identity and capability truth. The manifest is the single
// source of capability truth; nothing else in this module may claim limits
// the manifest does not declare.
const (
	// AdapterName is the consumer identity registered in credentialctl's
	// structured-judgment preset (consumer/capability pairing).
	AdapterName = "aigora-typesafe-judgment"
	// AdapterVersion follows the versioned manifest policy: protocol or
	// precision-rule changes require a version bump plus new fixtures.
	AdapterVersion = "0.1.0"

	// TransportProvider is the direct TypeSafe transport identity.
	TransportProvider = "typesafe"
	// ModelProvider is the upstream model provider.
	ModelProvider = "typesafe"
	// ExactModel is the only served model. The first target pins the exact
	// provider model; no alias resolution is implemented in this version.
	ExactModel = "jev-1.13.0"
	// PinLevel is the strongest identity this adapter can honor: an exact
	// provider model. It is not an immutable underlying-weights revision.
	PinLevel = judgment.PinProviderExactModel

	// ConsumerIdentity / GrantCapability form the credentialctl grant
	// pairing checked before every execution.
	ConsumerIdentity = "aigora-typesafe-judgment"
	GrantCapability  = "structured_judgment"

	// UpstreamEndpoint is fixed: the adapter binary accepts no arbitrary
	// upstream URL input, so Authorization can never be redirected.
	UpstreamEndpoint = "https://api.typesafe.ai/v1/systemone"

	// SecretEnvName is the environment variable credentialctl exec injects;
	// the adapter reads it only at the execution boundary.
	SecretEnvName = "TYPESAFE_API_KEY"

	// DefaultCredentialRef is the credentialctl reference used for grant
	// checks and documented exec injection (reference only, never a secret).
	DefaultCredentialRef = "typesafe/personal"
)

// Declared bounds. Provider-documented bounds are enforced as prechecks;
// bounds the provider does not document get conservative self-imposed caps
// and are never claimed as unlimited.
const (
	// MaxScoreLevels: TypeSafe documents score criteria of at least two and
	// up to ten levels.
	MaxScoreLevels = 10
	// MaxPairs is a conservative self-imposed cap on expanded provider
	// questions per request (the API documents no question-count limit).
	MaxPairs = 64
	// MaxPayloadBytes is a conservative self-imposed upstream payload bound.
	MaxPayloadBytes = 65536
	// MaxUpstreamResponseBytes bounds the upstream response read.
	MaxUpstreamResponseBytes = 128 << 10
	// DefaultDeadlineMS applies when the request declares no deadline.
	DefaultDeadlineMS = 20000
)

// NormalizationVersion versions the wire-to-contract normalization rules.
// Bumping it requires new conformance fixtures; history is never rewritten.
const NormalizationVersion = "aigora-adapters-typesafe-judgment.v1"

// Capabilities returns the versioned capability manifest as the typed,
// contract-validated capability envelope.
func Capabilities() *judgment.Capabilities {
	data := CapabilitiesJSON()
	caps, verr := judgment.ParseCapabilities(data)
	if verr != nil {
		// The manifest is a compile-time constant; failing to parse it is a
		// programming error that must never reach a consumer.
		panic("typesafejudgment: embedded manifest violates the contract: " + verr.Reason + ": " + verr.Detail)
	}
	return caps
}

// CapabilitiesJSON returns the versioned capability manifest document. The
// bytes are rebuilt from the constants above and must always parse through
// judgment.ParseCapabilities (asserted by tests and by Capabilities).
func CapabilitiesJSON() []byte {
	confidenceProvenance := judgment.ProvenanceProviderSelfReport
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
			"requested_model":              ExactModel,
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
		"confidence_available":   true,
		"confidence_provenance":  &confidenceProvenance,
		"distribution_tolerance": map[string]any{"version": "yeisme.judgment.precision.v1", "abs_sum_tolerance": 0.000001},
		"supports_idempotency":   false,
		"supports_reconcile":     false,
		"supports_cancel":        false,
		"extensions":             []string{},
	}
	// The document is served and validated as a unit; marshal determinism is
	// not required by the contract, but stable output keeps evidence diffs
	// readable.
	return mustMarshal(doc)
}
