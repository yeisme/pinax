## Why

Pinax 已经同时存在本地 CLI、stdio MCP、loopback REST/RPC、`--api-url` 远程 CLI 和 OpenAPI 导出，但 capability 声明、真实 transport binding、schema、凭据语义与远程写恢复合同尚未收敛，调用方无法可靠判断“声明支持”是否等于“当前可执行”。参考 Anatomia 的 typed owner API、stateless sibling MCP、HTTPS 安全边界、operation reconcile 与分层 readiness，本变更先建立 Pinax 多入口调用的可信合同基础，再把公网 MCP 与多用户服务交给正确 owner。

## What Changes

- 建立 `local-vault`、`remote-service`、`self-hosted-service` 三种连接模式，以及本地 CLI、远程 CLI、Go SDK、loopback REST/RPC、local stdio MCP 和 future sibling MCP 的调用矩阵。
- 将领域 capability 与 REST route、RPC method、MCP tool/resource、remote CLI binding 分离，由真实 adapter registration 编译 authoritative transport manifest；旧 `surfaces` 字段保留兼容，但不再作为唯一可用性判断。
- 扩充 OpenAPI 3.1 导出：增加 typed request/response/error schema、security scheme、operation identity、write gate、readiness 与 capability metadata；不得为未绑定能力生成 path/method。
- 加固 local stdio MCP：协议版本协商、typed input/output schema、`resources/read`、真实 discovery parity、JSON-RPC-only stdout；保持 MCP mutation 默认不发布，plan/read 工具继续复用 application service。
- 新增 `pkg/pinaxclient` typed HTTP client，供 `--api-url` remote mode 和未来 sibling MCP 复用；非 loopback endpoint 必须 HTTPS、禁止 redirect，并采用安全 token file/secret ref。
- 消除服务端 token store 与客户端 plaintext token file 的命名歧义：新增 canonical `pinax api serve --token-store`，保留 `--token-file` 兼容别名至少两个 minor release；客户端 `--api-token-file` 语义不变。
- 为远程 mutation 增加 idempotency key、operation ref、expected revision、receipt ref、status/reconcile 查询和 ambiguous outcome 处理；客户端不得在结果未知时盲目重放写请求。
- 新增分层 readiness projection，分别报告 contract、transport、auth、owner、mutation recovery 与 production 状态、blocker、next action 和 evidence ref，不再用一个布尔值代表完整可用性。
- 明确 split-owner handoff：`mcp/pinax` 负责未来 stateless remote-backed MCP adapter，`mcp/gateway` 负责 Streamable HTTP gateway；hosted HTTPS、多用户身份、组织权限、rate limit 与运营能力必须由根级另行批准 backend owner，`backend-server/pinax-service` 仅为候选名。本变更不创建这些项目或实现其运行时代码。
- 所有合同按 expand-then-contract 演进；本变更不删除现有 CLI、REST/RPC path、MCP tool、JSON/agent field 或 local-first vault 行为。

## Capabilities

### New Capabilities

- `pinax-connection-mode-readiness`: 定义多种连接模式、调用入口、endpoint/credential 安全规则和分层 readiness/doctor 合同。
- `pinax-transport-contract-parity`: 定义 capability 与真实 CLI/REST/RPC/OpenAPI/MCP binding 的权威清单、typed schema、MCP lifecycle 和兼容迁移。
- `pinax-remote-mutation-recovery`: 定义远程写入的 idempotency、revision precondition、operation/receipt、unknown outcome reconcile 和安全重试语义。

### Modified Capabilities

- 无。现有 `pinax-cli-remote-api-mode`、`pinax-web-client-contracts`、`pinax-agent-brain-layer` 和 read-only MCP 行为继续保留；本 change 通过 additive contract foundation 扩展它们，未来删除 legacy 字段或别名必须另立 change。

## Impact

- Owner-fit：`cli/pinax` 为 `fit`，继续拥有 vault/application/projection、本地 CLI、loopback API、local stdio MCP 和 public client SDK。
- Split-owner：future `mcp/pinax`、`mcp/gateway` 和未来由根级批准的 hosted backend 不在本 change 的写路径内，只接收 versioned handoff contract。
- 主要影响：`internal/app`、`internal/domain`、`internal/api`、`internal/mcpserver`、`internal/remoteapi`、`internal/cli`、`internal/config`、`internal/redaction`、新增 `internal/transportcatalog`、`internal/operation` 与 `pkg/pinaxclient`。
- 持久化：远程 mutation operation/receipt 使用 GORM additive schema 或现有 service-authored receipt boundary；禁止 handler/command 直接写结构化资产或硬编码业务 SQL。
- 稳定合同：CLI flags、config keys、REST/RPC、OpenAPI、MCP discovery/tool/resource、public Go API、token file semantics 和 readiness projection。
- 设计来源：根级 `docs/architecture/remote-api-mcp-access.md`、Anatomia `docs/contracts/public-interfaces.md`、`docs/contracts/unified-invocation-mapping.md`、`docs/deployment/video-mcp-readiness-runbook.md`，以及 `mcp/anatomia-video` 的 stateless HTTPS client 与 discovery contract。
