## 背景

真实 S3 双设备回归（2026-08-22，`pinax-manifest-v2-conflict-copy-identity` 证据链）暴露了 pull 冲突判定的结构性噪声：

- planner 的 `hasLocal && hasRemote` 且 blob 不同分支（`internal/sync/planner.go` L188-196）对 `download_blob` 操作**不携带 base 比较**：本地未改的纯远端更新与双方都改的真冲突落入同一分类。
- apply 侧（`internal/app/cloud_sync.go` L288）对该分类无条件 `preserveConflict=true`——只要本地文件字节 ≠ 远端内容就留 `<base>.<ts>.conflict.md` 副本。
- 结果：**多设备 vault 的每一次顺序编辑**（A 改→push→B pull）都会在 B 留下一份与 base 完全相同、毫无信息量的冲突副本。2026-08-22 修复后这些副本永久留在本地，`sync conflicts list` 被噪声淹没，用户会训练性忽略真冲突。
- 对照：`move` 分支（planner L185 带 `BaseRevision`；apply L319 `preserveConflict = op.BaseRevision == "" || op.LocalRevision != op.BaseRevision`）已经实现了正确规则，但没有覆盖主路径。

## 目标

- planner 在 `hasLocal && hasRemote` 分支计算 `localUnchanged`（对照 L201 同一规则），将 `LocalRevision`/`BaseRevision` 写入 `download_blob` 操作。
- apply 侧 `download_blob` 采用与 `move` 一致的 preserveConflict 规则：本地未偏离 base → 远端静默 fast-forward，不留副本；本地偏离 base → 保留现有冲突副本行为。
- 安全带：apply 时将本地文件 hash 与「上次同步的本地 manifest blob」比对，plan 计算后又被编辑的文件（TOCTOU）仍然保留副本，不静默覆盖。
- 收敛后的二次 pull 在无变更时报 `result=up_to_date`（与 push 侧 fast path 对齐；facts `sync.result`、`up_to_date`）。
- 真冲突（双方都改）行为完全不变：副本保留 + `sync.conflicts` 计数 + `conflicts resolve` 流程。

## 非目标

- 不实现内容级三方合并（auto-merge 语义不变，冲突仍走 manual resolve）。
- 不改变 push 侧、delete marker、move 语义。
- 不改变冲突副本命名与 v1 vault 同步行为。
- 不引入远端协议或 wire 格式变更。
