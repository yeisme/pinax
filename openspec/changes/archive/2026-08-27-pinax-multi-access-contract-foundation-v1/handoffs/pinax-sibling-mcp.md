# Future `mcp/pinax` owner handoff

## Status

`retained-next / blocked`。本文件冻结后续 owner、输入合同与验收边界，不授权创建远程仓库、submodule 或运行时代码。只有 `pinax-multi-access-contract-foundation-v1` 完成 public SDK、manifest、readiness 和 process evidence，且根级 OpenSpec 批准 split-owner 后，才能启动 sibling 项目。

## Owner Decision

- Canonical domain owner：`cli/pinax`，继续拥有 vault、application service、projection、body exposure、operation 和 receipt。
- MCP adapter owner：future independent `mcp/pinax` Go subproject。
- Network transport owner：采用 stdio first；stdio 由 sibling 自己持有，Streamable HTTP、session、Origin/Host、rate limit 和 gateway deployment 由 `mcp/gateway` 持有。
- Identity/hosted owner：adapter 不签发用户、组织或 provider credential；它只消费 owner API 已批准的 scoped credential。

## Objective

让 MCP host 能通过真实 MCP lifecycle 访问远程或 self-hosted Pinax owner，同时保持 adapter 无状态、path-free、credential-separated，并复用 `pkg/pinaxclient` 的 HTTPS、token、typed error、readiness 与 reconcile 逻辑。

## Required Inputs

- `pkg/pinaxclient` 的 stable experimental v1：strict transport、manifest、readiness、operation DTO、typed error 和 recovery primitive。
- `pinax.transport_manifest.v1` 与 `pinax.connection_readiness.v1`。
- Owner API 的 typed OpenAPI 3.1、Bearer scope、body exposure 和 error taxonomy。
- MCP 官方 SDK 支持的 protocol version 与 process e2e harness。
- 根级 owner-fit decision、独立 remote repository 和 submodule 创建授权。

## Public SDK Method Ledger

Sibling 只能消费 public package。当前已存在且可作为 bootstrap contract 的方法：

| Method | 用途 | Sibling 使用边界 |
| --- | --- | --- |
| `pinaxclient.New(Config)` | 构造 strict owner client。 | 非 loopback 必须 HTTPS；redirect 始终拒绝。 |
| `Ping` | bounded root smoke。 | 只用于 transport evidence，不替代 manifest。 |
| `Capabilities` | legacy capability compatibility。 | 不能单独决定 tool discovery。 |
| `Manifest` | authoritative capability/binding discovery。 | discovery 的主输入。 |
| `Readiness` | 六层 readiness。 | 不得把低层 ready 提升为 production ready。 |
| `Operation` | 读取同一 operation status。 | scope mismatch 与 missing 必须保持 opaque。 |
| `ReconcileOperation` | 从 owner evidence reconcile。 | 不 replay domain mutation。 |
| `CallRPC` | 受控 generic RPC compatibility。 | 只允许 manifest 中 available、readonly、body-bounded 的 allowlist method。 |
| `NewMutationIdentity` / `CallMutationRPC` | durable mutation recovery primitive。 | first slice 禁止用于发布 write tool；只为后续独立 mutation change 保留。 |

以下 typed domain methods 当前尚不存在，是 sibling 实现前的 blocking SDK follow-up；不得在 sibling 内用复制 HTTP/RPC client 绕过：

```go
ListNotes(ctx, NoteListRequest) (NoteList, error)
ReadNote(ctx, NoteReadRequest) (NoteDisplay, error)
Search(ctx, SearchRequest) (SearchResult, error)
MemoryContext(ctx, MemoryContextRequest) (ContextBundle, error)
```

每个 typed method 必须验证 response schema/capability id/body exposure/resource refs，并有 response limit、malformed 2xx、scope denial 与 redaction tests。若这些方法未稳定，handoff 状态保持 blocked；可以完成 manifest/readiness/operation-only scaffold spec，但不能发布 note/search tools。

## First Slice

First slice 只发布 read/inspect surface，不发布 create/edit/rename/delete/apply。与 local MCP 已存在的能力必须复用原 tool name；新增 name 只能 additive：

```text
pinax.search
pinax.note.read
pinax.note.list              # additive; requires typed SDK follow-up
pinax.memory.context         # additive; requires typed SDK follow-up
pinax.operation.status       # additive readonly operation lookup
```

Manifest/readiness 优先作为 resource，避免重复发布无参数 tool。建议资源 taxonomy：

```text
pinax://manifest
pinax://readiness
pinax://note/{note_id}
pinax://search/{query}
pinax://operations/{operation_id}
```

现有 `pinax.search`、`pinax.note.read`、`pinax://manifest`、`pinax://readiness`、`pinax://note/{note_id}` 与 `pinax://search/{query}` 不得删除或重解释。Sibling 不需要复制完整 legacy inventory；最终新增名称由根级 OpenSpec 对照 owner manifest 和 local MCP registry 冻结。

## Scope、readiness 与 discovery matrix

| Surface | Required owner facts | 发布规则 |
| --- | --- | --- |
| manifest | `contract` 可解析 | 即使部分 capability blocked，也可发布 bounded manifest resource。 |
| readiness | transport response 可解析 | 原样保留六层 status/maturity/blocker/evidence，不自行合成。 |
| note list/read/search/context | auth accepted；对应 capability available；read scope/group 允许；owner ready/degraded | 任一事实 unknown/blocked 时不发布 tool，或在协议允许时标记 unavailable；不得调用后再猜 scope。 |
| operation status/resource | operation capability available；principal/scope 与 owner ledger binding 匹配 | mismatch 与 missing 返回同一 not-found shape；resource URI 只含 opaque operation id。 |
| future mutation | `mutation_recovery=ready`；write scope；approval/revision/receipt policy 完整 | 必须另立 change；first slice 永不发布。 |

Sibling readiness 只能取 owner 返回事实与 adapter 自身 process/transport evidence 的交集。`production` 层必须由实际 deployment owner 证明；本地 stdio/process tests 只能证明 contract/transport adapter readiness。

Operation/resource refs 是恢复和审计的唯一跨边界 identity。Tool result 可以返回 bounded `operation_ref`、`resource_ref`、`revision_ref`、`receipt_ref`；不得返回 vault-relative content path 之外的 host path、DB primary key、request digest、principal/scope digest 或 idempotency key。

## Runtime Contract

```mermaid
flowchart LR
    HOST[MCP Host] -->|stdio JSON-RPC| ADAPTER[mcp/pinax]
    ADAPTER --> SDK[pkg/pinaxclient]
    SDK -->|HTTPS; loopback HTTP only| OWNER[Pinax Owner API]
    OWNER --> APP[Pinax Application Service]
```

- Adapter MUST NOT 读取 owner vault、`.pinax/**`、SQLite/GORM、Git、provider config、token store 或 host filesystem path。
- Adapter MUST NOT shell out 到 `pinax` CLI，也不得 import `github.com/yeisme/pinax/internal/...`。
- Adapter MUST NOT 手写第二套 URL、redirect、token file、typed error、operation retry 或 reconcile client；这些规则只来自 `pkg/pinaxclient`。
- Adapter MUST NOT 把 legacy capability `surfaces` 当作 authoritative availability，也不得发布 manifest 标记 `planned|blocked|future_owner` 的 binding。
- Endpoint 非 loopback 时 MUST 为 HTTPS，redirect MUST 被拒绝。
- Token file MUST 使用 public SDK 的 owner-only secure file policy；raw env token 只允许 loopback development。
- `tools/list`/`resources/list` MUST 按 principal scope、manifest available binding 与 readiness 发布真实 backed surface。
- Owner 返回 malformed 2xx、identity mismatch 或 ambiguous mutation 时，adapter MUST 保留 typed error/operation ref，不得伪造成功。
- stdout 只允许 MCP JSON-RPC；diagnostics 只写 stderr。

## Mutation Gate

V1 不发布 mutation tool。后续 write surface 必须另立 OpenSpec，并同时具备：

- owner capability manifest 报告 `mutation_recovery=ready`；
- MCP principal scope 与 approval facts；
- client-generated operation id/idempotency key；
- status/reconcile resource；
- plan/dry-run/yes/snapshot/receipt 与 body exposure policy；
- unknown outcome process e2e，证明不 blind retry。

## Acceptance Criteria

- 官方 MCP SDK process e2e 至少完成 current `server/discover -> tools/list -> tools/call -> resources/list/templates/list -> resources/read -> close`；若 sibling 声明 legacy compatibility，还必须单独完成 `initialize -> notifications/initialized -> inventory/read -> close`。
- Discovery 与 owner manifest 精确相交，不公布 planned/blocked/future-owner capability。
- Adapter 只通过 `pkg/pinaxclient`/HTTPS 调用 owner；静态 guard 证明无 internal import、vault/DB access 和 CLI shell-out。
- 非 loopback HTTP、redirect、unsafe token file、scope denial、malformed 2xx、oversized response 均 fail closed。
- Tool/resource output path-free、body-bounded、secret-free；operation/resource identity 与 owner response 一致。
- Integration evidence 写入 sibling project 的 `temp/integration-test-runs/<run-id>/`。

## Validation Commands

未来 sibling 项目至少提供：

```bash
go test ./...
go test -race ./internal/mcpserver ./internal/owneradapter
if rg -n 'github.com/yeisme/pinax/internal|os/exec|\.pinax|operations\.sqlite' --glob '*.go' .; then exit 1; fi
task integration:mcp-protocol
openspec validate --all
task check
```

## Rollback

- 停用或卸载 sibling MCP 不影响 `cli/pinax` local CLI、local stdio MCP、loopback API 或 remote CLI。
- Streamable HTTP 未成熟时继续只发布 stdio；不得用 REST bridge 冒充 MCP transport。
- Owner SDK/schema 不兼容时 adapter readiness 报告 blocked，工具从 discovery 移除或明确 unavailable，不 fallback 到 filesystem/CLI。
