# mcp 命令

`pinax mcp serve` 启动 local-vault owner 的只读 stdio MCP adapter。它只通过 application service 返回 bounded projection，不直接写 Markdown、`.pinax/**`、SQLite、Git、provider 或 remote state。

```bash
pinax mcp serve --vault ./my-notes
```

stdout 只输出 MCP JSON-RPC frame；诊断、startup、shutdown 与 redacted error detail 写 stderr。进程保持运行直到 stdin EOF、signal 或 context cancellation。

可选 `--collaboration` 增加独立的 Markdown / 结构化交互工具；正文读取和日常写入分别要求 `--allow-note-body` 与 `--allow-note-write`，不会改变旧工具的只读行为。完整合同、Codex 接入和测试方法见 [MCP 人机笔记协作](../interfaces/mcp-collaboration.md)。本页后续只读说明指默认连接与原有工具；新增能力以实例 discovery 为准。

## 已连接 MCP、客户端未安装 Pinax

`pinax mcp serve` 在拥有 vault 的 MCP host 上运行。调用方已连接时，不需要在自己的机器上安装 Pinax、克隆 vault 或运行 CLI doctor。客户端路径与 host 的 vault 路径不是同一个文件系统。

完成下方对应版本的协议生命周期后，通过 `resources/read` 读取：

```json
{"uri":"pinax://manifest"}
```

```json
{"uri":"pinax://readiness"}
```

工具名及完整 inputSchema 直接来自 `tools/list`；参数化资源来自 `resources/templates/list`。以本次连接的名称为准，Gateway 可能改写前缀。`pinax api manifest --json` 只是已安装 CLI 时的等价检查，不是 MCP 客户端的前置条件。

| 意图 | 无 CLI 客户端的处理 |
| --- | --- |
| 搜索、读笔记、读项目板、生成只读计划 | 使用真实 tools/list schema 或 advertised resource，保留返回的 note/project refs |
| 上传附件、导入本机文稿、保存笔记、执行计划 | 当前 MCP 只读，未提供这些写动作或上传协议；报告能力缺口，由已授权的 owner CLI/application service 完成后再读取 |
| 服务返回路径或导出建议 | 路径属于 owner，不能当作客户端已有文件或下载 URL；需要实际交付合同 |

不能通过猜测 `pinax.upload`、`note.add` 或发送 base64 绕过只读边界。输入 schema 不包含的字段不要从 CLI flag 推导。目录、附件或笔记正文不能由 MCP 直接写入客户端或绕过 application service 写入 vault。无法取得 schema 时，刷新 discovery 一次后报告具体缺口；不要让无 CLI 客户端重复执行不存在的本机命令。

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

显式协议版本的 stdio 响应只在 JSON-RPC 顶层携带 `jsonrpc`、`id`、`result` 或 `error`；列表从 `result.tools` / `result.resources` 读取。顶层 `tools` / `resources` 会被严格 MCP SDK 拒绝，表现为连接成功但列表超时。内部 Go Handle API 保留兼容投影；不带 protocolVersion 的历史私有 initialize 暂时保留旧投影并在 stderr 提示弃用，至少保留一个发布版本，本次不删除。请迁移到显式版本握手与标准 result 字段。

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

## 可选文件输入候选实现

新增入口、无产品 CLI 自动上传、一次性页面和原任务恢复见 [MCP 文件输入](../mcp-input-intake.md)。新入口默认关闭；旧只读或未部署连接继续按实时 capabilities 处理。

## 官方 SDK candidate runtime（实验）

`PINAX_MCP_RUNTIME=official-sdk` 显式启用基于官方 Go MCP SDK（`github.com/modelcontextprotocol/go-sdk` v1.7.0）的候选 stdio 运行时；缺省仍是既有手写实现，默认行为不变。两个 runtime 共享同一工具/资源注册目录与 `internal/mcpserver` dispatch 单点，schema 校验、错误码与脱敏语义同源。

已知差异（默认切换评审前需复核）：

- 未注册工具由 SDK 在 server 侧以 `-32602`（unknown tool）拒绝；legacy runtime 为 `-32601`。
- 私有 `server/discover` 握手与 unversioned 私有 envelope 在 candidate 中不可用。
- `tools/list`/`resources/list` 顶层的 `resultType/ttlMs/cacheScope` 缓存提示不投影。
- stdio 客户端关闭 stdin 即终止会话（EOF 归一化为零退出）；真实客户端在会话期间保持管道打开，不受影响。

验证：

```bash
go run ./tools/testkit/integrationevidence --profile mcp-official-sdk
PINAX_MCP_RUNTIME=official-sdk PINAX_TEST_BINARY=dist/pinax PINAX_MCP_SDK_DIR=<sdk-dir> \
  go run ./tools/testkit/integrationevidence --profile mcp-strict-client
```

默认切换（`pinax-mcp-official-sdk-v1` 4.4）要求先完成 4.2 的 Codex、Claude Code、Grok、Kimi Code 真实会话验收。
