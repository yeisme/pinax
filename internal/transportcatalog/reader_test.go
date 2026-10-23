package transportcatalog

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// TestReaderContract freezes the human reading transport catalog slice from
// pinax-human-reading-api-v1 P01: capability identities, GET-only REST routes,
// planned availability without backing, request/response schema wiring, error
// vocabulary and deterministic digest. Implementation tasks P02-P06 flip
// availability only after registering a real adapter backing; until then
// available_surfaces must stay empty for every reader capability.
func TestReaderContract(t *testing.T) {
	t.Parallel()

	manifest, err := CompileReaderContract()
	if err != nil {
		t.Fatalf("CompileReaderContract() error = %v", err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("reader manifest invalid: %v", err)
	}

	expectedRoutes := map[string]struct {
		capabilityID   string
		method, path   string
		requestSchema  string
		responseSchema string
	}{
		"rest.reader.recovery.lookup":   {"reader.recovery.lookup", "GET", "/v1/reader/recovery/operation", catalogschema.ReaderRecoveryLookupRequestV1Alpha1, catalogschema.OperationV1},
		"rest.reader.composed_document": {"reader.composed_document", "GET", "/v1/reader/notes/{object_id}/composed-document", catalogschema.ReaderComposedRequestV1Alpha1, catalogschema.ReaderComposedDocumentV1Alpha1},
		"rest.reader.rich_document":     {"reader.rich_document", "GET", "/v1/reader/notes/{object_id}/rich-document", catalogschema.ReaderRichDocumentRequestV1Alpha1, catalogschema.ReaderRichDocumentV1Alpha1},
		"rest.reader.daily.document":    {"reader.daily.document", "GET", "/v1/reader/journals/daily/document", catalogschema.ReaderDailyDocumentRequestV1Alpha1, catalogschema.ReaderDocumentV1Alpha1},
		RestReaderTagsBindingID:         {ReaderTagsCapabilityID, "GET", "/v1/reader/tags", catalogschema.ReaderTagsRequestV1Alpha1, catalogschema.ReaderTagsV1Alpha1},
		RestReaderGraphBindingID:        {ReaderGraphCapabilityID, "GET", "/v1/reader/graph", catalogschema.ReaderGraphRequestV1Alpha1, catalogschema.ReaderGraphV1Alpha1},
		RestReaderDailyBindingID:        {ReaderDailyCapabilityID, "GET", "/v1/reader/journals/daily", catalogschema.ReaderDailyRequestV1Alpha1, catalogschema.ReaderDailyV1Alpha1},
		RestReaderSourceBindingID:       {ReaderSourceCapabilityID, "GET", "/v1/reader/source", "pinax.reader.source.request.v1alpha1", "pinax.reader.source.v1alpha1"},
		RestReaderNotesBindingID:        {ReaderNotesCapabilityID, "GET", "/v1/reader/notes", "pinax.reader.notes.request.v1alpha1", "pinax.reader.note_list.v1alpha1"},
		RestReaderSearchBindingID:       {ReaderSearchCapabilityID, "GET", "/v1/reader/search", "pinax.reader.search.request.v1alpha1", "pinax.reader.search_results.v1alpha1"},
		RestReaderDocumentBindingID:     {ReaderDocumentCapabilityID, "GET", "/v1/reader/notes/{object_id}/document", "pinax.reader.document.request.v1alpha1", "pinax.reader.document.v1alpha1"},
		RestReaderLinksBindingID:        {ReaderLinksCapabilityID, "GET", "/v1/reader/notes/{object_id}/links", "pinax.reader.links.request.v1alpha1", "pinax.reader.link_page.v1alpha1"},
		RestReaderAssetBindingID:        {ReaderAssetCapabilityID, "GET", "/v1/reader/assets/{asset_ref}/content", "pinax.reader.asset_content.request.v1alpha1", "pinax.reader.asset_ref.v1alpha1"},
		RestReaderSourceExportBindingID: {ReaderSourceExportCapabilityID, "GET", "/v1/reader/notes/{object_id}/source", "pinax.reader.source_export.request.v1alpha1", "pinax.reader.source_export.v1alpha1"},
	}

	if len(manifest.Capabilities) != len(expectedRoutes) {
		t.Fatalf("reader capabilities = %d, want %d", len(manifest.Capabilities), len(expectedRoutes))
	}

	registry := catalogschema.Default()
	seenRoutes := make(map[string]bool, len(expectedRoutes))
	for _, capability := range manifest.Capabilities {
		if capability.Stability != StabilityExperimental {
			t.Errorf("capability %q stability = %q, want experimental", capability.ID, capability.Stability)
		}
		if !capability.Readonly {
			t.Errorf("capability %q must stay readonly", capability.ID)
		}
		if !reflect.DeepEqual(capability.DeclaredSurfaces, []string{"rest"}) {
			t.Errorf("capability %q declared surfaces = %#v, want [rest]", capability.ID, capability.DeclaredSurfaces)
		}
		// Since P06 the reader bindings are backed by real handlers, so the
		// manifest must advertise REST availability exactly and truthfully.
		if !reflect.DeepEqual(capability.AvailableSurfaces, []string{"rest"}) {
			t.Fatalf("capability %q available surfaces = %#v, want [rest]", capability.ID, capability.AvailableSurfaces)
		}
		if len(capability.Bindings) != 1 {
			t.Fatalf("capability %q bindings = %d, want 1", capability.ID, len(capability.Bindings))
		}
		binding := capability.Bindings[0]
		expected, ok := expectedRoutes[binding.ID]
		if !ok {
			t.Fatalf("unexpected reader binding %q", binding.ID)
		}
		seenRoutes[binding.ID] = true
		if binding.CapabilityID != expected.capabilityID || binding.Method != expected.method || binding.Path != expected.path {
			t.Errorf("binding %q identity drift: got %s %s %s", binding.ID, binding.CapabilityID, binding.Method, binding.Path)
		}
		if binding.RequestSchema != expected.requestSchema || binding.ResponseSchema != expected.responseSchema {
			t.Errorf("binding %q schema drift: %s -> %s", binding.ID, binding.RequestSchema, binding.ResponseSchema)
		}
		if binding.Transport != TransportREST {
			t.Errorf("binding %q transport = %q, want rest", binding.ID, binding.Transport)
		}
		if binding.Availability != AvailabilityAvailable {
			t.Fatalf("binding %q availability = %q, want available since P06 backing", binding.ID, binding.Availability)
		}
		if binding.BackingRef != "internal/api/http:"+binding.ID {
			t.Errorf("binding %q backing = %q", binding.ID, binding.BackingRef)
		}
		if binding.Readiness == nil || binding.Readiness.Status != ReadinessReady {
			t.Errorf("binding %q readiness = %#v", binding.ID, binding.Readiness)
		}
		if !registry.Has(binding.RequestSchema) || !registry.Has(binding.ResponseSchema) {
			t.Errorf("binding %q references unregistered schema", binding.ID)
		}
		if !containsString(capability.Errors, "insufficient_scope") {
			t.Errorf("capability %q must declare insufficient_scope", capability.ID)
		}
	}
	for bindingID := range expectedRoutes {
		if !seenRoutes[bindingID] {
			t.Errorf("expected binding %q missing from reader manifest", bindingID)
		}
	}

	// Error vocabulary freeze per docs/interfaces/human-reading-api.md §3.
	errorFreeze := map[string][]string{
		ReaderNotesCapabilityID:        {"reader_cursor_invalid", "reader_cursor_expired", "index_unavailable"},
		ReaderSearchCapabilityID:       {"reader_request_invalid", "index_unavailable"},
		ReaderDocumentCapabilityID:     {"revision_unavailable", "source_changed", "document_too_large", "document_budget_exceeded", "reader_unsupported", "reader_object_not_found"},
		ReaderLinksCapabilityID:        {"reader_object_not_found", "revision_unavailable"},
		ReaderAssetCapabilityID:        {"reader_range_invalid", "reader_object_not_found", "revision_unavailable"},
		ReaderSourceExportCapabilityID: {"document_too_large", "reader_range_invalid"},
	}
	for _, capability := range manifest.Capabilities {
		for _, code := range errorFreeze[capability.ID] {
			if !containsString(capability.Errors, code) {
				t.Errorf("capability %q is missing frozen error %q", capability.ID, code)
			}
		}
	}

	// Budget freeze: SourceDescriptor limits must encode the contract numbers.
	source, ok := registry.Lookup(catalogschema.ReaderSourceV1Alpha1)
	if !ok {
		t.Fatalf("schema %q is not registered", catalogschema.ReaderSourceV1Alpha1)
	}
	properties, _ := source["properties"].(map[string]any)
	limits, _ := properties["limits"].(map[string]any)
	limitProperties, _ := limits["properties"].(map[string]any)
	frozen := map[string]float64{
		"list_default_limit":            50,
		"list_max_limit":                200,
		"snippet_max_chars":             320,
		"outline_page_max_entries":      512,
		"document_page_max_text_bytes":  262144,
		"document_page_max_nodes":       512,
		"document_response_max_bytes":   1048576,
		"document_source_max_bytes":     8388608,
		"document_total_max_nodes":      65536,
		"asset_range_max_bytes":         8388608,
		"source_export_range_max_bytes": 1048576,
	}
	for name, want := range frozen {
		limitSchema, ok := limitProperties[name].(map[string]any)
		if !ok {
			t.Fatalf("limits schema is missing frozen budget %q", name)
		}
		got, ok := asConstInt(limitSchema["const"])
		if !ok || got != int64(want) {
			t.Errorf("budget %q = %v, want %v", name, limitSchema["const"], want)
		}
	}

	// Deterministic digest: recompile must be byte-stable.
	recompiled, err := CompileReaderContract()
	if err != nil {
		t.Fatal(err)
	}
	first, _ := json.Marshal(manifest)
	second, _ := json.Marshal(recompiled)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("reader manifest is not byte-stable:\n%s\n%s", first, second)
	}

	// Guard: a reader binding cannot become available without a backing_ref.
	available := ReaderTransportBindings()[0]
	available.Availability = AvailabilityAvailable
	available.BackingRef = ""
	if _, err := Compile(ReaderCapabilityDefinitions(), []TransportBinding{available}, nil); err == nil || !strings.Contains(err.Error(), "backing_ref") {
		t.Fatalf("available reader binding without backing must fail, got %v", err)
	}

	// Guard: adapter parity — every available reader binding must match a
	// registered HTTP handler operation and vice versa.
	adapters := make([]AdapterOperation, 0)
	for _, binding := range ReaderTransportBindings() {
		adapters = append(adapters, AdapterOperation{
			BindingID: binding.ID, CapabilityID: binding.CapabilityID,
			Transport: binding.Transport, BackingRef: binding.BackingRef,
			Method: binding.Method, Path: binding.Path,
		})
	}
	if err := ValidateConformance(manifest, adapters, TransportREST); err != nil {
		t.Fatalf("reader REST conformance failed: %v", err)
	}

	// Guard: schemas must not leak absolute paths or audience-style authz.
	requestSchemas := []string{
		catalogschema.ReaderSourceRequestV1Alpha1,
		catalogschema.ReaderNotesRequestV1Alpha1,
		catalogschema.ReaderSearchRequestV1Alpha1,
	}
	for _, id := range requestSchemas {
		document, _ := registry.Lookup(id)
		encoded, _ := json.Marshal(document)
		if strings.Contains(string(encoded), "audience") {
			t.Errorf("request schema %q must not accept an audience field", id)
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// asConstInt reads schema consts written as Go int literals.
func asConstInt(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		if typed == float64(int64(typed)) {
			return int64(typed), true
		}
	}
	return 0, false
}
