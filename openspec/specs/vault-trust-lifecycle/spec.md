# vault-trust-lifecycle Specification

## Purpose
TBD - created by archiving change pinax-okf-trust-discovery-v1. Update Purpose after archive.
## Requirements
### Requirement: 信任与生命周期 frontmatter 字段 SHALL 可选且 additive
Pinax notes 的 frontmatter SHALL 支持可选字段 `generated: {by, at}`、`verified`（事件列表，兼容 bare mapping 视为单元素列表）与 `stale_after`（带 UTC offset 的 ISO8601 绝对时刻）。未携带这些字段的存量 note MUST 继续按既有语义正常工作，MUST NOT 因缺失字段被拒绝或降级。未知 frontmatter 键 MUST 保留不删。

#### Scenario: 存量 vault 无信任字段
- **WHEN** note frontmatter 不含 `generated`/`verified`/`stale_after`
- **THEN** note 的读取、搜索、索引 MUST 与既有行为一致
- **AND** 派生分级 MUST 为 `unverified`，新鲜度 MUST 为 `fresh`。

#### Scenario: 非法时间戳
- **WHEN** `verified[].at` 或 `stale_after` 不是合法 ISO8601 时间
- **THEN** Pinax MUST 返回解析错误并指向具体 note 与字段
- **AND** MUST NOT 静默忽略该字段或按默认值继续。

### Requirement: Actor 约定与信任分级 MUST 消费时派生且绝不存储
信任分级 SHALL 由 actor 前缀派生：`verified` 为空或缺失 ⇒ `unverified`；存在事件但全部 actor 不以 `human:` 开头 ⇒ `machine`；任一 actor 以 `human:` 开头 ⇒ `human`。新鲜度 SHALL 为 `now >= stale_after` ⇒ `stale`，否则 `fresh`。派生结果 MUST NOT 写回 frontmatter 或 vault 正文；索引投影中的派生列 MUST 可由 vault 全量重建。

#### Scenario: human 前缀事件
- **WHEN** note 的 `verified` 含 `{by: "human:ye", at: ...}`
- **THEN** 派生分级 MUST 为 `human`
- **AND** `pinax search --trust human` MUST 能命中该 note。

#### Scenario: 未知 actor 前缀
- **WHEN** `verified` 含 actor 前缀不在 `human:` 约定内（如 `svc:ci`）
- **THEN** Pinax MUST 保留原文并按 `machine` 分级处理，MUST NOT 报错或丢弃事件。

### Requirement: 信任字段 MUST 只经显式 CLI 命令维护
`pinax note verify <ref>` SHALL 追加一条 verified 事件（actor 默认取已配置 identity，无配置时 MUST 要求显式 `--actor`），MUST 幂等（同 actor 同日重复调用不重复追加并返回既有事件），MUST 通过既有 atomic frontmatter patch 路径写入。`pinax metadata plan/apply` SHALL 支持 `trust_fields` 回填操作（填 `generated`/`stale_after`），复用既有 plan/apply 安全模型。Pinax MUST NOT 自动批量向 vault 注入信任字段。

#### Scenario: verify 幂等
- **WHEN** 对同一 note 连续两次执行 `pinax note verify <ref> --actor human:ye`
- **THEN** frontmatter 中该 actor 的事件 MUST 只有一条
- **AND** 第二次执行 MUST 输出幂等结果（含既有事件引用）而非错误。

#### Scenario: 无 identity 且未指定 actor
- **WHEN** 未配置 identity 且未传 `--actor`
- **THEN** `pinax note verify` MUST fail-closed 报错并提示 `--actor` 用法，MUST NOT 写入任何事件。

### Requirement: 索引投影 SHALL 缓存派生信任信号且可重建
SQLite 索引 note records SHALL 增加派生列（trust 分级、`stale_after`、最新 `verified.at`），由既有 rebuild/refresh 流程维护；该投影 MUST 保持可丢弃、可全量重建，MUST NOT 成为第二真源。

#### Scenario: 重建后信号一致
- **WHEN** 执行索引 rebuild
- **THEN** 派生列 MUST 与 frontmatter 重新派生的结果一致
- **AND** 重建 MUST NOT 修改任何 vault 正文或 frontmatter。

