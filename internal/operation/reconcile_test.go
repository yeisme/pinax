package operation

import (
	"context"
	"encoding/json"
	"testing"
)

func TestReconcilePolicyRequiresEvidenceAndNeverReplaysMutation(t *testing.T) {
	t.Parallel()

	current := OperationRow{OperationID: "op-reconcile-1", Status: StatusApplying}
	decision, err := DecideReconcile(current, ReconcileEvidence{Applied: true})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != StatusReconcileRequired || decision.NextAction == "" {
		t.Fatalf("evidence-free reconcile overclaimed outcome: %#v", decision)
	}

	decision, err = DecideReconcile(current, ReconcileEvidence{
		Applied: true, ProofRef: "receipt:verified", ReceiptRef: "receipts/verified.json",
		ResourceRef: "pinax://note/note_verified", RevisionAfter: "rev-2",
		Result: json.RawMessage(`{"note_ref":"note_verified"}`),
	})
	if err != nil || decision.Status != StatusSucceeded || decision.Outcome.ResourceRef == "" {
		t.Fatalf("applied evidence decision = %#v err=%v", decision, err)
	}

	decision, err = DecideReconcile(current, ReconcileEvidence{
		NotApplied: true, ProofRef: "revision:unchanged", ErrorCode: "revision_conflict",
		Retryable: false, ReplaySafe: false,
	})
	if err != nil || decision.Status != StatusFailed || decision.Outcome.ErrorCode != "revision_conflict" {
		t.Fatalf("not-applied evidence decision = %#v err=%v", decision, err)
	}
}

func TestReconcilePolicyRejectsContradictoryEvidence(t *testing.T) {
	t.Parallel()

	_, err := DecideReconcile(OperationRow{Status: StatusApplying}, ReconcileEvidence{Applied: true, NotApplied: true, ProofRef: "conflict"})
	if !IsCode(err, CodeEvidenceConflict) {
		t.Fatalf("contradictory evidence returned %v", err)
	}
}

func TestReconcileRepositoryKeepsAmbiguousOperationAndFinalizesSameIdentity(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	request := testCreateRequest(t, "op-reconcile-store-1", "idem-reconcile-store-1", map[string]any{"path": "inbox"})
	if _, err := store.CreateAccepted(ctx, request); err != nil {
		t.Fatal(err)
	}
	applying, err := store.StartApplying(ctx, request.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := DecideReconcile(applying, ReconcileEvidence{})
	if err != nil {
		t.Fatal(err)
	}
	ambiguous, err := store.ApplyReconcileDecision(ctx, request.OperationID, decision)
	if err != nil || ambiguous.Status != StatusReconcileRequired || ambiguous.ReconcileCount != 1 {
		t.Fatalf("ambiguous operation = %#v err=%v", ambiguous, err)
	}
	decision, err = DecideReconcile(ambiguous, ReconcileEvidence{
		Applied: true, ReceiptRef: "receipts/op-reconcile-store-1.json",
		ResourceRef: "pinax://note/note_reconciled", RevisionAfter: "rev-reconciled",
		Result: json.RawMessage(`{"note_ref":"note_reconciled"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.ApplyReconcileDecision(ctx, request.OperationID, decision)
	if err != nil || completed.OperationID != request.OperationID || completed.Status != StatusSucceeded || completed.ReconcileCount != 1 {
		t.Fatalf("reconciled operation = %#v err=%v", completed, err)
	}
}

func TestFailedOperationRetriesOnlyWithSameReplaySafeBinding(t *testing.T) {
	ctx := context.Background()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	request := testCreateRequest(t, "op-retry-1", "idem-retry-1", map[string]any{"title": "Retry"})
	if _, err := store.CreateAccepted(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartApplying(ctx, request.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteFailed(ctx, request.OperationID, Outcome{ErrorCode: "temporary_failure", Retryable: true, ReplaySafe: true}); err != nil {
		t.Fatal(err)
	}
	retried, err := store.RetryApplying(ctx, request.OperationID)
	if err != nil || retried.Status != StatusApplying || retried.Retryable || retried.ReplaySafe {
		t.Fatalf("retry applying = %#v err=%v", retried, err)
	}
	replayed, err := store.CreateAccepted(ctx, request)
	if err != nil || !replayed.Replay || replayed.Operation.OperationID != request.OperationID {
		t.Fatalf("same binding after retry = %#v err=%v", replayed, err)
	}
}
