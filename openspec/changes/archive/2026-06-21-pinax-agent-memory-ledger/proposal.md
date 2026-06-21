## Why

Pinax 已经具备基于 LanceDB 的语义 KB 投影，但 agent 长期协作需要的“记忆”不应只依赖向量相似度。工程记忆经常是确定性的：项目约定、设计决策、发布事件、后续承诺、配置来源、失败原因和证据路径。它们需要可解释、可审计、可过期、可引用，而不是只返回“看起来相关”的片段。

本变更为 Pinax 设计一个非向量的 agent memory ledger：以 Markdown vault 为真源，以 SQLite/GORM 维护 typed records、FTS5 全文检索和实体关系索引，为 agent 提供带来源、状态、置信度和召回原因的上下文包。

## What Changes

- 新增 `pinax memory` 命令族规划：`capture/list/recall/context/link/prune/stats`。
- 新增 4 类核心记忆类型：`fact`、`decision`、`event`、`task`。
- 新增 agent-safe 输出合同：`--json` 返回引用、状态、recall_reason；`--agent` 输出稳定 key=value facts。
- 新增本地 SQLite/GORM projection，包含 typed records、source citations、entity links、supersession/expiry 状态和 FTS5 索引。
- 新增从 Markdown 笔记、OpenSpec、release evidence、git metadata 提取候选记忆的 service boundary，但第一版不自动写入未确认记忆。
- 保持向量 KB 为可选增强；本 change 不要求 LanceDB、embedding provider 或 Python sidecar。

## Compatibility

本变更是新增合同，不移除或重命名现有命令、输出字段、配置键、数据库表或 KB 行为。

受影响合同面分类：

- CLI 命令：新增 `pinax memory`，additive。
- CLI `--json` / `--agent` 输出：新增 command 和 facts，additive。
- 配置键：新增 `memory.*`，additive。
- SQLite/GORM schema：新增 memory ledger tables 和 FTS projection，additive。
- OpenSpec：新增 `agent-memory-ledger` spec，additive。

## Non-Goals

- 不在第一版实现向量召回、LLM 自动总结或隐式长期画像。
- 不让 agent 直接手写 `.pinax/` 结构化资产。
- 不同步本地 memory projection 到 Cloud Sync 作为权威数据。
- 不引入 daemon、Web UI、远端 Pinax Cloud memory 服务。
- 不把未经用户确认的推断记忆标成 confirmed fact。
