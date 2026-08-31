# Pinax REST/RPC owner contract

Pinax REST/RPC 是 owner projection adapter：它把 application service 的 bounded projection 暴露给 dashboard、agent、remote CLI 与 SDK。`local-vault`、`remote-service`、`self-hosted-service` 表示 state owner mode；`embedded`、`loopback-http`、`https` 与 stdio MCP 表示 transport。mode 和 transport 必须分别解析，不能根据 URL 猜测并混写为一个字段。

This centralized local API mode is intentionally separate from Pinax Cloud distributed sync. Remote API clients call into one server-side vault; Cloud Sync keeps a local vault on every device and uses a backend service only to coordinate encrypted revisions, blobs, and conflicts.

The long-term client target is CLI capability parity through registered routes and RPC methods. This must evolve additively: new client-visible commands are added to the capability registry, unsupported commands keep returning `remote_command_unsupported`, and local runtime-control commands remain local-only unless a dedicated safe capability is designed. See [Client CLI Parity and Realtime Sync](./client-cli-parity-and-sync.md).

- `pinax api serve --port 0 --vault ./my-notes` 默认绑定 `127.0.0.1`，认证默认使用仅存在于进程内并只在 stderr 输出一次的 temp token。长期 token 使用 canonical `--token-store <hashed-registry>`；旧服务端 `--token-file` 仅作为同语义 deprecated alias；`--no-auth` 强制 loopback。
- Explicitly use `--allow-write` when folder, draft, inbox, sync, subproject, or memory mutation is needed. Dry-run memory capture remains non-persistent.
- REST handlers and the RPC dispatcher only perform parameter parsing, status code mapping, and projection JSON serialization; they must not directly read or write Markdown, `.pinax/`, SQLite/GORM repositories, Git, or providers.
- Auth middleware is a transport-layer concern and does not intrude into handler logic. Each route registers scope requirements by group.
- `--expose` and `--hide` control the exposed route groups; routes that are not exposed return `route_not_found`.
- Audit logs are written to `.pinax/events/api-audit.jsonl` and do not include token secrets, request bodies, or response bodies.
- stdout/stderr, events, fixtures, and evidence must not contain tokens, Authorization headers, Cookies, webhook URLs, provider raw payloads, or complete body leaks.

## Manifest 与 legacy registry

Authoritative discovery 来自以下等价入口：

```text
pinax api manifest --vault ./my-notes --json
GET /v1/manifest
RPC Pinax.Transport.Manifest
```

三者返回同一排序、schema ref、binding availability 与 digest 的 `pinax.transport_manifest.v1`。新消费者应根据真实 binding 判断 capability 是否可执行。旧 `pinax api routes`、`GET /v1/capabilities`、legacy `surfaces` 与 capability fields 全部保留原语义；planned/future-owner capability 不得被放进 `available_surfaces`。

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

## Agent Brain Capability Handoff

Agent Brain local API support is additive and staged. Current discovery may expose existing read-only capabilities such as memory context, KB context, search, graph, project board, proof loop, and MCP-adjacent projections only when the corresponding CLI/API/MCP route is implemented. `pinax.brain.context`, `pinax.brain.answer`, `pinax.brain.sources`, and `pinax.brain.maintenance_plan` are implemented stdio MCP tools, not HTTP routes. Planned local API capability ids such as `brain.context.bundle`, `brain.answer.preview`, `brain.sources.list`, `brain.maintenance.plan`, and `brain.provider.cost_status` must use a clear `local_only_reason` such as `planned`, `future-contract`, or `future-owner` until an implemented route exists; OpenAPI export must not invent HTTP paths for planned capabilities.

HTTP MCP, OAuth, hosted team/company KB mode, organization permission policy, and rate limit enforcement are future-owner concerns. `cli/pinax` may define local projection fields and failure codes, but production HTTP MCP or multi-user backend behavior must be owned by `mcp/gateway`, a hosted/backend subproject, or a later explicit OpenSpec. Without that owner and scope proof, Agent Brain routes must remain single-user local projections and return bounded failures such as `permission_unknown`, `scope_required`, `insufficient_scope`, or `future_owner_required` instead of synthesizing cross-user knowledge.

Future team/company projections must carry permission metadata rather than relying on note paths alone:

| Field | Meaning |
| --- | --- |
| `principal` | Current local user, service account, or future authenticated user subject. |
| `workspace` | Local vault workspace or future organization workspace id. |
| `source_acl` | Evidence of source-level access, when available. |
| `visibility` | `private`, `shared`, `team`, `company`, or `unknown`. |
| `redaction_policy` | Projection policy applied before returning snippets or claims. |
| `audit_ref` | Local receipt, API audit entry, or future gateway audit reference. |

## Read Paths

Current stable discovery and read paths:

```text
GET /
GET /v1/capabilities
GET /v1/manifest
GET /v1/readiness
GET /v1/operations/{operation_id}
GET /v1/projects
GET /v1/projects/{project}
GET /v1/projects/{slug}/board?note_display=card
GET /v1/projects/{slug}/board?subproject=stock-learning&note_display=card
GET /v1/projects/{slug}/subprojects
GET /v1/projects/{slug}/subprojects/{subproject}
GET /v1/project-items/{item_id}
GET /v1/notes/{ref}?display=card
GET /v1/folders?purpose=all&include_empty=true
GET /v1/folders?under=notes/projects/research&purpose=all
GET /v1/folders/{path}
GET /v1/inbox
GET /v1/inbox/{ref}
GET /v1/drafts
GET /v1/drafts/{ref}
GET /v1/memory
GET /v1/memory:recall?query=memory
GET /v1/memory:context?task=memory
GET /v1/memory:stats
RPC Pinax.ProjectBoard.Show
RPC Pinax.Project.Subproject.List
RPC Pinax.Project.Subproject.Show
RPC Pinax.Note.Read
RPC Pinax.Folder.List
RPC Pinax.Folder.Show
RPC Pinax.Folder.RepairPlan
RPC Pinax.Inbox.List
RPC Pinax.Inbox.Show
RPC Pinax.Draft.List
RPC Pinax.Draft.Show
RPC Pinax.Memory.List
RPC Pinax.Memory.Recall
RPC Pinax.Memory.Context
RPC Pinax.Memory.Stats
RPC Pinax.Sync.Push
RPC Pinax.Sync.Pull
RPC Pinax.Transport.Manifest
RPC Pinax.Connection.Readiness
RPC Pinax.Operation.Get
```

`project board` and `note read` return bounded `NoteDisplay` by default. `card/detail/context` does not output complete bodies; returning the local note body is allowed only with explicit `display=body`. `project board show` accepts optional `subproject`; when present, the adapter must return the same scoped projection as `pinax project board show <project> --subproject <slug> --json`.

## Write Plans

The first phase of remote write paths returns only plan/dry-run or gate projections:

```text
POST /v1/project-items/{ref}:{action}
RPC Pinax.ProjectItem.Plan
```

When confirmation is missing for archival or high-risk changes, return `approval_required`; when a version snapshot is missing, return `snapshot_required` and include a runnable `pinax version snapshot ...` action. Remote plans do not modify Markdown, `.pinax/`, Git, providers, or remote services; real writes are still executed through explicit CLI commands.

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

Project workspace mutation routes also reuse the CLI service and stay behind the same write gates:

```text
POST /v1/projects/{project}/subprojects?subproject={slug}&title={title}&template=scenario&yes=true
RPC Pinax.Project.Subproject.Create
```

The default readonly server returns `write_disabled` for project workspace creates even if `yes=true` is present. When `--allow-write` is enabled, missing confirmation returns `approval_required`; successful writes create only the standard workspace directories and CLI-authored registry through the application service.

Memory capture uses the same CLI memory service and never bypasses the ledger boundary:

```text
POST /v1/memory:capture?dry_run=true
POST /v1/memory:capture?yes=true
RPC Pinax.Memory.Capture
```

`dry_run=true` validates and returns a preview record without creating `.pinax/memory/ledger.sqlite`. Confirmed capture requires API write mode plus `yes=true`; otherwise the adapter returns `write_disabled` or `approval_required`.

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

### Recovery-enabled mutation

首批带 durable operation recovery 的 mutation 是 `inbox.capture` 与 `folder.rename`。它们在既有 route/RPC 之上 additive 接收 operation identity：

```text
POST /v1/inbox:capture
  X-Pinax-Operation-ID: <opaque-operation-id>
  Idempotency-Key: <opaque-idempotency-key>

POST /v1/folders/{path}:rename?target_path={new}&expected_revision={revision}&yes=true
  X-Pinax-Operation-ID: <opaque-operation-id>
  Idempotency-Key: <opaque-idempotency-key>

RPC Pinax.Inbox.Capture
  params.operation_id
  params.idempotency_key

RPC Pinax.Folder.Rename
  params.operation_id
  params.idempotency_key
  params.expected_revision
```

Server 在 domain write 前将 principal/scope、binding、canonical request digest、operation id 与 idempotency key 的 redacted binding 写入 GORM operation ledger。相同 key 与相同 request 返回 durable outcome；相同 key 与不同 request 返回 `idempotency_conflict`。`folder.rename` 的 revision 不匹配返回 `revision_conflict`，fresh snapshot 缺失返回 `snapshot_required`，均不得执行 rename。

Operation inspection/reconcile 是只读 owner evidence flow，不 replay mutation：

```text
GET  /v1/operations/{operation_id}
POST /v1/operations/{operation_id}:reconcile
RPC  Pinax.Operation.Get
RPC  Pinax.Operation.Reconcile
```

Operation projection 使用 `pinax.operation.v1`，包含 bounded binding/status/retry/replay/reconcile facts、revision/resource/receipt refs 与 timestamps；不包含 note body、raw request、principal/scope digest、idempotency key、token 或 absolute vault path。scope mismatch 与 missing operation 返回相同 opaque `operation_not_found` shape，避免泄漏存在性。


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
- `--api-token`, `--api-token-file`, `PINAX_API_TOKEN`, and `PINAX_API_TOKEN_FILE` configure a Bearer token. `--api-token-file` means exactly one plaintext bearer secret in an absolute owner-only `0600` regular file; it is not the server hashed registry. The token is sent only in the `Authorization` header and must not appear in stdout, stderr, test fixtures, projection errors, or configuration files.
- An explicit `--vault` is rejected in remote mode with `remote_vault_conflict`; this prevents accidental fallback to a local vault.
- Unsupported commands are rejected with `remote_command_unsupported`; remote mode must not silently execute unsupported commands locally.
- When remote mode comes only from `remote.api_url`, local control/configuration commands (`config`, `api`, `token`, `profile`, `vault`, `cloud`, and `sync`) remain local so users can inspect/update endpoints and manage local-first Cloud Sync state without being hijacked by Remote API Mode.
- Supported first-phase commands are the registered RPC capabilities for project board show, project subproject list/show/create, note list/read/show/preview, project item read and move/archive plan, folder list/show/create/rename/move/delete/adopt/repair, inbox list/show/capture/promote/discard, draft list/show/create/promote/archive/discard, memory list/capture/recall/context/stats, and explicit `sync push` / `sync pull`.
- Full CLI parity is tracked as a capability-by-capability expansion, not a fallback to arbitrary local execution. Read/status/plan commands should be added before write/apply/deploy commands; risky writes must keep the same approval, snapshot, dry-run, receipt, and redaction gates as the CLI path.
- `--json` renders the returned Projection envelope directly as JSON-only stdout; `--agent` renders the same Projection as key=value lines.

Canonical remote client code 位于公开包 `pkg/pinaxclient`。它负责 strict URL、redirect rejection、request/response limit、secure token file、typed manifest/readiness/operation 与 ambiguous mutation recovery。`internal/remoteapi` 继续作为 additive compatibility facade，旧调用方不需要立即迁移，但不得在其中复制第二套安全或 retry 规则。

当 mutation 遇到 timeout、reset、5xx、response read failure 或 malformed/oversized 2xx 时，client 必须先读取同一 `operation_id`，再按需 reconcile；不得生成第二个 operation/key，不得直接 local fallback。只有 durable terminal `failed && retryable && replay_safe` 才允许使用完全相同的 request/identity 最多重试一次。operation 不可见时返回 `mutation_outcome_unknown`，提示运行：

```bash
pinax operation show <operation-id> --api-url <url> --json
pinax operation reconcile <operation-id> --api-url <url> --json
```

## Connection inspect、doctor 与 readiness

```bash
pinax connection inspect --json
pinax connection doctor --json
pinax connection readiness --json
```

`inspect` 只解析 non-sensitive descriptor，不发网络请求；`doctor` 只做 manifest/transport/auth/readiness probes，不执行 domain mutation、credential rotation 或 remote write；`readiness` 返回固定六层 `contract`、`transport`、`auth`、`owner`、`mutation_recovery`、`production`。每层独立使用 `ready|degraded|blocked|not_configured|not_applicable` 与 `exploratory|first-support|mature`，较低层 ready 不得自动推导 production ready。`GET /v1/readiness` 与 `Pinax.Connection.Readiness` 返回同一 projection。

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
| Idempotency key is rebound to another request | non-2xx | `idempotency_conflict` |
| Folder revision precondition differs | non-2xx | `revision_conflict` |
| Operation evidence is insufficient | non-2xx or failed projection | `reconcile_required` |

RPC unknown method returns a failed projection with `error.code=rpc_method_not_found`; the hint must prompt the user to check `pinax api routes`.

## Serve Lifecycle Output

- Default human mode: stdout remains empty. Zap console logs are written to stderr, including startup readiness (`pinax api ready`), the localhost URL, auth mode, write mode, and per-request access logs (`api.request`) with method, path, status, route group, and duration.
- RPC requests also emit an `api.rpc` log with `rpc_method`, optional `rpc_id`, command, route group, readonly/write classification, HTTP status, duration, and projection `error_code` when present. RPC logs must not include params, request bodies, response bodies, note content, raw query strings, Authorization headers, cookies, tokens, or provider payloads.
- In temp-token auth mode, the generated temporary token is printed once to stderr as a startup log field. Request logs must not include Authorization headers, raw query strings, cookies, request bodies, response bodies, or provider payloads.
- `GET /` returns a JSON discovery projection with links to `/v1/capabilities` and runnable `pinax api routes` / schema commands; it is the intended smoke-test path for `curl http://127.0.0.1:<port>/`.
- `--readonly` is the explicit spelling of the default mode; `--allow-write` enables controlled mutation routes. The two cannot be used together.
- `--token-store <path>`: canonical long-lived server auth input; loads a hashed token registry with fine-grained route-group scopes.
- `--token-file <path>`: deprecated compatibility alias for the same hashed registry. It is mutually exclusive with `--token-store` and emits one path-free migration warning in human mode. It does not mean a client plaintext token file.
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
