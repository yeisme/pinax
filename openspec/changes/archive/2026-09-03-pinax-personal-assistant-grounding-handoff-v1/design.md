# Design：source-aware grounding projection

## Data flow

```text
confirmed memory / bounded handoff
        -> ContextPack compiler
        -> aggregate included SourceRef
        -> vault/repository source resolver
        -> keep openable refs, suppress fully revoked sourced content
        -> GroundingEvidence + bounded payload
        -> Personal Assistant consumer maps canonical turn state
```

## Grounding evidence

Schema：`pinax.agent_grounding_evidence.v0.1`。

- `grounded`：至少一个候选来源，且全部可解析；`source_total == source_openable >= 1`。
- `partially_grounded`：部分但非全部候选来源可解析；`0 < source_openable < source_total`。
- `ungrounded`：没有可用来源。为保持 Personal Assistant canonical invariant，`source_total=0`、`source_openable=0`；失效尝试通过 `candidate_source_total` 和 rejected buckets 保留为 evidence。
- `grounding_unavailable`：Pinax/tool 不可用，由 consumer/error adapter 构造；所有 canonical source counts 为 0。

`source_total` 是允许进入 canonical preview 计数的来源分母；`candidate_source_total` 记录解析前候选数量。这样全失效来源不会伪装成 citation，也不会制造违反四态不变量的 `ungrounded + nonzero source`。

## Content filtering

每个 sourced entry/memory/handoff 单独解析：

- 全部 source openable：保留内容与 refs；
- 部分 openable：保留 bounded 内容，只保留 openable refs，整体 evidence 为 partial；
- 全部失效：不返回该 sourced item 的 subject/summary/object/current state；
- 原本无 source 的 bounded item 可保留，但整体状态只能是 `ungrounded`，Personal Assistant 不得把它用于依赖项目事实的 Tier 1 自动执行。

过滤只发生在 Personal Assistant 使用的 readonly projection，不删除 Pinax canonical memory row，不跳过 proposal/review，也不改变 trash/tombstone restore 语义。

## Source resolution

复用 Pinax app service 的 vault object resolver 与 repository source resolver：

- note/asset：只接受唯一、当前可打开的 registered object；
- repository：只允许显式 canonical repo root；缺少 binding 时 unresolved；
- unknown kind：unresolved，旧 consumer 不崩溃；
- stale/ambiguous 不计入 `source_openable`。

## Proposal source persistence

当前 proposal row 不保存 source refs，approve 只能得到空列表。使用 GORM `AutoMigrate` additive 创建 `agent_proposal_sources`：主键由 `proposal_id + kind + ref` 的稳定 digest 产生，字段与 memory/handoff source row 对齐。`SaveProposal` 在同一事务写 proposal 与 sources，`AgentMemoryApprove` 通过 store API 读取后写入 confirmed memory。

旧 vault 自动新增空表；旧 proposal 没有可恢复的历史 source，因此继续按无来源 proposal 处理，不能伪造补录。新 proposal 才获得完整 round-trip。正常读取不硬编码 SQL。

## Failure and rollback

- resolver error 对单个 source fail closed，不泄漏 source body；
- Pinax 整体 error 不伪造 `ungrounded`，由 consumer 标为 `grounding_unavailable`；
- 回滚可同时移除新增 app adapter、MCP optional `grounding` 和新 tests，并恢复 MCP 使用原 readonly methods；`ContextPack.Sources` 的正确聚合可独立保留，因为它符合既有 optional 字段语义；
- proposal-source 表是 additive side table。代码回滚后旧 binary 会忽略该表，数据可保留；只有在确认无新 consumer 后才可由后续维护 change 删除，当前 rollback 不执行 destructive drop。

## Evidence scope

本 change 证明 local Pinax contract/component behavior；不证明运行中的 Personal Assistant API、Hermes adapter、真实 archive encryption 或跨 owner delete orchestration。
