# pipeline-unified-ux Specification

## Purpose
TBD - created by archiving change pinax-pipeline-unified-ux-v1. Update Purpose after archive.
## Requirements
### Requirement: pipeline status SHALL 聚合 pending plans 与最近 apply 记录
`pinax pipeline status` SHALL 以单一只读视图聚合：全部已保存待执行 plans（organize/metadata/repair/restore，含 plan_id、kind、操作计数、保存时间与 freshness 派生）与最近 N 条 apply 型 receipts（含 pipeline 类型、状态、changed 计数、时间）。该命令 MUST 只读，MUST NOT 写 vault、`.pinax/**` 或远端。单个 plan 文件损坏时 MUST 以 unreadable 条目 fail-closed 呈现，MUST NOT 中断其余条目。

#### Scenario: 跨管道聚合
- **WHEN** 同时存在 organize 与 metadata 的已保存 plans 及历史 receipts
- **THEN** status MUST 一次列出两类 pending plans 与 recent runs
- **AND** 输出 MUST 为每条 plan 提供下一步命令提示。

#### Scenario: 损坏 plan 文件
- **WHEN** 某已保存 plan 文件不可解析
- **THEN** 该条目 MUST 标记 unreadable 并附修复提示，其余条目 MUST 正常列出。

### Requirement: plan 读模型 SHALL 归一且不迁移存储
`pinax.plan.v1` 读模型 SHALL 通过各管道 reader adapter 从既有存储归一 plan 头字段（plan_id、kind、原 schema_version、created_at、操作计数、facts 摘要、freshness）。既有 plan 存储、schema 与命令行为 MUST 零迁移、零破坏。

#### Scenario: 原生命令回归
- **WHEN** 执行既有 `pinax organize plan/apply` 等命令
- **THEN** 行为与输出 MUST 与现状一致（读模型仅叠加）。

### Requirement: apply SHALL 拒绝落后于 vault 事实的已保存 plan
对已保存 plan 的 apply（organize/metadata/repair/restore），当 plan 记录的 facts 摘要与当前 vault 扫描指纹不一致时 MUST 拒绝执行并返回稳定错误（含变更概要与重新 plan 指引），除非显式 `--allow-stale`。拒绝时 `--events` MUST 发出 `stage.failed`（reason=plan_stale）。内存态 preview→apply 单命令流程 MUST 不受影响。

#### Scenario: stale plan 拒绝
- **WHEN** plan 保存后有 note 被修改，随后执行 apply --yes
- **THEN** apply MUST 拒绝并提示重新 plan 或 --allow-stale，MUST NOT 产生部分写入。

#### Scenario: 逃生门
- **WHEN** 同场景下显式传 `--allow-stale --yes`
- **THEN** apply MUST 按既有安全模型执行并照常写 receipt。

### Requirement: apply 型命令 SHALL 发出统一阶段事件
organize/metadata/repair/restore apply、sync push/pull、publish build/deploy 与 proof loop run SHALL 通过共享 helper 发出 `pinax.pipeline.stage.v1` NDJSON 事件（`stage.started`/`stage.completed`/`stage.failed`，携带 pipeline kind、plan_id/run id、stage 名与计数）。既有事件 MUST 保留原名原义；新事件类型为 additive，消费者 MUST 能忽略未知 type。

#### Scenario: 失败事件
- **WHEN** apply 因 plan_stale 拒绝且带 `--events`
- **THEN** 输出 MUST 含 `stage.failed` 且 `reason=plan_stale`。

#### Scenario: 成功事件
- **WHEN** organize apply 成功且带 `--events`
- **THEN** 输出 MUST 含 `stage.started` 与 `stage.completed` 且计数与 receipt 一致。

### Requirement: pipeline show SHALL 提供统一风险分组检视
`pinax pipeline show <plan-id|receipt-id>` SHALL 展示统一详情：plan（操作按 vault 结构写入 vs 元数据写入分组、changed paths、freshness、facts 摘要、下一步命令）或 receipt（applied 事实、changed paths、snapshot/ledger 引用）。changed paths MUST 走既有 path redaction 策略；未知 id MUST 返回稳定错误并提示可用 id。

#### Scenario: plan 详情分组
- **WHEN** `pinax pipeline show <plan-id>` 命中含 move 与 tags_patch 的 organize plan
- **THEN** 输出 MUST 将两类操作分组呈现并附 changed paths 与下一步 apply 命令提示。

#### Scenario: 未知 id
- **WHEN** show 的 id 既不匹配 plan 也不匹配 receipt
- **THEN** MUST 返回稳定错误，MUST NOT 猜测就近匹配。

