package schema

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultSchemaRegistryIsDeterministicClosedAndComplete(t *testing.T) {
	t.Parallel()

	first := Default()
	second := Default()
	if err := first.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("registry is not byte-stable:\n%s\n%s", firstJSON, secondJSON)
	}

	for _, id := range []string{
		ProjectionV1,
		ErrorV1,
		ReadinessV1,
		TransportManifestV1,
		OperationV1,
		MCPToolResultV1,
		TransportManifestReq,
		"pinax.note.search.request.v1",
		"pinax.agent.context.request.v1",
	} {
		if _, ok := first.Lookup(id); !ok {
			t.Errorf("required schema %q is not registered", id)
		}
	}

	projection, _ := first.Lookup(ProjectionV1)
	projection["type"] = "array"
	again, _ := first.Lookup(ProjectionV1)
	if again["type"] != "object" {
		t.Fatalf("lookup mutated registry: %#v", again)
	}

	ids := first.IDs()
	if !reflect.DeepEqual(ids, second.IDs()) {
		t.Fatalf("schema ids are not stable: %#v %#v", ids, second.IDs())
	}
}

func TestSchemaRegistryRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	valid := Definition{ID: "pinax.valid.v1", Schema: closedObject(nil)}
	tests := []struct {
		name        string
		definitions []Definition
		want        string
	}{
		{name: "invalid id", definitions: []Definition{{ID: "Pinax Invalid", Schema: closedObject(nil)}}, want: "invalid schema id"},
		{name: "duplicate", definitions: []Definition{valid, valid}, want: "duplicate schema"},
		{name: "implicit open object", definitions: []Definition{{ID: "pinax.open.v1", Schema: map[string]any{"type": "object", "properties": map[string]any{}}}}, want: "additionalProperties"},
		{name: "dangling ref", definitions: []Definition{{ID: "pinax.ref.v1", Schema: closedObject(map[string]any{"missing": refValue("pinax.missing.v1")})}}, want: "dangling reference"},
		{name: "external ref", definitions: []Definition{{ID: "pinax.ref.v1", Schema: closedObject(map[string]any{"external": map[string]any{"$ref": "https://example.com/schema"}})}}, want: "unsupported reference"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.definitions...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestSchemaRegistryWithAddsDefinitionsWithoutMutatingBase(t *testing.T) {
	t.Parallel()

	base := Default()
	extended, err := base.With(Definition{ID: "pinax.extension.v1", Schema: closedObject(map[string]any{"value": stringValue()}, "value")})
	if err != nil {
		t.Fatal(err)
	}
	if base.Has("pinax.extension.v1") {
		t.Fatal("With mutated base registry")
	}
	if !extended.Has("pinax.extension.v1") {
		t.Fatal("extended registry is missing added schema")
	}
}
