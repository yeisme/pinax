# Pinax Workbench Activity/Logs 查询计划

## 背景

Pinax 已经有多条日志和事件来源：vault event log、sync run receipts、sync daemon events、API audit log、record ledger。Workbench 原型需要一个统一的 Activity/Logs 面板来查看、过滤和打开这些活动，但客户端不能直接读取 `.pinax/**`，也不能绕过 application service。

## 目标

- 新增只读 `pinax activity` 命令族，统一查询现有活动和日志来源。
- 新增 Workbench Activity REST/RPC/capability，让桌面或 Web 客户端通过稳定 projection 消费。
- 保持现有 `pinax sync logs`、`pinax sync daemon logs`、`pinax record history` 行为不变。
- 所有输出复用 Pinax projection envelope，并对 facts、path、message 和 evidence 做脱敏。

## 非目标

- 不新增独立桌面/Web 客户端实现。
- 不新增通用远程 shell、终端会话执行或人工控制台写入日志。
- 不新增 destructive prune；`activity manage` 只报告来源状态和安全维护建议。
- 不改变现有 `.pinax/events.jsonl`、record ledger、sync daemon 或 sync run receipt schema。

## 稳定合同影响

- CLI：新增 `pinax activity sources|list|show|tail|manage`，属于 additive；不删除或改名现有命令。
- CLI output：新增 `activity.*` command projection 和 optional `data` 字段，属于 additive；共享 envelope 不变。
- HTTP/RPC/API：新增 readonly route、RPC method 和 capability，属于 additive；现有 path、method、status 语义不变。
- Stored files：只读读取现有日志来源，不新增或迁移持久 schema。

## 回滚

如果新增能力出现问题，移除 `activity` command、REST/RPC handler、capability registry entry 和 remote mapper；现有日志命令和日志文件保持不变。由于本变更不迁移持久数据，回滚不需要数据修复。
