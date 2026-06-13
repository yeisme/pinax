# Local REST/RPC Contract

Pinax's REST/RPC is a local projection adapter: it exposes existing application service projections to dashboards, agents, and local tools; it is not a public Internet hosted API, and it is not a remote Todo provider.

This centralized local API mode is intentionally separate from Pinax Cloud distributed sync. Remote API clients call into one server-side vault; Cloud Sync keeps a local vault on every device and uses a backend service only to coordinate encrypted revisions, blobs, and conflicts.

- `pinax api serve --port 0 --vault ./my-notes` binds to `127.0.0.1` by default, and the authentication mode defaults to a temp token (generated in process memory and printed once to stderr); it supports long-lived tokens with `--token-file` and unauthenticated mode with `--no-auth`.
- Explicitly use `--allow-write` when folder mutation is needed.
- REST handlers and the RPC dispatcher only perform parameter parsing, status code mapping, and projection JSON serialization; they must not directly read or write Markdown, `.pinax/`, SQLite/GORM repositories, Git, or providers.
- Auth middleware is a transport-layer concern and does not intrude into handler logic. Each route registers scope requirements by group.
- `--expose` and `--hide` control the exposed route groups; routes that are not exposed return `route_not_found`.
- Audit logs are written to `.pinax/events/api-audit.jsonl` and do not include token secrets, request bodies, or response bodies.
- stdout/stderr, events, fixtures, and evidence must not contain tokens, Authorization headers, Cookies, webhook URLs, provider raw payloads, or complete body leaks.

## Registry

Capabilities are read from `pinax api routes --vault ./my-notes --json`. Each route must include:

- `route_id`
- `surface`
- `method`
- REST `path` or RPC `rpc_method`
- `command`
- `capability_id`
- `schema_version`
- `readonly`
- `body_allowed`
- `approval_required`
- `snapshot_required`
- `errors`

The OpenAPI schema is derived from the same registry through `pinax api schema export --format openapi --vault ./my-notes --json`; a second route table is not maintained manually.

Exported OpenAPI paths/methods must come from the REST route registry one by one: for example, `rest.project.item.plan` must be exported as `post /v1/project-items/{ref}:{action}` and must not be hard-coded as `get`. Each operation includes at least:

- `operationId`
- `x-pinax-command`
- `x-pinax-capability`
- `x-pinax-readonly`
- `x-pinax-body-allowed`
- `x-pinax-approval-required`
- `x-pinax-snapshot-required`

## Read Paths

Current stable discovery and read paths:

```text
GET /
GET /v1/capabilities
GET /v1/projects/{slug}/board?note_display=card
GET /v1/notes/{ref}?display=card
GET /v1/folders?purpose=all&include_empty=true
GET /v1/folders/{path}
GET /v1/inbox
GET /v1/inbox/{ref}
GET /v1/drafts
GET /v1/drafts/{ref}
RPC Pinax.ProjectBoard.Show
RPC Pinax.Note.Read
RPC Pinax.Folder.List
RPC Pinax.Folder.Show
RPC Pinax.Folder.RepairPlan
RPC Pinax.Inbox.List
RPC Pinax.Inbox.Show
RPC Pinax.Draft.List
RPC Pinax.Draft.Show
```

`project board` and `note read` return bounded `NoteDisplay` by default. `card/detail/context` does not output complete bodies; returning the local note body is allowed only with explicit `display=body`.

## Write Plans

The first phase of remote write paths returns only plan/dry-run or gate projections:

```text
POST /v1/project-items/{ref}:{action}
RPC Pinax.ProjectItem.Plan
```

When confirmation is missing for archival or high-risk changes, return `approval_required`; when a version snapshot is missing, return `snapshot_required` and include a runnable `pinax version snapshot ...` action. Remote plans do not modify Markdown, `.pinax/`, Git, TaskBridge, providers, or remote services; real writes are still executed through explicit CLI commands.

Folder mutation routes reuse the CLI service and do not write the filesystem directly:

```text
POST /v1/folders?path={path}&purpose=notes&yes=true
POST /v1/folders/{path}:rename?target_path={new}&yes=true
POST /v1/folders/{path}:move?target_parent={parent}&yes=true
POST /v1/folders/{path}:delete?empty_only=true&yes=true
POST /v1/folders/{path}:adopt?purpose=assets&yes=true
POST /v1/folders:repair-plan
RPC Pinax.Folder.Create/Rename/Move/Delete/Adopt/RepairPlan
```

Inbox/Draft mutation routes reuse the lifecycle transition service:

```text
POST /v1/inbox:capture?title=...&yes=true
POST /v1/inbox/{ref}:promote?to=active&yes=true
POST /v1/inbox/{ref}:discard?yes=true
POST /v1/drafts?title=...&yes=true
POST /v1/drafts/{ref}:promote?status=active&yes=true
POST /v1/drafts/{ref}:archive?yes=true
POST /v1/drafts/{ref}:discard?yes=true
RPC Pinax.Inbox.Capture/Promote/Discard
RPC Pinax.Draft.Create/Promote/Archive/Discard
```

inbox/draft writes are constrained by the same `--allow-write` and `yes=true` gates. `discard` is not a hard delete; it only sets `status=discarded`.


The default readonly server returns `write_disabled` for folder mutations and does not write to disk even if the request includes `yes=true`. After startup with `--allow-write`, non-dry-run mutations must still include `yes=true`; otherwise they return `approval_required`.

## CLI Remote API Mode

Ordinary Pinax commands can be forwarded to a running local API service instead of reading a vault in the current process:

```bash
pinax api serve --vault ./my-notes --port 8787 --no-auth
pinax --api-url http://127.0.0.1:8787 folder list --purpose notes --json
PINAX_API_URL=http://127.0.0.1:8787 pinax inbox list --agent
pinax config set remote.api_url http://127.0.0.1:8787 --scope user
pinax folder list --json
pinax note list --status active --limit 20 --json
```

- `--api-url`, `PINAX_API_URL`, or user/project config key `remote.api_url` enables remote mode for supported commands. Precedence is explicit flag, environment variable, project config, user config, then default empty value.
- `--api-token`, `--api-token-file`, `PINAX_API_TOKEN`, and `PINAX_API_TOKEN_FILE` configure a Bearer token. The token is sent only in the `Authorization` header and must not appear in stdout, stderr, test fixtures, projection errors, or configuration files.
- An explicit `--vault` is rejected in remote mode with `remote_vault_conflict`; this prevents accidental fallback to a local vault.
- Unsupported commands are rejected with `remote_command_unsupported`; remote mode must not silently execute unsupported commands locally.
- When remote mode comes only from `remote.api_url`, local control/configuration commands (`config`, `api`, `token`, `profile`, `vault`, `cloud`, and `sync`) remain local so users can inspect/update endpoints and manage local-first Cloud Sync state without being hijacked by Remote API Mode.
- Supported first-phase commands are the registered RPC capabilities for project board show, note list/read/show/preview, project item move/archive plan, folder list/show/create/rename/move/delete/adopt/repair, inbox list/show/capture/promote/discard, and draft list/show/create/promote/archive/discard.
- `--json` renders the returned Projection envelope directly as JSON-only stdout; `--agent` renders the same Projection as key=value lines.

## Transport Status

HTTP status expresses only transport semantics, and the body always remains a Pinax projection envelope:

| Scenario | HTTP status | projection error |
| --- | --- | --- |
| Unknown REST path | `404` | `route_not_found` |
| Registered path uses an unsupported method | `405` | `method_not_allowed` |
| Unknown RPC method | `404` | `rpc_method_not_found` |
| Invalid RPC JSON body | `400` | `invalid_rpc_request` |
| Missing remote write confirmation | non-2xx, currently `400` | `approval_required` |
| readonly server receives a write route | non-2xx, currently `403` | `write_disabled` |
| Missing version snapshot | non-2xx, currently `400` | `snapshot_required` |

RPC unknown method returns a failed projection with `error.code=rpc_method_not_found`; the hint must prompt the user to check `pinax api routes`.

## Serve Lifecycle Output

- Default human mode: stdout remains empty. Zap console logs are written to stderr, including startup readiness (`pinax api ready`), the localhost URL, auth mode, write mode, and per-request access logs (`api.request`) with method, path, status, route group, and duration.
- RPC requests also emit an `api.rpc` log with `rpc_method`, optional `rpc_id`, command, route group, readonly/write classification, HTTP status, duration, and projection `error_code` when present. RPC logs must not include params, request bodies, response bodies, note content, raw query strings, Authorization headers, cookies, tokens, or provider payloads.
- In temp-token auth mode, the generated temporary token is printed once to stderr as a startup log field. Request logs must not include Authorization headers, raw query strings, cookies, request bodies, response bodies, or provider payloads.
- `GET /` returns a JSON discovery projection with links to `/v1/capabilities` and runnable `pinax api routes` / schema commands; it is the intended smoke-test path for `curl http://127.0.0.1:<port>/`.
- `--readonly` is the explicit spelling of the default mode; `--allow-write` enables controlled mutation routes. The two cannot be used together.
- `--token-file <path>`: loads a long-lived token from a file (scope is fine-grained to the route group).
- `--no-auth`: unauthenticated mode, with a forced loopback address check.
- `--expose notes,inbox`: exposes only the specified route groups.
- `--hide drafts,projects`: hides the specified route groups.
- `--events`: stdout outputs NDJSON lifecycle events, including at least `start`, `ready`, and `shutdown` during shutdown, and outputs `error` on startup failure; diagnostics must still not be mixed into stdout.
- `--json` / `--agent`: currently returns a failed projection with `error.code=unsupported_output_mode`, does not start the server, and does not write URLs, logs, or human-readable paragraphs to machine stdout.

## Auth Status

| Scenario | HTTP status | error code |
| --- | --- | --- |
| Missing Bearer token | `401` | `token_required` |
| Token validation failed | `401` | `invalid_token` |
| Token expired | `401` | `token_expired` |
| Token scope is insufficient | `403` | `insufficient_scope` |
| Non-loopback in `--no-auth` mode | `403` | `loopback_required` |

## Cache Behavior

Read-only GET routes return `Cache-Control` and `ETag` headers. Clients can send `If-None-Match` to receive a 304 response. POST/PUT/PATCH/RPC are not cached. See `cache-contract.md`.
