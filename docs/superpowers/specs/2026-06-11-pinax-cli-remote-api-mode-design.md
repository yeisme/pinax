# Pinax CLI Remote API Mode Design

Date: 2026-06-11

## Background

`pinax api serve --allow-write --no-auth --port 8787 --vault /tmp/pinax-notes` already exposes a local REST projection adapter for one vault. A second Pinax CLI process can manually call the HTTP routes with `curl`, but ordinary commands such as `pinax folder list`, `pinax note show`, or `pinax inbox capture` still execute against a local vault through `app.Service`.

The desired capability is a remote execution mode: another Pinax CLI should connect to the running local API service and forward supported ordinary commands to that service, while preserving the existing Projection envelope and output modes.

Current facts from the codebase:

- REST route and capability metadata are registered in `internal/app/remote.go` through `RemoteCapabilities()` and `RemoteRoutes()`.
- HTTP handlers in `internal/api/http.go` expose the REST projection routes and root discovery.
- `internal/api.RPCDispatcher` already maps `Pinax.*` RPC methods to `app.Service` use cases, but there is no HTTP `/v1/rpc` endpoint.
- `pinax vault remote refresh` expects a Cloud-style `/v1/vaults` discovery endpoint and is not compatible with `pinax api serve`.
- Existing API tests assert REST/RPC route registry parity and output projection envelopes.

## Goals

1. Add a Pinax CLI remote mode selected by `--api-url` or `PINAX_API_URL`.
2. Allow a second CLI process to run supported ordinary commands against a running `pinax api serve` instance.
3. Preserve the existing output contract: default human output, `--json`, and `--agent` all render from the same Projection envelope.
4. Avoid accidental local writes: remote mode must never silently fall back to local vault execution.
5. Reuse existing app services and route registry metadata; do not create a second business-logic path in the client.
6. Keep auth, write gates, audit, and redaction consistent with the existing local API contract.

## Non-goals

- Do not turn `pinax api serve` into a multi-vault Cloud control plane.
- Do not implement `/v1/vaults` discovery as part of this change.
- Do not make `pinax backend`, `sync`, `cloud`, `git`, `index rebuild`, `init`, or `version snapshot` transparently remote in the first version.
- Do not store raw tokens in vault metadata, logs, stdout, stderr, fixtures, or docs.
- Do not reinterpret `--vault` as a remote vault selector in the first version.
- Do not use REST path mapping as the main client dispatch mechanism.

## Approved approach

Use HTTP RPC as the CLI transport:

```http
POST /v1/rpc
Content-Type: application/json

{
  "id": "optional-client-request-id",
  "method": "Pinax.Folder.List",
  "params": {
    "purpose": "all",
    "include_empty": true
  }
}
```

The response is still a normal Pinax Projection envelope:

```json
{
  "spec_version": "1.0",
  "mode": "json",
  "command": "folder.list",
  "status": "success",
  "facts": {},
  "data": {}
}
```

This is not JSON-RPC 2.0. It is a Pinax RPC transport whose response contract remains `domain.Projection`. This avoids a second error envelope and keeps all output modes driven by existing renderers.

## User interface

### Start the service

```bash
pinax api serve --allow-write --no-auth --port 8787 --vault /tmp/pinax-notes
```

### Use remote mode explicitly

```bash
pinax --api-url http://127.0.0.1:8787 folder list --json
pinax --api-url http://127.0.0.1:8787 folder create spaces/api --purpose notes --yes
pinax --api-url http://127.0.0.1:8787 inbox list --json
pinax --api-url http://127.0.0.1:8787 draft create --title "Draft title" --body "Draft body" --yes
pinax --api-url http://127.0.0.1:8787 note show note_xxx --display card --json
```

### Use remote mode through environment

```bash
export PINAX_API_URL=http://127.0.0.1:8787
pinax folder list --json
```

### Authenticated service

When the API server requires bearer auth:

```bash
PINAX_API_URL=http://127.0.0.1:8787 \
PINAX_API_TOKEN="$TOKEN" \
pinax folder list --json
```

or:

```bash
pinax \
  --api-url http://127.0.0.1:8787 \
  --api-token-file ~/.config/pinax/api-token \
  folder list --json
```

## CLI remote mode rules

1. Remote mode is active when `--api-url` or `PINAX_API_URL` is non-empty.
2. In remote mode, supported commands call the remote API and do not read or write local vault content.
3. Unsupported commands fail with `remote_command_unsupported`.
4. Unsupported commands must not fall back to local execution.
5. If remote mode is active and `--vault` is explicitly supplied, return `remote_vault_conflict` in the first version.
6. `--api-url` normalizes the base URL by trimming trailing slashes and requires a valid `http` or `https` scheme.
7. `--api-token` and `--api-token-file` are mutually exclusive.
8. `--api-token` is accepted for development convenience but environment or token file is preferred in docs and examples.
9. Remote request diagnostics go to stderr only; machine stdout remains governed by the selected output mode.

The strict `--api-url` plus `--vault` conflict prevents users from believing they are writing to the remote service while accidentally targeting a local path. Multi-vault remote selection can be introduced later with a distinct remote profile or vault-selector design.

## Supported command set: first version

The first version supports only capabilities already present in the local API route registry.

```text
project.board.show
note.show / note.read
project.item.plan

folder.list
folder.show
folder.create
folder.rename
folder.move
folder.delete
folder.adopt
folder.repair

inbox.list
inbox.show
inbox.capture
inbox.promote
inbox.discard

draft.list
draft.show
draft.create
draft.promote
draft.archive
draft.discard
```

Out of scope for the first version:

```text
init
index rebuild
version snapshot
git commands
sync/cloud/backend profile commands
vault register/use/remote refresh
provider delivery commands
```

These commands either maintain local process state, touch Git or provider boundaries, or require a Cloud-style control plane rather than the current single-vault local API.

## Server design

### Route

Add:

```text
POST /v1/rpc
```

The route is added to the HTTP router and route metadata. The path group is `rpc`, but method-level metadata still determines whether the specific RPC method is readonly or write-capable.

### Request type

```go
type HTTPRPCRequest struct {
    ID     string         `json:"id,omitempty"`
    Method string         `json:"method"`
    Params map[string]any `json:"params,omitempty"`
}
```

### Method metadata

Add a helper near the route registry:

```go
func FindRemoteRPCMethod(method string) (domain.RemoteRoute, bool)
```

It scans `RemoteRoutes()` for `Surface == "rpc"` and `RPCMethod == method`.

Consumers:

- `/v1/rpc` handler authorization and group/write decisions.
- HTTP RPC unknown-method errors.
- CLI client capability validation.
- Registry parity tests.

### Dispatch

The `/v1/rpc` handler:

1. Accepts only `POST`.
2. Decodes `HTTPRPCRequest` with bounded body size.
3. Validates `method` is non-empty.
4. Looks up the method in `RemoteRoutes()`.
5. Applies auth scope and exposure rules using route metadata.
6. Calls `RPCDispatcher.Call(ctx, req)`.
7. Maps projection errors to HTTP status using the existing API status policy.
8. Writes a Projection envelope as JSON.

Business logic remains in `app.Service`; the handler only parses, authorizes, dispatches, and serializes.

### Write gates

Existing gates remain authoritative:

- Server started without `--allow-write`: write RPC returns `write_disabled`.
- Write RPC without `yes=true` and without `dry_run=true`: returns `approval_required`.
- Snapshot-required mutations still return `snapshot_required` when no usable snapshot exists.
- `--dry-run` must never mutate vault files or remote/provider state.

### HTTP statuses

| Scenario | HTTP status | `error.code` |
| --- | --- | --- |
| Bad JSON or missing method | `400` | `invalid_rpc_request` |
| Unknown RPC method | `404` or `400` | `rpc_method_not_found` |
| Readonly server receives write RPC | `403` | `write_disabled` |
| Write RPC missing confirmation | `400` | `approval_required` |
| Missing required snapshot | `400` | `snapshot_required` |
| Missing/invalid token | `401` | existing auth code |
| Insufficient scope | `403` | `insufficient_scope` |

### Logging and audit

Request logs continue to include method, path, status, duration, and group. RPC-specific logs may include sanitized metadata:

```json
{
  "event": "api.rpc",
  "rpc_method": "Pinax.Folder.List",
  "command": "folder.list",
  "readonly": true,
  "status": "success"
}
```

Never log Authorization headers, bearer tokens, cookies, raw request body, note body, provider payloads, or complete response bodies.

## Client design

Add a package:

```text
internal/remoteapi/
  client.go
  rpc.go
  capabilities.go
  errors.go
```

Core type:

```go
type Client struct {
    BaseURL string
    Token   string
    HTTP    *http.Client
}

func (c *Client) Ping(ctx context.Context) (domain.Projection, error)
func (c *Client) Capabilities(ctx context.Context) (domain.Projection, error)
func (c *Client) Call(ctx context.Context, method string, params map[string]any) (domain.Projection, error)
```

Client behavior:

- Normalize base URL once.
- Require `http` or `https` scheme.
- Use a default timeout when no client is injected.
- Set `Authorization: Bearer <token>` only on HTTP requests.
- On non-2xx responses, still attempt to decode a Projection envelope.
- If response decoding fails, return `remote_api_invalid_response`.
- If the service cannot be reached, return `remote_api_unreachable`.
- Do not include token values, Authorization headers, or request bodies in errors.

## CLI wiring design

Add global state to `commandBuildContext`:

```go
apiURL       *string
apiToken     *string
apiTokenFile *string
```

Add global flags:

```text
--api-url string          Pinax local API URL for remote mode
--api-token string        Bearer token for Pinax API; prefer env or token file
--api-token-file string   File containing Bearer token
```

Add environment variables:

```text
PINAX_API_URL
PINAX_API_TOKEN
PINAX_API_TOKEN_FILE
```

Precedence:

```text
explicit flag > env > empty
```

Config-file persistence for API profiles is intentionally deferred. First version avoids writing token-related structured assets and keeps the feature explicit.

### Dispatch helper

Add a CLI helper, for example:

```go
func (ctx commandBuildContext) remoteEnabled() bool
func (ctx commandBuildContext) remoteCall(cmd *cobra.Command, rpcMethod string, params map[string]any) (domain.Projection, error)
```

Each supported command follows this pattern:

```go
if ctx.remoteEnabled() {
    projection, err := ctx.remoteCall(cmd, "Pinax.Folder.List", map[string]any{
        "purpose": purpose,
        "include_empty": includeEmpty,
        "depth": depth,
    })
    return ctx.renderProjection(cmd, projection, err)
}

projection, err := ctx.svc.ListFolders(cmd.Context(), app.FolderListRequest{VaultPath: *ctx.vaultPath, Purpose: purpose, IncludeEmpty: includeEmpty, Depth: depth})
return ctx.renderProjection(cmd, projection, err)
```

This keeps command argument parsing in the existing Cobra layer and swaps only the execution backend.

## Output contract

Remote mode is not a new output mode. It changes where the command executes.

These commands must keep their existing output semantics:

```bash
pinax --api-url http://127.0.0.1:8787 folder list
pinax --api-url http://127.0.0.1:8787 folder list --json
pinax --api-url http://127.0.0.1:8787 folder list --agent
```

Requirements:

- `--json` stdout is one valid Projection JSON object and nothing else.
- `--agent` stdout is stable key=value output.
- Default human output stays concise and comes from the same projection.
- Remote transport diagnostics and logs go to stderr.
- Tokens and Authorization headers never appear in any output mode.

## Errors

New client-side errors:

```text
remote_api_url_invalid
remote_api_unreachable
remote_api_auth_required
remote_api_invalid_response
remote_command_unsupported
remote_vault_conflict
remote_capability_missing
remote_api_version_unsupported
api_token_conflict
api_token_file_read_failed
```

Example failed projection:

```json
{
  "spec_version": "1.0",
  "mode": "json",
  "command": "folder.list",
  "status": "failed",
  "error": {
    "code": "remote_api_unreachable",
    "message": "Pinax API is unreachable",
    "hint": "Check pinax api serve and retry"
  }
}
```

## Security and operational constraints

- `--no-auth` remains loopback-only on the server side.
- Remote mode does not weaken server auth; tokens are transport credentials only.
- Client errors must not echo request bodies or bearer token values.
- RPC audit records must avoid raw body and response body.
- Remote writes require both server `--allow-write` and request-level confirmation.
- The client must not implement a hidden local fallback after any remote failure.

## Tests

### Server tests

Add or extend tests in:

```text
internal/api/http_test.go
internal/api/rpc_test.go
internal/api/middleware_test.go
internal/app/remote_test.go
```

Cover:

- `POST /v1/rpc` calls `Pinax.Folder.List` and returns `folder.list` projection.
- Unknown method returns `rpc_method_not_found`.
- Bad JSON returns `invalid_rpc_request`.
- Readonly server write returns `write_disabled`.
- Allow-write server without `yes=true` returns `approval_required`.
- Allow-write server with `yes=true` can create a folder.
- Token scope enforcement works for readonly and write RPC methods.
- Hidden route group blocks RPC access consistently with REST exposure rules.
- Route registry and dispatcher remain in parity.
- Logs/audit do not include token or raw body fixtures.

### Client tests

Add:

```text
internal/remoteapi/client_test.go
```

Cover:

- Base URL normalization.
- Invalid scheme and empty URL.
- Authorization header is sent when token is configured.
- Token does not appear in returned errors.
- Non-2xx projection envelope is decoded.
- Invalid JSON response returns `remote_api_invalid_response`.
- Unreachable service returns `remote_api_unreachable`.
- Request timeout is respected.

### CLI tests

Add or extend command tests close to the changed command layer, and add process-level coverage where appropriate:

```text
internal/cli/*_test.go
tests/e2e/
```

Cover:

- `pinax --api-url <server> folder list --json` returns remote folder projection.
- `pinax --api-url <server> folder create remote-test --purpose notes --yes --json` writes through the server vault, not a local cwd vault.
- `--api-url` plus explicit `--vault` returns `remote_vault_conflict`.
- Unsupported commands return `remote_command_unsupported` and do not run locally.
- `--json` remote output is JSON only.
- `--agent` remote output is stable key=value.
- Token values do not appear in stdout or stderr snapshots.

## Implementation phases

### Phase 1: HTTP RPC server contract

- Add `POST /v1/rpc`.
- Add registry helper for RPC method lookup.
- Add auth, exposure, write gate, and status mapping tests.
- Update remote API contract docs.

### Phase 2: Remote API client

- Add `internal/remoteapi.Client`.
- Implement `Ping`, `Capabilities`, and `Call`.
- Add client tests for errors, auth header, non-2xx decoding, and redaction.

### Phase 3: CLI remote mode wiring

- Add `--api-url`, `--api-token`, `--api-token-file` and environment handling.
- Add remote dispatch helper.
- Wire first supported command set.
- Add conflict handling for explicit `--vault`.

### Phase 4: Output and documentation hardening

- Add CLI output contract tests for remote mode.
- Update command docs with real runnable examples.
- Keep Cloud/vault remote discovery docs separate from local API remote mode.

## Future extensions

- `pinax api profile add <name> --url <url> --token-ref env://PINAX_API_TOKEN` for named remote profiles.
- Remote vault selectors after a real multi-vault control plane exists.
- HTTP `/v1/rpc` compatibility with a future Cloud control plane.
- Additional command support after each command receives explicit route metadata and remote safety tests.

## Acceptance criteria

1. A user can start `pinax api serve` for one vault and run supported commands from another CLI with `--api-url`.
2. Remote reads and controlled writes execute against the server vault.
3. Unsupported remote commands fail without local fallback.
4. Existing output modes remain contract-compatible.
5. Server and client tests cover auth, write gates, registry parity, and redaction.
6. Docs clearly distinguish local API remote mode from Cloud backend and vault remote discovery.
