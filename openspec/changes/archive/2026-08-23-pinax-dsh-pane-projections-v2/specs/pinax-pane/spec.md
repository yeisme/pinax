## ADDED Requirements

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
