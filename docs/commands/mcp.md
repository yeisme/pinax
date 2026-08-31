# mcp 命令

`pinax mcp serve` 启动 local-vault owner 的只读 stdio MCP adapter。它只通过 application service 返回 bounded projection，不直接写 Markdown、`.pinax/**`、SQLite、Git、provider 或 remote state。

```bash
pinax mcp serve --vault ./my-notes
```

stdout 只输出 MCP JSON-RPC frame；诊断、startup、shutdown 与 redacted error detail 写 stderr。进程保持运行直到 stdin EOF、signal 或 context cancellation。

## 双生命周期协议

当前协议与 legacy 初始化路径同时保留，属于 additive compatibility contract。

### Current `2026-07-28`

Current client 不发送 `initialize`。每个请求的 `params._meta` 必须包含：

```json
{
  "io.modelcontextprotocol/protocolVersion": "2026-07-28",
  "io.modelcontextprotocol/clientCapabilities": {}
}
```

连接后先调用 mandatory `server/discover`，再调用 `tools/list`、`tools/call`、`resources/list`、`resources/templates/list` 或 `resources/read`。Current `resources/list` 只列 concrete URI，parameterized URI 只出现在 `resources/templates/list`。

不支持的 current version 返回 `UnsupportedProtocolVersionError`，JSON-RPC code `-32022`，并给出 bounded `requested`/`supported` facts。

### Legacy initialize

以下 legacy version 继续支持标准生命周期：

```text
2025-11-25
2025-06-18
2025-03-26
2024-11-05
```

client 先发送 `initialize`，收到 response 后发送无响应的 `notifications/initialized`，再调用 inventory/read methods。legacy operational request 在 initialize 前返回 `server_not_initialized`；不支持的 initialize version 返回 `-32602`，不会 echo 任意版本假装协商成功。

原有 initialize response 的 `capabilities`、`serverInfo`、`name` 与 `read_only` compatibility fields 均继续保留。现有 MCP tool name 与 resource URI 也未删除或重解释。

## Tool 与 resource 边界

当前 tool registry 包含 search、Agent Brain bounded previews、query/database view、note read/links/backlinks/context、graph summary、project board、task adoption plan、organize plan、snapshot plan，以及 experimental readonly agent memory/handoff reads。准确 inventory 以真实 `tools/list` 与 `pinax api manifest --json` 的 MCP binding 为准。

所有 tools 都是 readonly/plan-only，使用 closed input schema；结果包含 versioned `structuredContent`/schema ref。即使 legacy capability `surfaces` 曾声明 `mcp`，mutation 也不会因此进入 authoritative MCP binding。

新增的 authoritative concrete resources：

| URI | 用途 |
| --- | --- |
| `pinax://manifest` | 与 CLI/REST/RPC 同源的 bounded transport manifest。 |
| `pinax://readiness` | local stdio 的六层 connection readiness。 |
| `pinax://vault/current` | 当前 vault 的 bounded facts。 |
| `pinax://organize/plan` | 只读 organize plan。 |
| `pinax://vault/graph` | bounded graph health。 |

Parameterized resource templates 继续保留，包括 `pinax://note/{note_id}`、`pinax://search/{query}` 与 `pinax://project/{slug}/board`。每个 advertised resource 都必须可由 `resources/read` 返回，不允许只注册不可调用的名字。

## 六层 readiness

`pinax://readiness` 使用 `pinax.connection_readiness.v1`，固定报告：

```text
contract
transport
auth
owner
mutation_recovery
production
```

每层独立返回 `ready|degraded|blocked|not_configured|not_applicable` 与 `exploratory|first-support|mature`。本地 stdio smoke 不能推导成 production ready；当前 MCP 也不会因为 HTTP operation ledger ready 而发布 mutation tool。

## Future sibling MCP 边界

未来 remote-backed `mcp/pinax` 应只依赖公开 `pkg/pinaxclient` 或 stable HTTPS owner API，保持 stateless，不读取 vault、`.pinax/**`、SQLite、Git、provider config 或服务端 token store。Streamable HTTP、OAuth、rate limit 与多用户 permission backend 属于 `mcp/gateway` 或明确的 hosted owner；当前 `cli/pinax` stdio server 不自动获得这些职责。

验证 current/legacy process lifecycle 与实际 resource/tool 调用：

```bash
go test ./internal/mcpserver -run 'Initialize|Protocol|Lifecycle|Modern|Resource' -count=1
```

另见 [`api`](./api.md)、[Remote API Contract](../interfaces/remote-api-contract.md) 与 [Client CLI Parity and Realtime Sync](../interfaces/client-cli-parity-and-sync.md)。
