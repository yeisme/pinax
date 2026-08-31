package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/api"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/cli"
)

func TestAPIManifestCLIAndServerParity(t *testing.T) {
	root := t.TempDir()
	cliOutput := runCLI(t, "api", "manifest", "--json")
	cliProjection := decodeAPIManifestProjection(t, []byte(cliOutput))

	service := app.NewService()
	if _, err := service.InitVault(context.Background(), app.InitVaultRequest{VaultPath: root, Title: "Manifest parity"}); err != nil {
		t.Fatal(err)
	}
	server := api.NewServerWithOptions(service, root, api.ServerOptions{Manifest: cli.TransportManifestProjection})

	restRecorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(restRecorder, httptest.NewRequest(http.MethodGet, "/v1/manifest", nil))
	if restRecorder.Code != http.StatusOK {
		t.Fatalf("GET /v1/manifest status = %d body=%s", restRecorder.Code, restRecorder.Body.String())
	}
	restProjection := decodeAPIManifestProjection(t, restRecorder.Body.Bytes())

	rpcRecorder := httptest.NewRecorder()
	rpcBody := bytes.NewBufferString(`{"method":"Pinax.Transport.Manifest"}`)
	server.Handler().ServeHTTP(rpcRecorder, httptest.NewRequest(http.MethodPost, "/v1/rpc", rpcBody))
	if rpcRecorder.Code != http.StatusOK {
		t.Fatalf("Pinax.Transport.Manifest status = %d body=%s", rpcRecorder.Code, rpcRecorder.Body.String())
	}
	rpcProjection := decodeAPIManifestProjection(t, rpcRecorder.Body.Bytes())

	cliManifest := cliProjection["data"].(map[string]any)["manifest"]
	if !reflect.DeepEqual(cliManifest, restProjection["data"].(map[string]any)["manifest"]) ||
		!reflect.DeepEqual(cliManifest, rpcProjection["data"].(map[string]any)["manifest"]) {
		t.Fatal("CLI, REST and RPC manifest payloads differ")
	}
	for name, projection := range map[string]map[string]any{"cli": cliProjection, "rest": restProjection, "rpc": rpcProjection} {
		if projection["command"] != "api.manifest" || projection["status"] != "success" {
			t.Fatalf("%s projection identity = %#v", name, projection)
		}
		facts := projection["facts"].(map[string]any)
		if facts["schema_version"] != "pinax.transport_manifest.v1" || !strings.HasPrefix(fmt.Sprint(facts["digest"]), "sha256:") {
			t.Fatalf("%s manifest facts = %#v", name, facts)
		}
	}
}

func decodeAPIManifestProjection(t *testing.T, encoded []byte) map[string]any {
	t.Helper()
	var projection map[string]any
	if err := json.Unmarshal(encoded, &projection); err != nil {
		t.Fatalf("decode manifest projection: %v\n%s", err, encoded)
	}
	data, ok := projection["data"].(map[string]any)
	if !ok || data["manifest"] == nil {
		t.Fatalf("manifest projection data = %#v", projection["data"])
	}
	return projection
}

func TestAPIServeMachineModesAreQuietAndWriteModeConflictIsStable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, mode := range []string{"--json", "--agent"} {
		stdout, stderr, err := runAPIServeUntilCanceled(t, root, "api", "serve", "--port", "0", "--vault", root, mode)
		if err == nil || stderr != "" {
			t.Fatalf("api serve %s should fail without diagnostics on stderr: err=%v stderr=%q stdout=%s", mode, err, stderr, stdout)
		}
		if !strings.Contains(stdout, "unsupported_output_mode") || strings.Contains(stdout, "Pinax local API") || strings.Contains(stdout, "http://127.0.0.1:") {
			t.Fatalf("api serve %s stdout contract violated: %s", mode, stdout)
		}
	}
	stdout, stderr, err := runCLISeparate("api", "serve", "--readonly", "--allow-write", "--vault", root, "--json")
	if err == nil || stderr != "" || !strings.Contains(stdout, "write_mode_conflict") {
		t.Fatalf("api serve write mode conflict err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
}

func TestAPIServeTokenStoreAndTokenFileCompatibilityOutputContract(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	canonicalStore := filepath.Join(root, "canonical-store.json")
	legacyStore := filepath.Join(root, "legacy-secret-path-store.json")

	stdout, stderr, err := runCLISeparate("api", "serve", "--token-store", canonicalStore, "--token-file", legacyStore, "--json")
	if err == nil || stderr != "" || !strings.Contains(stdout, "auth_mode_conflict") || strings.Contains(stdout, canonicalStore) || strings.Contains(stdout, legacyStore) {
		t.Fatalf("store conflict err=%v stdout=%s stderr=%q", err, stdout, stderr)
	}
	stdout, stderr, err = runCLISeparate("api", "serve", "--token-store", canonicalStore, "--no-auth", "--json")
	if err == nil || stderr != "" || !strings.Contains(stdout, "auth_mode_conflict") || strings.Contains(stdout, canonicalStore) {
		t.Fatalf("no-auth conflict err=%v stdout=%s stderr=%q", err, stdout, stderr)
	}

	stdout, stderr, err = runAPIServeUntilCanceled(t, root, "api", "serve", "--port", "0", "--vault", root, "--token-file", legacyStore)
	if err != nil || stdout != "" || !strings.Contains(stderr, "Deprecated: server --token-file") || !strings.Contains(stderr, "--token-store <hashed-token-store>") || strings.Contains(stderr, legacyStore) {
		t.Fatalf("legacy alias err=%v stdout=%q stderr=%s", err, stdout, stderr)
	}

	stdout, stderr, err = runAPIServeUntilCanceled(t, root, "api", "serve", "--port", "0", "--vault", root, "--token-file", legacyStore, "--events")
	if err != nil || stderr != "" || strings.Contains(stdout, "Deprecated:") || strings.Contains(stdout, legacyStore) {
		t.Fatalf("legacy events err=%v stdout=%s stderr=%q", err, stdout, stderr)
	}
	for _, event := range parseNDJSONEvents(t, stdout) {
		if event["type"] == nil {
			t.Fatalf("invalid event: %#v", event)
		}
	}

	help := runCLI(t, "api", "serve", "--help")
	for _, want := range []string{"--token-store", "--token-file", "hashed token registry"} {
		if !strings.Contains(help, want) {
			t.Fatalf("api serve help missing %q: %s", want, help)
		}
	}
}

func TestAPIServeLifecycleOutput(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	stdout, stderr, err := runAPIServeUntilCanceled(t, root, "api", "serve", "--port", "0", "--vault", root)
	if err != nil || stdout != "" {
		t.Fatalf("api serve default err=%v stdout=%q stderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "pinax api ready") || !strings.Contains(stderr, "http://127.0.0.1:") || !strings.Contains(stderr, "auth_mode") {
		t.Fatalf("api serve default stderr missing zap startup log: %s", stderr)
	}

	stdout, stderr, err = runAPIServeUntilCanceled(t, root, "api", "serve", "--readonly", "--port", "0", "--vault", root, "--events")
	if err != nil || stderr != "" {
		t.Fatalf("api serve events err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	events := parseNDJSONEvents(t, stdout)
	for _, want := range []string{"start", "ready", "shutdown"} {
		if !hasEventType(events, want) {
			t.Fatalf("api serve events missing %s: %#v\n%s", want, events, stdout)
		}
	}
	for _, event := range events {
		if event["type"] == "ready" && !strings.Contains(fmt.Sprint(event["url"]), "http://127.0.0.1:") {
			t.Fatalf("ready event missing localhost URL: %#v", event)
		}
		if strings.Contains(fmt.Sprint(event["message"]), "Temp token:") {
			t.Fatalf("events leaked temp token log: %#v", event)
		}
	}
}

func TestAPIRoutesHumanOutputListsEndpointsCLI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	out := runCLI(t, "api", "routes", "--vault", root)
	for _, want := range []string{"API routes", "Method", "Endpoint", "Command", "Surface", "GET", "/v1/projects/{slug}/board", "CALL", "Pinax.Note.Read", "project.board.show"} {
		if !strings.Contains(out, want) {
			t.Fatalf("api routes human output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Evidence") {
		t.Fatalf("api routes human output should render endpoint rows instead of evidence dump:\n%s", out)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("api routes human output should not be JSON:\n%s", out)
	}
	agentOut := runCLI(t, "api", "routes", "--vault", root, "--agent")
	for _, want := range []string{"command=api.routes", "fact.routes=", "route.1.id=rest.transport.manifest", "route.1.path=/v1/manifest", "route.2.id=rest.connection.readiness", "route.2.path=/v1/readiness", "route.5.id=rest.workbench.status", "route.5.path=/v1/workbench/status"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("api routes agent output missing %q:\n%s", want, agentOut)
		}
	}
}

func TestAPIRoutesJSONExposesReleaseCoreCapabilitiesCLI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	out := runCLI(t, "api", "routes", "--vault", root, "--json")
	// Every release core capability must be discoverable with its proof-loop
	// metadata, including the local-only CLI capabilities that have no REST route.
	for _, want := range []string{
		`"release_core"`,
		`"id":"repair.apply"`,
		`"local_only_reason":"cli-proof-loop"`,
		`"copy_command":"pinax repair apply --vault <vault> --plan <plan-id> --yes --json"`,
		`"snapshot_required":true`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("api routes --json release core discovery missing %q:\n%s", want, out)
		}
	}
}

func TestAPISchemaExportCLIEmitsSemanticOpenAPIAndDigest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	out := runCLI(t, "api", "schema", "export", "--format", "openapi", "--vault", root, "--json")
	var projection map[string]any
	if err := json.Unmarshal([]byte(out), &projection); err != nil {
		t.Fatalf("decode api schema export: %v\n%s", err, out)
	}
	data, _ := projection["data"].(map[string]any)
	document, _ := data["schema"].(map[string]any)
	if err := app.ValidateOpenAPI(document); err != nil {
		t.Fatalf("CLI OpenAPI is invalid: %v", err)
	}
	digest, err := app.OpenAPIDigest(document)
	if err != nil {
		t.Fatal(err)
	}
	facts, _ := projection["facts"].(map[string]any)
	if facts["schema_digest"] != digest {
		t.Fatalf("CLI schema digest = %#v, want %q", facts["schema_digest"], digest)
	}
	components, _ := document["components"].(map[string]any)
	if components["schemas"] == nil || components["securitySchemes"] == nil {
		t.Fatalf("CLI OpenAPI components = %#v", components)
	}
}

func TestAPIWorkbenchStatusCLI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Workbench", "--json")
	out := runCLI(t, "api", "status", "--vault", root, "--json")
	for _, want := range []string{`"command":"workbench.status"`, `"ui_group":"workbench.status"`, `"vault_root":"` + root + `"`, `"index_status"`, `"write_mode":"local_cli"`, `"body_exposure_default":"none"`, `"pinax index refresh --vault `} {
		if !strings.Contains(out, want) {
			t.Fatalf("api status missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Authorization") || strings.Contains(out, "Bearer ") || strings.Contains(out, "raw prompt") {
		t.Fatalf("api status leaked sensitive text:\n%s", out)
	}
}
