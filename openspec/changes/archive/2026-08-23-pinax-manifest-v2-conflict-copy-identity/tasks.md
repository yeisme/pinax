## 任务

- [x] 1. `remote.IsConflictCopyPath` + `BuildManifestV2` 过滤冲突副本并保持 `EntryCount` 一致；`ValidateV2` 不变。
- [x] 2. `buildSyncManifestIdentityAudit` 在 scanNotes 与 manifest 条目两处跳过冲突副本；存在副本时 audit 保持 eligible。
- [x] 3. 单元测试 `TestBuildManifestV2KeepsConflictCopiesLocal`。
- [x] 4. 真实 S3 端到端回归 `TestSyncOutputRealDualDeviceRegression`（env 门控，经 `syncrealevidence` runner），证据 `temp/integration-test-runs/20260822T094645Z-1210789`（MinIO S3 直连 + manifest v2 + CAS）。
- [x] 5. 回归确认：`go test ./internal/remote ./internal/app ./tests/e2e`、`golangci-lint run`、`openspec validate --all` 全绿。

## 已知边界

- 误伤面：用户文件名恰好为 `*.<14位数字>.conflict.md` 会被视为本地副本不参与 v2 同步。
- COS 专属 addressing（virtual-hosted）路径未在本次真实回归覆盖（MinIO 为 path-style）；发布兼容评审时如需 COS 证据另跑。
