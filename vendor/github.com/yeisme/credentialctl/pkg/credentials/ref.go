// Package credentials is the stable, secret-free public library that
// credentialctl and its consumers share. It exposes credential references,
// registry entry shapes, the consumer/capability allowlist and a Resolver
// contract. Real secret bytes never live in this package: it only carries
// references, redacted digests and non-secret source metadata.
package credentials

import (
	"fmt"
	"regexp"
	"strings"
)

var refSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

const (
	// Scheme is the stable URI scheme for shared credential references.
	Scheme = "yeisme-credential"
	// DefaultPreset bundles the first-party local consumer/capability allowlist.
	DefaultPreset = "local-ai"
)

// Ref is a stable, secret-free reference to a shared credential of the form
// yeisme-credential://<provider>/<account>. It is safe to log, persist and
// print; it never carries the secret itself.
type Ref struct {
	Provider string
	Account  string
}

// String renders the ref as a yeisme-credential:// URI.
func (r Ref) String() string {
	return fmt.Sprintf("%s://%s/%s", Scheme, r.Provider, r.Account)
}

// Key is the registry/storage key for the ref (provider/account).
func (r Ref) Key() string {
	return r.Provider + "/" + r.Account
}

// Validate rejects an unsafe Ref even when a caller constructed the public
// struct directly instead of using ParseRef.
func (r Ref) Validate() error {
	parsed, err := ParseRef(r.String())
	if err != nil {
		return err
	}
	if parsed != r {
		return fmt.Errorf("credential ref does not round-trip")
	}
	return nil
}

// ParseRef parses a yeisme-credential://<provider>/<account> URI. Provider and
// account must be non-empty and must not contain spaces or slashes so the ref
// round-trips unambiguously.
func ParseRef(s string) (Ref, error) {
	prefix := Scheme + "://"
	if !strings.HasPrefix(s, prefix) {
		return Ref{}, fmt.Errorf("invalid credential ref %q: missing %q scheme", s, Scheme)
	}
	rest := strings.TrimPrefix(s, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Ref{}, fmt.Errorf("invalid credential ref %q: expected %s://<provider>/<account>", s, Scheme)
	}
	provider, account := parts[0], parts[1]
	if !validRefSegment(provider) || !validRefSegment(account) {
		return Ref{}, fmt.Errorf("invalid credential ref %q: provider and account must use safe alphanumeric, dot, underscore or dash segments", s)
	}
	return Ref{Provider: provider, Account: account}, nil
}

func validRefSegment(s string) bool {
	return s != "." && s != ".." && !strings.ContainsAny(s, `/\\:`) && refSegmentPattern.MatchString(s)
}
