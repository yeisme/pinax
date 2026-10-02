package app

import (
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// Writer REST wire contracts (pinax-human-reading-write-v1 W06). They extend
// the OpenAPI export with the implemented supervised write surface without
// touching the legacy RemoteRoutes facade: old REST/RPC/MCP/URI contracts
// stay byte-identical, and writer operations are additive POST-only paths
// guarded by the supervised session registry.

func writerRESTContracts() []RemoteRESTContract {
	contract := func(routeID, capabilityID, path, requestSchema, responseSchema, writeGate string, readonly, approval, snapshot bool) RemoteRESTContract {
		return RemoteRESTContract{
			RouteID:             routeID,
			CapabilityID:        capabilityID,
			Command:             capabilityID,
			Method:              "POST",
			Path:                path,
			Parameters:          []HTTPParameterSchema{},
			RequestBody:         &HTTPRequestBodySchema{ContentType: "application/json", SchemaID: requestSchema, Required: true, MaxBytes: 128 * 1024},
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
	return []RemoteRESTContract{
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
