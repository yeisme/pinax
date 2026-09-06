package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestToolInputSchemasUseSupportedKeywordsOnly(t *testing.T) {
	t.Parallel()

	for _, tool := range ToolInventory() {
		if err := validateSchemaKeywordSupport(tool.InputSchema); err != nil {
			t.Errorf("tool %s input schema: %v", tool.Name, err)
		}
	}
}

func TestValidateSchemaKeywordSupportRejectsUnsupportedKeywords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		schema map[string]any
		needle string
	}{
		{
			name:   "combiner",
			schema: map[string]any{"type": "object", "additionalProperties": false, "oneOf": []any{map[string]any{"type": "string"}}},
			needle: `unsupported schema keyword "oneOf"`,
		},
		{
			name:   "reference",
			schema: map[string]any{"type": "object", "additionalProperties": map[string]any{"$ref": "pinax://schema/pinax.error.v1"}},
			needle: `unsupported schema keyword "$ref"`,
		},
		{
			name:   "pattern properties",
			schema: map[string]any{"type": "object", "additionalProperties": false, "patternProperties": map[string]any{}},
			needle: `unsupported schema keyword "patternProperties"`,
		},
		{
			name:   "object size bound",
			schema: map[string]any{"type": "object", "additionalProperties": false, "minProperties": 1.0},
			needle: `unsupported schema keyword "minProperties"`,
		},
		{
			name:   "array uniqueness",
			schema: map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "uniqueItems": true},
			needle: `unsupported schema keyword "uniqueItems"`,
		},
		{
			name: "nested unsupported keyword",
			schema: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"filter": map[string]any{"type": "string", "deprecated": true},
				},
			},
			needle: `unsupported schema keyword "deprecated" at $/filter`,
		},
		{
			name: "malformed numeric bound",
			schema: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"limit": map[string]any{"type": "integer", "minimum": "one"},
				},
			},
			needle: `keyword "minimum" at $/limit must be a number`,
		},
		{
			name: "required with non-string member",
			schema: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           map[string]any{"query": map[string]any{"type": "string"}},
				"required":             []any{"query", 1.0},
			},
			needle: `keyword "required" at $ must be an array of strings`,
		},
		{
			name: "nested document identity",
			schema: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"filter": map[string]any{"type": "string", "$id": "pinax://schema/other"},
				},
			},
			needle: `only allowed at the schema root`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateSchemaKeywordSupport(tt.schema)
			if err == nil {
				t.Fatalf("expected rejection, got nil")
			}
			if !strings.Contains(err.Error(), tt.needle) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.needle)
			}
		})
	}
}

func TestValidateSchemaKeywordSupportAllowsDocumentIdentityAtRoot(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "pinax://schema/pinax.note.search.request.v1",
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "minLength": 1, "maxLength": 200, "description": "search text"},
		},
		"required": []string{"query"},
	}
	if err := validateSchemaKeywordSupport(schema); err != nil {
		t.Fatalf("expected support, got %v", err)
	}
}

func TestMatchesToolArgumentSchemaImplementsLockedSubset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		value  any
		schema map[string]any
		want   bool
	}{
		{
			name:  "enum accepts member",
			value: "card",
			schema: map[string]any{
				"type": "string",
				"enum": []string{"card", "metadata", "body"},
			},
			want: true,
		},
		{
			name:  "enum rejects non-member",
			value: "raw",
			schema: map[string]any{
				"type": "string",
				"enum": []any{"card", "metadata", "body"},
			},
			want: false,
		},
		{
			name:  "const equality",
			value: "pinax.mcp.tool_result.v1",
			schema: map[string]any{
				"type":  "string",
				"const": "pinax.mcp.tool_result.v1",
			},
			want: true,
		},
		{
			name:  "const mismatch",
			value: "pinax.projection.v1",
			schema: map[string]any{
				"type":  "string",
				"const": "pinax.mcp.tool_result.v1",
			},
			want: false,
		},
		{
			name:  "pattern anchors digest",
			value: "sha256:" + strings.Repeat("a", 64),
			schema: map[string]any{
				"type":    "string",
				"pattern": "^sha256:[0-9a-f]{64}$",
			},
			want: true,
		},
		{
			name:  "pattern rejects malformed digest",
			value: "sha256:XYZ",
			schema: map[string]any{
				"type":    "string",
				"pattern": "^sha256:[0-9a-f]{64}$",
			},
			want: false,
		},
		{
			name:  "string length bounds",
			value: "abcd",
			schema: map[string]any{
				"type":      "string",
				"minLength": 2,
				"maxLength": 3,
			},
			want: false,
		},
		{
			name:  "integer rejects fraction",
			value: 5.5,
			schema: map[string]any{
				"type":    "integer",
				"minimum": 1,
			},
			want: false,
		},
		{
			name:  "integer inclusive minimum",
			value: float64(1),
			schema: map[string]any{
				"type":    "integer",
				"minimum": 1,
			},
			want: true,
		},
		{
			name:  "number exclusive maximum",
			value: 1.5,
			schema: map[string]any{
				"type":             "number",
				"exclusiveMaximum": 1.5,
			},
			want: false,
		},
		{
			name:  "array item bounds",
			value: []any{"a"},
			schema: map[string]any{
				"type":     "array",
				"items":    map[string]any{"type": "string"},
				"minItems": 2,
			},
			want: false,
		},
		{
			name:  "nested object unknown property rejected",
			value: map[string]any{"known": "x", "extra": 1.0},
			schema: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"known": map[string]any{"type": "string"},
				},
			},
			want: false,
		},
		{
			name:  "nested object additionalProperties schema validates extras",
			value: map[string]any{"facts": map[string]any{"a": "1", "b": 2.0}},
			schema: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"facts": map[string]any{
						"type":                 "object",
						"additionalProperties": map[string]any{"type": "string"},
					},
				},
			},
			want: false,
		},
		{
			name:  "nested required presence enforced",
			value: map[string]any{"digest": map[string]any{}},
			schema: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"digest": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"properties":           map[string]any{"value": map[string]any{"type": "string"}},
						"required":             []string{"value"},
					},
				},
			},
			want: false,
		},
		{
			name:   "empty schema asserts nothing",
			value:  struct{ Arbitrary string }{"x"},
			schema: map[string]any{},
			want:   true,
		},
		{
			name:  "unknown type fails closed",
			value: "x",
			schema: map[string]any{
				"type": "null",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := matchesToolArgumentSchema(tt.value, tt.schema); got != tt.want {
				t.Fatalf("matchesToolArgumentSchema(%#v, %#v) = %v, want %v", tt.value, tt.schema, got, tt.want)
			}
		})
	}
}

func TestNestedSchemaViolationFailsToolCallBeforeApplicationService(t *testing.T) {
	t.Parallel()

	server := NewServer(app.NewService(), t.TempDir())
	// pinax.agent.context declares entities as an array of strings, so a
	// non-string item must be rejected as a value violation, not an unknown
	// argument, before the application service runs.
	_, err := server.Handle(context.Background(), Request{ID: 1, Method: "tools/call", Params: map[string]any{
		"name":      "pinax.agent.context",
		"arguments": map[string]any{"entities": []any{"workspace", 42.0}},
	}})
	mcpErr, ok := err.(*MCPError)
	if !ok || mcpErr.Code != -32602 {
		t.Fatalf("error = %#v", err)
	}
	if mcpErr.Data["argument"] != "entities" || mcpErr.Data["reason"] != "invalid_type_or_value" {
		t.Fatalf("error data = %#v", mcpErr.Data)
	}
}
