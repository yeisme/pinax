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
	// pinax-human-reading-write-v1 W01 registers the writer contracts as
	// schema components, W02 evolves the session bootstrap, and W06 registers
	// the implemented writer operations. Removing the writer paths, writer
	// schema components and the W02 bootstrap evolution must restore the
	// pre-writer digest exactly.
	const wantDigest = "sha256:bc8cc6059608d03a73fa92cccaaf6bc9024610cb95405a5f87ca0616acb9abd4"
	if digest != wantDigest {
		t.Fatalf("OpenAPI golden digest = %q, want %q", digest, wantDigest)
	}
	paths := roundTripped["paths"].(map[string]any)
	for path := range paths {
		if strings.HasPrefix(path, "/v1/writer/") {
			delete(paths, path)
		}
	}
	schemas := roundTripped["components"].(map[string]any)["schemas"].(map[string]any)
	for id := range schemas {
		if strings.HasPrefix(id, "pinax.writer.") {
			delete(schemas, id)
		}
	}
	bootstrapSchema := schemas["pinax.local_session.bootstrap.v1alpha1"].(map[string]any)
	bootstrapProps := bootstrapSchema["properties"].(map[string]any)
	bootstrapProps["readonly"] = map[string]any{"type": "boolean", "const": true}
	bootstrapScopes := bootstrapProps["scopes"].(map[string]any)
	delete(bootstrapScopes, "description")
	bootstrapItems := bootstrapScopes["items"].(map[string]any)
	bootstrapItems["enum"] = []string{"reader.metadata", "reader.body", "reader.relations", "reader.assets"}
	preWriterDigest, err := OpenAPIDigest(roundTripped)
	if err != nil || preWriterDigest != "sha256:07458efaaffb4473eee6eb2582b61948584510f3e9b10a044dc83d21f54811ed" {
		t.Fatal("OpenAPI changed beyond the admitted writer paths, schema components and bootstrap evolution", err)
	}
	// pinax-human-reading-api-v1 P06 registers the implemented reader
	// operations. Removing the reader paths plus their schema components
	// must restore the pre-reader digest exactly.
	for path := range paths {
		if strings.HasPrefix(path, "/v1/reader/") {
			delete(paths, path)
		}
	}
	for _, id := range []string{
		"rest.reader.source", "rest.reader.notes", "rest.reader.search", "rest.reader.document",
		"rest.reader.links", "rest.reader.asset", "rest.reader.source_export",
	} {
		if _, ok := paths["/v1/reader/"+id]; ok {
			t.Fatalf("reader path lookup drift for %q", id)
		}
	}
	for id := range schemas {
		if strings.HasPrefix(id, "pinax.reader.") || strings.HasPrefix(id, "pinax.local_session.") {
			delete(schemas, id)
		}
	}
	preReaderDigest, err := OpenAPIDigest(roundTripped)
	if err != nil || preReaderDigest != "sha256:cb01a21303e2d7736bf742b439c82ad75dc8f70ef1eceeedc9a7a9ce7b2360ef" {
		t.Fatal("OpenAPI changed beyond the admitted reader paths and schema components", err)
	}
	// Remove only this change's optional MCP parameters and preserve both older
	// exact digest pins; do not mask changes to REST/RPC or any other component.
	organizeProps := schemas["pinax.organize.plan.request.v1"].(map[string]any)["properties"].(map[string]any)
	delete(organizeProps, "note_refs")
	proposalProps := schemas["pinax.agent.note_proposals.request.v1alpha1"].(map[string]any)["properties"].(map[string]any)
	for _, key := range []string{"status", "kind", "limit", "cursor"} {
		delete(proposalProps, key)
	}
	delete(proposalProps["proposal_id"].(map[string]any), "minLength")
	previousDigest, err := OpenAPIDigest(roundTripped)
	if err != nil || previousDigest != "sha256:bddeeb9ad35a81ef0dc62c3e6d5851ade0c3942689ce42784aa46821bce688c7" {
		t.Fatal("OpenAPI changed beyond admitted optional MCP parameters", err)
	}
	for _, id := range []string{"pinax.agent.note_proposals.request.v1alpha1", "pinax.render_note.request.v1alpha1"} {
		if _, ok := schemas[id]; !ok {
			t.Fatal("new request schema missing")
		}
		delete(schemas, id)
	}
	legacyDigest, err := OpenAPIDigest(roundTripped)
	if err != nil || legacyDigest != "sha256:e6a61ee97fadb7258290533f591b91e5319a771d893da1ee9753f6d4e3ee577f" {
		t.Fatal("existing OpenAPI changed beyond the two admitted components", err)
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

func TestOpenAPIByteEndpointsDeclareRealProtocol(t *testing.T) {
	t.Parallel()
	// The generated contract must match the real wire protocol of the byte
	// endpoints: 200 AND 206 binary successes with ETag/Content-Range
	// headers, a Range request header, and the params the handlers reject
	// requests without (revision, note_id, note_revision) declared
	// required.
	document, err := BuildOpenAPI()
	if err != nil {
		t.Fatal(err)
	}
	paths := document["paths"].(map[string]any)
	for _, tc := range []struct {
		path     string
		media    string
		required []string
	}{
		{"/v1/reader/assets/{asset_ref}/content", "application/octet-stream", []string{"asset_ref", "note_id", "note_revision"}},
		{"/v1/reader/notes/{object_id}/source", "text/markdown", []string{"object_id", "revision"}},
	} {
		operation := paths[tc.path].(map[string]any)["get"].(map[string]any)
		responses := operation["responses"].(map[string]any)
		for status, withContentRange := range map[string]bool{"200": false, "206": true} {
			response, ok := responses[status].(map[string]any)
			if !ok {
				t.Fatalf("%s %s missing", tc.path, status)
			}
			content := response["content"].(map[string]any)
			if _, ok := content[tc.media]; !ok {
				t.Fatalf("%s %s media = %v", tc.path, status, content)
			}
			if format := content[tc.media].(map[string]any)["schema"].(map[string]any)["format"]; format != "binary" {
				t.Fatalf("%s %s not binary", tc.path, status)
			}
			headers := response["headers"].(map[string]any)
			if _, ok := headers["ETag"]; !ok {
				t.Fatalf("%s %s missing ETag header", tc.path, status)
			}
			if _, hasRange := headers["Content-Range"]; withContentRange && !hasRange {
				t.Fatalf("%s 206 missing Content-Range header", tc.path)
			}
		}
		if _, ok := responses["200"].(map[string]any)["content"].(map[string]any)["application/json"]; ok {
			t.Fatalf("%s 200 wrongly declared as JSON projection", tc.path)
		}
		required := map[string]bool{}
		hasRangeHeader := false
		for _, parameter := range operation["parameters"].([]map[string]any) {
			typed := parameter
			if typed["required"] == true {
				required[typed["name"].(string)] = true
			}
			if typed["name"] == "Range" && typed["in"] == "header" {
				hasRangeHeader = true
			}
		}
		for _, name := range tc.required {
			if !required[name] {
				t.Fatalf("%s parameter %s not required", tc.path, name)
			}
		}
		if !hasRangeHeader {
			t.Fatalf("%s missing Range request header", tc.path)
		}
	}
	// Links keeps its JSON projection but revision is required.
	links := paths["/v1/reader/notes/{object_id}/links"].(map[string]any)["get"].(map[string]any)
	revisionRequired := false
	for _, parameter := range links["parameters"].([]map[string]any) {
		typed := parameter
		if typed["name"] == "revision" && typed["required"] == true {
			revisionRequired = true
		}
	}
	if !revisionRequired {
		t.Fatal("links revision not required")
	}
}
