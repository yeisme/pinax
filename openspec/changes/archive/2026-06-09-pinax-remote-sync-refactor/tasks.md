## 1. 重构文件与基础结构准备

- [x] 1.1 将 `internal/cloud` 目录重命名为 `internal/remote`。
- [x] 1.2 将 `StorageBackend` 接口重命名为 `BlobStore`（位于 `internal/remote/backend.go`，建议也可更名为 `store.go`）。
- [x] 1.3 修复重命名引发的模块内部的引用错误，并执行初步的 `task fmt-check` 和 `task build` 验证包结构。

## 2. 引入 URI Scheme 注册表

- [x] 2.1 在 `internal/remote/registry.go` 中新增 `Register` 和 `NewStore(uri string)` 方法。
- [x] 2.2 提取原有的 `s3_backend.go` 和 `file_backend.go`（或 `local_backend.go`）中的初始化逻辑，使它们成为符合工厂签名的方法，并在各自 of init() 中通过 `remote.Register` 注入 "s3" 和 "file"。
- [x] 2.3 修改 `cloud.Load` (现改为 `remote.Load`) 中的配置载入逻辑，使之调用 `remote.NewStore` 而不是通过 hardcode 路由。

## 3. 应用层迁移与 CLI 命令修复

- [x] 3.1 迁移 `internal/app/service_sync_cloud.go`（可能需更名为 `service_sync_remote.go`）中使用到原来 `cloud` 包的方法为调用 `remote`。
- [x] 3.2 迁移 `cmd/pinax/sync_cmd.go` 中的 `pinax sync init` 逻辑：原 `--endpoint s3://...` 现在的合法性验证，改写为根据 scheme 到 `remote` 注册表中查找是否支持。
- [x] 3.3 修改 E2E 测试如 `cloud_sync.txt` 中的依赖代码，验证 CLI 参数调用与之前行为一致无断裂。

## 4. 验证与归档

- [x] 4.1 在根目录运行 `task test` 或 `go test ./...`，确保 `internal/remote` 及其相关调用的单元测试全部通过。
- [x] 4.2 执行完整的端到端 Sync 测试流（包含 push, pull 和 status 等指令），确保无任何功能倒退。
- [x] 4.3 提交前运行 `openspec validate --all` 并归档任务状态。
