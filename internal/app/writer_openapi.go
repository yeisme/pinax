package app

import (
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// Writer REST wire contracts (pinax-human-reading-write-v1 W06). They extend
// the OpenAPI export with the implemented supervised write surface without
// touching the legacy RemoteRoutes facade: old REST/RPC/MCP/URI contracts
// stay byte-identical, and writer operations are additive paths
// guarded by the supervised session registry.

func writerRESTContracts() []RemoteRESTContract {
	contract := func(routeID, capabilityID, path, requestSchema, responseSchema, writeGate string, readonly, approval, snapshot bool) RemoteRESTContract {
		maxBytes := int64(128 * 1024)
		if capabilityID == "writer.tags.lookup" {
			maxBytes = 2048
		}
		return RemoteRESTContract{
			RouteID:             routeID,
			CapabilityID:        capabilityID,
			Command:             capabilityID,
			Method:              "POST",
			Path:                path,
			Parameters:          []HTTPParameterSchema{},
			RequestBody:         &HTTPRequestBodySchema{ContentType: "application/json", SchemaID: requestSchema, Required: true, MaxBytes: maxBytes},
			RequestSchema:       requestSchema,
			SuccessSchema:       catalogschema.ProjectionV1,
			SuccessDataSchema:   responseSchema,
			ErrorSchema:         catalogschema.ProjectionV1,
			ErrorDetailSchema:   catalogschema.ErrorV1,
			BodyExposureDefault: "explicit",
			Readonly:            readonly,
			BodyAllowed:         true,
			ApprovalRequired:    approval,
			SnapshotRequired:    snapshot,
			UIGroup:             "human.writing",
			WriteGate:           writeGate,
			Stability:           "experimental",
		}
	}
	editorSource := contract("rest.writer.editor_source", "writer.editor_source", "/v1/writer/notes/{object_id}/editor-source", catalogschema.WriterEditorSourceRequestV1Alpha1, catalogschema.WriterEditorSourceV1Alpha1, "readonly", true, false, false)
	editorSource.Method = "GET"
	editorSource.RequestBody = nil
	editorSource.Parameters = []HTTPParameterSchema{pathString("object_id"), queryRequiredBoundedString("revision", 1, catalogschema.WriterRevisionMaxLength)}
	dailySource := editorSource
	dailySource.RouteID = "rest.writer.daily.editor_source"
	dailySource.CapabilityID = "writer.daily.editor_source"
	dailySource.Command = dailySource.CapabilityID
	dailySource.Path = "/v1/writer/journals/daily/editor-source"
	dailySource.RequestSchema = catalogschema.WriterDailyEditorSourceRequestV1Alpha1
	dailySource.Parameters = []HTTPParameterSchema{queryRequiredBoundedString("date", 10, 10), queryRequiredBoundedString("timezone", 1, 128), queryRequiredBoundedString("object_id", 1, 256), queryRequiredBoundedString("revision", 71, 71)}
	return []RemoteRESTContract{dailySource,
		contract("rest.writer.tags.plan", "writer.tags.plan", "/v1/writer/tags:plan", catalogschema.WriterTagsPlanRequestV1Alpha1, catalogschema.WriterTagsPlanV1Alpha1, "readonly", true, false, false),
		contract("rest.writer.tags.apply", "writer.tags.apply", "/v1/writer/tags:apply", catalogschema.WriterTagsApplyRequestV1Alpha1, catalogschema.WriterTagsReceiptV1Alpha1, "approval_and_snapshot", false, true, true),
		contract("rest.writer.tags.lookup", "writer.tags.lookup", "/v1/writer/tags:lookup", catalogschema.WriterTagsLookupRequestV1Alpha1, catalogschema.WriterTagsReceiptV1Alpha1, "readonly", true, false, false),
		contract("rest.writer.daily.edit.propose", "writer.daily.edit.propose", "/v1/writer/journals/daily/edit:propose", catalogschema.WriterDailyEditProposeRequestV1Alpha1, catalogschema.WriterDailyEditProposalV1Alpha1, "readonly", true, false, false),
		contract("rest.writer.daily.edit", "writer.daily.edit", "/v1/writer/journals/daily/edit:apply", catalogschema.WriterDailyEditApplyRequestV1Alpha1, catalogschema.WriterApplyReceiptV1Alpha1, "approval_and_snapshot", false, true, true),
		contract("rest.writer.daily.propose", "writer.daily.propose", "/v1/writer/journals/daily:propose", catalogschema.WriterDailyProposeRequestV1Alpha1, catalogschema.WriterDailyProposalV1Alpha1, "readonly", true, false, false),
		contract("rest.writer.daily.apply", "writer.daily.apply", "/v1/writer/journals/daily:apply", catalogschema.WriterDailyApplyRequestV1Alpha1, catalogschema.WriterApplyReceiptV1Alpha1, "approval_and_snapshot", false, true, true),
		editorSource,
		contract("rest.writer.capture", "writer.capture", "/v1/writer/capture",
			catalogschema.WriterCaptureRequestV1Alpha1, catalogschema.WriterCaptureReceiptV1Alpha1, "write", false, true, false),
		contract("rest.writer.propose", "writer.propose", "/v1/writer/proposals",
			catalogschema.WriterProposeRequestV1Alpha1, catalogschema.WriterProposalPreviewV1Alpha1, "readonly", true, false, false),
		contract("rest.writer.apply", "writer.apply", "/v1/writer/proposals:apply",
			catalogschema.WriterApplyRequestV1Alpha1, catalogschema.WriterApplyReceiptV1Alpha1, "approval_and_snapshot", false, true, true),
		contract("rest.writer.organize.plan", "writer.organize.plan", "/v1/writer/organize/plan",
			catalogschema.WriterOrganizePlanRequestV1Alpha1, catalogschema.WriterOrganizePlanV1Alpha1, "readonly", true, false, false),
		contract("rest.writer.organize.apply", "writer.organize.apply", "/v1/writer/organize/apply",
			catalogschema.WriterOrganizeApplyRequestV1Alpha1, catalogschema.WriterOrganizeApplyReceiptV1Alpha1, "approval_and_snapshot", false, true, true),
	}
}
