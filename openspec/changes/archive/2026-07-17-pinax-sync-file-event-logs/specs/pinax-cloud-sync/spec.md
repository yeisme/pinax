## ADDED Requirements

### Requirement: 同步日志提供脱敏的文件级时间线

Pinax SHALL 在同步 run 时间线中记录每个计划文件操作的安全元数据，并允许用户持续跟随新追加事件，而不暴露正文、凭据、provider payload 或违反路径策略的路径。

#### Scenario: 查看同步涉及的文件

- **GIVEN** 一次 sync run 包含上传、下载、删除或冲突操作
- **WHEN** 用户运行 `pinax sync logs tail --vault <vault>`
- **THEN** CLI SHALL 展示每个文件操作的 run ID、方向、操作类型、状态和允许公开的路径标识
- **AND** CLI SHALL 同时保留 run 级完成摘要。

#### Scenario: 持续跟随同步文件事件

- **WHEN** 用户运行 `pinax sync logs tail --follow --vault <vault>`
- **THEN** CLI SHALL 先输出当前尾部事件，再持续输出后续追加的同步事件，直到命令上下文取消
- **AND** `--events` SHALL 输出逐行有效 NDJSON，不混入 human prose 或 diagnostics。

#### Scenario: 文件事件遵守路径策略和脱敏

- **GIVEN** sync run 使用 `default`、`hash` 或 `omitted` path policy
- **WHEN** Pinax 持久化或渲染文件级同步事件
- **THEN** 文件路径 SHALL 使用对应策略处理
- **AND** 事件、stdout、stderr、测试 fixture 和收据 SHALL NOT 包含 note body、token、Authorization、Cookie 或 provider payload。

#### Scenario: JSON 不进入无限跟随模式

- **WHEN** 用户组合 `sync logs tail --follow --json`
- **THEN** CLI SHALL 返回稳定的参数错误和可执行提示
- **AND** SHALL NOT 输出不完整或无限增长的 JSON document。
