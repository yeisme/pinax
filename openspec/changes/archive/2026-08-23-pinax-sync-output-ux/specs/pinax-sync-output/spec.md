## ADDED Requirements

### Requirement: 统一同步视图

同步命令 SHALL 在 projection data 中以 optional `sync_view` 提供 `pinax.sync.output.v1`，并保留原有 plan、receipt 与 facts。

#### Scenario: 成功 push 或 pull

- **WHEN** 同步完成或确认 up-to-date
- **THEN** `sync_view.result` SHALL 为 `applied` 或 `up_to_date`
- **AND** counts SHALL 提供 added、modified、deleted、renamed、conflicts、unchanged、total
- **AND** changes SHALL 使用 A/M/D/R/C 与 applied/planned/failed/conflict 状态

#### Scenario: 缓存范围

- **WHEN** 远端 head 未成功读取
- **THEN** `sync_view.scope` SHALL 为 `cached`
- **AND** output SHALL NOT 把缓存结果表示为远端已确认

### Requirement: 机器输出隔离

JSON SHALL 是单一 envelope；agent SHALL 使用稳定 key=value；events SHALL 是 start/progress/end/error NDJSON。机器 stdout SHALL 不含表格、ANSI 或实时进度。

#### Scenario: JSON 与 agent

- **WHEN** 用户使用 `--json` 或 `--agent`
- **THEN** stdout SHALL 只包含对应机器格式
- **AND** stderr SHALL 不输出实时进度

#### Scenario: events 流

- **WHEN** 用户使用 `--events`
- **THEN** stdout SHALL 依序包含 start、progress 和 end/error NDJSON
- **AND** SHALL 不携带正文 diff

### Requirement: 有界预览与正文 diff

同步 SHALL 支持 `--preview status|diff|none`、`--limit N` 和显式 `--content-diff`。正文 diff SHALL 受文件数、单文件字节数、运行总字节数和脱敏门禁限制，且 SHALL 不写入 receipt、事件日志或远端。

#### Scenario: 默认 status 预览

- **WHEN** 用户不传 preview 参数
- **THEN** 人类输出 SHALL 显示统计和最多 10 条状态式路径
- **AND** 超出部分 SHALL 显示 shown/total/truncated

#### Scenario: 显式正文 diff

- **WHEN** 用户传入 `--content-diff`
- **THEN** 每个文件正文 SHALL 不超过 64 KiB、整次运行不超过 256 KiB、最多 10 个文件
- **AND** 敏感内容 SHALL 被替换或截断

### Requirement: 人类输出与进度

默认人类布局 SHALL 为 table；`--output-style compact` SHALL 输出无表格边框的紧凑行。实时进度 SHALL 写 stderr，TTY 自适应刷新，非 TTY 使用无 ANSI 阶段行，`--progress never` SHALL 关闭。

#### Scenario: 默认表格

- **WHEN** 用户运行普通同步命令
- **THEN** stdout SHALL 使用表格摘要和变更预览
- **AND** 默认 progress SHALL 不污染 stdout

#### Scenario: compact 与关闭进度

- **WHEN** 用户传入 `--output-style compact --progress never`
- **THEN** 人类输出 SHALL 使用紧凑行
- **AND** SHALL 不显示实时进度条或 ANSI 控制序列
