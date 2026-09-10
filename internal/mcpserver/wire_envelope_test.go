package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestMCPJSONRPCStrictEnvelopeDiscovery(t *testing.T) {
	for _, version := range legacyProtocolVersions {
		t.Run(version, func(t *testing.T) {
			frames := []Request{
				{JSONRPC: "2.0", ID: 0, Method: "initialize", Params: map[string]any{"protocolVersion": version}},
				{JSONRPC: "2.0", Method: "notifications/initialized"},
				{JSONRPC: "2.0", ID: "tools", Method: "tools/list"},
				{JSONRPC: "2.0", ID: "resources", Method: "resources/list"},
				{JSONRPC: "2.0", ID: "templates", Method: "resources/templates/list"},
				{JSONRPC: "2.0", ID: "missing", Method: "not-a-method"},
			}
			var input, output strings.Builder
			for _, frame := range frames {
				raw, _ := json.Marshal(frame)
				input.Write(raw)
				input.WriteByte('\n')
			}
			if err := ServeWithOptions(context.Background(), app.NewService(), t.TempDir(), strings.NewReader(input.String()), &output, ServerOptions{Collaboration: true, NotePolicy: app.CollaborationPolicy{AllowBody: true, AllowWrite: true}}); err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(output.String()), "\n")
			if len(lines) != 5 {
				t.Fatalf("got %d responses", len(lines))
			}
			for _, line := range lines {
				var frame map[string]json.RawMessage
				if err := json.Unmarshal([]byte(line), &frame); err != nil {
					t.Fatal(err)
				}
				if len(frame) != 3 || frame["jsonrpc"] == nil || frame["id"] == nil || (frame["result"] == nil) == (frame["error"] == nil) {
					t.Fatal("not a strict JSON-RPC response")
				}
				for key := range frame {
					if key != "jsonrpc" && key != "id" && key != "result" && key != "error" {
						t.Fatalf("SDK would reject field %q", key)
					}
				}
			}
		})
	}
}

func TestMCPJSONRPCErrorOmitsResultAndPreservesNullID(t *testing.T) {
	var out strings.Builder
	err := encodeMCPResponse(json.NewEncoder(&out), Response{Result: map[string]any{"unsafe": "ignored"}, Tools: []Tool{{Name: "ignored"}}, Error: newMCPError(-32700, "parse_error", "Invalid JSON")}, true)
	if err != nil {
		t.Fatal(err)
	}
	var frame map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out.String()), &frame); err != nil {
		t.Fatal(err)
	}
	if len(frame) != 3 || string(frame["id"]) != "null" || frame["error"] == nil || frame["result"] != nil {
		t.Fatal("invalid JSON-RPC error envelope")
	}
}
