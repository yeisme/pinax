package redaction

import (
	"strings"
	"testing"
)

func TestCryptoCloudRedaction(t *testing.T) {
	input := "Authorization: Bearer raw-token-123 path=notes/alpha.md secret_ref=op://pinax/cloud-token"
	got := Cloud(input)
	for _, forbidden := range []string{"raw-token-123", "notes/alpha.md", "op://pinax/cloud-token"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("redaction leaked %q in %q", forbidden, got)
		}
	}
	for _, want := range []string{"Authorization: Bearer [REDACTED]", "path=[REDACTED_PATH]", "secret_ref=[REDACTED_SECRET_REF]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("redaction missing %q in %q", want, got)
		}
	}
}
