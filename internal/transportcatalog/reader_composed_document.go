package transportcatalog

import catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"

func readerComposedCapabilities() []CapabilityDefinition {
	return []CapabilityDefinition{{ID: "reader.composed_document", Command: "reader.composed_document", Readonly: true, BodyAllowed: true, UIGroup: "human.reading", BodyExposureDefault: "explicit", WriteGate: "readonly", Stability: StabilityExperimental, RequestSchema: catalogschema.ReaderComposedRequestV1Alpha1, ResponseSchema: catalogschema.ReaderComposedDocumentV1Alpha1, Errors: []string{"insufficient_scope", "reader_disabled", "reader_unavailable", "reader_request_invalid", "reader_cursor_invalid", "reader_cursor_expired", "reader_unsupported", "reader_object_not_found", "revision_unavailable", "source_changed", "document_too_large", "document_budget_exceeded"}}}
}

func readerComposedBindings() []TransportBinding {
	return []TransportBinding{{ID: "rest.reader.composed_document", CapabilityID: "reader.composed_document", Transport: TransportREST, Availability: AvailabilityAvailable, BackingRef: "internal/api/http:rest.reader.composed_document", Method: "GET", Path: "/v1/reader/notes/{object_id}/composed-document", Readonly: true, WriteGate: "readonly", RequestSchema: catalogschema.ReaderComposedRequestV1Alpha1, ResponseSchema: catalogschema.ReaderComposedDocumentV1Alpha1, Readiness: &Readiness{Status: ReadinessReady, Maturity: MaturityExploratory, EvidenceRefs: []string{"test:TestReaderDocumentComposedHTTP"}}}}
}
