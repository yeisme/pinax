package projectsecrets

import "github.com/yeisme/credentialctl/internal/projectcrypto"

// Provider constants for the supported envelope providers.
const (
	ProviderPassphraseV1 = "passphrase-v1"
)

// ResolutionMetadata is the redacted view returned alongside a Snapshot. It
// never contains plaintext, ciphertext values, or the passphrase.
type ResolutionMetadata struct {
	Project    string   `json:"project"`
	Repository string   `json:"repository"`
	Provider   string   `json:"provider"`
	Digest     string   `json:"digest"`
	Source     string   `json:"source"` // prompt | keychain | file | env
	Entries    []string `json:"entries"`
}

// Snapshot holds short-lived decrypted entry bytes for one approved resolution.
// Callers MUST call Close after the provider operation so internal byte slices
// are best-effort cleared, and MUST NOT convert entries to long-lived strings,
// log them, or cache them.
type Snapshot struct {
	metadata ResolutionMetadata
	entries  map[string][]byte
	closed   bool
}

// Entry returns the plaintext bytes for one entry name. The returned slice is
// owned by the Snapshot; callers must copy before Close if they need to retain
// it for the duration of a provider call (and should wipe that copy after).
func (s *Snapshot) Entry(name string) ([]byte, bool) {
	if s == nil || s.closed {
		return nil, false
	}
	b, ok := s.entries[name]
	return b, ok
}

// Metadata returns the redacted resolution metadata.
func (s *Snapshot) Metadata() ResolutionMetadata {
	if s == nil {
		return ResolutionMetadata{}
	}
	return s.metadata
}

// Close best-effort zeroes the decrypted entry slices. After Close, Entry
// returns nil. Go does not guarantee physical zeroing, so callers must still
// avoid retaining copies.
func (s *Snapshot) Close() error {
	if s == nil {
		return nil
	}
	for name, b := range s.entries {
		projectcrypto.Wipe(b)
		delete(s.entries, name)
	}
	s.closed = true
	return nil
}
