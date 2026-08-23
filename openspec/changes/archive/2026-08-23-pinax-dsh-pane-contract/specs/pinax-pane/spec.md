## ADDED Requirements

### Requirement: Pane 快照 SHALL 只投影有界笔记摘要

`AssemblePaneSnapshot` SHALL 从 `note.list` 投影派生 `pane.event.v1alpha1` 快照：实体只含 `ref`、`version` 与有界 `value`（title/kind/status/tags）， SHALL NOT 含绝对路径、正文、frontmatter 或凭据。

#### Scenario: 普通快照

- **WHEN** `note.list` 投影状态为 `success`
- **THEN** envelope `status` SHALL 为 `ready`
- **AND** 实体 ref SHALL 为 note ID，不安全的 ref SHALL 被跳过

#### Scenario: 泄漏扫描

- **WHEN** 快照被序列化
- **THEN** 输出 SHALL NOT 含绝对路径、`token`、`authorization`、`cookie` 子串

### Requirement: 失败投影 SHALL 映射为 offline

`NewErrorProjection` 产生的失败投影（Status `failed`）SHALL 映射为 envelope `status=offline`；SHALL NOT 以 `ready` 加空实体伪装成功。`permission_denied` 投影 SHALL 保持 `permission_denied`。

#### Scenario: note.list 失败

- **WHEN** `note.list` 返回失败投影
- **THEN** envelope `status` SHALL 为 `offline`
- **AND** payload entities SHALL 为空

### Requirement: 时间戳与新鲜度 SHALL 反映真实观测

快照 `occurredAt`/`observedAt` SHALL 为组装时刻的 UTC RFC3339 时间；负例快照 `freshness` SHALL 为 `unknown`。SHALL NOT 输出固定占位时间戳。

#### Scenario: 组装快照

- **WHEN** 任一快照被组装
- **THEN** `observedAt` SHALL 不等于固定的占位常量

### Requirement: Artifact 引用 SHALL 不含文件系统路径

`PaneArtifactFromNote` SHALL 输出 `pane.artifact.v1alpha1`（owner=`pinax`、mediaType=`text/markdown`、capabilities=`open/link/attach_context`）；命中 `paneUnsafe` 红线的 ref SHALL 返回错误而不是产出引用。

#### Scenario: 绝对路径 ref

- **WHEN** note ref 是绝对路径或含 URL scheme
- **THEN** `PaneArtifactFromNote` SHALL 返回错误

### Requirement: 结构化 mutation SHALL 门控到 Pinax 命令

`PaneGatedActions` SHALL 声明 capture/sync 为 gated action，携带 expected revision、幂等与回执要求及 owner command。客户端提交未经 Pinax parser 的 metadata blob 时，`RejectHandwrittenMetadata` SHALL fail closed。

#### Scenario: 手写 metadata

- **WHEN** 客户端提交未经过 Pinax parser 的 metadata blob
- **THEN** SHALL 返回拒绝错误
- **AND** vault 状态 SHALL 保持不变

### Requirement: 负例 SHALL 覆盖 offline 与 permission_denied

`AssemblePaneNegative` SHALL 支持 `offline`、`permission_denied`（返回对应状态的空快照）与 `handwritten_metadata`（返回拒绝错误）；未知 kind SHALL 返回错误。客户端 SHALL NOT 以定时 refetch 伪装实时。

#### Scenario: 无事件流

- **WHEN** sync daemon 或事件 SDK 不可用
- **THEN** Pane SHALL 显示 `offline`
- **AND** SHALL NOT 用定时 refetch 伪装实时
