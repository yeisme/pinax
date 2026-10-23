package transportcatalog

import catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"

func readerRecoveryCapabilities() []CapabilityDefinition {
	return []CapabilityDefinition{{ID: "reader.recovery.lookup", Command: "reader.recovery.lookup", Readonly: true, BodyAllowed: false, UIGroup: "human.reading", BodyExposureDefault: "none", WriteGate: "readonly", Stability: StabilityExperimental, RequestSchema: catalogschema.ReaderRecoveryLookupRequestV1Alpha1, ResponseSchema: catalogschema.OperationV1, Errors: []string{"insufficient_scope", "reader_disabled", "reader_request_invalid", "reader_unavailable", "recovery_lookup_unavailable"}}}
}
func readerRecoveryBindings() []TransportBinding {
	return []TransportBinding{{ID: "rest.reader.recovery.lookup", CapabilityID: "reader.recovery.lookup", Transport: TransportREST, Availability: AvailabilityAvailable, BackingRef: "internal/api/http:rest.reader.recovery.lookup", Method: "GET", Path: "/v1/reader/recovery/operation", Readonly: true, WriteGate: "readonly", RequestSchema: catalogschema.ReaderRecoveryLookupRequestV1Alpha1, ResponseSchema: catalogschema.OperationV1, Readiness: &Readiness{Status: ReadinessReady, Maturity: MaturityExploratory, EvidenceRefs: []string{"test:TestReaderSessionRecoveryHTTP"}}}}
}
