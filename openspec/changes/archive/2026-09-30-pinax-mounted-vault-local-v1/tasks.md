# 任务

- [x] 1.1 `InitVault` 保留已有 notes 目录/符号链接。验证：`go test ./internal/app -run InitVault -count=1` Evidence: TestInitVaultKeepsExistingNotesSymlink PASS
- [x] 1.2 doctor 加法 facts 与 `control_plane_on_remote`。验证：`go test ./internal/app -run 'StorageDoctor|MountedVault|VaultDoctor' -count=1` Evidence: PASS
- [x] 1.3 文档 Pattern D：`docs/architecture/cloud-sync-design.md`、`docs/commands/storage.md`
- [x] 1.4 `openspec validate pinax-mounted-vault-local-v1 --strict --no-interactive` Evidence: valid
