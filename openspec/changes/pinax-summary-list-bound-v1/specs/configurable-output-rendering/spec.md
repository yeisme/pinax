## ADDED Requirements

### Requirement: Human summary lists SHALL stay bounded but complete

Pinax human summary list rendering SHALL bound default list rows at 20 and SHALL print a `showing N/M` hint whenever a list is truncated, while machine modes keep the complete projection.

#### Scenario: Ordinary local list renders in full

- **WHEN** 一个 human summary 列表（project registry、plan tasks、repair plans、subprojects、data list、named scalar list）包含不超过 20 条
- **THEN** 默认输出 SHALL 渲染全部条目
- **AND** SHALL NOT 输出 showing 提示。

#### Scenario: Truncation reports the bound

- **WHEN** 列表超过 20 条
- **THEN** 输出 SHALL 截断到 20 条并打印 `showing 20/M`
- **AND** `--json` SHALL 提供完整数据（既有逃生路径不变）。

#### Scenario: Machine modes stay complete

- **WHEN** 同一 projection 以 `--json` 或 `--agent` 渲染
- **THEN** 输出 SHALL 不受人类行界与提示影响。
