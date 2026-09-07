# pinax-local-async-substrate Specification

## Purpose
TBD - created by archiving change pinax-local-async-substrate-v1. Update Purpose after archive.
## Requirements
### Requirement: 服务边界 SHALL 保持本地优先

Pinax SHALL NOT 维护常驻服务 daemon 或非 loopback 监听端口。远程消费 SHALL 经 gateway 前挂本地 stdio MCP 或显式 `share`/`publish` 瞬态投影完成；远程 mutation SHALL 仍经 gateway 审批与本地 CLI-authored 结构化资产落地，MCP/tool/provider SHALL NOT 绕过 app service 直接写 vault。

#### Scenario: 远程 Agent 读取笔记

- **WHEN** 远程 Agent 经 gateway 调用 Pinax MCP resource
- **THEN** 请求 SHALL 到达本地 `pinax mcp` stdio 进程
- **AND** Pinax SHALL 无常驻监听端口

#### Scenario: 远程写请求

- **WHEN** 远程请求尝试直接写入 vault 内容
- **THEN** SHALL 经 gateway 审批面与本地结构化资产路径落地
- **AND** MCP 工具 SHALL NOT 绕过 app service 直写

### Requirement: Vault 索引 SHALL 使用统一 WAL DSN 与读池界

Pinax 所有 SQLite 打开点（operation、promptasset、agentmemory、memory ledger 及后续新增）SHALL 使用统一 DSN helper：WAL journal、busy_timeout、foreign_keys；连接池 SHALL 设置显式 MaxOpen/MaxIdle（默认 4/4），保持单写者语义。并发读写同一 vault 索引 SHALL 不产生 `SQLITE_BUSY` 风暴或读串行。

#### Scenario: MCP server 与 CLI 并发读

- **WHEN** `pinax mcp` 与另一 CLI 命令并发读同一 vault 索引
- **THEN** 双方读 SHALL 并行完成
- **AND** 写入方经 busy_timeout 有界等待，不报 BUSY 失败

#### Scenario: 新增打开点回归

- **WHEN** 代码新增 SQLite 打开点
- **THEN** 守卫测试或 review 清单 SHALL 要求走统一 DSN helper
- **AND** 裸 `sqlite.Open(path)` 直连 SHALL 被禁止

### Requirement: Sync 与 delivery 长任务 SHALL 提供 job 投影

执行预算超过阈值（默认 5s，可配置）的 sync/delivery 操作 SHALL 提供 status/cancel 投影与对齐 `--events` NDJSON 的事件模型（accepted/progress/terminal）。status SHALL 可从 event JSONL 幂等重放；cancel SHALL 在项边界停止后续项且已完成项 receipt 保留。同一作用域 SHALL 仅允许一个执行器（本地原子 claim）。

#### Scenario: 长同步可观测

- **WHEN** 用户启动批量 sync 并订阅事件流
- **THEN** SHALL 收到 accepted、逐项 progress、terminal 事件
- **AND** status 投影可从 event JSONL 重放出同一结论

#### Scenario: 取消长同步

- **WHEN** 用户在批量 sync 中途 cancel
- **THEN** 执行器 SHALL 在当前项完成后停止
- **AND** 已完成项 receipt 保留、未执行项不产生部分状态

#### Scenario: 双执行器互斥

- **WHEN** 同一 sync 作用域被并发启动两次
- **THEN** 恰一方持有执行权
- **AND** 另一方 SHALL 收到可解释的占用错误而非静默双跑

### Requirement: MCP stdio SHALL 具备受监督生命周期

`pinax mcp` SHALL 在 stdin EOF 后排空输出并干净退出；崩溃重启后 registry/resource 投影 SHALL 与 vault 状态一致；readiness 投影 SHALL 含 lifecycle 事实，满足 gateway supervise 语义。

#### Scenario: gateway 关闭上游

- **WHEN** gateway 关闭 Pinax stdio MCP 的 stdin
- **THEN** 进程 SHALL 干净退出不留孤儿
- **AND** 重启后 `pinax://readiness` 与 resource 投影一致

