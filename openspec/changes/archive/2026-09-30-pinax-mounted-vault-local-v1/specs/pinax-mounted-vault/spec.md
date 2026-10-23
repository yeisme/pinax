## ADDED Requirements

### Requirement: Pinax SHALL 把挂载目录当作普通本地 vault

对象存储正文若经 DriveBridge 呈现在本机路径上，Pinax SHALL 只用 `init`、`vault register`、`storage set local`、`note add`、`index refresh`。SHALL NOT 新增对象存储客户端，SHALL NOT 把 `storage set s3` 当正文真源。

#### Scenario: 写入只走本地路径
- **WHEN** vault 已 `storage set local --root /abs/vault` 且用户 `note add`
- **THEN** 只写该路径与本机 `.pinax`
- **AND** 成功信封不得声称 S3 API 或 Capsa `remote_write=true`

#### Scenario: storage kind 保持 local
- **WHEN** vault 指向 DriveBridge pinax-vault 叠加目录
- **THEN** `storage status` 仍报告 local

### Requirement: init SHALL 容忍已有内容挂载点

`pinax init` 在 `.pinax/config.yaml` 不存在时创建控制面。若 `notes/` 已是目录或指向目录的符号链接，SHALL 保留。SHALL NOT 删除挂载点或清空其目标。

#### Scenario: 已有 notes 符号链接
- **WHEN** `notes` 是指向已有目录的符号链接且尚未 init
- **THEN** `pinax init` 成功并创建 `.pinax/`
- **AND** 该符号链接目标不变

#### Scenario: 已初始化仍拒绝二次 init
- **WHEN** `.pinax/config.yaml` 已存在
- **THEN** 仍失败为 `vault_already_initialized`

### Requirement: doctor SHALL 报告挂载文件系统事实

`storage doctor` 与 `vault doctor` SHALL 增加 `control_plane_local`、`content_mount`、`content_writable`、`drivebridge_preset`。未装 DriveBridge 且未挂载时普通 doctor SHALL NOT 失败为 `drivebridge_not_installed`。`.pinax` 为符号链接时 SHALL 报告 `control_plane_on_remote`。

#### Scenario: 纯本地 vault
- **WHEN** 普通 `pinax init` 的 vault 跑 `storage doctor`
- **THEN** `drivebridge_preset=none`、`content_mount=none`、`control_plane_local=true`

#### Scenario: 控制面误为链接
- **WHEN** `.pinax` 是符号链接
- **THEN** `control_plane_local=false` 且含 `control_plane_on_remote`
