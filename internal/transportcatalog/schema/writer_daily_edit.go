package schema

const (
	WriterDailyEditorSourceRequestV1Alpha1 = "pinax.writer.daily_editor_source.request.v1alpha1"
	WriterDailyEditProposeRequestV1Alpha1  = "pinax.writer.daily_edit_propose.request.v1alpha1"
	WriterDailyEditProposalV1Alpha1        = "pinax.writer.daily_edit_proposal.v1alpha1"
	WriterDailyEditApplyRequestV1Alpha1    = "pinax.writer.daily_edit_apply.request.v1alpha1"
)

func writerDailyEditDefinitions() []Definition {
	text := func(max int) map[string]any {
		return map[string]any{"type": "string", "minLength": 1, "maxLength": max}
	}
	date := map[string]any{"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$"}
	operation := map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": "^[A-Za-z0-9][A-Za-z0-9._:-]*$"}
	body := map[string]any{"type": "string", "maxLength": WriterMaxBodyBytes}
	change := closedObject(map[string]any{"operation_id": operation, "action": constString("replace"), "note_ref": text(512), "expected_revision": writerDigestValue(), "body": body}, "operation_id", "action", "note_ref", "expected_revision")
	preview := closedObject(map[string]any{"change": change, "preview_digest": writerDigestValue(), "path": text(512), "before": body, "after": body, "state": constString("preview")}, "change", "preview_digest", "path", "before", "after", "state")
	return []Definition{
		define(WriterDailyEditorSourceRequestV1Alpha1, closedObject(map[string]any{"date": date, "timezone": text(128), "object_id": text(256), "revision": writerDigestValue()}, "date", "timezone", "object_id", "revision")),
		define(WriterDailyEditProposeRequestV1Alpha1, closedObject(map[string]any{"date": date, "timezone": text(128), "object_id": text(256), "revision": writerDigestValue(), "operation_id": operation, "body": body}, "date", "timezone", "object_id", "revision", "operation_id", "body")),
		define(WriterDailyEditProposalV1Alpha1, closedObject(map[string]any{"schema_version": constString(WriterDailyEditProposalV1Alpha1), "date": date, "timezone": text(128), "object_id": text(256), "preview": preview, "preview_digest": writerDigestValue()}, "schema_version", "date", "timezone", "object_id", "preview", "preview_digest")),
		define(WriterDailyEditApplyRequestV1Alpha1, closedObject(map[string]any{"proposal": refValue(WriterDailyEditProposalV1Alpha1), "authorization": enumValue("explicit_instruction", "reviewed_proposal")}, "proposal", "authorization")),
	}
}
