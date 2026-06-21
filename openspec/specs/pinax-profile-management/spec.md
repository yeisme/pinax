# pinax-profile-management Specification

## Purpose
TBD - created by archiving change pinax-backend-auth-profile-cache. Update Purpose after archive.
## Requirements
### Requirement: 全局 profile 持久化

Pinax SHALL 在全局配置目录存储命名后端连接配置。

#### Scenario: 添加 profile

- **GIVEN** 用户运行 `pinax profile add my-s3 --endpoint s3://my-bucket --workspace default --device laptop`
- **WHEN** 命令执行
- **THEN** `$XDG_CONFIG_HOME/pinax/profiles.yaml`（或 `~/.config/pinax/profiles.yaml`）SHALL 写入名为 `my-s3` 的 profile
- **AND** profile SHALL 包含 `endpoint`、`workspace`、`device`
- **AND** stdout SHALL 输出确认信息，包括 profile name 和 endpoint（脱敏后）

#### Scenario: secret_ref 安全存储

- **GIVEN** 用户运行 `pinax profile add my-s3 --endpoint s3://my-bucket --secret-ref env://PINAX_S3_SECRET`
- **WHEN** profile 写入文件
- **THEN** profiles.yaml SHALL 存储 `secret_ref: "env://PINAX_S3_SECRET"` 引用
- **AND** SHALL NOT 存储实际 secret 值
- **AND** stdout SHALL NOT 显示 secret 实际值

#### Scenario: 本地凭据只进入用户级 secret store

- **GIVEN** Pinax 增加本地明文凭据写入能力
- **WHEN** 用户提交 provider secret
- **THEN** secret SHALL 写入用户级本地配置或用户级 secret store
- **AND** `.pinax/` 项目资产、profiles.yaml、stdout、stderr、事件、fixture、运行证据和 Git SHALL NOT 包含实际 secret 值

#### Scenario: 列出 profile

- **WHEN** 用户运行 `pinax profile list`
- **THEN** 输出 SHALL 包含所有 profile 的 name、endpoint（脱敏）、workspace、device
- **AND** 输出 SHALL NOT 包含 secret 实际值

#### Scenario: 删除 profile

- **GIVEN** 存在 profile `my-s3`
- **WHEN** 用户运行 `pinax profile remove my-s3`
- **THEN** 该 profile SHALL 从 profiles.yaml 中移除
- **AND** 正在使用该 profile 的 server 或 sync 操作 SHALL NOT 被中断

### Requirement: sync --target 解析 profile name

`pinax sync` 的 `--target` 参数 SHALL 支持解析 profile name。

#### Scenario: 使用 profile name 同步

- **GIVEN** 存在 profile `my-s3`，endpoint 为 `s3://my-bucket`
- **WHEN** 用户运行 `pinax sync pull --target my-s3 --vault ./my-notes`
- **THEN** sync SHALL 使用 profile `my-s3` 的 endpoint、workspace、device 和 secret_ref
- **AND** 行为 SHALL 等同于直接传入 `--endpoint s3://my-bucket --workspace default --device laptop`

#### Scenario: profile 不存在

- **GIVEN** 不存在 profile `nonexistent`
- **WHEN** 用户运行 `pinax sync pull --target nonexistent`
- **THEN** SHALL 返回错误，error code 为 `profile_not_found`
- **AND** hint SHALL 建议 `pinax profile list` 查看可用 profile

#### Scenario: 直接传入 URI 兼容

- **WHEN** 用户运行 `pinax sync pull --target s3://other-bucket`
- **THEN** SHALL 直接解析为 endpoint URI，不查找 profile
- **AND** 与现有行为保持兼容

### Requirement: 默认 profile

用户 SHALL 能设置默认 profile。

#### Scenario: 使用默认 profile

- **GIVEN** profiles.yaml 中 `defaults.profile` 设置为 `my-s3`
- **WHEN** 用户运行 `pinax sync pull` 不带 `--target`
- **THEN** SHALL 使用 `my-s3` profile
- **AND** 等同于 `pinax sync pull --target my-s3`

#### Scenario: --target 覆盖默认

- **GIVEN** 默认 profile 为 `my-s3`
- **WHEN** 用户运行 `pinax sync pull --target local`
- **THEN** SHALL 使用 `local` profile，忽略默认值
