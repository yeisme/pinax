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
	// The sync-control routes are the additive DriveBridge consumer contract.
	const wantDigest = "sha256:792537ecc26d6778ed0eb09496821b8be3d8dd6c08d4caf448ca1cd8d6ea3519"
	if digest != wantDigest {
		t.Fatalf("OpenAPI golden digest = %q, want %q", digest, wantDigest)
	}
	for _, path := range []string{"/v1/sync/status", "/v1/sync:diff", "/v1/sync:push", "/v1/sync:pull", "/v1/sync/conflicts", "/v1/sync/conflicts:diff", "/v1/sync/conflicts:resolve"} {
		delete(roundTripped["paths"].(map[string]any), path)
	}
	for id := range roundTripped["components"].(map[string]any)["schemas"].(map[string]any) {
		if strings.HasPrefix(id, "pinax.sync.") && id != "pinax.sync.logs.status.request.v1" {
			delete(roundTripped["components"].(map[string]any)["schemas"].(map[string]any), id)
		}
	}
	delete(roundTripped["paths"].(map[string]any), "/v1/reader/recovery/operation")
	delete(roundTripped["components"].(map[string]any)["schemas"].(map[string]any), catalogschema.ReaderRecoveryLookupRequestV1Alpha1)
	preRecovery, err := OpenAPIDigest(roundTripped)
	if err != nil || preRecovery != "sha256:127fe7b474db1e7609dfff82fe5dcf69a5be3e75e7941f9775fb9f6ad31240c1" {
		t.Fatal("OpenAPI changed beyond recovery lookup", preRecovery, err)
	}
	delete(roundTripped["paths"].(map[string]any), "/v1/reader/notes/{object_id}/composed-document")
	preComposedRoute, err := OpenAPIDigest(roundTripped)
	if err != nil || preComposedRoute != "sha256:21820196e4db286959a2683190c170f8d8dbb431a5f147e1763d40ed848aa860" {
		t.Fatal("OpenAPI changed beyond composed route", preComposedRoute, err)
	}
	composedComponents := roundTripped["components"].(map[string]any)["schemas"].(map[string]any)
	for _, id := range []string{catalogschema.ReaderComposedRequestV1Alpha1, catalogschema.ReaderEmbedTargetV1Alpha1, catalogschema.ReaderEmbedV1Alpha1, catalogschema.ReaderNodeOriginV1Alpha1, catalogschema.ReaderComposedDocumentV1Alpha1} {
		delete(composedComponents, id)
	}
	preComposed, err := OpenAPIDigest(roundTripped)
	if err != nil || preComposed != "sha256:0e316364bebc98fa1ce37eec0c7530805d5856c97cbd489db0bb661c0e84cdda" {
		t.Fatal("OpenAPI changed beyond composed schemas", preComposed, err)
	}
	// The notebook source is additive: removing only its path and schemas
	// must restore the exact previous writer/reader/legacy contract.
	delete(roundTripped["paths"].(map[string]any), "/v1/reader/notes/{object_id}/rich-document")
	preRichRoute, err := OpenAPIDigest(roundTripped)
	if err != nil || preRichRoute != "sha256:9cbc3954185266e3246e7b50bbd0c98740b1217c83ccc4ccf429d932b1e7d4d9" {
		t.Fatal("OpenAPI changed beyond rich route", preRichRoute, err)
	}
	richComponents := roundTripped["components"].(map[string]any)["schemas"].(map[string]any)
	for _, id := range []string{catalogschema.ReaderRichDocumentRequestV1Alpha1, catalogschema.ReaderRichAnnotationV1Alpha1, catalogschema.ReaderRichDocumentV1Alpha1} {
		delete(richComponents, id)
	}
	preRich, err := OpenAPIDigest(roundTripped)
	if err != nil || preRich != "sha256:56e5577a782ed83614a7b0cb234fa1e1fe2646e6552342d8948ff6c8fc9d7cab" {
		t.Fatal("OpenAPI changed beyond rich schemas", preRich, err)
	}
	paths := roundTripped["paths"].(map[string]any)
	for _, path := range []string{"/v1/writer/tags:plan", "/v1/writer/tags:apply", "/v1/writer/tags:lookup"} {
		delete(paths, path)
	}
	preTagRoutes, err := OpenAPIDigest(roundTripped)
	if err != nil || preTagRoutes != "sha256:904b2cde178840262dadcec796de974ef6e7c7b3152f61f7575eb87c59f89c4c" {
		t.Fatal("OpenAPI changed beyond tag routes", preTagRoutes, err)
	}
	components := roundTripped["components"].(map[string]any)["schemas"].(map[string]any)
	for _, id := range []string{catalogschema.WriterTagsPlanRequestV1Alpha1, catalogschema.WriterTagsPlanV1Alpha1, catalogschema.WriterTagsApplyRequestV1Alpha1, catalogschema.WriterTagsLookupRequestV1Alpha1, catalogschema.WriterTagsReceiptV1Alpha1} {
		delete(components, id)
	}
	preTags, err := OpenAPIDigest(roundTripped)
	if err != nil || preTags != "sha256:79473f6d2313e13942f2c252b60ea295285b33d067886491f7451200b8ed3d15" {
		t.Fatal("OpenAPI changed beyond tag schemas", preTags, err)
	}
	for _, path := range []string{"/v1/writer/journals/daily/editor-source", "/v1/writer/journals/daily/edit:propose", "/v1/writer/journals/daily/edit:apply"} {
		delete(paths, path)
	}
	for _, id := range []string{catalogschema.WriterDailyEditorSourceRequestV1Alpha1, catalogschema.WriterDailyEditProposeRequestV1Alpha1, catalogschema.WriterDailyEditProposalV1Alpha1, catalogschema.WriterDailyEditApplyRequestV1Alpha1} {
		delete(components, id)
	}
	preDailyEdit, err := OpenAPIDigest(roundTripped)
	if err != nil || preDailyEdit != "sha256:d7117ab0a11baf434e833d66ba3d2311e007695d96b7a87265662b0c848d10e1" {
		t.Fatal("OpenAPI changed beyond daily edit", preDailyEdit, err)
	}
	delete(paths, "/v1/reader/journals/daily/document")
	delete(components, catalogschema.ReaderDailyDocumentRequestV1Alpha1)
	preDailyDocument, err := OpenAPIDigest(roundTripped)
	if err != nil || preDailyDocument != "sha256:aec005c500eff8f993d3ed18f6d2c099745bef834e8f1cab3d5a7abce5b6cf2e" {
		t.Fatal("OpenAPI changed beyond daily document", preDailyDocument, err)
	}
	delete(paths, "/v1/writer/journals/daily:propose")
	delete(paths, "/v1/writer/journals/daily:apply")
	for _, id := range []string{"pinax.writer.daily_propose.request.v1alpha1", "pinax.writer.daily_apply.request.v1alpha1", "pinax.writer.daily_proposal.v1alpha1"} {
		delete(components, id)
	}
	preDailyDigest, err := OpenAPIDigest(roundTripped)
	if err != nil || preDailyDigest != "sha256:5279a91d607e0039a514576424a699aa977ea143965c1edf8e7e7342cbdac6f6" {
		t.Fatal("OpenAPI changed beyond the admitted daily write paths and schemas", err)
	}
	for _, path := range []string{"/v1/reader/tags", "/v1/reader/graph", "/v1/reader/journals/daily"} {
		delete(paths, path)
	}
	for _, id := range []string{"tags", "graph", "daily"} {
		delete(components, "pinax.reader."+id+".v1alpha1")
		delete(components, "pinax.reader."+id+".request.v1alpha1")
	}
	preNavigationDigest, err := OpenAPIDigest(roundTripped)
	if err != nil || preNavigationDigest != "sha256:af9d7f3caca6268ed54f56de14175419d16ce0ce0cdf365c5dc78c3b6d57bff3" {
		t.Fatal("OpenAPI changed beyond the admitted navigation paths and schemas", err)
	}
	delete(paths, "/v1/writer/notes/{object_id}/editor-source")
	delete(components, "pinax.writer.editor_source.v1alpha1")
	delete(components, "pinax.writer.editor_source.request.v1alpha1")
	preEditorDigest, err := OpenAPIDigest(roundTripped)
	if err != nil || preEditorDigest != "sha256:7414054277f21ea2c48461c2f7b29fe8ad7e74310320d1e8f846b0dc50ecacfd" {
		t.Fatal("OpenAPI changed beyond the admitted editor source path and schemas", err)
	}
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
	if err != nil || preWriterDigest != "sha256:7857d6eebaf0b0eb6243ab550328ca56a8e0897af4d9d94ab9127a51a27ff5d8" {
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
	if err != nil || preReaderDigest != "sha256:44b9aca4d32411e1d8cb608948a9a090c427899de14fa9f7c17e2ced3e2a73c8" {
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
	if err != nil || previousDigest != "sha256:03d6f033fd8f20aa13518c37b3dd06121254415a648a55ece4031df5ae868d55" {
		t.Fatal("OpenAPI changed beyond admitted optional MCP parameters", err)
	}
	for _, id := range []string{"pinax.agent.note_proposals.request.v1alpha1", "pinax.render_note.request.v1alpha1"} {
		if _, ok := schemas[id]; !ok {
			t.Fatal("new request schema missing")
		}
		delete(schemas, id)
	}
	legacyDigest, err := OpenAPIDigest(roundTripped)
	if err != nil || legacyDigest != "sha256:8495dad290fb5d2e745a571f65fce98c81075d931e07c40c44947e53e0364556" {
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
