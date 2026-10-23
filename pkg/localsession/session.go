// Package localsession defines the private host protocol for a supervised
// Pinax process. Bootstrap frames belong on private stdin, never in renderer
// payloads, argv, environment variables, logs or persisted receipts.
package localsession

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strconv"
)

const (
	BootstrapVersion = "pinax.local_session.bootstrap.v1alpha1"
	EventVersion     = "pinax.local_session.event.v1alpha1"
	MaxFrameBytes    = 16 * 1024
)

// Bootstrap is the owner-defined private stdin frame. Creating this value is
// not authorization; the owner validates grants and the selected vault.
type Bootstrap struct {
	SchemaVersion string   `json:"schema_version"`
	SessionNonce  string   `json:"session_nonce"`
	Bearer        string   `json:"bearer"`
	BindingID     string   `json:"binding_id"`
	VaultRoot     string   `json:"vault_root"`
	Scopes        []string `json:"scopes"`
	Readonly      bool     `json:"readonly"`
}

// HasWriterScope uses the frozen writer vocabulary, not a prefix match.
func (f Bootstrap) HasWriterScope() bool {
	for _, scope := range f.Scopes {
		switch scope {
		case "writer.capture", "writer.editor", "writer.organizer":
			return true
		}
	}
	return false
}

// Event is the owner stdout frame; hosts must never forward raw upstream
// diagnostics. DecodeReady returns only a validated ready event.
type Event struct {
	SchemaVersion  string `json:"schema_version"`
	Type           string `json:"type"`
	SessionID      string `json:"session_id,omitempty"`
	SessionNonce   string `json:"session_nonce,omitempty"`
	BindingID      string `json:"binding_id,omitempty"`
	Address        string `json:"address,omitempty"`
	ContractDigest string `json:"contract_digest,omitempty"`
	Readonly       *bool  `json:"readonly,omitempty"`
	Code           string `json:"code,omitempty"`
	Message        string `json:"message,omitempty"`
}

// DecodeReady validates one bounded complete stdout frame against the host's
// expected session and manifest digest. The address is a literal loopback
// origin, not a caller-selected URL. Errors never include frame contents.
func DecodeReady(frame []byte, nonce, binding, manifestDigest string, readonly bool) (Event, error) {
	fail := func() (Event, error) { return Event{}, errors.New("session_ready_invalid") }
	if len(frame) == 0 || len(frame) > MaxFrameBytes || len(nonce) < 16 || len(nonce) > 128 || binding == "" || !digestValid(manifestDigest) {
		return fail()
	}
	// Check duplicate keys explicitly: encoding/json otherwise accepts the last
	// duplicate, which is ambiguous at a private-channel trust boundary.
	probe := json.NewDecoder(bytes.NewReader(frame))
	token, err := probe.Token()
	if err != nil || token != json.Delim('{') {
		return fail()
	}
	seen := map[string]bool{}
	for probe.More() {
		key, err := probe.Token()
		if err != nil {
			return fail()
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return fail()
		}
		switch name {
		case "schema_version", "type", "session_id", "session_nonce", "binding_id", "address", "contract_digest", "readonly":
		default:
			return fail()
		}
		seen[name] = true
		var value json.RawMessage
		if probe.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fail()
		}
	}
	if _, err = probe.Token(); err != nil {
		return fail()
	}
	if _, err = probe.Token(); err != io.EOF {
		return fail()
	}
	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()
	var event Event
	if decoder.Decode(&event) != nil {
		return fail()
	}
	if event.SchemaVersion != EventVersion || event.Type != "ready" || event.SessionNonce != nonce || event.BindingID != binding || event.ContractDigest != manifestDigest || event.Readonly == nil || *event.Readonly != readonly || event.Code != "" || event.Message != "" || event.SessionID == "" || len(event.SessionID) > 256 {
		return fail()
	}
	address, err := url.Parse(event.Address)
	if err != nil || address.Scheme != "http" || address.Hostname() != "127.0.0.1" || address.User != nil || address.Path != "" || address.RawQuery != "" || address.ForceQuery || address.Fragment != "" || address.Opaque != "" {
		return fail()
	}
	port, err := strconv.Atoi(address.Port())
	if err != nil || port < 1 || port > 65535 || event.Address != "http://127.0.0.1:"+strconv.Itoa(port) {
		return fail()
	}
	return event, nil
}
func digestValid(value string) bool {
	if len(value) != 71 || value[:7] != "sha256:" {
		return false
	}
	for _, c := range value[7:] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
