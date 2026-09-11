# 官方 SDK candidate runtime 实现验证记录（pinax-mcp-official-sdk-v1）

## 实现范围

- `internal/mcpserver/sdkruntime`：官方 Go SDK v1.7.0 candidate runtime。注册目录映射（20 默认工具、协作 6、输入 6；3 static 资源、6 模板、协作资源、`pinax://input/capabilities`），annotations 与 legacy 投影一致；所有 handler 委托 `mcpserver.Server.Handle` dispatch 单点，schema 校验、错误码、脱敏与恢复语义同源。
- `internal/cli/mcp_cmd.go`：`PINAX_MCP_RUNTIME=official-sdk` 显式开关；缺省仍走 legacy 手写 runtime。
- go.mod/vendor 接入 `github.com/modelcontextprotocol/go-sdk v1.7.0`。
- `mcpserver` 导出协作/输入 inventory 访问器（`CollaborationToolInventory` 等）供双 runtime 复用同一注册目录。
- 取消语义：per-request cancel 由 SDK 原生处理；stdio EOF 归一化为零退出（candidate 与 legacy 退出语义对齐）。

## 已通过验证

| 验证 | 结果 / 证据 |
| --- | --- |
| 组件 + 进程级（discovery/调用/错误码/协作 preview→apply→status/version/并发 8 路/取消/stdio 分帧） | `go run ./tools/testkit/integrationevidence --profile mcp-official-sdk`；`temp/integration-test-runs/20260910T191028Z-2676955/summary.json`（含 `go test -race` 单独通过） |
| 官方 TS SDK strict client 对 candidate | `PINAX_MCP_RUNTIME=official-sdk` + `--profile mcp-strict-client`；`temp/integration-test-runs/20260910T191047Z-2682741/summary.json`；sdk=1.30.0，legacy 协作双模式 PASS，20/26 工具，写闭环，protocol_errors=0 |
| 官方 Inspector v2 CLI 对 candidate | `--profile mcp-inspector` 三方法：tools/list（20）`20260910T191223Z-2713262`、resources/list `20260910T191229Z-2715309`、tools/call pinax.git.snapshot_plan `20260910T191236Z-2716969`，全部 passed |
| legacy runtime 回归 | 同 strict client 脚本对默认 runtime PASS（20/26 工具、protocol_errors=0）；全量 `go test -count=1 ./...` 绿 |
| 输入工具接线 | `TestSDKRuntimeInputIntakeToolsWired`：6 个 pinax.input.* 工具与 `pinax://input/capabilities` 资源经 candidate 可发现、可读取 |
| 全局门禁 | `task check`（fmt-check/lint/test/build/openspec validate --all）通过 |

## 已冻结差异（design.md「1.1 调研核实」差异表）

- 未注册工具：SDK server 侧 `-32602 unknown tool`；legacy `-32601`。
- 私有 `server/discover` 与 unversioned envelope：candidate 不提供（e2e 反向断言覆盖）。
- 顶层缓存提示字段（resultType/ttlMs/cacheScope）：不投影。
- SDK stdin EOF 即拆除会话：真实客户端保持管道打开不受影响；EOF 归一化零退出。

## 4.2 真实客户端验收（2026-09-11 完成）

`temp/mcp-client-validation-20260910-rEyDgS`（四客户端同 vault 同 server，配置均带 `PINAX_MCP_RUNTIME=official-sdk`；探针以未注册工具错误码 `-32602` vs legacy `-32001` 区分 runtime；`bin/pinax` 自 7953f75 重建——原 09-10 快照早于 sdkruntime 接线）：

| 客户端 | 结果 |
| --- | --- |
| Codex 0.154.0（exec） | 全清单通过：26 工具、capabilities 资源、搜索、正文读取、preview→apply→status→旧版本回读 |
| Claude Code 2.1.208（-p） | 全清单通过；含末尾无换行 append 边界二次预览、跨客户端陈旧预览 apply 被 `revision_conflict` 拒绝、安全渲染 `untrusted_user_content` 纯文本传递 |
| Kimi Code 0.42.0（-p） | 全清单通过（经用户级 `~/.kimi-code/mcp.json`；`-p` 模式不加载项目级配置，即使授予 workspace trust） |
| Grok 1.0.5 | 协议层握手+26 工具正常（doctor）；会话端把 `pinax-validation__pinax.*` 点号工具名判为 invalid/ambiguous 全部跳过（26 skip→tool_count=0→connection failed）。legacy runtime 对照完全一致 ⇒ grok 1.0.5 预存限制而非候选回归 |

服务端回执与 transcripts：同目录 `evidence/` 与 `validation-report.json`（`real_agent_conversations_verified=true`，`model_calls=9`）。

## 4.4 默认切换（2026-09-11 决策）

- **决策：默认 runtime 切换为官方 SDK**；legacy 保留为兼容窗口回退路径（`PINAX_MCP_RUNTIME=legacy`），未知取值 fail closed（`internal/cli/mcp_cmd.go`）。
- 依据：4.1 自动验收（TS SDK strict client + Inspector + 进程级 e2e）与 4.2 三客户端真实会话全通过；grok 限制为双 runtime 预存且已记录；在用注册消费方（hermes personal profile）走标准 stdio 工具调用。
- **冻结表修正**：受控探针确认私有 `server/discover`（带协议 `_meta`）经 `Server.Handle` 单点在两个 runtime 均可用，仅 legacy `read_only` envelope 字段不投影——比原"candidate 不提供私有握手"的冻结结论更安全，私有握手消费者切换后不受影响。
- 全局门禁：`task check`（fmt-check/lint/`go test ./...`/build/`openspec validate --all`/renderer test）通过；新增 `TestMCPRuntimeDefaultSwitch`、lifecycle 默认段与 parity 管道化适配。
