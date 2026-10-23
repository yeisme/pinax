package localsession

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

const RecoveryChallengeVersion = "pinax.local_session.recovery_challenge.v1alpha1"

// RecoveryChallenge is private host metadata emitted only in explicitly opted
// in recovery mode, before ready. It does not contain the old operation or
// authorize lookup. Never forward it into the renderer or application logs.
type RecoveryChallenge struct {
	SchemaVersion       string `json:"schema_version"`
	Type                string `json:"type"`
	SessionNonce        string `json:"session_nonce"`
	BindingID           string `json:"binding_id"`
	VaultIdentityDigest string `json:"vault_identity_digest"`
}

// DecodeRecoveryChallenge verifies the selected process's private challenge.
// The host must separately approve exactly which old receipt may be read.
func DecodeRecoveryChallenge(frame []byte, nonce, binding string) (RecoveryLookupContext, error) {
	fail := func() (RecoveryLookupContext, error) {
		return RecoveryLookupContext{}, errors.New("recovery_challenge_invalid")
	}
	if len(frame) == 0 || len(frame) > 1024 || !utf8.Valid(frame) {
		return fail()
	}
	d := json.NewDecoder(bytes.NewReader(frame))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return fail()
	}
	fields := map[string]bool{"schema_version": false, "type": false, "session_nonce": false, "binding_id": false, "vault_identity_digest": false}
	for d.More() {
		tok, err = d.Token()
		if err != nil {
			return fail()
		}
		key, ok := tok.(string)
		if !ok {
			return fail()
		}
		seen, known := fields[key]
		if !known || seen {
			return fail()
		}
		fields[key] = true
		tok, err = d.Token()
		if err != nil {
			return fail()
		}
		value, ok := tok.(string)
		if !ok {
			return fail()
		}
		for _, r := range value {
			if r < 0x21 || r > 0x7e {
				return fail()
			}
		}
	}
	if tok, err = d.Token(); err != nil || tok != json.Delim('}') {
		return fail()
	}
	if _, err = d.Token(); err != io.EOF {
		return fail()
	}
	for _, seen := range fields {
		if !seen {
			return fail()
		}
	}
	var c RecoveryChallenge
	if json.Unmarshal(frame, &c) != nil || c.SchemaVersion != RecoveryChallengeVersion || c.Type != "recovery_challenge" || c.SessionNonce != nonce || c.BindingID != binding || len(nonce) < 16 || len(nonce) > 128 || !recoveryOpaque.MatchString(nonce) || len(binding) == 0 || len(binding) > 128 || !recoveryOpaque.MatchString(binding) || !digestValid(c.VaultIdentityDigest) {
		return fail()
	}
	return RecoveryLookupContext{SessionNonce: c.SessionNonce, BindingID: c.BindingID, VaultIdentityDigest: c.VaultIdentityDigest}, nil
}
