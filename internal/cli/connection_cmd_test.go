package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectionInspectReportsLegacyDefaultWithoutSecret(t *testing.T) {
	t.Setenv("PINAX_API_URL", "")
	t.Setenv("PINAX_API_TOKEN", "")
	t.Setenv("PINAX_API_TOKEN_FILE", "")
	const secret = "connection-inspect-secret-sentinel"

	out, err := executeConnectionCommand(t, "--api-url", "https://notes.example.test", "--api-token", secret, "connection", "inspect", "--json")
	if err != nil {
		t.Fatalf("connection inspect: %v\n%s", err, out)
	}
	for _, want := range []string{`"command":"connection.inspect"`, `"mode":"remote-service"`, `"mode_source":"legacy_default"`, `"transport":"https"`, `"credential_source":"flag_token"`, `"readiness_status":"degraded"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("inspect output missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, secret) {
		t.Fatalf("inspect leaked bearer token: %s", out)
	}
}

func TestConnectionInspectModePrecedenceAndProfileIsolation(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "xdg")
	vault := filepath.Join(root, "vault")
	writeCLITestFile(t, filepath.Join(xdg, "pinax", "config.yaml"), "remote:\n  api_url: https://user.example.test\n  mode: remote-service\nstorage:\n  backend: local\n  profile: backend-only\n")
	writeCLITestFile(t, filepath.Join(vault, ".pinax", "config.yaml"), "remote:\n  api_url: https://project.example.test\n  mode: self-hosted-service\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("PINAX_API_URL", "https://env.example.test")
	t.Setenv("PINAX_CONNECTION_MODE", "remote-service")
	t.Setenv("PINAX_API_TOKEN", "env-token")
	t.Setenv("PINAX_API_TOKEN_FILE", "")

	out, err := executeConnectionCommand(t, "--vault", vault, "--api-url", "https://flag.example.test", "--connection-mode", "self-hosted-service", "--api-token", "flag-token", "connection", "inspect", "--json")
	if err != nil || !strings.Contains(out, `"credential_source":"conflict"`) || !strings.Contains(out, `"readiness_status":"blocked"`) {
		// Credential sources are conflict-rejected rather than ordered; endpoint
		// and mode still retain deterministic precedence.
		t.Fatalf("expected credential conflict descriptor, err=%v out=%s", err, out)
	}
	t.Setenv("PINAX_API_TOKEN", "")
	out, err = executeConnectionCommand(t, "--vault", vault, "--api-url", "https://flag.example.test", "--connection-mode", "self-hosted-service", "--api-token", "flag-token", "connection", "inspect", "--json")
	if err != nil {
		t.Fatalf("connection inspect: %v\n%s", err, out)
	}
	for _, want := range []string{`"mode":"self-hosted-service"`, `"mode_source":"flag"`, `"endpoint":"https://flag.example.test"`, `"endpoint_source":"flag"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("inspect output missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "backend-only") {
		t.Fatalf("backend profile leaked into connection descriptor: %s", out)
	}
}

func TestConnectionDoctorSeparatesTransportAndAuthFailure(t *testing.T) {
	t.Setenv("PINAX_API_URL", "")
	t.Setenv("PINAX_API_TOKEN", "")
	t.Setenv("PINAX_API_TOKEN_FILE", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/manifest" {
			t.Fatalf("doctor performed unexpected request %s %s", request.Method, request.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spec_version": "1.0",
			"command":      "api.manifest",
			"status":       "failed",
			"error":        map[string]string{"code": "auth_required", "message": "authentication required"},
		})
	}))
	defer server.Close()

	out, err := executeConnectionCommand(t, "--api-url", server.URL, "connection", "doctor", "--json")
	if err != nil {
		t.Fatalf("connection doctor: %v\n%s", err, out)
	}
	for _, want := range []string{`"transport_status":"ready"`, `"auth_status":"blocked"`, `"auth_code":"auth_required"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q: %s", want, out)
		}
	}
}

func TestConnectionDoctorUsesReadOnlyManifestAndReadinessProbes(t *testing.T) {
	t.Setenv("PINAX_API_URL", "")
	t.Setenv("PINAX_API_TOKEN", "")
	t.Setenv("PINAX_API_TOKEN_FILE", "")
	seen := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		seen = append(seen, request.Method+" "+request.URL.Path)
		switch request.URL.Path {
		case "/v1/manifest":
			writeConnectionProjection(t, w, "api.manifest", map[string]any{"manifest": map[string]any{
				"schema_version": "pinax.transport_manifest.v1",
				"digest":         "sha256:" + strings.Repeat("a", 64),
				"capabilities":   []any{},
			}})
		case "/v1/readiness":
			writeConnectionProjection(t, w, "connection.readiness", map[string]any{"readiness": map[string]any{
				"schema_version": "pinax.connection_readiness.v1",
				"overall":        map[string]any{"status": "degraded", "maturity": "first-support"},
				"layers":         connectionReadinessLayers("ready"),
			}})
		default:
			t.Fatalf("unexpected doctor path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	out, err := executeConnectionCommand(t, "--api-url", server.URL, "connection", "doctor", "--json")
	if err != nil {
		t.Fatalf("connection doctor: %v\n%s", err, out)
	}
	if strings.Join(seen, ",") != "GET /v1/manifest,GET /v1/readiness" {
		t.Fatalf("doctor requests = %#v", seen)
	}
	for _, want := range []string{`"contract_status":"ready"`, `"transport_status":"ready"`, `"auth_status":"ready"`, `"owner_status":"degraded"`, `"manifest_digest":"sha256:`} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q: %s", want, out)
		}
	}
}

func TestConnectionReadinessLocalHasSixLayersWithoutProductionOverclaim(t *testing.T) {
	t.Setenv("PINAX_API_URL", "")
	t.Setenv("PINAX_API_TOKEN", "")
	t.Setenv("PINAX_API_TOKEN_FILE", "")

	out, err := executeConnectionCommand(t, "connection", "readiness", "--json")
	if err != nil {
		t.Fatalf("connection readiness: %v\n%s", err, out)
	}
	for _, want := range []string{
		`"command":"connection.readiness"`,
		`"contract_status":"ready"`,
		`"transport_status":"ready"`,
		`"auth_status":"not_applicable"`,
		`"owner_status":"ready"`,
		`"mutation_recovery_status":"not_applicable"`,
		`"production_status":"not_applicable"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("readiness output missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, `"production":{"status":"ready"`) {
		t.Fatalf("local readiness overclaimed production readiness: %s", out)
	}
}

func TestConnectionReadinessPreservesUnknownOwnerStatusAsNonReady(t *testing.T) {
	t.Setenv("PINAX_API_URL", "")
	t.Setenv("PINAX_API_TOKEN", "")
	t.Setenv("PINAX_API_TOKEN_FILE", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/manifest":
			writeConnectionProjection(t, w, "api.manifest", map[string]any{"manifest": map[string]any{
				"schema_version": "pinax.transport_manifest.v1",
				"digest":         "sha256:" + strings.Repeat("b", 64),
				"capabilities":   []any{},
			}})
		case "/v1/readiness":
			writeConnectionProjection(t, w, "connection.readiness", map[string]any{"readiness": map[string]any{
				"schema_version": "pinax.connection_readiness.v1",
				"overall":        map[string]any{"status": "future_owner_state", "maturity": "first-support"},
				"layers":         connectionReadinessLayers("future_owner_state"),
			}})
		default:
			t.Fatalf("unexpected readiness path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	out, err := executeConnectionCommand(t, "--api-url", server.URL, "connection", "readiness", "--json")
	if err != nil {
		t.Fatalf("connection readiness: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"overall_status":"future_owner_state"`) || strings.Contains(out, `"overall_status":"ready"`) {
		t.Fatalf("unknown status was not preserved fail-safe: %s", out)
	}
}

func executeConnectionCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	command := NewRootCommand("test")
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs(args)
	err := command.Execute()
	return output.String(), err
}

func writeConnectionProjection(t *testing.T, w http.ResponseWriter, command string, data any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{"spec_version": "1.0", "command": command, "status": "success", "data": data}); err != nil {
		t.Fatal(err)
	}
}

func connectionReadinessLayers(ownerStatus string) map[string]any {
	return map[string]any{
		"contract":          map[string]any{"status": "ready", "maturity": "first-support"},
		"transport":         map[string]any{"status": "ready", "maturity": "first-support"},
		"auth":              map[string]any{"status": "ready", "maturity": "first-support"},
		"owner":             map[string]any{"status": ownerStatus, "maturity": "first-support"},
		"mutation_recovery": map[string]any{"status": "blocked", "maturity": "exploratory"},
		"production":        map[string]any{"status": "not_configured", "maturity": "exploratory"},
	}
}
