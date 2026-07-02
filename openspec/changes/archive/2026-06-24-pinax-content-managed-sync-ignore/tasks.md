# pinax-content-managed-sync-ignore Tasks

- [x] 1. 新增 `.pinaxignore` 解析与默认模板。
  - 证据：`go test ./internal/vaultignore -count=1`
- [x] 2. 将 Cloud Sync manifest 扩展为未忽略普通文件集合。
  - 证据：`go test ./internal/remote -count=1`
- [x] 3. 新 vault 写入 `.pinaxignore` 和 metadata-only `.gitignore`。
  - 证据：`go test ./internal/app -run 'TestVaultInitValidateSearchAndShow|TestVaultIgnoreStatusPlanApplyMaintainsPinaxBlocks' -count=1`
- [x] 4. 增加 `pinax vault ignore status|plan|apply`。
  - 证据：`go test ./cmd/pinax -run 'TestInitWithoutArgUsesVaultFlagDefault|TestVaultHelpAndDashboard' -count=1`
- [x] 5. 补充 focused tests、OpenSpec 验证和 `task check` 证据。
  - 证据：`go test ./internal/vaultignore ./internal/remote -count=1`
  - 证据：`go test ./internal/app -run 'TestServerBackedSyncPushUpdatesExistingUploadOnlyMetadata|TestRcloneDirectSyncUsesObjectStoreEngineAndLockRecovery|TestCloudSyncPullPreservesScriptMode' -count=1`
  - 证据：`go test ./cmd/pinax -run 'TestAssetRelationshipCommandsCLI|TestVersionRestoreApplyContractAcrossModes|TestVersionRestoreApplyRevertsBadLocalApply|TestDirectCloudPushPullCLI' -count=1`
  - 证据：`task check`
- [x] 6. 将 version restore 从 Git 内容 checkout 迁到 Pinax local content objects。
  - 证据：`go test ./internal/version -run 'TestLocalBackendImplementsVersionBackendContract|TestLocalBackendSnapshotStoresReadableContentObjects|TestVersionBackendCapabilityGuardsReturnStableErrors' -count=1`
  - 证据：`go test ./cmd/pinax -run 'TestVersionRestoreApplyRevertsBadLocalApply|TestVersionRestoreApplyRefusesStalePlan|TestVersionRestoreApplyContractAcrossModes|TestVersionRestoreApplyUsesLocalSnapshotWithoutGitCommit' -count=1`
  - 证据：`go test ./tests/e2e -run TestDemoRestore -count=1`
  - 证据：`task check`
