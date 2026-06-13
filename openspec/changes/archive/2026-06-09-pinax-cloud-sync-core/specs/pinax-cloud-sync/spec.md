## MODIFIED Requirements

### Requirement: Sync plan 支持 dry-run 和冲突
Sync planner SHALL 支持 dry-run 模式，并提供将并发冲突无痛在原目录另存副本的冲突检测与处理机制。
#### Scenario: dry-run sync

- **WHEN** 用户运行 `pinax sync diff --dry-run`
- **THEN** 输出 SHALL 显示 sync plan 但不执行任何写入
- **AND** 不改写 vault

#### Scenario: 冲突检测与原目录副本保留

- **WHEN** base revision mismatch 且存在本地冲突变更
- **THEN** sync SHALL 不直接中断退出
- **AND** 本地冲突文件 SHALL 在原目录下被重命名为 `<filename>.<timestamp>.conflict.md` 进行保留
	- **AND** 服务器端的最新版本 SHALL 覆盖到原本的文件路径

## ADDED Requirements

### Requirement: 冲突辅助处理 CLI

CLI SHALL 提供查看和比对本地历史遗留冲突文件的相关命令。

#### Scenario: 机器可读的冲突列举（面向 AI）

- **WHEN** 用户运行 `pinax sync conflicts list --json`
- **THEN** CLI SHALL 输出符合 `ai-native-cli-output-contract` 规范的 JSON 数组，包含每个冲突文件和对应的主干文件路径

#### Scenario: 差异对比

- **WHEN** 用户运行 `pinax sync conflicts diff <conflict-file>`
- **THEN** CLI SHALL 找到其对应的主干文件，并输出两者之间差异的 diff 视图

#### Scenario: 机器友好的内容导出（面向 AI）

- **WHEN** 用户或 AI 运行 `pinax sync conflicts show <conflict-file> --json`
- **THEN** CLI SHALL 输出 JSON 格式
- **AND** 包含 `original_content`（主干原文）和 `conflict_content`（冲突副本原文）的纯文本以供外部智能合并

#### Scenario: 快速解决冲突

- **WHEN** 用户运行 `pinax sync conflicts resolve <file> --keep-local`
- **THEN** CLI SHALL 将冲突文件内容覆盖到主干文件，并删除该冲突文件
- **AND** 若指定 `--keep-remote`
- **THEN** CLI SHALL 直接删除本地冲突文件
- **AND** 若指定 `--merged <merged-file>`
- **THEN** CLI SHALL 将提供的 `<merged-file>` 内容覆写到主干文件，并删除原始冲突文件

### Requirement: 同步连接管理与状态 CLI

CLI SHALL 提供用于初始化配置和查看同步健康度的基础命令。

#### Scenario: 初始化同步密码

- **WHEN** 用户运行 `pinax sync init`
- **THEN** CLI SHALL 引导配置后端凭证和端到端加密的 Sync Secret，并将其安全存储

#### Scenario: 状态健康检测

- **WHEN** 用户运行 `pinax sync status`
- **THEN** CLI SHALL 连接远端尝试获取最新清单版本，并输出当前与远端的差异大纲（如多少文件待推拉）

### Requirement: 单向与双向同步拆分

同步逻辑 SHALL 拆分支持单独的推、拉，以及默认的双向一致性合并。

#### Scenario: 仅拉取 (Pull Only)

- **WHEN** 用户运行 `pinax sync pull`
- **THEN** CLI SHALL 仅从服务端下载变更覆盖到本地（并产生必要的冲突标记）
- **AND** SHALL NOT 将本地未同步的新增和修改推送到服务端

#### Scenario: 仅推送 (Push Only)

- **WHEN** 用户运行 `pinax sync push`
- **THEN** CLI SHALL 仅将本地修改推送到服务端
- **AND** 若发生 Base Revision 失配（有其它设备已更新），CLI SHALL 拒绝推送并要求用户先执行 pull 操作

### Requirement: 多种存储后端支持与并发锁

同步模块 SHALL 通过抽象的 Storage Backend 支持不同的云端存储介质，并对所有介质提供乐观并发控制。

#### Scenario: S3 或兼容对象存储 (S3 API)

- **WHEN** 用户配置后端为 `s3://` 协议
- **THEN** 系统 SHALL 通过 S3 原生的 `If-Match` 和 ETag 机制来进行 Manifest 更新的并发保护

#### Scenario: 本地/网络挂载文件系统 (JuiceFS / FUSE / SMB)

- **WHEN** 用户配置后端为 `file://` 等本地目录协议
- **THEN** 系统 SHALL 直接使用 Go 核心的文件系统 API 进行读写
- **AND** SHALL 利用原子级的临时文件重命名或跨平台的 `flock` 机制来保证 Manifest 的并发一致性
