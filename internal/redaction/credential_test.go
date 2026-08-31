package redaction

import (
	"strings"
	"testing"
)

func TestCredentialsRedactTokenFileSecretAndAuthorization(t *testing.T) {
	t.Parallel()

	const secret = "pinax-token-file-sensitive-value"
	input := "request failed Authorization: Bearer " + secret + " token=" + secret
	redacted := Credentials(input, secret)
	if strings.Contains(redacted, secret) {
		t.Fatalf("credential redaction leaked secret: %s", redacted)
	}
	if !strings.Contains(redacted, "[REDACTED]") {
		t.Fatalf("credential redaction marker missing: %s", redacted)
	}
}
