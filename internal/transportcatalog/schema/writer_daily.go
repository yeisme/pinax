package schema

const (
	WriterDailyProposeRequestV1Alpha1 = "pinax.writer.daily_propose.request.v1alpha1"
	WriterDailyProposalV1Alpha1       = "pinax.writer.daily_proposal.v1alpha1"
	WriterDailyApplyRequestV1Alpha1   = "pinax.writer.daily_apply.request.v1alpha1"
)

func writerDailyDefinitions() []Definition {
	date := map[string]any{"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$"}
	timezone := map[string]any{"type": "string", "minLength": 1, "maxLength": 128}
	operation := map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": "^[A-Za-z0-9][A-Za-z0-9._:-]*$"}
	return []Definition{
		define(WriterDailyProposeRequestV1Alpha1, closedObject(map[string]any{"date": date, "timezone": timezone, "operation_id": operation}, "date", "timezone", "operation_id")),
		define(WriterDailyProposalV1Alpha1, closedObject(map[string]any{
			"schema_version": constString(WriterDailyProposalV1Alpha1), "date": date, "timezone": timezone, "operation_id": operation,
			"object_id": map[string]any{"type": "string", "pattern": "^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"},
			"path":      map[string]any{"type": "string", "minLength": 1, "maxLength": 512}, "title": map[string]any{"type": "string", "minLength": 1, "maxLength": 512},
			"before": constString(""), "after": map[string]any{"type": "string", "maxLength": WriterMaxBodyBytes},
			"template_digest": writerDigestValue(), "preview_digest": writerDigestValue(),
			"created_at": dateTimeValue(), "binding_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 256},
			"frontmatter_protected": map[string]any{"type": "boolean", "const": true},
		}, "schema_version", "date", "timezone", "operation_id", "object_id", "path", "title", "before", "after", "template_digest", "preview_digest", "created_at", "binding_id", "frontmatter_protected")),
		define(WriterDailyApplyRequestV1Alpha1, closedObject(map[string]any{
			"proposal": refValue(WriterDailyProposalV1Alpha1), "authorization": enumValue("explicit_instruction", "reviewed_proposal"),
		}, "proposal", "authorization")),
	}
}
