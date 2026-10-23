package transportcatalog

import catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"

func writerDailyEditCapabilities() []CapabilityDefinition {
	base := writerDailyCapabilities()[0]
	base.Errors = append(append([]string{}, base.Errors...), "revision_conflict", "note_not_found", "note_identity_ambiguous", "frontmatter_unsupported", "body_invalid", "change_invalid", "write_disabled")
	source := base
	source.ID = "writer.daily.editor_source"
	source.Command = source.ID
	source.RequestSchema = catalogschema.WriterDailyEditorSourceRequestV1Alpha1
	source.ResponseSchema = catalogschema.WriterEditorSourceV1Alpha1
	propose := base
	propose.ID = "writer.daily.edit.propose"
	propose.Command = propose.ID
	propose.RequestSchema = catalogschema.WriterDailyEditProposeRequestV1Alpha1
	propose.ResponseSchema = catalogschema.WriterDailyEditProposalV1Alpha1
	apply := base
	apply.ID = "writer.daily.edit"
	apply.Command = apply.ID
	apply.Readonly = false
	apply.ApprovalRequired = true
	apply.SnapshotRequired = true
	apply.WriteGate = "approval_and_snapshot"
	apply.RequestSchema = catalogschema.WriterDailyEditApplyRequestV1Alpha1
	apply.ResponseSchema = catalogschema.WriterApplyReceiptV1Alpha1
	return []CapabilityDefinition{source, propose, apply}
}

func writerDailyEditBindings() []TransportBinding {
	out := []TransportBinding{}
	for _, c := range writerDailyEditCapabilities() {
		path, method := "/v1/writer/journals/daily/edit:propose", "POST"
		if c.ID == "writer.daily.edit" {
			path = "/v1/writer/journals/daily/edit:apply"
		}
		if c.ID == "writer.daily.editor_source" {
			path = "/v1/writer/journals/daily/editor-source"
			method = "GET"
		}
		id := "rest." + c.ID
		out = append(out, TransportBinding{ID: id, CapabilityID: c.ID, Transport: TransportREST, Availability: AvailabilityAvailable, BackingRef: "internal/api/http:" + id, Method: method, Path: path, Readonly: c.Readonly, WriteGate: c.WriteGate, RequestSchema: c.RequestSchema, ResponseSchema: c.ResponseSchema, Readiness: &Readiness{Status: ReadinessReady, Maturity: MaturityExploratory, EvidenceRefs: []string{"test:TestWriterDailyEditHTTP"}}})
	}
	return out
}
