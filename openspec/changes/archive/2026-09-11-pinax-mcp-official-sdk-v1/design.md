## Context

本变更在用户中断设计时仅完成 SDK 可用性调研：官方 Go SDK v1.7.0 覆盖当前已声明的协议版本；尚未完成迁移设计或实现。

## Scope decision

2026-09-10 用户明确取消 Pinax HTML/MCP Apps。移除原任务 3.1（Apps bridge）和 4.3（桌面 UI 验收）；其余任务限普通 MCP 与 Markdown。此处不继续扩展 UI 设计，不将取消任务标为实现完成。原任务文件通过项目临时维护脚本备份后更新。

## Next stage

若继续协议迁移，先完成普通 MCP 的合同、弃用与回滚设计；不宣称已经采用官方 SDK。HTML 退役由 pinax-mcp-collaboration-v1 记录和验证。

## 1.1 调研核实（2026-09-10，源码级）

以下结论直接核对 `github.com/modelcontextprotocol/go-sdk@v1.7.0` module cache 源码与本仓 `internal/mcpserver`：

### 版本与协议面

- 官方 Go SDK 版本取 **v1.7.0**（本机 module cache：v0.8.0/v1.0.0/v1.7.0，取最新稳定）。SDK `go.mod` 要求 go ≥ 1.25.0；本仓 go 1.26.1 满足，无需 toolchain 变更。
- 协议版本完全对齐：SDK `mcp/shared.go` 定义 `2026-07-28 / 2025-11-25 / 2025-06-18 / 2025-03-26 / 2024-11-05`，与本仓 `supportedProtocolVersions()`（modern `2026-07-28` + 四个 legacy）一一对应，无版本缺口。
- 2026-07-28 起 SEP-2577 弃用 roots/sampling/logging；Pinax 未消费这些服务端能力，迁移无影响。

### 现有实现与 SDK 的协议差异清单

| 面 | 现有手写实现 | 官方 SDK v1.7.0 | 迁移处置 |
| --- | --- | --- | --- |
| 私有握手 | `server/discover` + unversioned 私有 envelope（已打弃用诊断） | 不支持私有方法 | candidate 不提供；兼容窗口内仅 legacy runtime 保留，默认切换前必须确认无在用客户端依赖（4.4 决策点） |
| tools/list 结果 | 自定义 `resultType/ttlMs/cacheScope` 缓存字段（在 result 内） | `ListToolsResult` 强类型 struct，自定义字段需走 `_meta` | candidate 放弃顶层自定义字段；缓存提示改 `_meta` 或暂不投影，不影响工具可用性 |
| 工具定义投影 | `standardToolDefinitions()` 手工投影 annotations（readOnlyHint/destructiveHint/idempotentHint/openWorldHint） | `mcp.Tool.Annotations` 原生字段 | 转换层直填原生 annotations |
| schema 校验 | `validateToolArguments()`（maxLength/minimum/enum/additionalProperties 自研校验）+ transportcatalog 注册表 | SDK 输入 schema 经 `jsonschema.Schema` 校验 | transportcatalog 仍是唯一 schema 真源；转换层 map→`jsonschema.Schema`，保留自研校验作为第二道闸（同一 dispatch 单点） |
| 错误 | `MCPError{Code,Message,Data}`（-32601/-32602 等） | SDK protocol error 类型 | 映射层保留 code/data；候选阶段错误码不改名 |
| 取消 | 无 `notifications/cancelled` 处理，仅 stdin EOF 整体退出 | 原生 per-request cancel（`ServerSession.cancel`） | candidate 获得 per-request 取消；dispatch 收到的 ctx 取消后按既有保守恢复路径（同 operation_id 查询，不重放） |
| 分页 | 一次性返回 | `PageSize` 分页 | candidate 单页返回全量（与现行为一致，工具数 ≤32） |
| 日志 | 显式 diagnostics writer | `ServerOptions.Logger`（slog） | candidate 默认丢弃 logger，禁止正文进日志 |

### 能力保留清单（迁移不得丢失）

- 默认工具 20 个（`registry.go` `toolRegistrations()`，全 readonly bounded projection）。
- 协作工具 6 个（`collaborationTools()`：search/read/status/version 恒开 + policy 门控 preview/apply）与使用说明 instructions。
- 输入工具 6 个（`inputTools()`：prepare/status/renew/abort/preview/apply）。
- 资源 3 static（manifest/readiness/vault/current）+ 6 templates + collaboration 资源 + `pinax://input/capabilities`。
- output schema（`mcp.tool.result.v1`）、脱敏 bounded projection、owner 权限、operation 恢复与 preview digest 绑定。
- 已取消的 HTML/MCP Apps 不得以任何形式回归。

## 1.2 分层设计、迁移路径与验收合同

### 分层

```
cmd/pinax mcp（cli 层：参数校验）
  └─ runtime 选择：PINAX_MCP_RUNTIME=official-sdk → sdkruntime；缺省 → legacy（默认不变）
internal/app（领域服务，不动）
internal/mcpserver（统一注册目录 + dispatch 单点）
  ├─ ToolInventory / ResourceInventory / ResourceTemplateInventory（既有导出）
  ├─ Server.Handle（tools/call、resources/read 等 dispatch；schema 校验、错误、脱敏、恢复单点）
  ├─ ServeWithOptions（legacy stdio runtime，默认）
  └─ sdkruntime/（官方 SDK candidate runtime，新增）
       ├─ 注册目录 → mcp.Server 映射（AddTool/AddResource/AddResourceTemplate，原生 annotations）
       ├─ handler 统一委托 Server.Handle（不绕过 app service）
       └─ mcp.NewServer + StdioTransport，Logger 丢弃
```

关键不变量：sdkruntime 不自带业务逻辑，所有调用经 `Server.Handle`，保证两个 runtime 的 schema 校验、错误码、脱敏与恢复语义同源。

### 迁移步骤与回滚

1. 接线 candidate（2.1）：go.mod 增 SDK 依赖；`sdkruntime` 包 + cli env 开关；默认关闭。
2. 迁移面（2.2）：工具/资源/schema/错误/脱敏按上表处置；组件测试用 `NewInMemoryTransports()` 进程内验证。
3. 取消/并发/恢复验证（2.3）：per-request cancel 中断后同 operation_id 查询；并发 tools/call；preview→apply 恢复路径在 candidate runtime 重放既有验收。
4. 真实客户端验收（4.2，外部门）完成前不切默认；legacy runtime 保留为回滚路径（4.4 兼容窗口）。

### 真实客户端验收合同

> 4.4 修正（2026-09-11）：受控探针确认私有 `server/discover`（带协议 `_meta`）经 `Server.Handle` 单点在两个 runtime 均可用；上表"candidate 不提供私有握手"仅对无 `_meta` 错误路径成立。默认切换对私有握手消费者无破坏。

- 自动（4.1）：官方 TS SDK strict client（复用 `tools/testkit/mcpstrictclient.mjs`）+ Inspector + 进程内 Go SDK client；覆盖 20+6+6 工具发现与调用、资源枚举/读取、错误码、大 frame、per-request 取消。
- 真实（4.2，外部）：Codex、Claude Code、Grok、Kimi Code 真实会话；本 change 内不执行、不勾选。
- 切换（4.4）：以上证据齐备后单独决策默认 runtime；未齐备前 default=legacy。
