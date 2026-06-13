package remoteapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestClientPingAndCapabilitiesUseReadEndpoints(t *testing.T) {
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.Method+" "+r.URL.Path] = true
		switch r.URL.Path {
		case "/":
			_ = json.NewEncoder(w).Encode(domain.NewProjection("api.root", "ready"))
		case "/v1/capabilities":
			_ = json.NewEncoder(w).Encode(domain.NewProjection("api.capabilities", "capabilities"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	ping, err := client.Ping(context.Background())
	if err != nil || ping.Command != "api.root" || !seen["GET /"] {
		t.Fatalf("ping projection=%#v err=%v seen=%#v", ping, err, seen)
	}
	caps, err := client.Capabilities(context.Background())
	if err != nil || caps.Command != "api.capabilities" || !seen["GET /v1/capabilities"] {
		t.Fatalf("capabilities projection=%#v err=%v seen=%#v", caps, err, seen)
	}
}

func TestClientDefaultTimeoutAndTransportErrorAreRedacted(t *testing.T) {
	const secret = "pinax-secret-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	client := NewClient(Config{BaseURL: url, Token: secret})
	if client.http.Timeout <= 0 {
		t.Fatalf("default client timeout must be bounded")
	}
	projection, err := client.Ping(context.Background())
	encoded, marshalErr := json.Marshal(projection)
	if marshalErr != nil {
		t.Fatalf("marshal projection: %v", marshalErr)
	}
	if err == nil || projection.Error == nil || projection.Error.Code != "remote_api_request_failed" {
		t.Fatalf("projection=%#v err=%v", projection, err)
	}
	if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "Authorization") {
		t.Fatalf("transport error leaked secret/header: %s", string(encoded))
	}
}

func TestClientCallsRPCAndSendsBearerTokenOnlyAsHeader(t *testing.T) {
	const secret = "pinax-secret-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rpc" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+secret {
			t.Fatalf("authorization header = %q", got)
		}
		var req RPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Method != "Pinax.Folder.List" || req.Params["include_empty"] != true {
			t.Fatalf("request = %#v", req)
		}
		_ = json.NewEncoder(w).Encode(domain.NewProjection("folder.list", "listed"))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Token: secret})
	projection, err := client.Call(context.Background(), RPCRequest{Method: "Pinax.Folder.List", Params: map[string]any{"include_empty": true}})
	if err != nil || projection.Command != "folder.list" {
		t.Fatalf("projection=%#v err=%v", projection, err)
	}
}

func TestClientReturnsProjectionForNon2xxResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(domain.NewErrorProjection("folder.create", &domain.CommandError{Code: "write_disabled", Message: "remote writes disabled"}))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})
	projection, err := client.Call(context.Background(), RPCRequest{Method: "Pinax.Folder.Create", Params: map[string]any{"path": "secret-path", "yes": true}})
	if err == nil || projection.Error == nil || projection.Error.Code != "write_disabled" || projection.Command != "folder.create" {
		t.Fatalf("projection=%#v err=%v", projection, err)
	}
}

func TestClientInvalidResponsesUseRedactedProjection(t *testing.T) {
	const secret = "pinax-secret-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json: Authorization Bearer pinax-secret-token"))
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL, Token: secret})
	projection, err := client.Call(context.Background(), RPCRequest{Method: "Pinax.Folder.List", Params: map[string]any{"token": secret}})
	encoded, marshalErr := json.Marshal(projection)
	if marshalErr != nil {
		t.Fatalf("marshal projection: %v", marshalErr)
	}
	body := string(encoded)
	if err == nil || projection.Error == nil || projection.Error.Code != "remote_api_invalid_response" {
		t.Fatalf("projection=%#v err=%v", projection, err)
	}
	if strings.Contains(body, secret) || strings.Contains(body, "Authorization") {
		t.Fatalf("invalid response error leaked secret/header: %s", body)
	}
}

func TestClientRejectsUnsupportedURLScheme(t *testing.T) {
	client := NewClient(Config{BaseURL: "file:///tmp/pinax.sock", Token: "secret"})
	projection, err := client.Call(context.Background(), RPCRequest{Method: "Pinax.Folder.List"})
	if err == nil || projection.Error == nil || projection.Error.Code != "remote_api_invalid_config" {
		t.Fatalf("projection=%#v err=%v", projection, err)
	}
}
