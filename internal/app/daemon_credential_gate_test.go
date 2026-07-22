package app

import (
	"testing"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
)

func TestDaemonGateDeviceProfileAllowed(t *testing.T) {
	r := AssessDaemonCredentialReadiness("device-profile", nil, false)
	if !r.Allowed {
		t.Fatalf("device-profile should be allowed without a source: %s", r.Reason)
	}
	if r.Mode != "device-profile" {
		t.Fatalf("mode: %s", r.Mode)
	}
}

func TestDaemonGateRepositoryEncryptedRequiresEnvelope(t *testing.T) {
	r := AssessDaemonCredentialReadiness("repository-encrypted", projectsecrets.EnvSource("X"), false)
	if r.Allowed {
		t.Fatal("should be degraded when envelope missing")
	}
	if r.Reason == "" {
		t.Fatal("reason should be set")
	}
}

func TestDaemonGateRepositoryEncryptedRequiresNonInteractiveSource(t *testing.T) {
	// envelope present but no source → degraded.
	r := AssessDaemonCredentialReadiness("repository-encrypted", nil, true)
	if r.Allowed {
		t.Fatal("should be degraded without a non-interactive source")
	}
	// prompt source → rejected (daemon must not prompt).
	r = AssessDaemonCredentialReadiness("repository-encrypted", projectsecrets.NewPromptSource(), true)
	if r.Allowed {
		t.Fatal("prompt unlock must be rejected for the daemon")
	}
}

func TestDaemonGateRepositoryEncryptedAllowsNonInteractive(t *testing.T) {
	for _, src := range []projectsecrets.UnlockSource{
		projectsecrets.EnvSource("PINAX_REPO_PASS"),
		projectsecrets.StaticSource([]byte("x")),
	} {
		r := AssessDaemonCredentialReadiness("repository-encrypted", src, true)
		if !r.Allowed {
			t.Fatalf("non-interactive source should be allowed: %s", r.Reason)
		}
		if r.Source == "" {
			t.Fatal("source descriptor should be set")
		}
	}
}

func TestDaemonGateRejectsUnsupportedMode(t *testing.T) {
	r := AssessDaemonCredentialReadiness("unknown-mode", nil, true)
	if r.Allowed {
		t.Fatal("unsupported mode should be rejected")
	}
}

func TestDaemonGateEmptyModeDefaultsDeviceProfile(t *testing.T) {
	r := AssessDaemonCredentialReadiness("", nil, false)
	if !r.Allowed || r.Mode != "device-profile" {
		t.Fatalf("empty mode should default to device-profile allowed: %+v", r)
	}
}
