package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// S3CredentialFormat is the typed payload format identifier stored as the entry
// format in the secrets envelope and the declaration.
const S3CredentialFormat = "s3_credentials.v1"

// S3Credentials is the decrypted logical structure of one S3/COS static
// credential bundle. The session token is optional (used for STS-derived
// temporary credentials); access key id and secret access key are required.
type S3Credentials struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token,omitempty"`
}

// s3CredentialsRaw mirrors S3Credentials for strict decoding: json.RawMessage
// fields let us detect and reject unknown top-level keys without ever surfacing
// the supplied values in errors.
type s3CredentialsRaw struct {
	AccessKeyID     json.RawMessage `json:"access_key_id"`
	SecretAccessKey json.RawMessage `json:"secret_access_key"`
	SessionToken    json.RawMessage `json:"session_token,omitempty"`
}

// ParseS3Credentials decodes a strict s3_credentials.v1 payload. It rejects
// malformed JSON, duplicate keys, unknown fields and missing required fields.
// Errors reference field NAMES only; they never include the supplied value,
// control characters or the raw payload.
func ParseS3Credentials(payload []byte) (S3Credentials, error) {
	// Reject duplicate keys before strict decoding (DisallowUnknownFields does
	// not catch them) by decoding into a raw map keyed on json.RawMessage.
	if err := rejectDuplicateKeys(payload); err != nil {
		return S3Credentials{}, err
	}
	var raw s3CredentialsRaw
	dec := json.NewDecoder(strings.NewReader(string(payload)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return S3Credentials{}, fmt.Errorf("s3_credentials: invalid payload: %w", redactJSONError(err))
	}
	if len(raw.AccessKeyID) == 0 || strings.TrimSpace(string(trimRawString(raw.AccessKeyID))) == "" {
		return S3Credentials{}, errors.New("s3_credentials: missing required field access_key_id")
	}
	if len(raw.SecretAccessKey) == 0 || strings.TrimSpace(string(trimRawString(raw.SecretAccessKey))) == "" {
		return S3Credentials{}, errors.New("s3_credentials: missing required field secret_access_key")
	}
	accessKey, err := decodeStringField("access_key_id", raw.AccessKeyID)
	if err != nil {
		return S3Credentials{}, err
	}
	secret, err := decodeStringField("secret_access_key", raw.SecretAccessKey)
	if err != nil {
		return S3Credentials{}, err
	}
	var session string
	if len(raw.SessionToken) > 0 {
		session, err = decodeStringField("session_token", raw.SessionToken)
		if err != nil {
			return S3Credentials{}, err
		}
	}
	return S3Credentials{AccessKeyID: accessKey, SecretAccessKey: secret, SessionToken: session}, nil
}

func decodeStringField(name string, raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", fmt.Errorf("s3_credentials: field %s must be a string", name)
	}
	return s, nil
}

func trimRawString(raw json.RawMessage) []byte {
	s := strings.TrimSpace(string(raw))
	s = strings.TrimPrefix(s, `"`)
	s = strings.TrimSuffix(s, `"`)
	return []byte(s)
}

// redactJSONError strips values from json error messages so a malformed payload
// never leaks through an error string.
func redactJSONError(err error) error {
	msg := err.Error()
	for _, marker := range []string{"not a string", "cannot unmarshal", "invalid character"} {
		if strings.Contains(msg, marker) {
			return errors.New("malformed credential payload")
		}
	}
	return err
}

// rejectDuplicateKeys returns an error if the JSON object contains the same key
// twice. Duplicate keys could otherwise let a caller shadow a field with a
// differently-typed value.
func rejectDuplicateKeys(payload []byte) error {
	tokens, err := tokenizeJSONObject(payload)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, key := range tokens {
		if seen[key] {
			return fmt.Errorf("s3_credentials: duplicate field %s", key)
		}
		seen[key] = true
	}
	return nil
}

// tokenizeJSONObject extracts the top-level string keys of a JSON object using
// encoding/json's token decoder. It does not evaluate values.
func tokenizeJSONObject(payload []byte) ([]string, error) {
	dec := json.NewDecoder(strings.NewReader(string(payload)))
	t, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("s3_credentials: payload is not a JSON object")
	}
	delim, ok := t.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("s3_credentials: payload is not a JSON object")
	}
	var keys []string
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("s3_credentials: malformed object key")
		}
		key, ok := t.(string)
		if !ok {
			return nil, fmt.Errorf("s3_credentials: object key is not a string")
		}
		keys = append(keys, key)
		// Skip the value (any JSON value, including nested objects/arrays).
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("s3_credentials: malformed value for field %s", key)
		}
	}
	return keys, nil
}

// ValidateS3Credentials reports field-level problems without echoing values.
func ValidateS3Credentials(c S3Credentials) error {
	if strings.TrimSpace(c.AccessKeyID) == "" {
		return errors.New("s3_credentials: missing access_key_id")
	}
	if strings.TrimSpace(c.SecretAccessKey) == "" {
		return errors.New("s3_credentials: missing secret_access_key")
	}
	return nil
}
