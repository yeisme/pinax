## MODIFIED Requirements

### Requirement: 多种存储后端支持与并发锁

同步模块 SHALL 通过抽象的 Remote BlobStore 支持不同的云端盲存储介质，并通过注册表模式加载它们，并对所有介质提供乐观并发控制。

#### Scenario: S3 或兼容对象存储 (S3 API)

- **WHEN** 用户配置后端为 `s3://` 协议
- **THEN** 系统 SHALL 通过 S3 原生的 `If-Match` 和 ETag 机制来进行 Manifest 更新的并发保护

#### Scenario: 本地/网络挂载文件系统 (JuiceFS / FUSE / SMB)

- **WHEN** 用户配置后端为 `file://` 等本地目录协议
- **THEN** 系统 SHALL 直接使用 Go 核心的文件系统 API 进行读写
- **AND** SHALL 利用原子级的临时文件重命名或跨平台的 `flock` 机制来保证 Manifest 的并发一致性

#### Scenario: 动态 URI Scheme 注册路由

- **WHEN** 系统加载存储介质时
- **THEN** 必须通过 `remote.NewStore(uri)` 根据协议头动态匹配工厂函数，不得在配置解析模块硬编码。
