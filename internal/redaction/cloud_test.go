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

func TestCloudRedactsS3AndRcloneCredentialForms(t *testing.T) {
	in := "login failed: access_key_id=AKIDEXAMPLE secret_access_key=wJalrXUtnFEMI password=hunter2 api_key=sk-123 rclone: refresh blocked"
	out := Cloud(in)
	for _, leaked := range []string{"AKIDEXAMPLE", "wJalrXUtnFEMI", "hunter2", "sk-123"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("cloud redaction leaked %q: %s", leaked, out)
		}
	}
	for _, want := range []string{"access_key_id=[REDACTED]", "secret_access_key=[REDACTED]", "password=[REDACTED]", "api_key=[REDACTED]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("cloud redaction missing %q: %s", want, out)
		}
	}
}
