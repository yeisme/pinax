package transportcatalog

// Reader capability and binding IDs for the planned human reading surface.
// The routes, scopes, errors and budgets are frozen by
// pinax-human-reading-api-v1 P01; the human contract lives in
// docs/interfaces/human-reading-api.md and the machine schemas live in
// internal/transportcatalog/schema/reader.go. Every binding stays planned
// until a real adapter registers a backing; flipping one to available without
// backing_ref fails Compile.
const (
	ReaderSourceCapabilityID       = "reader.source"
	ReaderNotesCapabilityID        = "reader.notes"
	ReaderSearchCapabilityID       = "reader.search"
	ReaderDocumentCapabilityID     = "reader.document"
	ReaderLinksCapabilityID        = "reader.links"
	ReaderAssetCapabilityID        = "reader.asset"
	ReaderSourceExportCapabilityID = "reader.source_export"

	RestReaderSourceBindingID       = "rest.reader.source"
	RestReaderNotesBindingID        = "rest.reader.notes"
	RestReaderSearchBindingID       = "rest.reader.search"
	RestReaderDocumentBindingID     = "rest.reader.document"
	RestReaderLinksBindingID        = "rest.reader.links"
	RestReaderAssetBindingID        = "rest.reader.asset"
	RestReaderSourceExportBindingID = "rest.reader.source_export"

	// ReaderImplementationPendingBlocker marks every planned reader binding:
	// P02-P06 implement handlers, P06 registers real backings.
	ReaderImplementationPendingBlocker = "implementation_pending_pinax_human_reading_api_v1"
)

// ReaderCapabilityDefinitions returns the additive human reading
// capabilities. They are readonly, experimental, and disjoint from the legacy
// Agent projection surface; the owner grants reader scopes explicitly.
func ReaderCapabilityDefinitions() []CapabilityDefinition {
	return append(append(append(append(readerRecoveryCapabilities(), readerComposedCapabilities()...), readerRichCapabilities()...), readerNavigationCapabilities()...), []CapabilityDefinition{
		{
			ID: ReaderSourceCapabilityID, Command: ReaderSourceCapabilityID,
			Readonly: true, BodyAllowed: false,
			UIGroup: "human.reading", BodyExposureDefault: "none", WriteGate: "readonly",
			RequestSchema:  "pinax.reader.source.request.v1alpha1",
			ResponseSchema: "pinax.reader.source.v1alpha1",
			Errors:         []string{"insufficient_scope", "reader_disabled", "reader_unavailable"},
			Stability:      StabilityExperimental,
		},
		{
			ID: ReaderNotesCapabilityID, Command: ReaderNotesCapabilityID,
			Readonly: true, BodyAllowed: false,
			UIGroup: "human.reading", BodyExposureDefault: "none", WriteGate: "readonly",
			RequestSchema:  "pinax.reader.notes.request.v1alpha1",
			ResponseSchema: "pinax.reader.note_list.v1alpha1",
			Errors:         []string{"reader_request_invalid", "reader_cursor_invalid", "reader_cursor_expired", "insufficient_scope", "reader_disabled", "index_unavailable"},
			Stability:      StabilityExperimental,
		},
		{
			ID: ReaderSearchCapabilityID, Command: ReaderSearchCapabilityID,
			Readonly: true, BodyAllowed: true,
			UIGroup: "human.reading", BodyExposureDefault: "explicit", WriteGate: "readonly",
			RequestSchema:  "pinax.reader.search.request.v1alpha1",
			ResponseSchema: "pinax.reader.search_results.v1alpha1",
			Errors:         []string{"reader_request_invalid", "reader_cursor_invalid", "reader_cursor_expired", "insufficient_scope", "reader_disabled", "index_unavailable"},
			Stability:      StabilityExperimental,
		},
		{
			ID: ReaderDocumentCapabilityID, Command: ReaderDocumentCapabilityID,
			Readonly: true, BodyAllowed: true,
			UIGroup: "human.reading", BodyExposureDefault: "explicit", WriteGate: "readonly",
			RequestSchema:  "pinax.reader.document.request.v1alpha1",
			ResponseSchema: "pinax.reader.document.v1alpha1",
			Errors: []string{
				"reader_request_invalid", "reader_cursor_invalid", "reader_cursor_expired",
				"insufficient_scope", "reader_disabled", "reader_object_not_found",
				"revision_unavailable", "source_changed", "document_too_large",
				"document_budget_exceeded", "reader_unsupported", "index_unavailable",
			},
			Stability: StabilityExperimental,
		},
		{
			ID: ReaderLinksCapabilityID, Command: ReaderLinksCapabilityID,
			Readonly: true, BodyAllowed: false,
			UIGroup: "human.reading", BodyExposureDefault: "none", WriteGate: "readonly",
			RequestSchema:  "pinax.reader.links.request.v1alpha1",
			ResponseSchema: "pinax.reader.link_page.v1alpha1",
			Errors:         []string{"reader_request_invalid", "reader_cursor_invalid", "reader_cursor_expired", "insufficient_scope", "reader_disabled", "reader_object_not_found", "revision_unavailable", "index_unavailable"},
			Stability:      StabilityExperimental,
		},
		{
			ID: ReaderAssetCapabilityID, Command: ReaderAssetCapabilityID,
			Readonly: true, BodyAllowed: false,
			UIGroup: "human.reading", BodyExposureDefault: "none", WriteGate: "readonly",
			RequestSchema:  "pinax.reader.asset_content.request.v1alpha1",
			ResponseSchema: "pinax.reader.asset_ref.v1alpha1",
			Errors:         []string{"reader_request_invalid", "insufficient_scope", "reader_disabled", "reader_object_not_found", "revision_unavailable", "reader_range_invalid", "reader_unsupported"},
			Stability:      StabilityExperimental,
		},
		{
			ID: ReaderSourceExportCapabilityID, Command: ReaderSourceExportCapabilityID,
			Readonly: true, BodyAllowed: true,
			UIGroup: "human.reading", BodyExposureDefault: "explicit", WriteGate: "readonly",
			RequestSchema:  "pinax.reader.source_export.request.v1alpha1",
			ResponseSchema: "pinax.reader.source_export.v1alpha1",
			Errors:         []string{"reader_request_invalid", "insufficient_scope", "reader_disabled", "reader_object_not_found", "revision_unavailable", "document_too_large", "reader_range_invalid"},
			Stability:      StabilityExperimental,
		},
	}...)
}

// ReaderTransportBindings returns the REST bindings for the human reading
// routes, now backed by real handlers since P02-P06: GET-only, loopback,
// owner-authorized. Each binding names its HTTP handler backing so manifest
// consumers can trace availability to implementation.
func ReaderTransportBindings() []TransportBinding {
	bound := func(id, capabilityID, method, path, requestSchema, responseSchema string) TransportBinding {
		return TransportBinding{
			ID: id, CapabilityID: capabilityID,
			Transport:    TransportREST,
			Availability: AvailabilityAvailable,
			BackingRef:   "internal/api/http:" + id,
			Method:       method, Path: path,
			Readonly:       true,
			WriteGate:      "readonly",
			RequestSchema:  requestSchema,
			ResponseSchema: responseSchema,
			Readiness: &Readiness{
				Status:   ReadinessReady,
				Maturity: MaturityExploratory,
				EvidenceRefs: []string{
					"test:TestReaderAccess", "test:TestReaderDocument",
					"test:TestReaderResources", "test:TestReaderSession", "test:TestReaderClient",
				},
			},
		}
	}
	return append(append(append(append(readerRecoveryBindings(), readerComposedBindings()...), readerRichBindings()...), readerNavigationBindings()...), []TransportBinding{
		bound(RestReaderSourceBindingID, ReaderSourceCapabilityID, "GET", "/v1/reader/source",
			"pinax.reader.source.request.v1alpha1", "pinax.reader.source.v1alpha1"),
		bound(RestReaderNotesBindingID, ReaderNotesCapabilityID, "GET", "/v1/reader/notes",
			"pinax.reader.notes.request.v1alpha1", "pinax.reader.note_list.v1alpha1"),
		bound(RestReaderSearchBindingID, ReaderSearchCapabilityID, "GET", "/v1/reader/search",
			"pinax.reader.search.request.v1alpha1", "pinax.reader.search_results.v1alpha1"),
		bound(RestReaderDocumentBindingID, ReaderDocumentCapabilityID, "GET", "/v1/reader/notes/{object_id}/document",
			"pinax.reader.document.request.v1alpha1", "pinax.reader.document.v1alpha1"),
		bound(RestReaderLinksBindingID, ReaderLinksCapabilityID, "GET", "/v1/reader/notes/{object_id}/links",
			"pinax.reader.links.request.v1alpha1", "pinax.reader.link_page.v1alpha1"),
		bound(RestReaderAssetBindingID, ReaderAssetCapabilityID, "GET", "/v1/reader/assets/{asset_ref}/content",
			"pinax.reader.asset_content.request.v1alpha1", "pinax.reader.asset_ref.v1alpha1"),
		bound(RestReaderSourceExportBindingID, ReaderSourceExportCapabilityID, "GET", "/v1/reader/notes/{object_id}/source",
			"pinax.reader.source_export.request.v1alpha1", "pinax.reader.source_export.v1alpha1"),
	}...)
}

// ReaderDeclaredSurfaces declares the REST surface for reader capabilities.
// Declared surfaces are intent only; available_surfaces stay empty until a
// real adapter makes them callable.
func ReaderDeclaredSurfaces() map[string][]string {
	declared := make(map[string][]string, 7)
	for _, definition := range ReaderCapabilityDefinitions() {
		declared[definition.ID] = []string{"rest"}
	}
	return declared
}

// CompileReaderContract freezes the human reading catalog slice: reader
// capabilities plus their planned REST bindings. Deterministic and
// digest-stable; used by the P01 drift gate and composed into the full
// transport manifest by internal/transportmanifest.
func CompileReaderContract() (Manifest, error) {
	return Compile(ReaderCapabilityDefinitions(), ReaderTransportBindings(), ReaderDeclaredSurfaces())
}
