## Why

当前系统在处理远程存储和外部能力时，将 `StorageBackend` 和 `Provider` 都笼统地视作 `Backend` 或者 `Cloud` 逻辑处理，概念混淆且难以管理。
我们需要将架构中的“盲存储”（负责 Pinax 加密块同步的系统）与“能力适配器”（如飞书文档等外部服务接口）明确解耦。通过将核心模块重命名为 `Sync Remote` 并引入 URI Scheme 模式，能够使同步后端的使用与具体实现解耦，从而更方便地管理和新增未来的其他存储介质（如 WebDAV, PostgreSQL 等）。

## What Changes

1. **命名重构**：将现有的 `internal/cloud` 目录重命名为 `internal/remote`。
2. **接口规范与重命名**：将 `StorageBackend` 接口重命名为 `BlobStore` (或 `RemoteStorage`)。
3. **引入 URI Scheme 注册表**：在 `internal/remote` 中实现工厂模式注册机制。支持通过如 `s3://`, `file://`, `webdav://`, `postgres://` 的 URI 来统一加载初始化具体的 `BlobStore` 实例。
4. **移除硬编码**：删除同步代码里类似 `if endpoint == "s3"` 的路由逻辑。
5. **(可选) 新介质补充设计**：为基于 HTTP 拓展的 WebDAV 或基于 SQL 的 PostgreSQL 等留好标准接口或进行基础实现（此任务重在重构和基建搭设）。

## Capabilities

### New Capabilities

- `pinax-remote-sync-registry`: URI Scheme 抽象与存储后端的注册表路由机制。

### Modified Capabilities

- `pinax-cloud-sync`: 修改云端同步机制架构，将其中的 Backend 抽象替换为 Remote URI Scheme 架构。

## Impact

- `internal/cloud` 文件夹会被重命名，导致包级别的重构。
- `internal/app` 和 `internal/sync` 中原先调用 `cloud.Backend` 或 `cloud.Load` 的代码需要迁移。
- CLI 命令 `sync init` 时录入 endpoint 的验证与解析逻辑会变动。
