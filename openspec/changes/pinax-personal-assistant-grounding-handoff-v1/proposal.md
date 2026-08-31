# Proposal：Personal Assistant grounding handoff

## Why

Personal Assistant 已批准只读消费 `pinax.agent.context`、`pinax.agent.memory_recall` 和 `pinax.agent.handoff_read`，但当前 `ContextEntry` 中的 source refs 没有聚合到 `ContextPack.Sources`，三个 MCP 结果也没有统一的 source count/openability evidence。更严重的是，confirmed memory 或 handoff 即使引用的 transcript note 已删除，现有只读 projection 仍可能返回其 bounded summary。

这使 consumer 无法可靠区分 `grounded|partially_grounded|ungrounded|grounding_unavailable`，也缺少 transcript delete 后的负向检索证据。

## What Changes

- 修复 `ContextPack.Sources`：只聚合实际进入 bounded pack 的 entry source refs，按 `kind+ref` 去重。
- 通过 GORM additive 新增 proposal-source 表，保证 source refs 从 propose 经人工 approve 进入 confirmed memory；不修改既有 proposal/memory 列。
- 新增 pre-1.0 `pinax.agent_grounding_evidence.v0.1` projection，返回 canonical source counts、候选来源计数、缺失/过期/歧义计数和 freshness fact。
- 为 context、memory recall 和 handoff read 增加 source-aware readonly projection：全部来源失效的 sourced item 不返回内容；部分可解析时仅保留可解析 refs。
- 三个既有 MCP tool additive 返回 `grounding` 字段，不删除或重命名旧字段。
- 增加四态 consumer fixture 与 transcript note 删除后 context/memory/handoff/search 均不再返回 sentinel 的负向测试。

## Ownership

Pinax 只拥有 source resolution、bounded projection 和 readonly evidence。`grounding` 是 consumer evidence，不是 Personal Assistant turn 的 canonical state；tool timeout/Pinax unavailable 仍由 Personal Assistant 映射为 `grounding_unavailable`。

## Compatibility

三个 Agent MCP surface 均为 experimental/pre-1.0。本变更只增加 optional output、修复当前未填充的 optional `ContextPack.Sources`，并收紧已删除来源的内容投影；数据库只 additive 新增 `agent_proposal_sources` 表，不修改或删除旧表/列；无 CLI command/field 删除、无 Provider 或 credential 变化。
