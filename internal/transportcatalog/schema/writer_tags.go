package schema

const (
	WriterTagsPlanRequestV1Alpha1   = "pinax.writer.tags_plan.request.v1alpha1"
	WriterTagsPlanV1Alpha1          = "pinax.writer.tags_plan.v1alpha1"
	WriterTagsApplyRequestV1Alpha1  = "pinax.writer.tags_apply.request.v1alpha1"
	WriterTagsLookupRequestV1Alpha1 = "pinax.writer.tags_lookup.request.v1alpha1"
	WriterTagsReceiptV1Alpha1       = "pinax.writer.tags_receipt.v1alpha1"
)

func writerTagsDefinitions() []Definition {
	text := func(max int) map[string]any {
		return map[string]any{"type": "string", "minLength": 1, "maxLength": max}
	}
	op := map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": "^[A-Za-z0-9][A-Za-z0-9._:-]*$"}
	tag := text(512)
	tagID := map[string]any{"type": "string", "pattern": "^tag-v1:[a-f0-9]{64}$"}
	tags := map[string]any{"type": "array", "maxItems": 32, "items": tag}
	target := closedObject(map[string]any{"object_id": text(256), "revision": writerDigestValue()}, "object_id", "revision")
	item := closedObject(map[string]any{"object_id": text(256), "revision": writerDigestValue(), "path": text(512), "before": tags, "after": tags}, "object_id", "revision", "path", "before", "after")
	// Canonical tag grammar, UTF-8 byte budgets and ID/path relationships stay
	// in the application service; schema lengths count Unicode code points.
	targets := map[string]any{"type": "array", "minItems": 1, "maxItems": 32, "items": target}
	items := map[string]any{"type": "array", "minItems": 1, "maxItems": 32, "items": item}
	requestBranch := func(kind string) map[string]any {
		next := map[string]any{"const": ""}
		if kind == "rename" {
			next = tag
		}
		return closedObject(map[string]any{"operation_id": op, "operation": constString(kind), "old_tag": tag, "new_tag": next, "targets": targets}, "operation_id", "operation", "old_tag", "new_tag", "targets")
	}
	planBranch := func(kind string) map[string]any {
		next := map[string]any{"const": ""}
		nextID := map[string]any{"const": ""}
		if kind == "rename" {
			next = tag
			nextID = tagID
		}
		return closedObject(map[string]any{"schema_version": constString(WriterTagsPlanV1Alpha1), "operation_id": op, "operation": constString(kind), "old_tag": tag, "old_tag_id": tagID, "new_tag": next, "new_tag_id": nextID, "items": items, "preview_digest": writerDigestValue()}, "schema_version", "operation_id", "operation", "old_tag", "old_tag_id", "new_tag", "new_tag_id", "items", "preview_digest")
	}
	resultItem := closedObject(map[string]any{"object_id": text(256), "operation_id": op, "status": enumValue("not_started", "unknown", "accepted", "applying", "succeeded", "failed", "reconcile_required"), "revision_after": writerDigestValue()}, "object_id", "operation_id", "status")
	result := closedObject(map[string]any{"items": map[string]any{"type": "array", "minItems": 1, "maxItems": 32, "items": resultItem}}, "items")
	receipt := closedObject(map[string]any{
		"schema_version": constString(WriterTagsReceiptV1Alpha1), "operation_id": op, "capability_id": constString("writer.tags.apply"), "binding_id": constString("rest.writer.tags.apply"), "request_digest": writerDigestValue(),
		"status": writerReceiptStatusValue(), "retryable": map[string]any{"const": false}, "replay_safe": map[string]any{"const": false}, "reconcile_required": boolValue(), "required_action": constString("inspect_and_reconcile"),
		"result": result, "receipt": result, "error": closedObject(map[string]any{"code": text(128), "message": text(512)}, "code", "message"),
		"created_at": dateTimeValue(), "updated_at": dateTimeValue(), "accepted_at": dateTimeValue(), "applying_at": dateTimeValue(), "completed_at": dateTimeValue(),
	}, "schema_version", "operation_id", "capability_id", "binding_id", "request_digest", "status", "retryable", "replay_safe", "reconcile_required", "created_at", "updated_at", "accepted_at")
	return []Definition{
		define(WriterTagsPlanRequestV1Alpha1, map[string]any{"oneOf": []any{requestBranch("rename"), requestBranch("delete")}}),
		define(WriterTagsPlanV1Alpha1, map[string]any{"oneOf": []any{planBranch("rename"), planBranch("delete")}}),
		define(WriterTagsApplyRequestV1Alpha1, closedObject(map[string]any{"plan": refValue(WriterTagsPlanV1Alpha1), "authorization": enumValue("explicit_instruction", "reviewed_proposal")}, "plan", "authorization")),
		define(WriterTagsLookupRequestV1Alpha1, closedObject(map[string]any{"operation_id": op}, "operation_id")),
		define(WriterTagsReceiptV1Alpha1, receipt),
	}
}
