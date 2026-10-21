# notebook-daily-write Specification

## Purpose

受监督日记首建必须先预览并经人确认，同一本地日期只发布一个对象，丢失响应时用原 operation 回放，证据不足则保持 unknown。

## Requirements

### Requirement: 日记首建必须先预览确认

Pinax MUST 提供受监督 `/v1/writer/journals/daily:propose` 与 `:apply`，要求 writer.editor、reader.body、reader.metadata 交集。预览固定date/timezone、原operation、object ID、模板digest、目标与正文。propose不得创建vault状态或执行模板查询。

#### Scenario: 缺失日记预览

- **WHEN** 获授权用户为合法本地日期提出首次创建
- **THEN** 返回exact before/after和受保护metadata，无文件副作用；apply必须携带原proposal和digest并经人确认

#### Scenario: 已有日记或模板漂移

- **WHEN** 目标已存在，或确认时模板、目标或固定日期不再匹配
- **THEN** 拒绝覆盖，不自动改名或更新到新proposal

### Requirement: 同日期唯一发布与原操作恢复

Pinax MUST 使用既有vault锁、operation ledger、快照和record gate，以无覆盖发布保证新接口同日期并发只产生一个对象。receipt与unknown恢复沿用原operation，不存正文到ledger。

#### Scenario: 两个并发首建

- **WHEN** 两个不同operation同时确认同日首建
- **THEN** 最多一个发布成功，其余返回忙碌或日记已存在；不产生第二个后缀文件

#### Scenario: 成功响应丢失

- **WHEN** 客户端再次提交完全相同的operation与proposal
- **THEN** 回放原receipt，不重复创建或追加正文

#### Scenario: 发布后进程中断

- **WHEN** ledger尚未成功落回执但目标文件已发布
- **THEN** owner reconcile仅凭预期对象/内容digest与record证据恢复；证据不足保持unknown，不盲目重新执行创建
