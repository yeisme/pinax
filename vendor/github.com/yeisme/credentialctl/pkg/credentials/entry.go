package credentials

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Entry is a registry record for one shared credential. It is the only thing
// persisted about a credential besides the secret bytes (which live in the
// backend). Entry never contains the secret: RedactedDigest is a truncated hash
// that lets humans confirm "same key as before" without revealing the key.
type Entry struct {
	Ref                 Ref       `json:"ref"`
	Provider            string    `json:"provider"`
	Account             string    `json:"account"`
	Backend             string    `json:"backend"`
	Revision            string    `json:"revision"`
	AllowedConsumers    []string  `json:"allowed_consumers"`
	AllowedCapabilities []string  `json:"allowed_capabilities"`
	Disabled            bool      `json:"disabled,omitempty"`
	UpdatedAt           time.Time `json:"updated_at"`
	RedactedDigest      string    `json:"redacted_digest"`
}

// RedactedDigestSize is the number of hex characters retained from the sha256
// of the secret. Enough to distinguish rotations, far too little to recover it.
const RedactedDigestSize = 8

// DigestSecret returns the redacted digest "sha256:<first8>" for a secret. The
// full digest is intentionally never stored or returned.
func DigestSecret(secret []byte) string {
	sum := sha256.Sum256(secret)
	return "sha256:" + hex.EncodeToString(sum[:])[:RedactedDigestSize]
}

// MarshalJSON renders Ref as its URI string so the registry/output reads
// "yeisme-credential://..." rather than a nested object.
func (r Ref) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

// UnmarshalJSON restores a Ref from its URI string.
func (r *Ref) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := ParseRef(s)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}
