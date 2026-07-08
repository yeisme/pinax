# pinax-cli-sync-unification Tasks

## Task 1: 移除 pinax cloud 命令组

**文件**：`internal/cli/ops_integration_cmd.go`

- [ ] 删除 `addCloudCommands` 函数（L57-123）
- [ ] `internal/cli/root.go`：删除 `addCloudCommands(cmd, ctx)` 调用
- [ ] 验证：`go build ./...`

## Task 2: 移除 pinax git snapshot 命令组

**文件**：`internal/cli/git_cmd.go`

- [ ] 删除整个 `git_cmd.go` 文件
- [ ] `internal/cli/root.go`：删除 `addGitCommands(cmd, ctx)` 调用
- [ ] 验证：`go build ./...`

## Task 3: 移除 pinax storage Hidden 命令

**文件**：`internal/cli/storage_cmd.go`

- [ ] 删除 `storageSetLocalCmd`（L10-21，Hidden=true）
- [ ] 删除 `storageSetS3Cmd`（L22-38，Hidden=true）
- [ ] 保留 `storageSetCmd` + `storage set local|s3`（非 Hidden）
- [ ] 保留 `storage status` + `storage doctor`
- [ ] 验证：`go build ./...`

## Task 4: 移除 pinax backend sync 操作

**文件**：`internal/cli/ops_integration_cmd.go`

- [ ] 删除 `backendPushCmd`（L284-287）
- [ ] 删除 `backendPullCmd`（L288-291）
- [ ] 删除 `backendDiffCmd`（L281-283）
- [ ] 保留 `backend list|add|show|doctor|capabilities|remove`
- [ ] 保留 `backend object list|stat`
- [ ] 保留 `backend notes summary|list|stat`
- [ ] 验证：`go build ./...`

## Task 5: 清理测试引用

**文件**：`internal/cli/operation_catalog_test.go`

- [ ] 搜索并删除 cloud 相关测试条目
- [ ] 搜索并删除 git snapshot 相关测试条目
- [ ] 搜索并删除 storage Hidden 相关测试条目
- [ ] 搜索并删除 backend sync 操作相关测试条目

**文件**：`internal/cli/output_contract_test.go`

- [ ] 搜索并删除 cloud 相关用例
- [ ] 搜索并删除 git snapshot 相关用例
- [ ] 搜索并删除 storage Hidden 相关用例
- [ ] 搜索并删除 backend sync 操作相关用例

## Task 6: 验证

- [ ] `go build -tags=nomsgpack ./...`
- [ ] `go test -tags=nomsgpack ./... -count=1`
- [ ] `go run -tags=nomsgpack ./cmd/pinax --help`（无 cloud/git snapshot 冗余）
- [ ] `go run -tags=nomsgpack ./cmd/pinax sync push --help`（仍工作）
- [ ] `go run -tags=nomsgpack ./cmd/pinax backend object list --help`（仍工作）
- [ ] `go run -tags=nomsgpack ./cmd/pinax capsa login --help`（仍工作）
- [ ] `go run -tags=nomsgpack ./cmd/pinax storage set local --help`（仍工作）
- [ ] `openspec validate pinax-cli-sync-unification --strict`
- [ ] `task check`（pinax，如存在）

## Task 7: 文档更新

- [ ] `docs/cli-reference.md`：删除 cloud/git snapshot/storage Hidden/backend sync 操作文档
- [ ] `docs/cli-reference.md`：更新命令树（统一到 sync + capsa + storage + backend blob 浏览）
- [ ] 添加迁移说明（cloud→capsa, backend push→sync push）

## Task 8: OpenSpec 归档准备

- [ ] 确认所有 task 完成
- [ ] 确认所有验证通过
- [ ] 准备归档（主 session 统一提交）

## 依赖顺序

Task 1 → Task 2 → Task 3 → Task 4 → Task 5 → Task 6 → Task 7 → Task 8

（每步后验证 `go build ./...`，确保增量正确）
