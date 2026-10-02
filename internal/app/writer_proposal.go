package app

import (
	"context"
	"strings"

	"github.com/yeisme/pinax/internal/operation"
)

// Supervised writer proposal projection (pinax-human-reading-write-v1 W03).
//
// Human edits follow the frozen proposal gate: propose prepares an exact
// preview without writing; apply accepts only the unchanged change plus its
// preview digest and a caller-asserted authorization (explicit_instruction
// or reviewed_proposal — a caller assertion, NEVER a verified host
// approval). The shared flow lives in collaborationApplyWithRequest; the
// writer surface swaps in the writer session operation identity so receipts
// stay attributable to the supervised session, never the owner-local MCP
// identity.

// WriterProposal is the prepared preview returned by propose; the shapes
// mirror the collaboration preview exactly (no widened HTTP form).
type WriterProposal = CollaborationPreview

// writerPolicy is the owner policy the supervised writer surface runs
// under: the host granted the writer.editor scope for this session, which
// is the enabling decision the collaboration policy represents.
func writerPolicy() CollaborationPolicy {
	return CollaborationPolicy{AllowBody: true, AllowWrite: true}
}

// WriterPropose prepares an edit proposal without writing. The preview
// carries the exact before/after and the digest apply must echo unchanged.
func (s *Service) WriterPropose(ctx context.Context, root string, change CollaborationChange) (WriterProposal, error) {
	return s.CollaborationPreview(ctx, root, change, writerPolicy())
}

// WriterApply saves an unchanged preview under the supervised writer
// identity. The operation identity is change.operation_id — required, and
// the only identity a recovery query may use; blind retries under a fresh
// identity are structurally impossible because the ledger binds
// idempotency to this operation id.
func (s *Service) WriterApply(ctx context.Context, root string, change CollaborationChange, digest, authorization string, identity RemoteOperationIdentity) (operation.View, error) {
	operationID := strings.TrimSpace(change.OperationID)
	if operationID == "" {
		return operation.View{}, collaborationError("operation_identity_required", "Writer apply requires change.operation_id; reuse it to recover the receipt")
	}
	principalDigest := strings.TrimSpace(identity.Access.PrincipalDigest)
	scopeDigest := strings.TrimSpace(identity.Access.ScopeDigest)
	if principalDigest == "" || scopeDigest == "" {
		return operation.View{}, collaborationError("operation_identity_required", "Writer apply requires the supervised session identity")
	}
	request := operation.CreateRequest{
		OperationID: operationID, IdempotencyKey: operationID,
		CapabilityID: "writer.apply", BindingID: "rest.writer.apply",
		PrincipalDigest: principalDigest, ScopeDigest: scopeDigest,
		RequestDigest: digest, RevisionBefore: change.ExpectedRevision,
	}
	return s.collaborationApplyWithRequest(ctx, root, change, digest, authorization, writerPolicy(), request)
}
