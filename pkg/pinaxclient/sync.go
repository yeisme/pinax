package pinaxclient

import (
	"context"
)

// SyncRequest is the bounded control input shared by the supervised
// DriveBridge sync panel and the Pinax RPC surface. It contains no vault path,
// credential, or provider payload.
type SyncRequest struct {
	Identity       MutationIdentity
	Target         string
	Yes            bool
	DryRun         bool
	BaseRevision   string
	RemoteRevision string
	ContentDiff    bool
}

type SyncConflictRequest struct {
	Identity   MutationIdentity
	File       string
	KeepLocal  bool
	KeepRemote bool
	MergedPath string
	Yes        bool
}

func syncTarget(target string) string {
	if target == "" {
		return "capsa"
	}
	return target
}

func (c *Client) SyncStatus(ctx context.Context) (Projection, error) {
	return c.CallRPC(ctx, RPCRequest{Method: "Pinax.Sync.Status"})
}

func (c *Client) SyncDiff(ctx context.Context, request SyncRequest) (Projection, error) {
	return c.CallRPC(ctx, RPCRequest{Method: "Pinax.Sync.Diff", Params: map[string]any{
		"target":          syncTarget(request.Target),
		"base_revision":   request.BaseRevision,
		"remote_revision": request.RemoteRevision,
		"content_diff":    request.ContentDiff,
	}})
}

func (c *Client) SyncPush(ctx context.Context, request SyncRequest) (Projection, error) {
	return c.callSyncMutation(ctx, request.Identity, RPCRequest{Method: "Pinax.Sync.Push", Params: map[string]any{
		"target":          syncTarget(request.Target),
		"yes":             request.Yes,
		"dry_run":         request.DryRun,
		"base_revision":   request.BaseRevision,
		"remote_revision": request.RemoteRevision,
	}})
}

func (c *Client) SyncPull(ctx context.Context, request SyncRequest) (Projection, error) {
	return c.callSyncMutation(ctx, request.Identity, RPCRequest{Method: "Pinax.Sync.Pull", Params: map[string]any{
		"target":          syncTarget(request.Target),
		"yes":             request.Yes,
		"dry_run":         request.DryRun,
		"base_revision":   request.BaseRevision,
		"remote_revision": request.RemoteRevision,
	}})
}

func (c *Client) SyncConflictsList(ctx context.Context) (Projection, error) {
	return c.CallRPC(ctx, RPCRequest{Method: "Pinax.Sync.Conflicts.List"})
}

func (c *Client) SyncConflictsDiff(ctx context.Context, file string) (Projection, error) {
	return c.CallRPC(ctx, RPCRequest{Method: "Pinax.Sync.Conflicts.Diff", Params: map[string]any{"file": file}})
}

func (c *Client) SyncConflictsResolve(ctx context.Context, request SyncConflictRequest) (Projection, error) {
	return c.callSyncMutation(ctx, request.Identity, RPCRequest{Method: "Pinax.Sync.Conflicts.Resolve", Params: map[string]any{
		"file":        request.File,
		"keep_local":  request.KeepLocal,
		"keep_remote": request.KeepRemote,
		"merged_path": request.MergedPath,
		"yes":         request.Yes,
	}})
}

func (c *Client) callSyncMutation(ctx context.Context, identity MutationIdentity, request RPCRequest) (Projection, error) {
	if identity.OperationID == "" && identity.IdempotencyKey == "" {
		return c.CallRPC(ctx, request)
	}
	request.Params["operation_id"] = identity.OperationID
	request.Params["idempotency_key"] = identity.IdempotencyKey
	return c.CallMutationRPC(ctx, request, identity)
}
