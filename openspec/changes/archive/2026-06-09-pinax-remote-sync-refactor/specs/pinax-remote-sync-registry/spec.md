## ADDED Requirements

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
