package mcpserver

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestToolSchemasAreClosedAndVersioned(t *testing.T) {
	t.Parallel()

	tools := ToolInventory()
	if len(tools) == 0 {
		t.Fatal("tool inventory is empty")
	}
	for _, tool := range tools {
		tool := tool
		t.Run(tool.Name, func(t *testing.T) {
			t.Parallel()
			if tool.InputSchema["type"] != "object" || tool.InputSchema["additionalProperties"] != false {
				t.Fatalf("tool input schema is not closed: %#v", tool.InputSchema)
			}
			if _, ok := tool.InputSchema["properties"].(map[string]any); !ok {
				t.Fatalf("tool input properties = %#v", tool.InputSchema["properties"])
			}
			if tool.OutputSchema["type"] != "object" || tool.OutputSchema["additionalProperties"] != false {
				t.Fatalf("tool output schema is not closed: %#v", tool.OutputSchema)
			}
			properties, _ := tool.OutputSchema["properties"].(map[string]any)
			schemaVersion, _ := properties["schema_version"].(map[string]any)
			if schemaVersion["const"] != mcpToolResultSchemaVersion {
				t.Fatalf("tool output schema version = %#v", schemaVersion)
			}
		})
	}

	definitions := standardToolDefinitions(tools)
	if len(definitions) != len(tools) {
		t.Fatalf("standard definitions = %d, tools = %d", len(definitions), len(tools))
	}
	for _, definition := range definitions {
		if definition["inputSchema"] == nil || definition["outputSchema"] == nil {
			t.Fatalf("standard tool definition lacks schema: %#v", definition)
		}
	}
}

func TestToolSchemasComeFromSharedRegistry(t *testing.T) {
	t.Parallel()

	registry := catalogschema.Default()
	registrations := toolRegistrations()
	if len(registrations) != len(ToolInventory()) {
		t.Fatalf("registrations = %d, inventory = %d", len(registrations), len(ToolInventory()))
	}
	for _, registration := range registrations {
		input, ok := registry.Lookup(registration.RequestSchema)
		if !ok {
			t.Errorf("tool %s request schema %q is not registered", registration.Name, registration.RequestSchema)
			continue
		}
		if !reflect.DeepEqual(registration.InputSchema, input) {
			t.Errorf("tool %s input schema drifted from shared registry", registration.Name)
		}
		if registration.OutputSchema != catalogschema.MCPToolResultV1 {
			t.Errorf("tool %s output schema id = %q", registration.Name, registration.OutputSchema)
		}
		output, ok := registry.Lookup(registration.OutputSchema)
		if !ok || !reflect.DeepEqual(registration.Tool.OutputSchema, output) {
			t.Errorf("tool %s output schema drifted from shared registry", registration.Name)
		}
	}
}

func TestInvalidToolArgumentsFailBeforeApplicationService(t *testing.T) {
	t.Parallel()

	server := NewServer(app.NewService(), t.TempDir())
	tests := []struct {
		name       string
		tool       string
		arguments  map[string]any
		wantArg    string
		wantReason string
	}{
		{name: "unknown", tool: "pinax.search", arguments: map[string]any{"query": "x", "secret": "do-not-log"}, wantArg: "secret", wantReason: "unknown_argument"},
		{name: "required", tool: "pinax.query.run", arguments: map[string]any{}, wantArg: "sql", wantReason: "required_argument_missing"},
		{name: "type", tool: "pinax.agent.context", arguments: map[string]any{"entities": "not-an-array"}, wantArg: "entities", wantReason: "invalid_type_or_value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.Handle(context.Background(), Request{ID: 1, Method: "tools/call", Params: map[string]any{
				"name": tt.tool, "arguments": tt.arguments,
			}})
			mcpErr, ok := err.(*MCPError)
			if !ok || mcpErr.Code != -32602 || mcpErr.Data["legacy_code"] != "invalid_tool_arguments" {
				t.Fatalf("error = %#v", err)
			}
			if mcpErr.Data["argument"] != tt.wantArg || mcpErr.Data["reason"] != tt.wantReason {
				t.Fatalf("error data = %#v", mcpErr.Data)
			}
			if strings.Contains(fmt.Sprint(mcpErr), "do-not-log") {
				t.Fatalf("validation error leaked argument value: %#v", mcpErr)
			}
		})
	}
}

func TestToolStructuredContentMatchesVersionedOutputSchema(t *testing.T) {
	t.Parallel()

	server := NewServer(app.NewService(), t.TempDir())
	response, err := server.Handle(context.Background(), Request{ID: 2, Method: "tools/call", Params: map[string]any{
		"name": "pinax.git.snapshot_plan", "arguments": map[string]any{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	structured, ok := response.Result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %#v", response.Result["structuredContent"])
	}
	if structured["schema_version"] != mcpToolResultSchemaVersion || structured["status"] != "success" || structured["summary"] == "" {
		t.Fatalf("structured content identity = %#v", structured)
	}
	if _, ok := structured["data"].(map[string]any); !ok {
		t.Fatalf("structured content data = %#v", structured["data"])
	}
}
