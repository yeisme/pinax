package app

import (
	"encoding/json"
	"testing"

	"github.com/yeisme/pinax/internal/operation"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func TestWriterTagsPlanPublicSchemaConformance(t *testing.T) {
	svc, req := tagPlanFixture(t)
	plan, err := svc.WriterTagsPlan(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	check := func(id string, value any) map[string]any {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err = json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		if err = catalogschema.ValidatePayload(catalogschema.Default(), id, payload); err != nil {
			t.Fatal(err)
		}
		return payload
	}
	check(catalogschema.WriterTagsPlanV1Alpha1, plan)
	view, err := svc.WriterTagsApply(t.Context(), req.Writer, plan, "reviewed_proposal")
	if err != nil {
		t.Fatal(err)
	}
	receipt := func(view operation.View) {
		t.Helper()
		raw, _ := json.Marshal(view)
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		payload["schema_version"] = catalogschema.WriterTagsReceiptV1Alpha1
		check(catalogschema.WriterTagsReceiptV1Alpha1, payload)
	}
	receipt(view)
	// Early durable states may legitimately lack item outcomes. Do not fill
	// them with inferred zero writes or fabricate terminal receipts.
	store, err := operation.Open(req.Writer.VaultPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	other := req.Writer
	other.OperationID = "early-tag-op"
	created, err := store.CreateAccepted(t.Context(), writerTagsOperation(other, plan.Digest))
	if err != nil {
		t.Fatal(err)
	}
	receipt(operation.ViewFromRow(created.Operation))
	row, err := store.StartApplying(t.Context(), other.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	receipt(operation.ViewFromRow(row))
	row, err = store.RequireReconcile(t.Context(), other.OperationID, operation.Outcome{ErrorCode: "outcome_unknown", ErrorMessage: "Inspect the original operation"})
	if err != nil {
		t.Fatal(err)
	}
	receipt(operation.ViewFromRow(row))
}
