package syncdaemon

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorCodeClassifiesKeyIDMismatch(t *testing.T) {
	t.Parallel()
	err := errors.New("key ID mismatch: envelope=key_old, key=key_new")
	if got := errorCode(err); got != "encryption_key_mismatch" {
		t.Fatalf("errorCode = %q", got)
	}
	message := daemonErrorMessage(err, "encryption_key_mismatch")
	if !strings.Contains(message, "restore the previous encryption secret") {
		t.Fatalf("message lacks safe recovery hint: %q", message)
	}
}
