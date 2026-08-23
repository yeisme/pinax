## 背景

`pinax-sync-output-ux` task 6 的真实 S3 双设备回归（2026-08-22，MinIO `pinax-e2e-20260822` 桶）暴露了一个 manifest v2 与冲突保留机制的组合缺陷：

1. pull 的冲突保留采用「remote wins + 本地留 `<base>.<ts>.conflict.md` 副本」模型，副本完整保留原笔记 frontmatter（含 canonical `note_id`）。
2. manifest v2 要求每个同步对象拥有唯一 canonical object ID；`BuildManifestV2`/`buildSyncManifestIdentityAudit` 把冲突副本当作独立对象扫描，与活笔记同 ID。
3. 结果：v2 vault 一旦在 pull 中产生任意冲突副本，identity audit 立即 `duplicate_object_id` 不合格，此后**所有 push 永久失败**，仅有手动修复提示，无自动恢复路径。

## 目标

- 冲突副本定义为本地保留快照（manual merge 用），不是受管同步对象：`BuildManifestV2` 过滤 `*.<14位时间戳>.conflict.md` 条目并保持 `EntryCount` 一致。
- `buildSyncManifestIdentityAudit` 在扫描笔记与 manifest 条目两处跳过冲突副本路径，audit 保持 eligible。
- v1 manifest 行为不变（冲突副本仍按原样同步）。
- 真实 S3 双设备回归（`TestSyncOutputRealDualDeviceRegression`）作为端到端证据：收敛、二次 push up_to_date、双向编辑、冲突副本保留、删除标记。

## 非目标

- 不改变冲突副本的创建与命名（`syncConflictCopyPath` 不动）。
- 不实现冲突自动合并或副本自动清理；manual merge 语义保持。
- 不处理 v1 vault 中已同步的历史冲突副本（无 identity 约束，无影响）。
