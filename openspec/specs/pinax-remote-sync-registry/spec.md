# pinax-remote-sync-registry Specification

## Purpose

TBD - created by archiving change pinax-remote-sync-refactor.
## Requirements
### Requirement: URI Scheme 解析与工厂路由支持
系统 SHALL 支持通过标准格式的 URI Scheme（如 `s3://...`，`webdav://...` 等）来解析和初始化具体的远程盲存储介质（BlobStore），且不可硬编码路由。

#### Scenario: 从 URI 自动匹配支持的存储介质
- **WHEN** 用户通过 `sync init` 传入 `--endpoint webdav://some-server/path`
- **THEN** 系统 SHALL 解析出 `webdav` 协议
- **AND** 自动路由到注册表中相匹配的工厂函数进行实例化
- **AND** 若不存在对应的存储驱动，SHALL 抛出 `unsupported remote scheme: webdav` 错误

### Requirement: 插件化 BlobStore 注册
系统 SHALL 提供一个全局的注册接口，允许不同的存储实现在初始化时将自身的抽象注入系统。

#### Scenario: 注册新的存储驱动
- **WHEN** 应用启动时或引入新的 backend 包
- **THEN** 该包的 init() 或等效方法 SHALL 调用 `remote.Register(scheme, factory)`
- **AND** 使得用户能够透明地在命令行中使用该 scheme

### Requirement: Cloud Sync protects local-first moves and deletes

Pinax SHALL use the latest Cloud Sync path as the default sync target and SHALL plan pull/sync operations from base, local, and remote manifests instead of blindly replaying the remote manifest over the local vault.

#### Scenario: Pull does not restore a locally moved note

- **GIVEN** a device has synced `index/home.md` from Cloud Sync
- **AND** the user locally moves it to `notes/home.md` without pushing
- **WHEN** the user runs `pinax sync pull --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL fail with `LOCAL_UNPUSHED_CHANGES`
- **AND** it SHALL NOT recreate `index/home.md`
- **AND** it SHALL recommend `pinax sync --vault ./my-notes --yes`.

#### Scenario: Bidirectional sync pushes the local move

- **GIVEN** a device has synced `index/home.md` from Cloud Sync
- **AND** the user locally moves it to `notes/home.md`
- **WHEN** the user runs `pinax sync --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL push the current manifest to Cloud Sync
- **AND** another device pulling that revision SHALL remove `index/home.md` and create `notes/home.md`.

#### Scenario: Sync subcommands default to Cloud Sync

- **WHEN** a user runs `pinax sync diff`, `pinax sync push`, or `pinax sync pull` without `--target`
- **THEN** Pinax SHALL use `capsa` as the default target
- **AND** `--target git`, `--target s3`, and `--target cloud` SHALL remain explicit compatibility paths.

