package operation

import (
	"context"
	"encoding/json"
	"strings"
)

type ReconcileEvidence struct {
	Applied       bool
	NotApplied    bool
	ProofRef      string
	Result        json.RawMessage
	Receipt       json.RawMessage
	ReceiptRef    string
	ResourceRef   string
	RevisionAfter string
	ErrorCode     string
	ErrorMessage  string
	Retryable     bool
	ReplaySafe    bool
}

type ReconcileDecision struct {
	Status     Status
	Outcome    Outcome
	NextAction string
}

// DecideReconcile 只根据 receipt/revision/resource inspector 提供的证据决策。
// crash window 中 operation 可能仍是 applying，但 domain mutation 已完成；
// 此处绝不能重新执行 mutation 猜测结果。证据不足时必须保持
// reconcile_required，等待人工或后续 inspector 提供可验证证据。
func DecideReconcile(current OperationRow, evidence ReconcileEvidence) (ReconcileDecision, error) {
	if current.Status == StatusSucceeded || current.Status == StatusFailed {
		return ReconcileDecision{Status: current.Status}, nil
	}
	if current.Status != StatusApplying && current.Status != StatusReconcileRequired {
		return ReconcileDecision{}, operationError(CodeIllegalTransition, "Operation is not ready for reconciliation", nil)
	}
	if evidence.Applied && evidence.NotApplied {
		return ReconcileDecision{}, operationError(CodeEvidenceConflict, "Reconcile evidence is contradictory", nil)
	}
	proofAvailable := strings.TrimSpace(evidence.ProofRef) != "" || strings.TrimSpace(evidence.ReceiptRef) != "" || strings.TrimSpace(evidence.ResourceRef) != "" || strings.TrimSpace(evidence.RevisionAfter) != "" || len(evidence.Receipt) > 0
	if evidence.Applied && proofAvailable {
		return ReconcileDecision{Status: StatusSucceeded, Outcome: Outcome{
			Result: evidence.Result, Receipt: evidence.Receipt, ReceiptRef: evidence.ReceiptRef,
			ResourceRef: evidence.ResourceRef, RevisionAfter: evidence.RevisionAfter,
		}}, nil
	}
	if evidence.NotApplied && proofAvailable {
		return ReconcileDecision{Status: StatusFailed, Outcome: Outcome{
			ErrorCode: evidence.ErrorCode, ErrorMessage: evidence.ErrorMessage,
			Retryable: evidence.Retryable, ReplaySafe: evidence.ReplaySafe,
		}}, nil
	}
	return ReconcileDecision{
		Status:     StatusReconcileRequired,
		Outcome:    Outcome{ErrorCode: string(CodeReconcileRequired), ErrorMessage: "Operation outcome still requires reconciliation"},
		NextAction: "Inspect the original receipt, resource identity, and revision evidence; do not replay the mutation",
	}, nil
}

func (s *Store) ApplyReconcileDecision(ctx context.Context, operationID string, decision ReconcileDecision) (OperationRow, error) {
	switch decision.Status {
	case StatusSucceeded:
		return s.CompleteSucceeded(ctx, operationID, decision.Outcome)
	case StatusFailed:
		return s.CompleteFailed(ctx, operationID, decision.Outcome)
	case StatusReconcileRequired:
		return s.RequireReconcile(ctx, operationID, decision.Outcome)
	default:
		return OperationRow{}, operationError(CodeIllegalTransition, "Reconcile decision status is invalid", nil)
	}
}
