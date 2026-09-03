# pinax-personal-assistant-grounding-handoff Specification

## ADDED Requirements

### Requirement: Context pack 必须投影实际包含条目的来源

Pinax SHALL 将实际进入 bounded `ContextPack` 的 entry source refs 聚合到 `ContextPack.Sources`，按 `kind+ref` 去重；因 item/char budget 未进入 pack 的 entry source MUST NOT 进入该列表。

#### Scenario: Context 被 budget 截断

- **GIVEN** 两条 confirmed memory 分别引用不同来源
- **WHEN** context budget 只允许一条 memory
- **THEN** `ContextPack.Sources` 只包含被选中 memory 的来源
- **AND** `truncated=true`

### Requirement: Grounding evidence 必须满足四态计数不变量

Pinax SHALL 提供 pre-1.0 `pinax.agent_grounding_evidence.v0.1`，并 SHALL 以 `grounded|partially_grounded|ungrounded|grounding_unavailable` 表达 consumer evidence。Pinax MUST NOT 将 missing/stale/ambiguous source 计为 openable。

#### Scenario: 部分来源可打开

- **GIVEN** 两个候选 source 中只有一个可唯一解析
- **WHEN** readonly consumer projection 生成
- **THEN** state 为 `partially_grounded`
- **AND** `source_total=2`、`source_openable=1`

#### Scenario: 所有候选来源失效

- **GIVEN** sourced content 的全部 source 已删除或不可解析
- **WHEN** readonly consumer projection 生成
- **THEN** state 为 `ungrounded`
- **AND** canonical `source_total=0`、`source_openable=0`
- **AND** `candidate_source_total` 仍记录失效候选数量

#### Scenario: Pinax 不可用

- **WHEN** Personal Assistant adapter 收到 Pinax transport/service error
- **THEN** 它将结果映射为 `grounding_unavailable`
- **AND** canonical source counts 均为 0
- **AND** Pinax 不伪造成功 context pack

### Requirement: Proposal source 必须跨 review round-trip

Pinax SHALL 在 proposal 持久化时保存 bounded source refs，并 SHALL 在 owner approve 创建 confirmed memory 时恢复相同 refs。存储扩展 MUST 使用 GORM additive side table，MUST NOT 修改或删除既有 proposal/memory 列。

#### Scenario: 有来源 proposal 获得批准

- **GIVEN** adapter 提交包含 note source ref 的 memory proposal
- **WHEN** owner 明确 approve
- **THEN** confirmed memory 携带相同 `kind|ref|label|span`
- **AND** context compiler 能把该 ref 聚合到 `ContextPack.Sources`
- **AND** 旧 vault 只新增兼容 side table

### Requirement: 删除来源后不得继续投影来源派生内容

对于 Personal Assistant 使用的 context、memory recall 和 handoff read projection，Pinax SHALL suppress 全部 source 已失效的 sourced item；部分可解析时 SHALL 只保留 openable refs。Pinax SHALL NOT 因此静默确认、删除或改写 canonical memory state。

#### Scenario: 删除 transcript note

- **GIVEN** confirmed memory 与 bounded handoff 均引用一个 transcript note，并包含可识别 sentinel 摘要
- **WHEN** transcript note 被确认删除并形成 tombstone
- **THEN** note search 不返回 sentinel
- **AND** context、memory recall、handoff read 不返回 sentinel
- **AND** grounding evidence 为 `ungrounded`，canonical source counts 为 0
- **AND** tombstone 仍可验证，restore 生命周期未被破坏

### Requirement: MCP 变更必须 additive

`pinax.agent.context`、`pinax.agent.memory_recall` 与 `pinax.agent.handoff_read` SHALL 保持现有 tool 名、request schema、`status`、`command`、`body_exposure` 和 payload key，并 MAY additive 返回 `grounding`。

#### Scenario: 旧 consumer 忽略新字段

- **WHEN** 旧 consumer 只读取既有 result keys
- **THEN** 调用继续成功
- **AND** 不要求迁移、重命名或新的必填参数
