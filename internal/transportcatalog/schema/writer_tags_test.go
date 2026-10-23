package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWriterContractTagsClosedSchemas(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	tagID := "tag-v1:" + strings.Repeat("b", 64)
	target := map[string]any{"object_id": "note_one", "revision": digest}
	plan := map[string]any{"schema_version": WriterTagsPlanV1Alpha1, "operation_id": "tag-op", "operation": "rename", "old_tag": "old", "old_tag_id": tagID, "new_tag": "next", "new_tag_id": tagID, "preview_digest": digest, "items": []any{map[string]any{"object_id": "note_one", "revision": digest, "path": "notes/one.md", "before": []any{"old"}, "after": []any{"next"}}}}
	request := map[string]any{"operation_id": "tag-op", "operation": "rename", "old_tag": "old", "new_tag": "next", "targets": []any{target}}
	receipt := map[string]any{"schema_version": WriterTagsReceiptV1Alpha1, "operation_id": "tag-op", "capability_id": "writer.tags.apply", "binding_id": "rest.writer.tags.apply", "request_digest": digest, "status": "accepted", "retryable": false, "replay_safe": false, "reconcile_required": false, "created_at": "2026-10-06T00:00:00Z", "updated_at": "2026-10-06T00:00:00Z", "accepted_at": "2026-10-06T00:00:00Z"}
	clone := func(v any) map[string]any {
		raw, _ := json.Marshal(v)
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	check := func(t *testing.T, id string, v any, valid bool) {
		t.Helper()
		err := ValidatePayload(Default(), id, v)
		if (err == nil) != valid {
			t.Fatalf("%s valid=%v error=%v", id, valid, err)
		}
	}
	check(t, WriterTagsPlanRequestV1Alpha1, request, true)
	check(t, WriterTagsPlanV1Alpha1, plan, true)
	check(t, WriterTagsApplyRequestV1Alpha1, map[string]any{"plan": plan, "authorization": "reviewed_proposal"}, true)
	check(t, WriterTagsLookupRequestV1Alpha1, map[string]any{"operation_id": "tag-op"}, true)
	for _, status := range []string{"accepted", "applying", "succeeded", "failed", "reconcile_required"} {
		v := clone(receipt)
		v["status"] = status
		check(t, WriterTagsReceiptV1Alpha1, v, true)
	}
	deletion := clone(plan)
	deletion["operation"] = "delete"
	deletion["new_tag"] = ""
	deletion["new_tag_id"] = ""
	check(t, WriterTagsPlanV1Alpha1, deletion, true)
	for _, test := range []struct {
		name, id string
		value    map[string]any
		mutate   func(map[string]any)
	}{
		{"authority", WriterTagsPlanRequestV1Alpha1, request, func(v map[string]any) { v["session_id"] = "injected" }},
		{"empty-selection", WriterTagsPlanRequestV1Alpha1, request, func(v map[string]any) { v["targets"] = []any{} }},
		{"target-closed", WriterTagsPlanRequestV1Alpha1, request, func(v map[string]any) { v["targets"].([]any)[0].(map[string]any)["path"] = "untrusted" }},
		{"too-many", WriterTagsPlanRequestV1Alpha1, request, func(v map[string]any) {
			items := []any{}
			for range 33 {
				items = append(items, target)
			}
			v["targets"] = items
		}},
		{"delete-target", WriterTagsPlanRequestV1Alpha1, request, func(v map[string]any) { v["operation"] = "delete" }},
		{"missing-old", WriterTagsPlanRequestV1Alpha1, request, func(v map[string]any) { delete(v, "old_tag") }},
		{"plan-body", WriterTagsPlanV1Alpha1, plan, func(v map[string]any) { v["items"].([]any)[0].(map[string]any)["body"] = "private" }},
		{"null-tags", WriterTagsPlanV1Alpha1, plan, func(v map[string]any) { v["items"].([]any)[0].(map[string]any)["after"] = nil }},
		{"bad-hmac", WriterTagsPlanV1Alpha1, plan, func(v map[string]any) { v["preview_digest"] = "md5:bad" }},
		{"receipt-data", WriterTagsReceiptV1Alpha1, receipt, func(v map[string]any) { v["result"] = map[string]any{"body": "private"} }},
		{"receipt-item", WriterTagsReceiptV1Alpha1, receipt, func(v map[string]any) {
			v["result"] = map[string]any{"items": []any{map[string]any{"object_id": "n", "operation_id": "o", "status": "unknown", "tags": []any{"private"}}}}
		}},
		{"receipt-retry", WriterTagsReceiptV1Alpha1, receipt, func(v map[string]any) { v["retryable"] = true }},
		{"child-as-parent", WriterTagsReceiptV1Alpha1, receipt, func(v map[string]any) { v["capability_id"] = "writer.tags.item" }},
	} {
		t.Run(test.name, func(t *testing.T) { v := clone(test.value); test.mutate(v); check(t, test.id, v, false) })
	}
	check(t, WriterTagsApplyRequestV1Alpha1, map[string]any{"plan": plan, "authorization": "host_approved"}, false)
	check(t, WriterTagsLookupRequestV1Alpha1, map[string]any{"operation_id": "tag-op", "plan": plan}, false)
}
