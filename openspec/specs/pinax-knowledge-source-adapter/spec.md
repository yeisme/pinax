# pinax-knowledge-source-adapter Specification

## Purpose
TBD - created by archiving change pinax-knowledge-source-adapter-v1. Update Purpose after archive.
## Requirements
### Requirement: 导出 SHALL 只覆盖 allowlisted 投影

knowledge 投影导出 SHALL 只包含同时满足显式路径 allowlist 与条目 allow 标记的笔记；默认 allowlist 为空即零导出；MUST NOT 导出未授权内容或完整 vault 副本。

#### Scenario: 默认零导出

- **WHEN** 未配置 allowlist 时运行导出
- **THEN** 导出包为空并明示原因
- **AND** 不读取非 allowlist 内容

#### Scenario: 双条件缺一

- **WHEN** 笔记在路径 allowlist 但缺 allow 标记（或反之）
- **THEN** 该笔记不进入投影

### Requirement: 投影 SHALL 携带 permission/citation/freshness/revocation 元数据

每条投影 SHALL 携带 refs/digest、permission、citation 元数据、freshness（digest+时间）与 revocation（tombstone）；删除 SHALL 以 tombstone 记录，MUST NOT 静默消失。

#### Scenario: 笔记删除

- **WHEN** allowlisted 笔记被删除后增量导出
- **THEN** 输出 tombstone 条目
- **AND** 消费方可据此撤销

### Requirement: vault SHALL 保持唯一真源

adapter MUST NOT 写 Inferrum/txtai DB 或任何检索索引；一切下游状态由消费方按投影自建。

#### Scenario: 只读边界

- **WHEN** 导出运行
- **THEN** vault 与外部 DB 均无写入
- **AND** 架构测试锁定只读

