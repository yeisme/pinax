## Why

当前 Pinax 对笔记文件已有 `note delete` 移入 `.pinax/trash/` 的保护语义，但 project、subproject、board 配置、view、template 等结构化资产仍缺少统一删除生命周期。用户把 `history` 改成 `history-learning` 后，旧 project 仍留在 `.pinax/projects.json`，说明“删除目录”与“删除注册索引”没有被同一条 CLI-authored 路径管理。

这个问题如果扩展到 Cloud Sync，会更危险：仅靠 manifest 中缺少某个文件无法判断是用户删除、同步遗漏、忽略规则变化，还是远端旧设备覆盖。因此需要补齐统一回收站、tombstone、索引刷新和同步删除记录。

## What Changes

- 新增统一 vault 回收站能力，覆盖 note 以外的结构化对象：project、subproject workspace、project board config、database/view/template 等后续可逐步接入。
- 新增 `pinax trash list/show/restore/purge` 读写面；默认删除只进入回收站，真实清除必须显式 `--hard --yes` 或 purge policy。
- 新增 `pinax project delete <project>` 和 `pinax project subproject delete <project> <subproject>`，删除时同步更新 registry projection、写入 tombstone、移动可恢复内容到 `.pinax/trash/<date>/...`，并刷新索引。
- 扩展 Cloud Sync manifest 语义：删除必须通过 encrypted tombstone/delete marker 传播，回收站备份作为 encrypted content entry 同步；pull 必须按 tombstone 更新本地 registry/index，而不是只写远端 manifest 中存在的文件。
- 扩展索引维护：删除/恢复 project 或 subproject 后，搜索、board、project list 默认排除 trashed/deleted 对象，并提供显式 include-trash 查询。
- 保持向后兼容：不移除现有 `note delete`、`template delete`、`view delete` 行为；新增字段和命令为 additive，旧输出字段保留。

## Capabilities

### New Capabilities

- `vault-trash-lifecycle`: 统一回收站、tombstone、恢复、清理和误删保护语义。

### Modified Capabilities

- `project-board-workspace`: project/subproject 删除、恢复、默认隐藏 trashed/deleted workspace，并保持 CLI 输出合同。
- `pinax-cloud-sync`: encrypted manifest 支持 tombstone/delete marker 和 trash backup，pull/push 同步删除语义。
- `notebook-index-search`: 索引 refresh/rebuild 对删除和恢复后的 note/project/workspace projection 进行收敛。
- `vault-record-ledger`: ledger tombstone 从 note-only 扩展为可记录 vault object lifecycle 事件。

## Impact

- 代码范围：`internal/app/service.go`、`internal/app/project_workspace.go`、`internal/cli/project_cmd.go`、新增 trash service/CLI、`internal/domain` lifecycle/tombstone 类型、`internal/records` ledger、`internal/remote`/`internal/cloudsync` manifest、`internal/index` 增量删除/恢复路径、`internal/output` 投影渲染、`cmd/pinax` contract tests。
- 稳定面：新增 CLI 命令和 `--json`/`--agent` facts；新增 registry/tombstone fields；新增 Cloud Sync manifest optional fields。全部按 additive 演进，不删除或重命名既有字段。
- 验证范围：Go 单元测试、process CLI tests、sync 双设备 fixture、integration evidence wrapper、`openspec validate --all`、`task check`。
