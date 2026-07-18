# pinax-cli-sync-unification Tasks

## Task 1: 移除 pinax cloud 命令组

**文件**：`internal/cli/ops_integration_cmd.go`

- [x] 删除 `addCloudCommands` 函数（L57-123）
- [x] `internal/cli/root.go`：删除 `addCloudCommands(cmd, ctx)` 调用
- [x] 验证：`go build ./...`

## Task 2: 移除 pinax git snapshot 命令组

**文件**：`internal/cli/git_cmd.go`

- [x] 删除整个 `git_cmd.go` 文件
- [x] `internal/cli/root.go`：删除 `addGitCommands(cmd, ctx)` 调用
- [x] 验证：`go build ./...`

## Task 3: 移除 pinax storage Hidden 命令

**文件**：`internal/cli/storage_cmd.go`

- [x] 删除 `storageSetLocalCmd`（L10-21，Hidden=true）
- [x] 删除 `storageSetS3Cmd`（L22-38，Hidden=true）
- [x] 保留 `storageSetCmd` + `storage set local|s3`（非 Hidden）
- [x] 保留 `storage status` + `storage doctor`
- [x] 验证：`go build ./...`

## Task 4: 移除 pinax backend sync 操作

**文件**：`internal/cli/ops_integration_cmd.go`

- [x] 删除 `backendPushCmd`（L284-287）
- [x] 删除 `backendPullCmd`（L288-291）
- [x] 删除 `backendDiffCmd`（L281-283）
- [x] 保留 `backend list|add|show|doctor|capabilities|remove`
- [x] 保留 `backend object list|stat`
- [x] 保留 `backend notes summary|list|stat`
- [x] 验证：`go build ./...`

## Task 5: 清理测试引用

**文件**：`internal/cli/operation_catalog_test.go`

- [x] 搜索并删除 cloud 相关测试条目
- [x] 搜索并删除 git snapshot 相关测试条目
- [x] 搜索并删除 storage Hidden 相关测试条目
- [x] 搜索并删除 backend sync 操作相关测试条目

**文件**：`internal/cli/output_contract_test.go`

- [x] 搜索并删除 cloud 相关用例
- [x] 搜索并删除 git snapshot 相关用例
- [x] 搜索并删除 storage Hidden 相关用例
- [x] 搜索并删除 backend sync 操作相关用例

## Task 6: 验证

- [x] `go build ./...`
- [x] `go test ./... -count=1`
- [x] `pinax --help`（无 cloud/git snapshot 冗余）
- [x] `pinax sync push --help`（仍工作）
- [x] `pinax backend object list --help`（仍工作）
- [x] `pinax capsa login --help`（仍工作）
- [x] `pinax storage set local --help`（仍工作）
- [x] `openspec validate pinax-cli-sync-unification --strict`

## Task 7: 文档更新

- [x] `docs/commands/`：cloud.md 已替换为 capsa.md
- [x] `docs/commands/README.md`：命令树已统一到 sync + capsa + storage + backend blob 浏览
- [x] 迁移说明已在 capsa.md 和 sync.md 中体现

## Task 8: OpenSpec 归档准备

- [x] 确认所有 task 完成
- [x] 确认所有验证通过
- [x] 准备归档（主 session 统一提交）
