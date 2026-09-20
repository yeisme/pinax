## ADDED Requirements

### Requirement: DriveBridge attach 不改变 Remote API 单一 vault 真源

客户端使用 `pinax --api-url`、`PINAX_API_URL` 或 `remote.api_url` 进入 Remote API Mode 时，真源 SHALL 仍是服务端 `pinax api serve` 附着的那一个 vault。DriveBridge attach 或 `bind-working-copy` SHALL NOT 把该模式变成多设备文件同步，SHALL NOT 让 Remote API 客户端在本地 hydrate 另一份 vault 来执行这些远程命令。

#### Scenario: Remote API 仍指向单一 vault

- **WHEN** 客户端使用 `pinax --api-url` Remote API Mode，且本机或服务端 vault 已 attach DriveBridge
- **THEN** 被转发的普通命令 SHALL 仍操作服务端附着的那一个 vault
- **AND** SHALL NOT 因 DriveBridge bind 改为多设备文件同步

#### Scenario: storage attach 与 capsa/sync 同属本地控制面

- **WHEN** 用户配置了 `remote.api_url` 或 `PINAX_API_URL`，并运行 `pinax storage attach-drivebridge`、`pinax storage hydrate`、`pinax storage bind-working-copy` 或 `pinax storage doctor`
- **THEN** 这些命令 SHALL 与 `pinax capsa` / `pinax sync` 一样在本地 vault 执行
- **AND** SHALL NOT 返回 `remote_command_unsupported`
- **AND** SHALL NOT 把 DriveBridge hydrate 当作对 Remote API 单一 vault 的写入
