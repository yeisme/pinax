# Personal Assistant Grounding Handoff

本文定义 Pinax 向 Personal Assistant 提供可信上下文、长期记忆和 bounded handoff 的本地只读合同。Pinax 只拥有来源解析、内容过滤和 grounding evidence；Personal Assistant 仍拥有会话状态、风险分级、审批、执行和承诺闭环。

## 消费入口

Personal Assistant 通过以下既有 MCP 工具读取 Pinax：

- `pinax.agent.context`：返回 bounded `ContextPack`。
- `pinax.agent.memory_recall`：返回经过来源检查的 confirmed memory projection。
- `pinax.agent.handoff_read`：返回经过来源检查的 bounded handoff projection。

三个工具保持原有名称、请求 schema 和 `status`、`command`、`body_exposure`、payload key，只增量返回 `grounding`。旧 consumer 可以忽略新字段继续工作。

## Grounding evidence

当前 pre-1.0 schema 为 `pinax.agent_grounding_evidence.v0.1`：

| state | 含义 | canonical count 不变量 |
|---|---|---|
| `grounded` | 至少一个候选来源，且全部可唯一打开 | `source_total == source_openable >= 1` |
| `partially_grounded` | 只有部分候选来源可打开 | `0 < source_openable < source_total` |
| `ungrounded` | 没有可用于 canonical preview 的来源 | `source_total=0`、`source_openable=0` |
| `grounding_unavailable` | Pinax transport 或 service 不可用 | `source_total=0`、`source_openable=0` |

`candidate_source_total` 记录解析前候选数；missing、stale 和 ambiguous bucket 记录拒绝原因。它们不会伪装成 citation，也不会计入 `source_openable`。

Personal Assistant 必须把 `grounding_unavailable` 与成功但无来源的 `ungrounded` 区分开。`ungrounded` 内容不得用于依赖项目事实的 Tier 1 自动执行；consumer 可把它降级为无来源草稿或要求用户确认。

## 来源和内容过滤

Pinax 复用 vault object resolver 与 repository source resolver：

- note/asset 只接受唯一、当前可打开的 registered object；
- repository 只接受显式 canonical repository root；
- unknown kind、missing、stale 或 ambiguous source 均 fail closed；
- `ContextPack.Sources` 只聚合实际进入 bounded pack 的 entry refs，并按 `kind+ref` 去重；被 item/char budget 截掉的 entry 不贡献来源。

对每个 sourced entry、memory 或 handoff：

- 全部来源可打开：保留 bounded 内容和全部 refs；
- 部分来源可打开：保留 bounded 内容，但只返回 openable refs；
- 全部来源失效：从 Personal Assistant projection 中移除该来源派生内容；
- 原本无来源的 bounded item 可以保留，但整体只能标为 `ungrounded`。

过滤只作用于 Personal Assistant 的只读 projection。Pinax 不会据此删除或改写 confirmed memory，也不会绕过 proposal/review。

## Proposal 到 confirmed memory

有来源的 memory proposal 使用 GORM additive side table `agent_proposal_sources` 保存 refs。owner approve 时，Pinax 从 store 恢复 `kind`、`ref`、`label` 和 `span`，再写入 confirmed memory source rows。

旧 vault 通过 `AutoMigrate` 增加空 side table，既有表和列不变。旧 proposal 没有可恢复的历史来源，继续作为无来源 proposal 处理；Pinax 不伪造回填。

## 删除和负向检索保证

确认删除 transcript note 后，Pinax 会刷新可重建索引。若刷新失败，删除结果标为 partial、index status 标为 stale，并返回显式 rebuild action。

对已成功删除且形成 tombstone 的 note：

- search 不再返回正文或 sentinel；
- context、memory recall 和 handoff read 不再返回只由该 note 支撑的 subject、summary、object 或 current state；
- grounding 变为 `ungrounded`，canonical source counts 为 0，missing bucket 保留拒绝证据；
- canonical confirmed memory row 仍存在；
- trash tombstone 和 restore 生命周期保持可验证。

这项保证是 Pinax local owner contract，不等同于 Personal Assistant 加密 archive 的跨 owner 删除编排。archive 正文、索引和媒体引用的清理仍由 Personal Assistant owner 负责，并需要 system test 验证。

## 兼容与回滚

- MCP 输出仅新增 optional `grounding`，不移除旧字段。
- `ContextPack.Sources` 修复既有 optional 字段的聚合语义。
- 数据库只增加 side table；代码回滚后旧 binary 会忽略它，当前 rollback 不执行 destructive drop。
- 可以独立撤回 PA adapter 和 MCP optional 字段，同时保留正确的 context source 聚合和 proposal source 数据。

## 验证

在 Pinax 子项目运行：

```bash
go test ./internal/agentcontext ./internal/agentmemory -run 'PersonalAssistantSources|ProposalSources' -count=1
go test ./internal/app -run 'PersonalAssistantGrounding|PersonalAssistantTranscriptDelete' -count=1
go test ./internal/mcpserver -run 'PersonalAssistantMCPGrounding|AgentMemoryMCP' -count=1
openspec validate pinax-personal-assistant-grounding-handoff-v1 --strict
```

生成脱敏 integration/e2e evidence：

```bash
go run ./tools/testkit/integrationevidence --profile personal-assistant-grounding
```

该命令把 `summary.json`、命令、stdout/stderr、环境摘要和 artifacts 写入 `temp/integration-test-runs/<run-id>/`。最终 package gate 还应覆盖 `internal/agentcontext`、`internal/agentmemory`、`internal/app` 和 `internal/mcpserver` 的完整测试集，并对当前 worktree 的既有或并发失败单独归因。
