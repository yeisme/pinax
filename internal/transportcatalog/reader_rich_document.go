package transportcatalog

import catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"

func readerRichCapabilities() []CapabilityDefinition {
	return []CapabilityDefinition{{ID: "reader.rich_document", Command: "reader.rich_document", Readonly: true, BodyAllowed: true, UIGroup: "human.reading", BodyExposureDefault: "explicit", WriteGate: "readonly", Stability: StabilityExperimental, RequestSchema: catalogschema.ReaderRichDocumentRequestV1Alpha1, ResponseSchema: catalogschema.ReaderRichDocumentV1Alpha1, Errors: []string{"insufficient_scope", "reader_disabled", "reader_unavailable", "reader_request_invalid", "reader_cursor_invalid", "reader_cursor_expired", "reader_unsupported", "reader_object_not_found", "revision_unavailable", "source_changed", "document_too_large", "document_budget_exceeded"}}}
}

func readerRichBindings() []TransportBinding {
	return []TransportBinding{{ID: "rest.reader.rich_document", CapabilityID: "reader.rich_document", Transport: TransportREST, Availability: AvailabilityAvailable, BackingRef: "internal/api/http:rest.reader.rich_document", Method: "GET", Path: "/v1/reader/notes/{object_id}/rich-document", Readonly: true, WriteGate: "readonly", RequestSchema: catalogschema.ReaderRichDocumentRequestV1Alpha1, ResponseSchema: catalogschema.ReaderRichDocumentV1Alpha1, Readiness: &Readiness{Status: ReadinessReady, Maturity: MaturityExploratory, EvidenceRefs: []string{"test:TestReaderDocumentRichHTTP"}}}}
}
