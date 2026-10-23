package pinaxclient

import (
	"context"
	"strings"
)

// InboxCaptureRequest is the bounded remote inbox capture payload. The owner
// still enforces write mode, approval and operation identity at the API.
type InboxCaptureRequest struct {
	Title          string
	Body           string
	Tags           []string
	Slug           string
	DryRun         bool
	Yes            bool
	OperationID    string
	IdempotencyKey string
}

// InboxCapture executes Pinax.Inbox.Capture through the recovery-aware RPC
// path. The same identity must be retained if the transport result is
// ambiguous.
func (c *Client) InboxCapture(ctx context.Context, request InboxCaptureRequest, identity MutationIdentity) (Projection, error) {
	if strings.TrimSpace(request.Title) == "" {
		return Projection{}, newError(CodeRequestInvalid, "Pinax inbox capture requires a title", nil)
	}
	if request.OperationID == "" {
		request.OperationID = identity.OperationID
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = identity.IdempotencyKey
	}
	if request.OperationID != identity.OperationID || request.IdempotencyKey != identity.IdempotencyKey {
		return Projection{}, newError(CodeRequestInvalid, "Pinax inbox capture identity does not match the retained binding", nil)
	}
	params := map[string]any{
		"title":           request.Title,
		"body":            request.Body,
		"tags":            request.Tags,
		"slug":            request.Slug,
		"dry_run":         request.DryRun,
		"yes":             request.Yes,
		"operation_id":    identity.OperationID,
		"idempotency_key": identity.IdempotencyKey,
	}
	return c.CallMutationRPC(ctx, RPCRequest{Method: "Pinax.Inbox.Capture", Params: params}, identity)
}

// FolderRenameRequest is the revision-bound remote folder rename payload.
type FolderRenameRequest struct {
	Path             string
	TargetPath       string
	ExpectedRevision string
	DryRun           bool
	Yes              bool
	OperationID      string
	IdempotencyKey   string
}

// FolderRename executes Pinax.Folder.Rename through the recovery-aware RPC
// path. It never falls back to a local filesystem mutation.
func (c *Client) FolderRename(ctx context.Context, request FolderRenameRequest, identity MutationIdentity) (Projection, error) {
	if strings.TrimSpace(request.Path) == "" || strings.TrimSpace(request.TargetPath) == "" {
		return Projection{}, newError(CodeRequestInvalid, "Pinax folder rename requires source and target paths", nil)
	}
	if request.OperationID == "" {
		request.OperationID = identity.OperationID
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = identity.IdempotencyKey
	}
	if request.OperationID != identity.OperationID || request.IdempotencyKey != identity.IdempotencyKey {
		return Projection{}, newError(CodeRequestInvalid, "Pinax folder rename identity does not match the retained binding", nil)
	}
	params := map[string]any{
		"path":              request.Path,
		"target_path":       request.TargetPath,
		"expected_revision": request.ExpectedRevision,
		"dry_run":           request.DryRun,
		"yes":               request.Yes,
		"operation_id":      identity.OperationID,
		"idempotency_key":   identity.IdempotencyKey,
	}
	return c.CallMutationRPC(ctx, RPCRequest{Method: "Pinax.Folder.Rename", Params: params}, identity)
}
