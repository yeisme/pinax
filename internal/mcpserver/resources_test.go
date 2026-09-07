package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestResourcesListEntriesAreReadableAndBounded(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := t.TempDir()
	service := app.NewService()
	if _, err := service.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Resource vault"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateProject(ctx, app.ProjectRequest{VaultPath: root, Slug: "research", Name: "Research", NotesPrefix: "research"}); err != nil {
		t.Fatal(err)
	}
	writeMCPFixture(t, root, "research/resource.md", "---\nschema_version: pinax.note.v1\nnote_id: note_resource\ntitle: Resource Note\nproject: research\nkind: task\nstatus: active\n---\n\nSECRET_RESOURCE_BODY should never be exposed.\n")
	// sync job status resource 的可读实例：预置一条已终态的 sync run 事件流。
	writeMCPFixture(t, root, ".pinax/events.jsonl",
		`{"schema_version":"pinax.event.v1","type":"sync.run","status":"running","ts":"2026-09-07T10:00:00Z","facts":{"run_id":"sync_mcp_1","command":"sync.push","direction":"push","backend_kind":"embedded","remote_write":"true"}}`+"\n"+
			`{"schema_version":"pinax.event.v1","type":"sync.file","status":"running","ts":"2026-09-07T10:00:01Z","facts":{"run_id":"sync_mcp_1","command":"sync.push","direction":"push","backend_kind":"embedded","kind":"upload_blob","operation_status":"applied","change_code":"A","change_state":"applied","path":"notes/a.md"}}`+"\n"+
			`{"schema_version":"pinax.event.v1","type":"sync.run","status":"success","ts":"2026-09-07T10:00:02Z","facts":{"run_id":"sync_mcp_1","command":"sync.push","direction":"push","backend_kind":"embedded","remote_write":"true"}}`+"\n")

	server := NewServer(service, root)
	listed, err := server.Handle(ctx, Request{ID: 1, Method: "resources/list"})
	if err != nil {
		t.Fatal(err)
	}
	concreteURI := map[string]string{
		"pinax://manifest":             "pinax://manifest",
		"pinax://readiness":            "pinax://readiness",
		"pinax://vault/current":        "pinax://vault/current",
		"pinax://note/{note_id}":       "pinax://note/note_resource",
		"pinax://search/{query}":       "pinax://search/Resource",
		"pinax://organize/plan":        "pinax://organize/plan",
		"pinax://vault/graph":          "pinax://vault/graph",
		"pinax://project/{slug}/board": "pinax://project/research/board",
		"pinax://sync/job/{run_id}":    "pinax://sync/job/sync_mcp_1",
	}
	if len(listed.Resources) != len(concreteURI) {
		t.Fatalf("resources/list returned %d resources, want %d", len(listed.Resources), len(concreteURI))
	}

	for _, resource := range listed.Resources {
		uri, ok := concreteURI[resource.URI]
		if !ok {
			t.Fatalf("resource %q has no readable test instance", resource.URI)
		}
		t.Run(resource.URI, func(t *testing.T) {
			response, readErr := server.Handle(ctx, Request{ID: 2, Method: "resources/read", Params: map[string]any{"uri": uri}})
			if readErr != nil {
				t.Fatalf("resources/read %q: %v", uri, readErr)
			}
			text := resourceContentText(t, response.Result, uri)
			if strings.Contains(text, root) {
				t.Fatalf("resource %q leaked absolute vault path: %s", uri, text)
			}
			if strings.Contains(text, "SECRET_RESOURCE_BODY") || strings.Contains(text, `"body":`) {
				t.Fatalf("resource %q leaked note body: %s", uri, text)
			}
			limit := maxResourceContentBytes
			if uri == "pinax://manifest" {
				limit = maxManifestResourceBytes
			}
			if len(text) > limit {
				t.Fatalf("resource %q content bytes = %d, limit = %d", uri, len(text), limit)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(text), &payload); err != nil {
				t.Fatalf("resource %q content is not JSON: %v\n%s", uri, err, text)
			}
			if payload["schema_version"] != resourceContentSchemaVersion || payload["uri"] != uri {
				t.Fatalf("resource %q identity mismatch: %#v", uri, payload)
			}
			if uri == "pinax://manifest" {
				data, ok := payload["data"].(map[string]any)
				if !ok {
					t.Fatalf("manifest resource data = %#v", payload["data"])
				}
				manifest, ok := data["manifest"].(map[string]any)
				if !ok || manifest["schema_version"] != "pinax.transport_manifest.v1" || manifest["digest"] == "" {
					t.Fatalf("manifest resource payload = %#v", data["manifest"])
				}
			}
			if uri == "pinax://readiness" {
				data, ok := payload["data"].(map[string]any)
				readiness, ready := data["readiness"].(map[string]any)
				layers, layered := readiness["layers"].(map[string]any)
				if !ok || !ready || !layered || len(layers) != 6 || readiness["schema_version"] != "pinax.connection_readiness.v1" {
					t.Fatalf("readiness resource payload = %#v", payload["data"])
				}
			}
		})
	}
}

func TestResourcesReadRejectsMissingAndUnknownURI(t *testing.T) {
	t.Parallel()

	server := NewServer(app.NewService(), t.TempDir())
	tests := []struct {
		name       string
		params     map[string]any
		legacyCode string
	}{
		{name: "missing", params: map[string]any{}, legacyCode: "resource_uri_required"},
		{name: "unknown", params: map[string]any{"uri": "pinax://unknown/value"}, legacyCode: "resource_not_found"},
		{name: "empty template value", params: map[string]any{"uri": "pinax://note/"}, legacyCode: "resource_not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.Handle(context.Background(), Request{ID: 3, Method: "resources/read", Params: tt.params})
			mcpErr, ok := err.(*MCPError)
			if !ok {
				t.Fatalf("error = %T %v, want MCPError", err, err)
			}
			if got, _ := mcpErr.Data["legacy_code"].(string); got != tt.legacyCode {
				t.Fatalf("legacy_code = %q, want %q", got, tt.legacyCode)
			}
		})
	}
}

func TestResourceTemplateInventoryMatchesModernDiscovery(t *testing.T) {
	t.Parallel()

	server := NewServerWithOptions(app.NewService(), t.TempDir(), ServerOptions{StrictLifecycle: true})
	response, err := server.Handle(context.Background(), Request{
		ID:     4,
		Method: "resources/templates/list",
		Params: map[string]any{"_meta": map[string]any{
			protocolVersionMetaKey:                       "2026-07-28",
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	discovered, ok := response.Result["resourceTemplates"].([]map[string]any)
	if !ok {
		t.Fatalf("resourceTemplates = %#v", response.Result["resourceTemplates"])
	}
	want := ResourceTemplateInventory()
	if len(discovered) != len(want) {
		t.Fatalf("discovered templates = %d, inventory = %d", len(discovered), len(want))
	}
	for _, template := range discovered {
		uri := fmt.Sprint(template["uriTemplate"])
		if !strings.Contains(uri, "{") {
			t.Fatalf("non-template URI in resource template discovery: %#v", template)
		}
	}
}

func resourceContentText(t *testing.T, result map[string]any, wantURI string) string {
	t.Helper()
	contents, ok := result["contents"].([]map[string]any)
	if !ok || len(contents) != 1 {
		t.Fatalf("contents = %#v, want one MCP content item", result["contents"])
	}
	content := contents[0]
	if content["uri"] != wantURI || content["mimeType"] != "application/json" {
		t.Fatalf("content identity = %#v", content)
	}
	text, ok := content["text"].(string)
	if !ok || text == "" {
		t.Fatalf("content text = %#v", content["text"])
	}
	return text
}
