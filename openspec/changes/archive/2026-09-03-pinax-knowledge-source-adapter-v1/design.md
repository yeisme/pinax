# Design: pinax-knowledge-source-adapter-v1

## Context

Pinax 是 local-first 笔记真源（vault/Markdown）。Inferrum 企业知识平台需要可选 source adapter 消费 allowlisted 投影；root 合同明确 vault 仍为真源、只导出 allowlisted projection、不阻塞 Inferrum native 路线。

## Goals / Non-Goals

**Goals**

1. allowlist 驱动的 projection 导出（refs/digest/permission/citation/freshness/revocation）。
2. 增量变更检测。
3. provider-neutral 包格式（消费端由 Inferrum 拥有）。

**Non-Goals**

- Inferrum DB 直写、完整 vault 导出、检索/QA 实现。

## Decisions

- D1 allowlist 为路径/frontmatter 双条件（显式路径 + allow 标记），默认空（零导出）。
- D2 包格式带 schema version；条目级 revocation 以 tombstone 记录（删除不静默）。
- D3 freshness = 内容 digest + 变更时间；增量按 digest 差集。

## Risks / Trade-offs

- [误导出敏感笔记] → 默认空 allowlist + 双条件 + 导出审计行数。

## Migration Plan

导出命令 → allowlist/审计 → 增量 → Inferrum 消费联调（其 owner change）；additive，rollback 停用命令。

## Open Questions

（无。）
