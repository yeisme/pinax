package transportcatalog

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// TestWriterContract freezes the supervised write transport catalog slice
// from pinax-human-reading-write-v1 W01: capability identities, POST-only
// writer routes, request/response schema wiring, gate and error vocabulary,
// scope isolation from the reader bootstrap, and deterministic digest.
// W02-W04 implemented the handlers; since W06 the bindings are backed and
// available, and adapter parity is enforced like the reader surface.
func TestWriterContract(t *testing.T) {
	t.Parallel()

	manifest, err := CompileWriterContract()
	if err != nil {
		t.Fatalf("CompileWriterContract() error = %v", err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("writer manifest invalid: %v", err)
	}

	expectedRoutes := map[string]struct {
		capabilityID   string
		method, path   string
		requestSchema  string
		responseSchema string
	}{
		"rest.writer.tags.plan":           {"writer.tags.plan", "POST", "/v1/writer/tags:plan", catalogschema.WriterTagsPlanRequestV1Alpha1, catalogschema.WriterTagsPlanV1Alpha1},
		"rest.writer.tags.apply":          {"writer.tags.apply", "POST", "/v1/writer/tags:apply", catalogschema.WriterTagsApplyRequestV1Alpha1, catalogschema.WriterTagsReceiptV1Alpha1},
		"rest.writer.tags.lookup":         {"writer.tags.lookup", "POST", "/v1/writer/tags:lookup", catalogschema.WriterTagsLookupRequestV1Alpha1, catalogschema.WriterTagsReceiptV1Alpha1},
		"rest.writer.daily.editor_source": {"writer.daily.editor_source", "GET", "/v1/writer/journals/daily/editor-source", catalogschema.WriterDailyEditorSourceRequestV1Alpha1, catalogschema.WriterEditorSourceV1Alpha1},
		"rest.writer.daily.edit.propose":  {"writer.daily.edit.propose", "POST", "/v1/writer/journals/daily/edit:propose", catalogschema.WriterDailyEditProposeRequestV1Alpha1, catalogschema.WriterDailyEditProposalV1Alpha1},
		"rest.writer.daily.edit":          {"writer.daily.edit", "POST", "/v1/writer/journals/daily/edit:apply", catalogschema.WriterDailyEditApplyRequestV1Alpha1, catalogschema.WriterApplyReceiptV1Alpha1},
		RestWriterDailyProposeBindingID:   {WriterDailyProposeCapabilityID, "POST", "/v1/writer/journals/daily:propose", catalogschema.WriterDailyProposeRequestV1Alpha1, catalogschema.WriterDailyProposalV1Alpha1},
		RestWriterDailyApplyBindingID:     {WriterDailyApplyCapabilityID, "POST", "/v1/writer/journals/daily:apply", catalogschema.WriterDailyApplyRequestV1Alpha1, catalogschema.WriterApplyReceiptV1Alpha1},
		RestWriterEditorSourceBindingID:   {WriterEditorSourceCapabilityID, "GET", "/v1/writer/notes/{object_id}/editor-source", "pinax.writer.editor_source.request.v1alpha1", "pinax.writer.editor_source.v1alpha1"},
		RestWriterCaptureBindingID:        {WriterCaptureCapabilityID, "POST", "/v1/writer/capture", "pinax.writer.capture.request.v1alpha1", "pinax.writer.capture_receipt.v1alpha1"},
		RestWriterProposeBindingID:        {WriterProposeCapabilityID, "POST", "/v1/writer/proposals", "pinax.writer.propose.request.v1alpha1", "pinax.writer.proposal_preview.v1alpha1"},
		RestWriterApplyBindingID:          {WriterApplyCapabilityID, "POST", "/v1/writer/proposals:apply", "pinax.writer.apply.request.v1alpha1", "pinax.writer.apply_receipt.v1alpha1"},
		RestWriterOrganizePlanBindingID:   {WriterOrganizePlanCapabilityID, "POST", "/v1/writer/organize/plan", "pinax.writer.organize_plan.request.v1alpha1", "pinax.writer.organize_plan.v1alpha1"},
		RestWriterOrganizeApplyBindingID:  {WriterOrganizeApplyCapabilityID, "POST", "/v1/writer/organize/apply", "pinax.writer.organize_apply.request.v1alpha1", "pinax.writer.organize_apply_receipt.v1alpha1"},
	}

	if len(manifest.Capabilities) != len(expectedRoutes) {
		t.Fatalf("writer capabilities = %d, want %d", len(manifest.Capabilities), len(expectedRoutes))
	}

	registry := catalogschema.Default()
	seenRoutes := make(map[string]bool, len(expectedRoutes))
	for _, capability := range manifest.Capabilities {
		if capability.Stability != StabilityExperimental {
			t.Errorf("capability %q stability = %q, want experimental", capability.ID, capability.Stability)
		}
		if !reflect.DeepEqual(capability.DeclaredSurfaces, []string{"rest"}) {
			t.Errorf("capability %q declared surfaces = %#v, want [rest]", capability.ID, capability.DeclaredSurfaces)
		}
		// W06: the writer bindings are backed by real handlers with
		// component evidence, so the manifest must advertise REST availability
		// exactly and truthfully.
		if !reflect.DeepEqual(capability.AvailableSurfaces, []string{"rest"}) {
			t.Fatalf("capability %q available surfaces = %#v, want [rest]", capability.ID, capability.AvailableSurfaces)
		}
		if len(capability.Bindings) != 1 {
			t.Fatalf("capability %q bindings = %d, want 1", capability.ID, len(capability.Bindings))
		}
		binding := capability.Bindings[0]
		expected, ok := expectedRoutes[binding.ID]
		if !ok {
			t.Fatalf("unexpected writer binding %q", binding.ID)
		}
		seenRoutes[binding.ID] = true
		if binding.CapabilityID != expected.capabilityID || binding.Method != expected.method || binding.Path != expected.path {
			t.Errorf("binding %q identity drift: got %s %s %s", binding.ID, binding.CapabilityID, binding.Method, binding.Path)
		}
		if binding.RequestSchema != expected.requestSchema || binding.ResponseSchema != expected.responseSchema {
			t.Errorf("binding %q schema drift: %s -> %s", binding.ID, binding.RequestSchema, binding.ResponseSchema)
		}
		if binding.Transport != TransportREST {
			t.Errorf("binding %q transport = %q, want rest", binding.ID, binding.Transport)
		}
		if binding.Availability != AvailabilityAvailable {
			t.Fatalf("binding %q availability = %q, want available since W06 backing", binding.ID, binding.Availability)
		}
		if binding.BackingRef != "internal/api/http:"+binding.ID {
			t.Errorf("binding %q backing = %q", binding.ID, binding.BackingRef)
		}
		if binding.Readiness == nil || binding.Readiness.Status != ReadinessReady {
			t.Errorf("binding %q readiness = %#v", binding.ID, binding.Readiness)
		}
		if !registry.Has(binding.RequestSchema) || !registry.Has(binding.ResponseSchema) {
			t.Errorf("binding %q references unregistered schema", binding.ID)
		}
		if !containsString(capability.Errors, "insufficient_scope") || !containsString(capability.Errors, "writer_disabled") {
			t.Errorf("capability %q must declare insufficient_scope and writer_disabled", capability.ID)
		}
	}
	for bindingID := range expectedRoutes {
		if !seenRoutes[bindingID] {
			t.Errorf("expected binding %q missing from writer manifest", bindingID)
		}
	}

	// Gate freeze: approval/snapshot semantics per capability. Authorization
	// is a caller assertion on apply; the snapshot gate is frozen on every
	// mutating capability.
	gates := map[string]struct {
		readonly, approval, snapshot bool
		writeGate                    string
	}{
		"writer.tags.plan":              {true, false, false, "readonly"},
		"writer.tags.lookup":            {true, false, false, "readonly"},
		"writer.tags.apply":             {false, true, true, "approval_and_snapshot"},
		"writer.daily.editor_source":    {true, false, false, "readonly"},
		"writer.daily.edit.propose":     {true, false, false, "readonly"},
		"writer.daily.edit":             {false, true, true, "approval_and_snapshot"},
		WriterDailyProposeCapabilityID:  {true, false, false, "readonly"},
		WriterDailyApplyCapabilityID:    {false, true, true, "approval_and_snapshot"},
		WriterEditorSourceCapabilityID:  {true, false, false, "readonly"},
		WriterCaptureCapabilityID:       {false, true, false, "write"},
		WriterProposeCapabilityID:       {true, false, false, "readonly"},
		WriterApplyCapabilityID:         {false, true, true, "approval_and_snapshot"},
		WriterOrganizePlanCapabilityID:  {true, false, false, "readonly"},
		WriterOrganizeApplyCapabilityID: {false, true, true, "approval_and_snapshot"},
	}
	for _, capability := range manifest.Capabilities {
		want := gates[capability.ID]
		if capability.Readonly != want.readonly || capability.ApprovalRequired != want.approval || capability.SnapshotRequired != want.snapshot || capability.WriteGate != want.writeGate {
			t.Errorf("capability %q gate drift: readonly=%v approval=%v snapshot=%v gate=%q", capability.ID, capability.Readonly, capability.ApprovalRequired, capability.SnapshotRequired, capability.WriteGate)
		}
	}

	// Error vocabulary freeze per the design audit: every code below must
	// stay declared so W02-W04 projections cannot silently drop a gate.
	errorFreeze := map[string][]string{
		WriterCaptureCapabilityID:       {"token_required", "invalid_token", "loopback_required", "source_changed", "operation_identity_required", "idempotency_conflict", "approval_required", "write_disabled", "operation_store_unavailable", "reconcile_required", "title_invalid", "body_invalid", "note_path_conflict", "writer_unavailable"},
		WriterProposeCapabilityID:       {"token_required", "invalid_token", "loopback_required", "source_changed", "change_invalid", "revision_conflict", "action_unsupported", "body_too_large", "unsafe_path", "note_not_found", "writer_unavailable"},
		WriterApplyCapabilityID:         {"token_required", "invalid_token", "loopback_required", "source_changed", "preview_mismatch", "authorization_required", "revision_conflict", "outcome_unknown", "body_invalid", "note_not_found", "writer_unavailable"},
		WriterOrganizePlanCapabilityID:  {"token_required", "invalid_token", "loopback_required", "source_changed", "vault_not_initialized", "plan_save_failed", "organize_selection_invalid", "organize_identity_required", "organize_plan_schema_invalid", "writer_unavailable"},
		WriterOrganizeApplyCapabilityID: {"token_required", "invalid_token", "loopback_required", "source_changed", "snapshot_required", "plan_save_failed", "plan_not_found", "plan_stale", "approval_required", "organize_selection_invalid", "organize_identity_required", "organize_plan_schema_invalid", "organize_plan_not_planned", "scoped_plan_override_forbidden", "organize_operation_failed", "writer_unavailable"},
	}
	for _, capability := range manifest.Capabilities {
		for _, code := range errorFreeze[capability.ID] {
			if !containsString(capability.Errors, code) {
				t.Errorf("capability %q is missing frozen error %q", capability.ID, code)
			}
		}
	}

	// Budget freeze: the change schema must encode the frozen numbers so the
	// human projection never widens the collaboration budgets.
	change, ok := registry.Lookup(catalogschema.WriterChangeV1Alpha1)
	if !ok {
		t.Fatalf("schema %q is not registered", catalogschema.WriterChangeV1Alpha1)
	}
	properties, _ := change["properties"].(map[string]any)
	budgets := map[string]struct {
		field string
		want  int64
	}{
		"body max bytes":  {"body", 65536},
		"title max bytes": {"title", 512},
	}
	for name, budget := range budgets {
		fieldSchema, _ := properties[budget.field].(map[string]any)
		got, ok := asConstMaxLength(fieldSchema)
		if !ok || got != budget.want {
			t.Errorf("budget %q = %v, want %v", name, fieldSchema["maxLength"], budget.want)
		}
	}
	tags, _ := properties["tags"].(map[string]any)
	if maxItems, _ := tags["maxItems"].(int); maxItems != 32 {
		t.Errorf("tags maxItems = %v, want 32", tags["maxItems"])
	}

	// Scope freeze (W02): the supervised session allowlist admits the
	// writer scopes alongside the reader scopes, and no legacy scope.
	bootstrap, _ := registry.Lookup(catalogschema.LocalSessionBootstrapV1Alpha1)
	bootstrapProps, _ := bootstrap["properties"].(map[string]any)
	scopes, _ := bootstrapProps["scopes"].(map[string]any)
	items, _ := scopes["items"].(map[string]any)
	admitted := make(map[string]bool, 7)
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
	for _, scope := range []string{catalogschema.WriterScopeCapture, catalogschema.WriterScopeEditor, catalogschema.WriterScopeOrganizer, catalogschema.ReaderScopeBody, catalogschema.ReaderScopeAssets} {
		if !admitted[scope] {
			t.Errorf("session allowlist must admit scope %q", scope)
		}
	}
	for _, scope := range []string{"read", "write", "admin"} {
		if admitted[scope] {
			t.Errorf("session allowlist must never admit legacy scope %q", scope)
		}
	}

	// Deterministic digest: recompile must be byte-stable.
	recompiled, err := CompileWriterContract()
	if err != nil {
		t.Fatal(err)
	}
	first, _ := json.Marshal(manifest)
	second, _ := json.Marshal(recompiled)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("writer manifest is not byte-stable:\n%s\n%s", first, second)
	}

	// Guard: a writer binding cannot become available without a backing_ref.
	available := WriterTransportBindings()[0]
	available.Availability = AvailabilityAvailable
	available.BackingRef = ""
	if _, err := Compile(WriterCapabilityDefinitions(), []TransportBinding{available}, nil); err == nil || !strings.Contains(err.Error(), "backing_ref") {
		t.Fatalf("available writer binding without backing must fail, got %v", err)
	}

	// Guard: adapter parity — every available writer binding must match a
	// registered HTTP handler operation and vice versa.
	adapters := make([]AdapterOperation, 0)
	for _, binding := range WriterTransportBindings() {
		adapters = append(adapters, AdapterOperation{
			BindingID: binding.ID, CapabilityID: binding.CapabilityID,
			Transport: binding.Transport, BackingRef: binding.BackingRef,
			Method: binding.Method, Path: binding.Path,
		})
	}
	if err := ValidateConformance(manifest, adapters, TransportREST); err != nil {
		t.Fatalf("writer REST conformance failed: %v", err)
	}

	// Guard: schemas must not leak absolute paths or audience-style authz.
	requestSchemas := []string{
		catalogschema.WriterCaptureRequestV1Alpha1,
		catalogschema.WriterProposeRequestV1Alpha1,
		catalogschema.WriterApplyRequestV1Alpha1,
		catalogschema.WriterOrganizeApplyRequestV1Alpha1,
	}
	for _, id := range requestSchemas {
		document, _ := registry.Lookup(id)
		encoded, _ := json.Marshal(document)
		if strings.Contains(string(encoded), "audience") {
			t.Errorf("request schema %q must not accept an audience field", id)
		}
	}
}

// asConstMaxLength reads schema maxLength limits written as Go int literals.
func asConstMaxLength(value map[string]any) (int64, bool) {
	switch typed := value["maxLength"].(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	}
	return 0, false
}
