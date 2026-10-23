package app

import (
	"context"
	"os"

	"github.com/yeisme/pinax/internal/operation"
)

// WriterTagsLookupRequest never carries a proposal, tag draft or preview key.
// Session info is supplied by the trusted host, not transport request fields.
type WriterTagsLookupRequest struct {
	VaultPath   string
	Session     ReaderSessionInfo
	OperationID string
}

// WriterTagsLookup returns the original parent operation, not current note
// state. It grants no source access and cannot resume or reconcile a mutation.
func (s *Service) WriterTagsLookup(ctx context.Context, req WriterTagsLookupRequest) (operation.View, error) {
	var empty operation.View
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := writerTagsIdentityGate(WriterTagsRequest{Session: req.Session, OperationID: req.OperationID}); err != nil {
		return empty, err
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return empty, err
	}
	before, err := os.Stat(root)
	if err != nil {
		return empty, collaborationError("writer_unavailable", "The authorized vault is unavailable")
	}
	store, cleanup, err := operation.OpenReadSnapshot(root)
	if err != nil {
		return empty, err
	}
	defer cleanup()
	access := writerTagsOperationAccess(req.Session)
	row, err := store.GetAuthorized(ctx, req.OperationID, access.Access.PrincipalDigest, access.Access.ScopeDigest)
	if err != nil {
		return empty, err
	}
	if row.CapabilityID != writerTagsApplyCapability || row.BindingID != "rest.writer.tags.apply" {
		return empty, &operation.Error{Code: operation.CodeNotFound, Message: "Operation was not found"}
	}
	after, err := os.Stat(root)
	if err != nil || !os.SameFile(before, after) {
		return empty, collaborationError("source_changed", "The authorized vault changed during lookup")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return operation.ViewFromRow(row), nil
}
