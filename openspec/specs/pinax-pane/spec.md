# pinax-pane Specification

## Purpose
TBD - created by archiving change pinax-dsh-pane-contract. Update Purpose after archive.
## Requirements
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

### Requirement: Pane SHALL 提供有界 backlinks 投影

`AssemblePaneBacklinks` SHALL 从 backlinks 投影派生目标笔记的反向链接实体，每笔记 SHALL 不超过 50 条且仅含 ref/title/kind/status；不安全 ref SHALL 跳过并计入 `dropped_unsafe`。

#### Scenario: 有界输出

- **WHEN** 目标笔记有超过 50 条反向链接
- **THEN** 输出 SHALL 截断至 50 条
- **AND** 序列化结果 SHALL NOT 含路径或凭据子串

#### Scenario: 无链接笔记

- **WHEN** 目标笔记无反向链接
- **THEN** envelope SHALL 为 `status=ready` 的空实体快照

### Requirement: Pane SHALL 提供无路径 graph 摘要

`AssemblePaneGraphSummary` SHALL 输出节点/边/连通分量计数与度数 top-k（k≤20，仅 ref 与度数）；SHALL NOT 输出任何文件系统路径或笔记正文。

#### Scenario: top-k 摘要

- **WHEN** 链接图包含超过 20 个节点
- **THEN** 摘要 SHALL 只含计数值与 top-20 度数列表
- **AND** 列表项 SHALL 仅含安全 ref 与度数整数

### Requirement: Pane SHALL 提供有界 history 时间线

`AssemblePaneHistory` SHALL 从 record ledger 修订事件派生 `payload.timeline`，事件数 SHALL 不超过 100，超出 SHALL 标记 `truncated=true`；事件 SHALL 仅含 op/ref/revision/时间字段。

#### Scenario: 超长历史

- **WHEN** 笔记修订事件超过 100 条
- **THEN** timeline SHALL 截断至 100 条并标记 `truncated`
- **AND** 事件 SHALL NOT 含正文、路径或凭据

### Requirement: 三面 SHALL 复用统一红线与 envelope

backlinks/graph/history 组装 SHALL 复用 `paneUnsafe` ref 红线与共享 envelope 构造器；失败/离线投影的状态映射 SHALL 与 note list 快照一致。

#### Scenario: 失败投影

- **WHEN** 上游投影状态为 `failed`
- **THEN** 对应 pane 面 SHALL 输出 `status=offline`
