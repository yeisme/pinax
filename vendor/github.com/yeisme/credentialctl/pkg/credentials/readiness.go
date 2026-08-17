package credentials

import "time"

const (
	ReadinessSpecVersion = "credential_readiness.v0.1"
	OwnerCredentialctl   = "credentialctl"

	ReadinessConfigured  = "configured"
	ReadinessMissing     = "missing"
	ReadinessDisabled    = "disabled"
	ReadinessCorrupt     = "corrupt"
	ReadinessUnavailable = "unavailable"
)

// RemediationAction is selected from a fixed command allowlist. It contains no
// shell fragments derived from unvalidated input.
type RemediationAction struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

// Readiness is the versioned, client-safe local projection of a credential.
// It never includes secret bytes and its construction performs no network I/O.
type Readiness struct {
	SpecVersion    string             `json:"spec_version"`
	Ref            Ref                `json:"ref"`
	Configured     bool               `json:"configured"`
	State          string             `json:"state"`
	Source         string             `json:"source"`
	Backend        string             `json:"backend,omitempty"`
	Revision       string             `json:"revision,omitempty"`
	RedactedDigest string             `json:"redacted_digest,omitempty"`
	Capabilities   []string           `json:"capabilities"`
	Owner          string             `json:"owner"`
	Action         *RemediationAction `json:"action,omitempty"`
}

// ReadinessResolver exposes the existing local-only, secret-free readiness
// projection to approved owner processes. It performs no provider network
// request and never returns credential bytes.
type ReadinessResolver interface {
	Readiness(Ref) Readiness
}

// CredentialUseGrant is an experimental pre-v1 policy claim for one local
// credential use. Consumer/Owner strings are not strong process identity; the
// V0.1 trust boundary is the current OS user.
type CredentialUseGrant struct {
	Ref         Ref       `json:"ref"`
	Consumer    string    `json:"consumer"`
	Capability  string    `json:"capability"`
	Owner       string    `json:"owner"`
	Operation   string    `json:"operation"`
	Profile     string    `json:"profile,omitempty"`
	Revision    string    `json:"revision"`
	Project     string    `json:"project"`
	User        string    `json:"user"`
	ApprovalRef string    `json:"approval_ref,omitempty"`
	Audience    string    `json:"audience"`
	ExpiresAt   time.Time `json:"expires_at"`
}
