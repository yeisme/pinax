# human-reading-write Specification

## Purpose

受监督人类写入：独立 write scope、提案确认、revision/快照/receipt，以及按证据开放 binding。

## Requirements

### Requirement: 写 scope 独立授予
受控写 SHALL 通过独立 write scope（owner 授予、受监督会话 bootstrap 显式携带）授权；旧 read/write/admin token 与 reader scope 不自动获得写能力，请求字段不构成授权。

#### Scenario: 旧 reader 会话请求写入
- **WHEN** 仅持 reader scopes 的会话请求 capture 或 apply
- **THEN** 拒绝 insufficient_scope，不降级、不静默排队

### Requirement: 一切编辑经提案与宿主确认
人类编辑 SHALL 以 proposal（change 与 preview 摘要）为唯一入口；apply 只接受未变更的 preview，并要求宿主显式 authorization 断言与 owner 门禁；不存在直接覆盖正文的旁路。

#### Scenario: preview 被篡改后 apply
- **WHEN** apply 携带的 preview 摘要与 propose 返回值不一致
- **THEN** 拒绝并要求重新提案，不部分应用

### Requirement: revision、快照与 operation receipt 语义保持
apply SHALL 携带 expected_revision，冲突显式失败；snapshot_required 能力先建快照；每个 apply 发 operation receipt，unknown 结果走既有 show/reconcile 恢复，禁止换 identity 重试。

#### Scenario: apply 后宿主崩溃
- **WHEN** apply 已发出但宿主未收到响应
- **THEN** 以同一 operation identity 查询 receipt 恢复，不重复 apply

### Requirement: 写能力可用性按证据门推进
写 binding SHALL 仅在实现与组件证据齐备后标记 available；合同冻结期（W01）保持 planned 且无 backing，不得提前声明可用；不新增 MCP 自批路径或普通文件覆盖旁路。

#### Scenario: 合同期检查 manifest
- **WHEN** 消费者在合同冻结期读取 transport manifest
- **THEN** writer binding 均为 planned 且 available_surfaces 为空

#### Scenario: 组件证据后检查 manifest
- **WHEN** 消费者在 W06 组件证据落地后读取 transport manifest 或 OpenAPI
- **THEN** 五个 writer binding 均为 available，带 internal/api/http backing 与测试证据引用，且不出现 MCP 自批路径
