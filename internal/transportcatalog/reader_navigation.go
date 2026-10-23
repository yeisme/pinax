package transportcatalog

import catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"

const (
	ReaderTagsCapabilityID   = "reader.tags"
	ReaderGraphCapabilityID  = "reader.graph"
	ReaderDailyCapabilityID  = "reader.daily"
	RestReaderTagsBindingID  = "rest.reader.tags"
	RestReaderGraphBindingID = "rest.reader.graph"
	RestReaderDailyBindingID = "rest.reader.daily"
)

func readerNavigationCapabilities() []CapabilityDefinition {
	capability := func(id, request, response string) CapabilityDefinition {
		return CapabilityDefinition{ID: id, Command: id, Readonly: true, UIGroup: "human.reading", BodyExposureDefault: "none", WriteGate: "readonly", RequestSchema: request, ResponseSchema: response, Stability: StabilityExperimental,
			Errors: []string{"insufficient_scope", "reader_disabled", "reader_unavailable", "reader_request_invalid", "reader_cursor_invalid", "reader_cursor_expired", "reader_unsupported", "reader_object_not_found", "revision_unavailable", "source_changed", "index_unavailable"}}
	}
	dailyDocument := capability("reader.daily.document", catalogschema.ReaderDailyDocumentRequestV1Alpha1, catalogschema.ReaderDocumentV1Alpha1)
	dailyDocument.BodyExposureDefault = "explicit"
	dailyDocument.Errors = append(dailyDocument.Errors, "document_too_large", "document_budget_exceeded")
	return []CapabilityDefinition{dailyDocument,
		capability(ReaderTagsCapabilityID, catalogschema.ReaderTagsRequestV1Alpha1, catalogschema.ReaderTagsV1Alpha1),
		capability(ReaderGraphCapabilityID, catalogschema.ReaderGraphRequestV1Alpha1, catalogschema.ReaderGraphV1Alpha1),
		capability(ReaderDailyCapabilityID, catalogschema.ReaderDailyRequestV1Alpha1, catalogschema.ReaderDailyV1Alpha1),
	}
}

func readerNavigationBindings() []TransportBinding {
	bound := func(id, capability, path, request, response string) TransportBinding {
		return TransportBinding{ID: id, CapabilityID: capability, Transport: TransportREST, Availability: AvailabilityAvailable, BackingRef: "internal/api/http:" + id, Method: "GET", Path: path, Readonly: true, WriteGate: "readonly", RequestSchema: request, ResponseSchema: response,
			Readiness: &Readiness{Status: ReadinessReady, Maturity: MaturityExploratory, EvidenceRefs: []string{"test:TestReaderNavigation", "test:TestReaderNavigationHTTP", "test:TestReaderNavigationClient"}}}
	}
	return []TransportBinding{
		bound("rest.reader.daily.document", "reader.daily.document", "/v1/reader/journals/daily/document", catalogschema.ReaderDailyDocumentRequestV1Alpha1, catalogschema.ReaderDocumentV1Alpha1),
		bound(RestReaderTagsBindingID, ReaderTagsCapabilityID, "/v1/reader/tags", catalogschema.ReaderTagsRequestV1Alpha1, catalogschema.ReaderTagsV1Alpha1),
		bound(RestReaderGraphBindingID, ReaderGraphCapabilityID, "/v1/reader/graph", catalogschema.ReaderGraphRequestV1Alpha1, catalogschema.ReaderGraphV1Alpha1),
		bound(RestReaderDailyBindingID, ReaderDailyCapabilityID, "/v1/reader/journals/daily", catalogschema.ReaderDailyRequestV1Alpha1, catalogschema.ReaderDailyV1Alpha1),
	}
}
