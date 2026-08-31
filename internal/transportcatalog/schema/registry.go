package schema

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	Draft202012            = "https://json-schema.org/draft/2020-12/schema"
	RegistrySchemaV1       = "pinax.schema_registry.v1"
	ProjectionV1           = "pinax.projection.v1"
	ErrorV1                = "pinax.error.v1"
	ReadinessV1            = "pinax.readiness.v1"
	ConnectionReadinessV1  = "pinax.connection_readiness.v1"
	TransportManifestV1    = "pinax.transport_manifest.v1"
	OperationV1            = "pinax.operation.v1"
	MCPToolResultV1        = "pinax.mcp.tool_result.v1"
	TransportManifestReq   = "pinax.transport_manifest.request.v1"
	ConnectionReadinessReq = "pinax.connection_readiness.request.v1"
)

var schemaIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

type Definition struct {
	ID     string
	Schema map[string]any
}

type Registry struct {
	schemas map[string]map[string]any
}

type Snapshot struct {
	SchemaVersion string                    `json:"schema_version"`
	Schemas       map[string]map[string]any `json:"schemas"`
}

func Ref(id string) string {
	return "pinax://schema/" + id
}

func New(definitions ...Definition) (*Registry, error) {
	registry := &Registry{schemas: make(map[string]map[string]any, len(definitions))}
	for _, definition := range definitions {
		id := strings.TrimSpace(definition.ID)
		if !schemaIDPattern.MatchString(id) {
			return nil, fmt.Errorf("invalid schema id %q", definition.ID)
		}
		if _, exists := registry.schemas[id]; exists {
			return nil, fmt.Errorf("duplicate schema %q", id)
		}
		document, ok := cloneValue(definition.Schema).(map[string]any)
		if !ok || document == nil {
			return nil, fmt.Errorf("schema %q document is required", id)
		}
		if declared, _ := document["$schema"].(string); declared != "" && declared != Draft202012 {
			return nil, fmt.Errorf("schema %q declares unsupported dialect %q", id, declared)
		}
		if declared, _ := document["$id"].(string); declared != "" && declared != Ref(id) {
			return nil, fmt.Errorf("schema %q declares mismatched $id %q", id, declared)
		}
		document["$schema"] = Draft202012
		document["$id"] = Ref(id)
		registry.schemas[id] = document
	}
	if err := registry.Validate(); err != nil {
		return nil, err
	}
	return registry, nil
}

func Default() *Registry {
	return defaultRegistry.clone()
}

func (r *Registry) Lookup(id string) (map[string]any, bool) {
	if r == nil {
		return nil, false
	}
	document, ok := r.schemas[id]
	if !ok {
		return nil, false
	}
	return cloneValue(document).(map[string]any), true
}

func (r *Registry) Has(id string) bool {
	if r == nil {
		return false
	}
	_, ok := r.schemas[id]
	return ok
}

func (r *Registry) With(definitions ...Definition) (*Registry, error) {
	if r == nil {
		return nil, fmt.Errorf("schema registry is nil")
	}
	combined := make([]Definition, 0, len(r.schemas)+len(definitions))
	for _, id := range r.IDs() {
		combined = append(combined, Definition{ID: id, Schema: cloneValue(r.schemas[id]).(map[string]any)})
	}
	combined = append(combined, definitions...)
	return New(combined...)
}

func (r *Registry) Require(id string) (map[string]any, error) {
	document, ok := r.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("schema %q is not registered", id)
	}
	return document, nil
}

func (r *Registry) IDs() []string {
	if r == nil {
		return nil
	}
	ids := make([]string, 0, len(r.schemas))
	for id := range r.schemas {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r *Registry) Snapshot() Snapshot {
	schemas := make(map[string]map[string]any, len(r.schemas))
	for _, id := range r.IDs() {
		schemas[id] = cloneValue(r.schemas[id]).(map[string]any)
	}
	return Snapshot{SchemaVersion: RegistrySchemaV1, Schemas: schemas}
}

func (r *Registry) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.Snapshot())
}

func (r *Registry) Validate() error {
	if r == nil {
		return fmt.Errorf("schema registry is nil")
	}
	for _, id := range r.IDs() {
		document := r.schemas[id]
		if document["$schema"] != Draft202012 {
			return fmt.Errorf("schema %q does not declare draft 2020-12", id)
		}
		if document["$id"] != Ref(id) {
			return fmt.Errorf("schema %q has mismatched $id", id)
		}
		if err := validateNodePolicy(id, "$", document, r.schemas); err != nil {
			return err
		}
	}
	return nil
}

func validateNodePolicy(schemaID, path string, value any, known map[string]map[string]any) error {
	switch node := value.(type) {
	case map[string]any:
		if ref, ok := node["$ref"].(string); ok {
			if !strings.HasPrefix(ref, "pinax://schema/") {
				return fmt.Errorf("schema %q has unsupported reference %q at %s", schemaID, ref, path)
			}
			target := strings.TrimPrefix(ref, "pinax://schema/")
			if _, exists := known[target]; !exists {
				return fmt.Errorf("schema %q has dangling reference %q at %s", schemaID, ref, path)
			}
		}
		_, hasProperties := node["properties"]
		if node["type"] == "object" || hasProperties {
			if _, explicit := node["additionalProperties"]; !explicit {
				return fmt.Errorf("schema %q object at %s must declare additionalProperties", schemaID, path)
			}
		}
		keys := make([]string, 0, len(node))
		for key := range node {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := validateNodePolicy(schemaID, path+"/"+key, node[key], known); err != nil {
				return err
			}
		}
	case []any:
		for index, item := range node {
			if err := validateNodePolicy(schemaID, fmt.Sprintf("%s/%d", path, index), item, known); err != nil {
				return err
			}
		}
	case []map[string]any:
		for index, item := range node {
			if err := validateNodePolicy(schemaID, fmt.Sprintf("%s/%d", path, index), item, known); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Registry) clone() *Registry {
	if r == nil {
		return nil
	}
	copy := &Registry{schemas: make(map[string]map[string]any, len(r.schemas))}
	for id, document := range r.schemas {
		copy.schemas[id] = cloneValue(document).(map[string]any)
	}
	return copy
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copy := make(map[string]any, len(typed))
		for key, item := range typed {
			copy[key] = cloneValue(item)
		}
		return copy
	case []any:
		copy := make([]any, len(typed))
		for index, item := range typed {
			copy[index] = cloneValue(item)
		}
		return copy
	case []string:
		return append([]string(nil), typed...)
	case []map[string]any:
		copy := make([]map[string]any, len(typed))
		for index, item := range typed {
			copy[index] = cloneValue(item).(map[string]any)
		}
		return copy
	default:
		return typed
	}
}
