package connection

import "testing"

func TestConnectionLegacyDefaultAndTransportSeparation(t *testing.T) {
	t.Parallel()

	legacy := Resolve(ResolveInput{Endpoint: "https://notes.example.test", EndpointSource: "flag", CredentialSource: "flag_token_file"})
	if legacy.Mode != ModeRemoteService || legacy.ModeSource != "legacy_default" || legacy.Transport != TransportHTTPS || legacy.Status != "degraded" {
		t.Fatalf("legacy descriptor = %#v", legacy)
	}
	selfHosted := Resolve(ResolveInput{Endpoint: "https://notes.example.test", EndpointSource: "project", Mode: ModeSelfHostedService, ModeSource: "project", CredentialSource: "env_token_file"})
	if selfHosted.Mode != ModeSelfHostedService || selfHosted.Transport != TransportHTTPS || selfHosted.Status != "ready" {
		t.Fatalf("self-hosted descriptor = %#v", selfHosted)
	}
}

func TestConnectionLoopbackAndEmptyResolveToLocalVault(t *testing.T) {
	t.Parallel()

	embedded := Resolve(ResolveInput{})
	if embedded.Mode != ModeLocalVault || embedded.Transport != TransportEmbedded || embedded.ModeSource != "default" {
		t.Fatalf("embedded descriptor = %#v", embedded)
	}
	loopback := Resolve(ResolveInput{Endpoint: "http://127.0.0.1:8080", EndpointSource: "env"})
	if loopback.Mode != ModeLocalVault || loopback.Transport != TransportLoopbackHTTP || loopback.TLSRequired {
		t.Fatalf("loopback descriptor = %#v", loopback)
	}
}

func TestConnectionRejectsRemoteHTTPModeConflictAndCredentialConflict(t *testing.T) {
	t.Parallel()

	descriptor := Resolve(ResolveInput{Endpoint: "http://notes.example.test", Mode: ModeLocalVault, ModeSource: "flag", CredentialSource: "conflict"})
	if descriptor.Status != "blocked" || descriptor.Transport != "http" {
		t.Fatalf("blocked descriptor = %#v", descriptor)
	}
	for _, want := range []string{"tls_required", "connection_mode_endpoint_conflict", "credential_source_conflict"} {
		found := false
		for _, blocker := range descriptor.Blockers {
			found = found || blocker == want
		}
		if !found {
			t.Fatalf("descriptor missing blocker %q: %#v", want, descriptor)
		}
	}
}

func TestConnectionInspectSanitizesEndpoint(t *testing.T) {
	t.Parallel()

	descriptor := Resolve(ResolveInput{Endpoint: "https://user:pass@notes.example.test/root?opaque=value#fragment", CredentialSource: "flag_token_file"})
	if descriptor.Endpoint != "https://notes.example.test/root" || descriptor.ManifestRef != "https://notes.example.test/root/v1/manifest" {
		t.Fatalf("sanitized descriptor = %#v", descriptor)
	}
}
