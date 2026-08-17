package projectsecrets

import (
	"fmt"
	"time"
)

// Grant binds a single resolution attempt to project, repository, consumer,
// capability, audience, operation, an entry allowlist, the envelope digest, an
// expiry and an approval reference. The Resolver verifies every field before
// any plaintext is touched; a mismatch returns GrantDeniedError and never reads
// or compares entry values.
type Grant struct {
	Project    string
	Repository string
	Consumer   string // approved owner consumer (e.g. "pinax"); never a broker/client
	Capability string // e.g. "object-storage"
	Audience   string // optional binding (e.g. remote bucket or workspace)
	Operation  string // e.g. "sync-pull"
	Entries    []string
	Digest     string    // envelope digest the grant was issued against
	Expiry     time.Time // zero = no expiry (only for trusted local callers)
	Approval   string    // opaque approval reference (auditable, non-secret)
}

// Validate checks the structural completeness of a grant. It does not check the
// consumer allowlist or digest match — those require envelope/policy context.
func (g Grant) Validate() error {
	if g.Project == "" {
		return fmt.Errorf("grant: project required")
	}
	if g.Repository == "" {
		return fmt.Errorf("grant: repository required")
	}
	if g.Consumer == "" {
		return fmt.Errorf("grant: consumer required")
	}
	if g.Capability == "" {
		return fmt.Errorf("grant: capability required")
	}
	if len(g.Entries) == 0 {
		return fmt.Errorf("grant: at least one entry required")
	}
	if g.Digest == "" {
		return fmt.Errorf("grant: digest required")
	}
	if !g.Expiry.IsZero() && time.Now().After(g.Expiry) {
		return fmt.Errorf("grant: expired")
	}
	return nil
}

// ConsumerScope defines what an approved consumer may resolve. Entries, when
// non-empty, further narrows the per-call entry allowlist.
type ConsumerScope struct {
	Capabilities []string
	Entries      []string // optional superset constraint; empty = no extra constraint
}

// AllowlistPolicy approves named owner consumers and denies everything else.
// Browser, client-runtime broker, composition agent and any unlisted consumer
// are denied without reading plaintext.
type AllowlistPolicy struct {
	Consumers map[string]ConsumerScope
}

// NewAllowlistPolicy builds a policy from a consumer→(capabilities, entries) map.
func NewAllowlistPolicy(consumers map[string]ConsumerScope) *AllowlistPolicy {
	return &AllowlistPolicy{Consumers: consumers}
}

// Approve verifies the grant against the policy and the loaded envelope. It
// returns nil only when the consumer is an approved owner with a matching
// capability, every requested entry is permitted, and the envelope digest
// matches the grant (so a swapped/newer envelope fails closed).
func (p *AllowlistPolicy) Approve(g Grant, env EnvelopeInfo) error {
	if err := g.Validate(); err != nil {
		return GrantDeniedError(err.Error())
	}
	scope, ok := p.Consumers[g.Consumer]
	if !ok {
		return GrantDeniedError("consumer not approved")
	}
	if !contains(scope.Capabilities, g.Capability) {
		return GrantDeniedError("capability not approved for consumer")
	}
	if g.Project != env.Project || g.Repository != env.Repository {
		return GrantDeniedError("project/repository mismatch")
	}
	if g.Digest != env.Digest {
		return GrantDeniedError("envelope digest mismatch")
	}
	allowed := scope.Entries
	for _, entry := range g.Entries {
		if _, ok := env.Entries[entry]; !ok {
			return EntryMissingError("entry " + entry)
		}
		if len(allowed) > 0 && !contains(allowed, entry) {
			return GrantDeniedError("entry not approved for consumer")
		}
	}
	return nil
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
