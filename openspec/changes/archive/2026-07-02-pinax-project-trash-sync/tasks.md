## 1. 设计与合同基线

- [x] 1.1 在 `internal/domain` 新增通用 vault object lifecycle/tombstone 类型，覆盖 `kind=note|project|subproject|registry_asset|view|template`，并保留现有 note lifecycle 兼容别名。
- [x] 1.2 新增 trash service 单元测试，先覆盖 active object trash、restore、purge dry-run、同名 trash path 冲突、禁止 path escape。
- [x] 1.3 实现 trash service 的最小读写模型：只通过 application service 写 `.pinax/trash/**`、`.pinax/records/tombstones.json` 或新增 trash registry。
- [x] 1.4 更新 CLI output contract tests，固定 `trash.list`、`trash.restore`、`project.delete`、`project.subproject.delete` 的 `--json`、`--agent`、默认 human 输出。

## 2. Project 与 Subproject 删除恢复

- [x] 2.1 为 `pinax project delete <project>` 写失败测试：缺少 `--yes` 返回 `approval_required`，删除 current project 自动切换或清空 current project，删除不存在 project 返回 `project_not_found`。
- [x] 2.2 实现 `ProjectDelete` app service：从 active registry 移除 project，移动 project registry fragment 和可选 notes prefix/workspace 内容到 trash，写 tombstone 和 event。
- [x] 2.3 为 `pinax project subproject delete <project> <subproject>` 写失败测试：非空 workspace 无近期 snapshot 返回 `snapshot_required`，缺少 `--yes` 不写任何文件。
- [x] 2.4 实现 `ProjectSubprojectDelete` app service：移动 workspace 目录、workspace registry、board config/current workspace 引用到 trash，并更新 active list。
- [x] 2.5 实现 `pinax trash restore project/<slug>` 和 `pinax trash restore subproject/<project>/<slug>`，恢复 registry 和 workspace path，冲突时返回 `restore_conflict`。

## 3. 索引和列表收敛

- [x] 3.1 写 `project list/show/subproject list/board show` 回归测试，确认 trashed/deleted 默认隐藏，错误 next action 指向 `pinax trash restore ...`。
- [x] 3.2 扩展 `index refresh`/incremental delete 测试，确认 project/subproject 删除后 board/search/link projection 不再引用已删除 workspace。
- [x] 3.3 实现 index refresh 读取 tombstone/lifecycle，默认排除 deleted/trashed；新增显式 include-trash 查询只在 trash 命令或后续筛选中暴露。
- [x] 3.4 更新 vault doctor/repair plan：发现 active registry 缺文件时生成 review plan，不直接删除；发现 tombstone 缺 trash backup 时报告 `trash_backup_missing`。

## 4. Cloud Sync 删除传播

- [x] 4.1 为 manifest 写兼容测试：旧 `entries` 仍可解析，新 `deletes` optional 字段包含 path_hash/object_kind/tombstone_id/trash_blob_id，不暴露明文路径或正文。
- [x] 4.2 扩展 `BuildManifest`：包含 active content entries、encrypted trash backup entries、delete marker summary，并继续 hard-deny `.pinax/index.sqlite`、cache、secret 路径。
- [x] 4.3 扩展 push：上传 missing trash backup blob 和 manifest delete marker；只有 revision CAS commit 成功后 `remote_write=true`。
- [x] 4.4 扩展 pull：按 delete marker 更新本地 registry/tombstone/index；如果本地对象有未同步改动，保留 conflict copy 并返回 conflict next actions。
- [x] 4.5 增加双设备 e2e/testscript：A 删除 project 并 push，B pull 后 `project list` 不显示旧 project，`trash list` 可见 tombstone，`trash restore` 可恢复。

## 5. 文档、验证与关闭

- [x] 5.1 更新 `README.md`、`docs/commands/project.md`、`docs/commands/sync.md`，示例只使用真实 `pinax` 命令。
- [x] 5.2 运行 `go test ./internal/app ./internal/remote ./internal/cloudsync ./cmd/pinax -run 'Trash|Project.*Delete|Subproject.*Delete|Manifest.*Delete|Sync.*Delete' -count=1`，预期通过。
- [x] 5.3 运行 `task check`，预期 fmt、lint、test、build、kb sidecar protocol、OpenSpec validate 全部通过。
- [x] 5.4 运行 `openspec validate --all`，预期无 OpenSpec 错误；若失败，修正 spec delta 后重跑。

## 实施证据

- 2026-06-27：新增 `cmd/pinax/project_trash_sync_command_test.go`、`internal/remote/manifest_delete_test.go`、`internal/cloudsync/manifest_delete_test.go`，先观察到缺少 `trash`/`project delete`/`deletes` 字段导致 RED，再实现并通过 GREEN。
- 2026-06-27：`go test ./internal/app ./internal/remote ./internal/cloudsync ./cmd/pinax -run 'Trash|Project.*Delete|Subproject.*Delete|Manifest.*Delete|Sync.*Delete' -count=1` 通过。
- 2026-06-27：`openspec validate pinax-project-trash-sync` 通过。
- 2026-06-27：`task check` 通过，覆盖 fmt-check、lint、`go test ./...`、build、kb sidecar protocol、`openspec validate --all`。
- 2026-07-01：补齐 `internal/app/trash_test.go`，覆盖 project trash/restore、purge dry-run、重复 trash path 避让和 path escape；`go test ./internal/app -run TestTrashServiceRestorePurgeDryRunAndPathBoundary -count=1` 通过。
- 2026-07-01：补齐 `cmd/pinax/project_trash_sync_command_test.go`，覆盖 `project.delete`/`trash.list`/`trash.restore`/`trash.purge` 的 `--json`、`--agent`、默认 human 输出，current project 删除切换、`project_not_found`、board show 隐藏已删除 subproject、index/search 不暴露 trash backup；`go test ./cmd/pinax -run 'TestProject(DeleteTrashRestoreCLIContract|SubprojectDeleteRequiresSnapshotAndRestoresWorkspace|DeleteEdgeCasesAndOutputModes)' -count=1` 通过。
- 2026-07-01：补齐 Cloud Sync pull delete marker 应用：device A 删除 project 并 push，device B pull 后从 active registry 移除 project、写本地 tombstone、`trash restore` 可恢复；`go test ./internal/app -run TestCloudSyncPullAppliesProjectDeleteMarker -count=1` 通过。
- 2026-07-01：补齐 Cloud Sync pull 本地内容冲突信号：远端 delete marker 命中本地含内容 project 时，把本地内容保留为 trash conflict backup，`sync.pull` 返回 `conflicts=1`、`conflict.1.file` 和 `pinax trash restore ...` next action；`go test ./internal/app -run TestCloudSyncPullDeleteMarkerReportsLocalContentConflict -count=1` 通过。
- 2026-07-01：补齐双设备 e2e/testscript `cloud_direct_two_device.txt`，覆盖 delete marker push/pull/list/restore；`go test ./tests/e2e -run TestCloud/cloud_direct_two_device -count=1` 通过。
- 2026-07-01：补齐 vault doctor/repair review-only 诊断：active workspace 缺路径生成 `project_workspace_missing`，tombstone 缺 trash backup 生成 `trash_backup_missing`，不自动删除 registry；`go test ./internal/app -run 'TestVaultDoctorAndRepairPlan(ReportMissingTrashBackup|ReviewMissingActiveWorkspace)' -count=1` 通过。
- 2026-07-01：聚焦回归 `go test ./internal/app ./internal/remote ./internal/cloudsync ./cmd/pinax ./tests/e2e -run 'Trash|Project.*Delete|Subproject.*Delete|Manifest.*Delete|Sync.*Delete|CloudSyncPull|TestCloud/cloud_direct_two_device|VaultDoctorAndRepairPlan' -count=1` 通过。
- 2026-07-01：最终门禁 `task check` 通过，覆盖 `openspec validate --all`、`golangci-lint run`、`golangci-lint fmt --diff`、`go test ./...`、Go build、KB sidecar protocol、Pinax web renderer build/test。

## 延期/未完成说明

- 无剩余延期项。当前实现已补齐 project/subproject trash lifecycle、CLI 输出合同、index/search/board 收敛、doctor/repair review-only 诊断、Cloud Sync delete marker pull 应用和双设备 e2e。后续若要扩展到 template/view/database delete，应另建增量 change 复用 trash service。
