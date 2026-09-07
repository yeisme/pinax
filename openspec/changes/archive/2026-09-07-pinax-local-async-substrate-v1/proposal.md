# Pinax 本地异步基座

## Why

Pinax 是本地优先统一笔记 Agent（vault + SQLite/GORM 索引 + Git），服务消费路径已存在：`pinax mcp`（stdio、六层 readiness）、`share`/`publish` 瞬态 HTTP 投影、sync/delivery receipt。但 2026-09-07 跨项目 review（根合同 `cross-project-async-runtime-substrate` handoff）确认运行时缺口：多处 `gorm.Open(sqlite.Open(裸路径))` 无 WAL/busy_timeout/池界（MCP server 与 CLI 并发读写同一 vault 索引时会踩锁）、sync/delivery 长任务缺 job 投影（无法 status/cancel）、本地优先边界下的服务化路径未冻结。

## What Changes

- 冻结本地优先服务边界：Pinax SHALL NOT 常驻 daemon 或自建公网端口；远程消费 = gateway 前挂本地 stdio MCP + share/publish 显式投影。
- vault 索引 SQLite 统一 DSN（WAL + busy_timeout + foreign_keys）与读池界（MaxOpen/Idle >1，写经 busy_timeout 串行），对齐 sonora 已落地模式。
- sync/delivery 长任务 job 投影：在既有 event JSONL/receipt 模型上补 status/cancel 投影，事件对齐 `--events` NDJSON；本地单执行器原子 claim（不引入分布式租约）。
- MCP stdio 生命周期健壮性：stdin EOF 干净退出、崩溃重启后 registry 投影一致，满足 gateway supervise。

## 不做什么

- 不建常驻服务 daemon、不开公网监听、不做远程鉴权（远程一律经 gateway）。
- 不改 MCP tool 名、resource URI、envelope 字段与既有 receipt 语义。
- 不引入 Postgres 或分布式队列；job 状态保留在本地 vault 结构化资产内。

## Impact

- 实现：`internal/operation/repository.go`、`internal/promptasset/repository.go`、`internal/agentmemory/store.go`、`internal/memory/store.go`（统一 DSN + 池界）、`internal/app/sync*.go`/`delivery`（job 投影）、`internal/mcpserver/`（生命周期）。
- 文档：`docs/` 本地服务边界说明；MCP readiness 补 lifecycle 事实。
