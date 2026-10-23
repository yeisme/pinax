package localsession

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"time"
	"unicode/utf8"
)

const (
	RecoveryGrantVersion     = "pinax.local_session.recovery_lookup.v1alpha1"
	MaxRecoveryGrantBytes    = 4096
	RecoveryGrantMaxLifetime = 5 * time.Minute
)

// RecoveryLookupGrant is a private host instruction, NOT a bearer credential.
// Only an authenticated, explicitly approved host channel may supply it. A
// renderer, HTTP caller or stored digest cannot create that authority. It is
// inert until the owner binds it to the newly created session and selected
// vault, and verifies the original ledger row. Never log or persist the frame.
// No old session secret, body, proposal or filesystem path belongs here.
// This contract does not authorize apply, retry, reconcile or source access.
type RecoveryLookupGrant struct {
	SchemaVersion       string `json:"schema_version"`
	SessionNonce        string `json:"session_nonce"`
	BindingID           string `json:"binding_id"`
	VaultIdentityDigest string `json:"vault_identity_digest"`
	OperationID         string `json:"operation_id"`
	CapabilityID        string `json:"capability_id"`
	OperationBindingID  string `json:"operation_binding_id"`
	PrincipalDigest     string `json:"principal_digest"`
	ScopeDigest         string `json:"scope_digest"`
	RequestDigest       string `json:"request_digest"`
	Action              string `json:"action"`
	IssuedAt            string `json:"issued_at"`
	ExpiresAt           string `json:"expires_at"`
}

// RecoveryLookupContext must come from trusted host/session state, not from
// the grant itself. VaultIdentityDigest is an owner-issued binding to the
// selected vault identity; a hash of a caller-selected pathname is insufficient.
type RecoveryLookupContext struct {
	SessionNonce        string
	BindingID           string
	VaultIdentityDigest string
}

var recoveryOpaque = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)

// DecodeRecoveryLookupGrant validates one closed flat frame and its temporal
// and identity binding. Success establishes consistency, NOT host authority or
// ledger ownership. Consumers must revalidate expiry on every lookup, reject
// revoked sessions and compare all original row identities before disclosure.
func DecodeRecoveryLookupGrant(frame []byte, expected RecoveryLookupContext, now time.Time) (RecoveryLookupGrant, error) {
	fail := func() (RecoveryLookupGrant, error) {
		return RecoveryLookupGrant{}, errors.New("recovery_grant_invalid")
	}
	if len(frame) == 0 || len(frame) > MaxRecoveryGrantBytes || !utf8.Valid(frame) {
		return fail()
	}
	decoder := json.NewDecoder(bytes.NewReader(frame))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return fail()
	}
	fields := map[string]bool{"schema_version": false, "session_nonce": false, "binding_id": false, "vault_identity_digest": false, "operation_id": false, "capability_id": false, "operation_binding_id": false, "principal_digest": false, "scope_digest": false, "request_digest": false, "action": false, "issued_at": false, "expires_at": false}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return fail()
		}
		name, ok := token.(string)
		if !ok {
			return fail()
		}
		seen, known := fields[name]
		if !known || seen {
			return fail()
		}
		fields[name] = true
		token, err = decoder.Token()
		if err != nil {
			return fail()
		}
		value, ok := token.(string)
		if !ok {
			return fail()
		}
		// All private identifiers and timestamps in this version are ASCII.
		// This also rejects JSON's lossy replacement of unpaired surrogates.
		for _, r := range value {
			if r < 0x21 || r > 0x7e {
				return fail()
			}
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return fail()
	}
	if _, err = decoder.Token(); err != io.EOF {
		return fail()
	}
	for _, seen := range fields {
		if !seen {
			return fail()
		}
	}
	var grant RecoveryLookupGrant
	if json.Unmarshal(frame, &grant) != nil || !grant.ValidFor(expected, now) {
		return fail()
	}
	return grant, nil
}

// ValidFor is also required immediately before a consumer discloses a row.
// It does not verify the caller's trusted-channel provenance or vault identity.
func (g RecoveryLookupGrant) ValidFor(expected RecoveryLookupContext, now time.Time) bool {
	if g.SchemaVersion != RecoveryGrantVersion || g.Action != "lookup" || now.IsZero() {
		return false
	}
	opaque := func(s string, min, max int) bool {
		return len(s) >= min && len(s) <= max && recoveryOpaque.MatchString(s)
	}
	if !opaque(g.SessionNonce, 16, 128) || !opaque(g.BindingID, 1, 128) || !opaque(g.OperationID, 1, 128) {
		return false
	}
	if g.SessionNonce != expected.SessionNonce || g.BindingID != expected.BindingID || g.VaultIdentityDigest != expected.VaultIdentityDigest {
		return false
	}
	for _, d := range []string{g.VaultIdentityDigest, g.PrincipalDigest, g.ScopeDigest, g.RequestDigest} {
		if !digestValid(d) {
			return false
		}
	}
	switch g.CapabilityID {
	case "writer.apply", "writer.daily.apply", "writer.daily.edit", "writer.tags.apply":
		if g.OperationBindingID != "rest."+g.CapabilityID {
			return false
		}
	default:
		return false
	}
	issued, e1 := time.Parse(time.RFC3339Nano, g.IssuedAt)
	expires, e2 := time.Parse(time.RFC3339Nano, g.ExpiresAt)
	if e1 != nil || e2 != nil || g.IssuedAt != issued.UTC().Format(time.RFC3339Nano) || g.ExpiresAt != expires.UTC().Format(time.RFC3339Nano) {
		return false
	}
	return !issued.After(now) && expires.After(now) && expires.After(issued) && expires.Sub(issued) <= RecoveryGrantMaxLifetime
}
