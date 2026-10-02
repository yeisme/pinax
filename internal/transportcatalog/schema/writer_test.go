package schema

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestWriterContract freezes the supervised write machine schemas from
// pinax-human-reading-write-v1 W01. Positive fixtures mirror the frozen
// proposal/apply/capture/organize shapes; negative fixtures prove closed
// objects, required fields, enums, digests, lengths and frozen budget
// consts reject drift — including that no authorization value can claim
// verified host approval. Pure contract test: no vault, no model, no sync.
func TestWriterContract(t *testing.T) {
	t.Parallel()

	registry := Default()
	for _, id := range []string{
		WriterChangeV1Alpha1,
		WriterProposalPreviewV1Alpha1,
		WriterApplyReceiptV1Alpha1,
		WriterCaptureReceiptV1Alpha1,
		WriterOrganizePlanV1Alpha1,
		WriterOrganizeApplyReceiptV1Alpha1,
		WriterCaptureRequestV1Alpha1,
		WriterProposeRequestV1Alpha1,
		WriterApplyRequestV1Alpha1,
		WriterOrganizePlanRequestV1Alpha1,
		WriterOrganizeApplyRequestV1Alpha1,
	} {
		if !registry.Has(id) {
			t.Fatalf("writer schema %q is not registered in the default registry", id)
		}
	}

	changeCreate := `{"action": "create", "operation_id": "op-1", "title": "捕获", "body": "正文", "tags": ["idea"], "status": "inbox"}`
	positive := []struct {
		schemaID string
		document string
	}{
		{WriterChangeV1Alpha1, changeCreate},
		{WriterChangeV1Alpha1, `{"action": "append", "note_ref": "notes/foo.md", "expected_revision": "sha256:aa", "body": "追加"}`},
		{WriterChangeV1Alpha1, `{"action": "tags", "note_ref": "notes/foo.md", "tags": ["a", "b"], "tag_operation": "add"}`},
		{WriterChangeV1Alpha1, `{"action": "archive", "note_ref": "notes/foo.md", "expected_revision": "sha256:bb"}`},
		{WriterProposalPreviewV1Alpha1, `{
			"schema_version": "pinax.writer.proposal_preview.v1alpha1",
			"change": {"action": "append", "note_ref": "notes/foo.md", "body": "追加"},
			"preview_digest": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			"path": "notes/foo.md", "before": "旧正文", "after": "旧正文\n追加", "state": "prepared"
		}`},
		{WriterApplyReceiptV1Alpha1, `{
			"schema_version": "pinax.writer.apply_receipt.v1alpha1",
			"operation_id": "op-1", "capability_id": "writer.apply", "binding_id": "rest.writer.apply",
			"status": "succeeded", "retryable": false, "replay_safe": true, "reconcile_required": false,
			"result": {"path": "notes/foo.md"}, "receipt": {"snapshot": "snapshot:1"},
			"resource_ref": "pinax://note/notes%2Ffoo.md",
			"revision_before": "sha256:aa", "revision_after": "sha256:cc",
			"created_at": "2026-10-02T12:00:00Z", "updated_at": "2026-10-02T12:00:01Z",
			"accepted_at": "2026-10-02T12:00:00Z", "completed_at": "2026-10-02T12:00:01Z"
		}`},
		{WriterApplyReceiptV1Alpha1, `{
			"schema_version": "pinax.writer.apply_receipt.v1alpha1",
			"operation_id": "op-2", "status": "reconcile_required",
			"retryable": false, "replay_safe": false, "reconcile_required": true,
			"required_action": "pinax operation show <operation-id>",
			"error": {"code": "outcome_unknown", "message": "Write outcome is unknown"},
			"created_at": "2026-10-02T12:00:00Z", "updated_at": "2026-10-02T12:00:01Z",
			"accepted_at": "2026-10-02T12:00:00Z"
		}`},
		{WriterCaptureReceiptV1Alpha1, `{
			"schema_version": "pinax.writer.capture_receipt.v1alpha1",
			"operation_id": "op-3", "capability_id": "writer.capture", "binding_id": "rest.writer.capture",
			"status": "succeeded", "retryable": false, "replay_safe": true, "reconcile_required": false,
			"note_id": "note-1", "path": "inbox/capture.md", "record_event_id": "re-1",
			"revision_after": "sha256:dd",
			"created_at": "2026-10-02T12:00:00Z", "updated_at": "2026-10-02T12:00:01Z",
			"accepted_at": "2026-10-02T12:00:00Z"
		}`},
		{WriterOrganizePlanV1Alpha1, `{
			"schema_version": "pinax.writer.organize_plan.v1alpha1",
			"plan_id": "plan-1", "status": "planned",
			"created_at": "2026-10-02T12:00:00Z", "expires_at": "2026-10-02T18:00:00Z",
			"operation_count": 1,
			"operations": [{"operation_id": "organize-1", "object_id": "note-1", "kind": "move", "mode": "managed", "risk": "low", "path": "projects/a/note.md"}]
		}`},
		{WriterOrganizeApplyReceiptV1Alpha1, `{
			"schema_version": "pinax.writer.organize_apply_receipt.v1alpha1",
			"operation_id": "op-4", "plan_id": "plan-1",
			"capability_id": "writer.organize.apply", "binding_id": "rest.writer.organize.apply",
			"status": "succeeded", "retryable": false, "replay_safe": true, "reconcile_required": false,
			"snapshot_id": "snap-1",
			"created_at": "2026-10-02T12:00:00Z", "updated_at": "2026-10-02T12:00:01Z",
			"accepted_at": "2026-10-02T12:00:00Z"
		}`},
		{WriterCaptureRequestV1Alpha1, `{"title": "阅读摘录", "body": "一段摘录", "tags": ["reading"], "slug": "excerpt", "dir": "inbox"}`},
		{WriterCaptureRequestV1Alpha1, `{"title": "极简捕获"}`},
		{WriterProposeRequestV1Alpha1, `{"change": {"action": "create", "title": "新笔记"}}`},
		{WriterApplyRequestV1Alpha1, fmt.Sprintf(`{"change": {"action": "append", "note_ref": "notes/foo.md", "body": "追加"}, "preview_digest": "sha256:%s", "authorization": "reviewed_proposal"}`, strings.Repeat("ab", 32))},
		{WriterApplyRequestV1Alpha1, fmt.Sprintf(`{"change": {"action": "create", "title": "新笔记"}, "preview_digest": "sha256:%s", "authorization": "explicit_instruction"}`, strings.Repeat("cd", 32))},
		{WriterOrganizePlanRequestV1Alpha1, `{}`},
		{WriterOrganizePlanRequestV1Alpha1, `{"note_refs": ["notes/a.md", "notes/b.md"]}`},
		{WriterOrganizeApplyRequestV1Alpha1, `{"plan_id": "plan-1", "snapshot_message": "整理前快照", "allow_stale": false}`},
		{WriterOrganizeApplyRequestV1Alpha1, `{"plan_id": "plan-1"}`},
	}
	for _, fixture := range positive {
		if err := validateWriterFixture(registry, fixture.schemaID, fixture.document); err != nil {
			t.Errorf("positive fixture for %s failed: %v", fixture.schemaID, err)
		}
	}

	negative := []struct {
		name     string
		schemaID string
		document string
	}{
		{"change missing action", WriterChangeV1Alpha1, `{"title": "t"}`},
		{"change unknown action", WriterChangeV1Alpha1, `{"action": "delete", "note_ref": "n"}`},
		{"change closed object", WriterChangeV1Alpha1, `{"action": "create", "title": "t", "audience": "human"}`},
		{"change bad tag operation", WriterChangeV1Alpha1, `{"action": "tags", "note_ref": "n", "tags": ["a"], "tag_operation": "clear"}`},
		{"change bad status", WriterChangeV1Alpha1, `{"action": "create", "title": "t", "status": "archived"}`},
		{"preview missing digest", WriterProposalPreviewV1Alpha1, `{"schema_version": "pinax.writer.proposal_preview.v1alpha1", "change": {"action": "create", "title": "t"}, "path": "p", "before": "", "after": "b", "state": "prepared"}`},
		{"preview bad digest", WriterProposalPreviewV1Alpha1, `{"schema_version": "pinax.writer.proposal_preview.v1alpha1", "change": {"action": "create", "title": "t"}, "preview_digest": "md5:zz", "path": "p", "before": "", "after": "b", "state": "prepared"}`},
		{"preview closed object", WriterProposalPreviewV1Alpha1, `{"schema_version": "pinax.writer.proposal_preview.v1alpha1", "change": {"action": "create", "title": "t"}, "preview_digest": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "path": "p", "before": "", "after": "b", "state": "prepared", "approved": true}`},
		{"apply missing authorization", WriterApplyRequestV1Alpha1, fmt.Sprintf(`{"change": {"action": "create", "title": "t"}, "preview_digest": "sha256:%s"}`, strings.Repeat("ab", 32))},
		{"apply authorization cannot claim host approval", WriterApplyRequestV1Alpha1, fmt.Sprintf(`{"change": {"action": "create", "title": "t"}, "preview_digest": "sha256:%s", "authorization": "host_approved"}`, strings.Repeat("ab", 32))},
		{"apply missing preview digest", WriterApplyRequestV1Alpha1, `{"change": {"action": "create", "title": "t"}, "authorization": "explicit_instruction"}`},
		{"apply closed object", WriterApplyRequestV1Alpha1, fmt.Sprintf(`{"change": {"action": "create", "title": "t"}, "preview_digest": "sha256:%s", "authorization": "explicit_instruction", "yes": true}`, strings.Repeat("ab", 32))},
		{"receipt bad status", WriterApplyReceiptV1Alpha1, `{"schema_version": "pinax.writer.apply_receipt.v1alpha1", "operation_id": "op-1", "status": "pending", "retryable": false, "replay_safe": true, "reconcile_required": false, "created_at": "2026-10-02T12:00:00Z", "updated_at": "2026-10-02T12:00:00Z", "accepted_at": "2026-10-02T12:00:00Z"}`},
		{"receipt closed object", WriterApplyReceiptV1Alpha1, `{"schema_version": "pinax.writer.apply_receipt.v1alpha1", "operation_id": "op-1", "status": "succeeded", "retryable": false, "replay_safe": true, "reconcile_required": false, "created_at": "2026-10-02T12:00:00Z", "updated_at": "2026-10-02T12:00:00Z", "accepted_at": "2026-10-02T12:00:00Z", "vault_root": "/v"}`},
		{"capture receipt missing operation identity", WriterCaptureReceiptV1Alpha1, `{"schema_version": "pinax.writer.capture_receipt.v1alpha1", "status": "succeeded", "retryable": false, "replay_safe": true, "reconcile_required": false, "created_at": "2026-10-02T12:00:00Z", "updated_at": "2026-10-02T12:00:00Z", "accepted_at": "2026-10-02T12:00:00Z"}`},
		{"capture request missing title", WriterCaptureRequestV1Alpha1, `{"body": "b"}`},
		{"capture request empty title", WriterCaptureRequestV1Alpha1, `{"title": ""}`},
		{"capture request closed object", WriterCaptureRequestV1Alpha1, `{"title": "t", "yes": true}`},
		{"plan missing plan id", WriterOrganizeApplyRequestV1Alpha1, `{"snapshot_message": "m"}`},
		{"plan bad note refs", WriterOrganizePlanRequestV1Alpha1, `{"note_refs": []}`},
		{"plan operations missing kind", WriterOrganizePlanV1Alpha1, `{"schema_version": "pinax.writer.organize_plan.v1alpha1", "plan_id": "plan-1", "status": "planned", "created_at": "2026-10-02T12:00:00Z", "expires_at": "2026-10-02T18:00:00Z", "operation_count": 1, "operations": [{"operation_id": "organize-1"}]}`},
	}
	for _, fixture := range negative {
		if err := validateWriterFixture(registry, fixture.schemaID, fixture.document); err == nil {
			t.Errorf("negative fixture %q for %s unexpectedly passed", fixture.name, fixture.schemaID)
		}
	}

	// Frozen budgets: body 64 KiB, tags 32 items. Constructed documents keep
	// the fixture table small while still pinning the numbers.
	oversizedBody, _ := json.Marshal(map[string]any{"action": "create", "title": "t", "body": strings.Repeat("字", WriterMaxBodyBytes+1)})
	if err := validateWriterFixture(registry, WriterChangeV1Alpha1, string(oversizedBody)); err == nil {
		t.Error("change body over 64 KiB unexpectedly passed")
	}
	tooManyTags, _ := json.Marshal(map[string]any{"action": "create", "title": "t", "tags": strings.Split(strings.Repeat("a,", WriterTagsMaxItems+1), ",")[:WriterTagsMaxItems+1]})
	if err := validateWriterFixture(registry, WriterChangeV1Alpha1, string(tooManyTags)); err == nil {
		t.Error("change tags over 32 items unexpectedly passed")
	}

	// Scope vocabulary freeze (W02): the supervised session allowlist
	// admits exactly the reader and writer scopes; writer scopes require
	// readonly=false (correlation enforced by the bootstrap reader in
	// code), and legacy read/write/admin scopes stay outside the enum.
	bootstrap, _ := registry.Lookup(LocalSessionBootstrapV1Alpha1)
	bootstrapProps, _ := bootstrap["properties"].(map[string]any)
	scopes, _ := bootstrapProps["scopes"].(map[string]any)
	items, _ := scopes["items"].(map[string]any)
	admitted := map[string]bool{}
	switch enum := items["enum"].(type) {
	case []string:
		for _, scope := range enum {
			admitted[scope] = true
		}
	case []any:
		for _, value := range enum {
			if text, ok := value.(string); ok {
				admitted[text] = true
			}
		}
	}
	for _, scope := range []string{WriterScopeCapture, WriterScopeEditor, WriterScopeOrganizer, ReaderScopeBody} {
		if !admitted[scope] {
			t.Errorf("session allowlist must admit scope %q", scope)
		}
	}
	for _, scope := range []string{"read", "write", "admin", "reader.write"} {
		if admitted[scope] {
			t.Errorf("session allowlist must never admit legacy scope %q", scope)
		}
	}
	readonly, _ := bootstrapProps["readonly"].(map[string]any)
	if _, hasConst := readonly["const"]; hasConst {
		t.Error("session readonly must be a plain boolean so writer frames stay representable")
	}
}

func validateWriterFixture(registry *Registry, schemaID, document string) error {
	var value any
	if err := json.Unmarshal([]byte(document), &value); err != nil {
		return fmt.Errorf("fixture is not JSON: %w", err)
	}
	return ValidatePayload(registry, schemaID, value)
}
