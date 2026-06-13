## Context

目前系统的 `internal/cloud` 模块提供了 `StorageBackend` 接口，但随着项目的推进，我们发现概念存在重叠和混淆。例如，原本的 "Cloud" 既可以指代进行盲存储同步的远端（如 S3、JuiceFS），又容易被误认为和外部系统的接口能力（如飞书、Notion，我们目前称为 Provider）相似。此外，由于当前的架构基于硬编码的配置判断，新增新的后端（例如 WebDAV, PostgreSQL）不够解耦。

## Goals / Non-Goals

**Goals:**
- 将 `internal/cloud` 重命名为 `internal/remote`，消除歧义。
- 建立抽象的 `BlobStore` 接口替换旧有的 `StorageBackend`。
- 实现注册表模式（Registry Pattern）以及根据 URI Scheme（例如 `s3://`、`webdav://`、`postgres://`）自动工厂路由机制。
- 将相关的 CLI 和 App 层进行安全迁移，确保原本 `sync init` 时能解析对应的 endpoint 并分发。

**Non-Goals:**
- 不涉及 Provider（Lark, ntn等）层的调整。
- 本次重构目标是建立规范并无损迁移 S3/File，并不强制要求实现所有计划中的新 Storage Backend（可以作为后续任务扩展）。

## Decisions

1. **统一名词与包路径调整**：原 `internal/cloud` 更名为 `internal/remote`。使用 "Remote" 而非 "Cloud"，以精确表达其为 Sync 的“远端实体”，不管它部署在本地挂载盘、关系型数据库还是真正的公有云上。
2. **URI 驱动与工厂注册 (Registry Pattern)**：不再在配置解析逻辑中写大量的 `if endpoint == "s3"`。我们将实现 `Register(scheme string, factory StoreFactory)`，并在各自的子模块或 `init()` 中注册对应的介质支持。
3. **接口名称变更**：将 `StorageBackend` 更名为 `BlobStore`（或 `RemoteStorage`）。其契约保持不变：基于 `baseRev` / `ETag` 提供支持并发锁（乐观控制）的存取。

## Risks / Trade-offs

- **Risk: 兼容性中断** → Mitigation: 当前的配置 `.pinax/cloud/config.json` 如果有遗留结构（比如 `Type: s3` 字段），迁移时可能要保证新配置机制对 `Endpoint` URI 的向后兼容。
- **Risk: 测试断裂** → Mitigation: 现有的 `cloud_sync.txt` e2e 测试与 E2E fake server 必须同步更新，确保 `BlobStore` 迁移后 CLI 端到端流程依旧能够正常闭环。
