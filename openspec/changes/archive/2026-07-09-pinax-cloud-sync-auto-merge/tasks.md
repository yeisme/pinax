## Tasks

- [x] 为本地移动后 pull 不恢复旧 `index/...` 路径补服务层回归测试。
- [x] 为 `pinax sync` 自动推送本地移动、另一设备删除旧路径补两设备测试。
- [x] 增加 manifest baseline cache，并让 pull 按三方计划应用。
- [x] 将 `sync diff/push/pull` 默认 target 切到 `cloud`，保留显式 legacy target。
- [x] 为 note soft delete marker、远端 note delete marker pull 写回 trash lifecycle 补测试和实现。
- [x] 运行聚焦测试：`go test ./internal/app ./internal/records ./cmd/pinax -run 'TestCloudSync|TestNoteSoftDeleteCreatesCloudDeleteMarker|TestLedgerTrashedNoteMaterializesTombstone|TestSyncSubcommandsDefaultToCapsaCLI' -count=1`。
