# pinax-workbench-continuity-projection Specification

## Purpose
TBD - created by archiving change pinax-workbench-continuity-projection-v1. Update Purpose after archive.
## Requirements
### Requirement: Typed continuity projection facade MUST 固化 machine envelope 合同
Pinax MUST 为 `pinax continue` machine data、binding status 诊断与 `continue checkpoint` 输出提供 versioned typed envelope（合同 identity/version/digest、稳定错误码、唯一恢复 action），供 Workbench BFF 消费。Envelope MUST NOT 携带 raw note/handoff 正文、transcript、provider payload、credential 或绝对路径。

#### Scenario: Workbench 消费 Resume Card machine data
- **WHEN** Workbench BFF 以 machine 输出调用 `pinax continue`
- **THEN** 返回 envelope MUST 声明合同 identity/version/digest，并保持 dogfood 冻结的 bounded 字段语义
- **AND** stale/conflict/missing 状态 MUST 以稳定错误码与恢复 action 透传，不得被 Workbench 端猜测或静默修复。

### Requirement: Safe projectRef 映射 MUST 保持 Pinax binding 权威
Workbench 与本 facade 之间 MUST 只使用 opaque `projectRef`。Binding 解析、vault/scope 选择与歧义判定 MUST 保留在 Pinax CLI-authored binding registry；missing/ambiguous/invalid MUST 原样返回对应稳定错误码与唯一下一步（bind/inspect/rebind），不做跨 vault 搜索或目录名匹配。

#### Scenario: projectRef 未绑定
- **WHEN** Workbench 携带的 projectRef 无法解析到 enabled exact binding
- **THEN** facade MUST 返回 `binding_status=missing` 与 bind next action
- **AND** MUST NOT 选择其他 vault/scope 或声称已识别项目。

#### Scenario: 多 enabled exact bindings
- **WHEN** 同一 repository 存在多个 enabled exact binding 记录
- **THEN** facade MUST fail closed 返回 `continuity_binding_ambiguous`
- **AND** MUST 提供 inspect/rebind recovery action，不任意选择。

### Requirement: Provider packet 与投影时效 MUST 可被根仓接收
Pinax MUST 提供 provider packet 输出：合同 identity/version/digest、支持动作及 effects、scope/revision、receipt、错误/恢复语义、availability、固定调用入口与脱敏证据引用。Workbench 持有的投影 MUST time-bounded：freshness 由 evidence observed/revision 推导，过期只能重新解析。

#### Scenario: packet 生成
- **WHEN** 根仓请求 provider packet
- **THEN** Pinax MUST 输出上述完整清单且不包含私密 vault 内容
- **AND** 每个动作有唯一合同 identity/version 与稳定错误/恢复语义。

#### Scenario: 投影过期
- **WHEN** Workbench 持有的 continuity 投影超出时效或 evidence revision 漂移
- **THEN** 投影 MUST 标记 stale 且不得本地续命
- **AND** 后续消费 MUST 重新解析，不得使用过期投影冒充可信续接。

