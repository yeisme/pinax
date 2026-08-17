## ADDED Requirements

### Requirement: Pinax 默认展示个人本地知识核心命令

Pinax SHALL 在 root help 中优先展示个人本地知识工作流，并 SHALL NOT 把所有高级平台能力作为同层默认导航。

#### Scenario: Root help 只展示核心入口

- **WHEN** 用户运行 `pinax --help`
- **THEN** 输出 SHALL 展示 `init`、`vault`、`note`、`inbox`、`journal`、`search`、`project`、`backup` 和 `commands`
- **AND** 输出 SHALL NOT 展示 `sync`、`capsa`、`api`、`agent`、`publish`、`plugin` 或 `backend` 作为默认 root 命令。

### Requirement: Pinax 提供个人本地备份 facade

Pinax SHALL 提供 `pinax backup` 作为个人默认安全入口，并 SHALL 复用现有 version/Git application service 与存储证据。

#### Scenario: Backup 默认检查本地状态

- **WHEN** 用户运行 `pinax backup --vault <vault>` 或 `pinax backup status --vault <vault>`
- **THEN** 命令 SHALL 检查现有本地 version backend 状态
- **AND** Projection command SHALL 为 `backup.status`
- **AND** 命令 SHALL NOT 访问 S3、rclone、Capsa server 或启动 daemon。

#### Scenario: 创建与查看本地备份

- **WHEN** 用户运行 `pinax backup create --message <message>` 或 `pinax backup history`
- **THEN** 命令 SHALL 分别复用现有 version snapshot 与 history service
- **AND** SHALL 使用 `backup.create` 或 `backup.history` Projection command
- **AND** 现有 `pinax version snapshot|history` SHALL 继续保持原合同。

#### Scenario: 恢复仍使用只读计划和显式批准

- **WHEN** 用户运行 `pinax backup restore <path> --revision <revision> --plan`
- **THEN** 命令 SHALL 生成现有 version restore plan，不直接写 Markdown
- **AND** 只有 `pinax backup restore apply --plan <id> --yes` SHALL 应用已保存计划
- **AND** Projection command SHALL 分别为 `backup.restore` 与 `backup.restore.apply`。

#### Scenario: 远端同步不伪装成默认备份

- **WHEN** 用户查看 `pinax backup --help`
- **THEN** help SHALL 明确默认能力是 local version/Git snapshot
- **AND** SHALL NOT 宣称 S3、rclone 或 Capsa 已经是稳定默认 backup
- **AND** 现有远端命令 SHALL 继续通过 `pinax commands` 发现并执行。

#### Scenario: 默认帮助指向完整命令目录

- **WHEN** 用户运行 `pinax --help`
- **THEN** 输出 SHALL 提示用户运行 `pinax commands` 查看完整命令目录
- **AND** 提示 SHALL 使用普通终端可读的英文 CLI 文案。

### Requirement: Pinax 提供完整命令目录

Pinax SHALL 提供 `pinax commands`，从当前 Cobra command tree 生成 core 与 advanced 命令目录，并通过共享 Projection 渲染所有输出模式。

#### Scenario: Human 模式列出核心与高级命令

- **WHEN** 用户运行 `pinax commands`
- **THEN** 输出 SHALL 包含核心命令和高级命令的名称、摘要、分组与可见性
- **AND** 命令 SHALL 不读取 vault 内容、不访问网络、不执行远端写入。

#### Scenario: JSON 模式输出稳定 envelope

- **WHEN** 用户运行 `pinax commands --json`
- **THEN** stdout SHALL 是 command 为 `commands.list` 的有效 Pinax JSON envelope
- **AND** `data.commands` SHALL 包含完整 root command 目录
- **AND** stdout SHALL 不包含 ANSI、日志或额外 prose。

#### Scenario: Agent 和 events 模式保持可解析

- **WHEN** 用户运行 `pinax commands --agent` 或 `pinax commands --events`
- **THEN** 输出 SHALL 使用共享 Projection 的稳定 agent key=value 或 NDJSON 合同
- **AND** 输出 SHALL 不包含 secrets、provider payload、凭据引用或 vault 正文。

### Requirement: 高级命令保持兼容可执行

从默认 help 降级的命令 SHALL 继续保留现有命令路径、flags、输出合同和应用服务行为。

#### Scenario: 高级命令 help 仍可访问

- **WHEN** 用户运行 `pinax sync --help`、`pinax capsa --help`、`pinax api --help` 或 `pinax agent --help`
- **THEN** 相应命令 help SHALL 正常返回
- **AND** 命令 SHALL NOT 因默认 root help 过滤而变成 unknown command。

#### Scenario: 机器合同不因导航变化而改变

- **WHEN** 现有脚本运行任一高级命令并选择 `--json`、`--agent` 或 `--events`
- **THEN** 其稳定 command 名、envelope 字段、agent keys 和 event types SHALL 保持兼容
- **AND** 本变更 SHALL NOT 引入数据或配置迁移。

### Requirement: 未接线的研究原型不得保留

Pinax SHALL NOT 保留没有生产调用方、且只返回 fake fixture 的独立 research adapter 实现。

#### Scenario: Briefing 不依赖废弃 research 包

- **WHEN** 构建和测试 `cmd/pinax` 与 briefing application service
- **THEN** 依赖图 SHALL NOT 包含 `internal/research`
- **AND** briefing 现有 recipe、fake evidence、candidate 与 delivery 行为 SHALL 保持不变
- **AND** 删除 SHALL NOT 修改任何 vault 数据或稳定 CLI/API/SDK 合同。
