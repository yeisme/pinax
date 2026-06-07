# Tasks: Pinax MCP Readonly Surface

## 1. OpenSpec 和依赖边界

- [x] 1.1 完成本 change 的 proposal/design/tasks/spec。
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: `pinax-local-vault-organize` app service design
  - Acceptance: `openspec validate pinax-mcp-readonly-surface`
  - Failure re-check: 如果 MCP 写能力进入本 change，移出范围。
  - Evidence: 2026-06-06 运行 `openspec validate pinax-mcp-readonly-surface`，退出码 0，显示 change valid。

## 2. MCP 只读 server

- [x] 2.1 用 TDD 增加 MCP initialize、resources/list、tools/list 和 tools/call 测试。
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: local vault service
  - Acceptance: `go test ./internal/mcpserver ./cmd/pinax -run MCP -count=1`
  - Failure re-check: 如果测试需要真实 MCP client 或长期进程，改用 stdin/stdout buffer fixture。
  - Evidence: 2026-06-06 先运行聚焦测试，因缺少 `NewServer`、`Request`、`Tool`、`Resource` 失败；实现后运行 `go test ./internal/app ./internal/mcpserver ./cmd/pinax -run 'Vault|Note|Search|Init|Validate|Metadata|Organize|MCP|Agent|LocalVault' -count=1`，退出码 0。

- [x] 2.2 实现 `pinax mcp serve --vault <path>` 和只读 adapter。
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 2.1 red
  - Acceptance: `go test ./internal/mcpserver ./cmd/pinax -run MCP -count=1`
  - Failure re-check: 如果 MCP 绕过 app service 直接读写 `.pinax/`，退回 adapter 边界。
  - Evidence: 2026-06-06 新增 `internal/mcpserver`，`pinax.search`、`pinax.note.read`、`pinax.organize.plan` 路由到 `internal/app` service；聚焦测试退出码 0。

## 3. 合同和验证

- [x] 3.1 增加 MCP output/redaction 合同测试。
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 2.2
  - Acceptance: `go test ./internal/mcpserver ./internal/output -count=1`
  - Failure re-check: 如果响应包含 secret、raw payload 或写 tool，阻塞发布。
  - Evidence: 2026-06-06 MCP 测试确认 tools/list 不包含 `pinax.organize.apply`，未知写工具返回错误；运行 `go test ./...`，退出码 0。

- [x] 3.2 更新文档并运行完整门禁。
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: 3.1
  - Acceptance: `task check`
  - Failure re-check: 如果本机无 `task`，运行 `go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`、`openspec validate --all`。
  - Evidence: 2026-06-06 已更新 README 和 docs 的 MCP 只读说明；运行 `task check`，退出码 0。
