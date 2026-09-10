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

## 未完成 / 阻塞

- 4.2 Codex、Claude Code、Grok、Kimi Code 真实会话验收：外部真实客户端，本 change 不执行。
- 4.4 默认 runtime 切换：依赖 4.2 证据，未决策。
- 调研核实（1.1）与分层设计（1.2）见 change `design.md`；candidate 未宣称成为默认 runtime。
