package app

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestOpenAPIUsesTypedRESTContractsAndSharedSchemas(t *testing.T) {
	t.Parallel()

	document, err := BuildOpenAPI()
	if err != nil {
		t.Fatal(err)
	}
	if document["openapi"] != "3.1.0" || document["jsonSchemaDialect"] != catalogschema.Draft202012 {
		t.Fatalf("OpenAPI identity = %#v", document)
	}
	components, _ := document["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	for _, id := range []string{catalogschema.ProjectionV1, catalogschema.ErrorV1, catalogschema.ReadinessV1, catalogschema.TransportManifestV1, catalogschema.OperationV1} {
		if _, ok := schemas[id]; !ok {
			t.Errorf("OpenAPI components missing shared schema %q", id)
		}
	}
	securitySchemes, _ := components["securitySchemes"].(map[string]any)
	bearer, _ := securitySchemes["BearerAuth"].(map[string]any)
	if bearer["type"] != "http" || bearer["scheme"] != "bearer" {
		t.Fatalf("BearerAuth = %#v", bearer)
	}

	paths, _ := document["paths"].(map[string]any)
	contracts, err := RemoteRESTContracts()
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range contracts {
		pathItem, _ := paths[contract.Path].(map[string]any)
		operation, _ := pathItem[strings.ToLower(contract.Method)].(map[string]any)
		if operation == nil {
			t.Errorf("missing OpenAPI operation for %s %s", contract.Method, contract.Path)
			continue
		}
		if operation["operationId"] != contract.RouteID || operation["x-pinax-capability-id"] != contract.CapabilityID || operation["x-pinax-write-gate"] != contract.WriteGate {
			t.Errorf("operation metadata drift for %s: %#v", contract.RouteID, operation)
		}
		parameters, _ := operation["parameters"].([]map[string]any)
		if len(parameters) != len(contract.Parameters) {
			t.Errorf("operation %s parameters = %d, want %d", contract.RouteID, len(parameters), len(contract.Parameters))
		}
		_, hasBody := operation["requestBody"]
		if hasBody != (contract.RequestBody != nil) {
			t.Errorf("operation %s requestBody presence = %v, want %v", contract.RouteID, hasBody, contract.RequestBody != nil)
		}
		responses, _ := operation["responses"].(map[string]any)
		success, _ := responses["200"].(map[string]any)
		if success["x-pinax-data-schema"] != componentRef(contract.SuccessDataSchema) {
			t.Errorf("operation %s success data schema = %#v", contract.RouteID, success["x-pinax-data-schema"])
		}
	}
}

func TestOpenAPIRewritesRegistryReferencesToComponents(t *testing.T) {
	t.Parallel()

	document, err := BuildOpenAPI()
	if err != nil {
		t.Fatal(err)
	}
	components := document["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	manifest := schemas[catalogschema.TransportManifestV1].(map[string]any)
	if containsSchemaRefPrefix(manifest, "pinax://schema/") {
		t.Fatalf("manifest schema retained registry URI refs: %#v", manifest)
	}
	if !containsSchemaRefPrefix(manifest, "#/components/schemas/") {
		t.Fatalf("manifest schema has no component refs: %#v", manifest)
	}
	first, _ := BuildOpenAPI()
	if !reflect.DeepEqual(document, first) {
		t.Fatal("OpenAPI generation is not semantically deterministic")
	}
}

func TestOpenAPISemanticValidationRoundTripAndGoldenDigest(t *testing.T) {
	t.Parallel()

	document, err := BuildOpenAPI()
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateOpenAPI(document); err != nil {
		t.Fatalf("ValidateOpenAPI() error = %v", err)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var roundTripped map[string]any
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOpenAPI(roundTripped); err != nil {
		t.Fatalf("round-tripped OpenAPI is invalid: %v", err)
	}
	digest, err := OpenAPIDigest(document)
	if err != nil {
		t.Fatal(err)
	}
	const wantDigest = "sha256:298b6b47a7c2a922bf520d93fb0773ec88fc8d0664ed08647962ff23b3090e54"
	if digest != wantDigest {
		t.Fatalf("OpenAPI golden digest = %q, want %q", digest, wantDigest)
	}
}

func TestOpenAPISemanticValidatorRejectsDuplicateOperationsAndDanglingRefs(t *testing.T) {
	t.Parallel()

	t.Run("duplicate operation id", func(t *testing.T) {
		document, err := BuildOpenAPI()
		if err != nil {
			t.Fatal(err)
		}
		paths := document["paths"].(map[string]any)
		first := paths["/v1/manifest"].(map[string]any)["get"].(map[string]any)
		second := paths["/v1/workbench/status"].(map[string]any)["get"].(map[string]any)
		second["operationId"] = first["operationId"]
		if err := ValidateOpenAPI(document); err == nil || !strings.Contains(err.Error(), "duplicate OpenAPI operationId") {
			t.Fatalf("ValidateOpenAPI() error = %v", err)
		}
	})

	t.Run("dangling schema ref", func(t *testing.T) {
		document, err := BuildOpenAPI()
		if err != nil {
			t.Fatal(err)
		}
		paths := document["paths"].(map[string]any)
		operation := paths["/v1/manifest"].(map[string]any)["get"].(map[string]any)
		responses := operation["responses"].(map[string]any)
		success := responses["200"].(map[string]any)
		content := success["content"].(map[string]any)
		media := content["application/json"].(map[string]any)
		media["schema"] = map[string]any{"$ref": "#/components/schemas/pinax.missing.v1"}
		if err := ValidateOpenAPI(document); err == nil || !strings.Contains(err.Error(), "dangling schema ref") {
			t.Fatalf("ValidateOpenAPI() error = %v", err)
		}
	})

	t.Run("unknown security scheme", func(t *testing.T) {
		document, err := BuildOpenAPI()
		if err != nil {
			t.Fatal(err)
		}
		paths := document["paths"].(map[string]any)
		operation := paths["/v1/manifest"].(map[string]any)["get"].(map[string]any)
		operation["security"] = []map[string]any{{"MissingAuth": []string{}}}
		if err := ValidateOpenAPI(document); err == nil || !strings.Contains(err.Error(), "unknown security scheme") {
			t.Fatalf("ValidateOpenAPI() error = %v", err)
		}
	})
}

func containsSchemaRefPrefix(value any, prefix string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if key == "$ref" {
				if ref, ok := item.(string); ok && strings.HasPrefix(ref, prefix) {
					return true
				}
			}
			if containsSchemaRefPrefix(item, prefix) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if containsSchemaRefPrefix(item, prefix) {
				return true
			}
		}
	case []map[string]any:
		for _, item := range typed {
			if containsSchemaRefPrefix(item, prefix) {
				return true
			}
		}
	}
	return false
}
