# Proposal: Pinax MCP Readonly Surface

## 背景

Pinax 的定位是统一笔记 Agent CLI。主线本地 vault 能力落地后，agent 需要一个稳定、只读、可发现的入口来查询 vault、读取笔记和预览整理计划，而不是直接读写 `.pinax/` metadata。

## 目标

- 提供 `pinax mcp serve --vault <path>` 的本地 stdio MCP surface。
- 暴露只读 resources/tools：vault current、note read、search、organize plan、git snapshot plan。
- MCP 调用必须复用 CLI application service、projection 和 redaction 规则。
- 写能力和 provider remote write 不进入 MVP MCP surface。

## 非目标

- 不接入 `mcp/gateway` 的 HTTP、多租户、集中审批或预算策略。
- 不暴露 `metadata apply`、`organize apply`、`sync push` 等写操作。
- 不实现长期 daemon。

## 影响

- 子项目代码：`cmd/pinax`、`internal/mcpserver`、`internal/app`、`internal/output`。
- 依赖主线 `pinax-local-vault-organize` 的 vault scan、note read、search 和 organize plan services。
