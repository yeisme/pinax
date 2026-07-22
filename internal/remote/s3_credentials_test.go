package remote

import (
	"strings"
	"testing"
)

func TestParseS3CredentialsRoundTrip(t *testing.T) {
	payload := []byte(`{"access_key_id":"AKID123","secret_access_key":"SK456","session_token":"tok"}`)
	got, err := ParseS3Credentials(payload)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.AccessKeyID != "AKID123" || got.SecretAccessKey != "SK456" || got.SessionToken != "tok" {
		t.Fatalf("fields mismatch: %+v", got)
	}
}

func TestParseS3CredentialsOptionalSessionToken(t *testing.T) {
	got, err := ParseS3Credentials([]byte(`{"access_key_id":"AKID","secret_access_key":"SK"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.SessionToken != "" {
		t.Fatalf("session token should be empty, got %q", got.SessionToken)
	}
}

func TestParseS3CredentialsRejectsUnknownField(t *testing.T) {
	_, err := ParseS3Credentials([]byte(`{"access_key_id":"AKID","secret_access_key":"SK","extra":"leak"}`))
	if err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestParseS3CredentialsRejectsMissingRequired(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"secret_access_key":"SK"}`),
		[]byte(`{"access_key_id":"AKID"}`),
		[]byte(`{"access_key_id":"","secret_access_key":"SK"}`),
	}
	for i, c := range cases {
		_, err := ParseS3Credentials(c)
		if err == nil {
			t.Fatalf("case %d: missing required accepted", i)
		}
	}
}

func TestParseS3CredentialsRejectsDuplicateKey(t *testing.T) {
	_, err := ParseS3Credentials([]byte(`{"access_key_id":"AKID","access_key_id":"x","secret_access_key":"SK"}`))
	if err == nil {
		t.Fatal("duplicate key accepted")
	}
}

func TestParseS3CredentialsRejectsNonObject(t *testing.T) {
	for _, c := range [][]byte{
		[]byte(`[]`),
		[]byte(`"string"`),
		[]byte(`42`),
		[]byte(``),
	} {
		_, err := ParseS3Credentials(c)
		if err == nil {
			t.Fatalf("non-object accepted: %q", c)
		}
	}
}

func TestParseS3CredentialsRejectsMalformed(t *testing.T) {
	_, err := ParseS3Credentials([]byte(`{not json`))
	if err == nil {
		t.Fatal("malformed accepted")
	}
}

func TestParseS3CredentialsErrorsNeverLeakValue(t *testing.T) {
	secret := "SUPERSECRET_VALUE_42"
	payload := []byte(`{"access_key_id":"` + secret + `","secret_access_key":"` + secret + `","bogus":1}`)
	_, err := ParseS3Credentials(payload)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked value: %v", err)
	}
}
