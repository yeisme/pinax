# 设计

## 判定

`remote.IsConflictCopyPath(rel)`：正则 `\.\d{14}\.conflict\.md$`，与 `syncConflictCopyPath` 的 `<base>.<yyyyMMddHHmmss>.conflict.md` 命名一一对应。误伤面：用户自建文件名恰好带 14 位数字段 + `.conflict.md` 后缀——极小概率，且此类文件本就是用户对冲突的显式命名。

## 修改点

1. `internal/remote/manifest.go` `BuildManifestV2`：在 identity 装配前过滤冲突副本条目（副本既不需要 identity，也不会以同 ID 重复出现在远端 manifest）；同步修正 `EntryCount`。`ValidateV2` 的唯一性校验因此天然通过。
2. `internal/app/sync_manifest_migration.go` `buildSyncManifestIdentityAudit`：`scanNotes` 循环与 manifest 条目循环均跳过冲突副本路径。副本不再进入 `identities`/`managedNotePaths`，audit 在存在冲突副本时保持 eligible。

## 语义

- 冲突副本 = 冻结的 pre-pull 内容快照，与 trash/tombstone 同属「本地保留、不传播」类别。它不是第二个对象；把它同步到所有设备只会复制 ID 歧义。
- 兼容性：旧远端 manifest 里若已含冲突副本条目，新版本本地构建不再包含它——对象从 manifest 消失不产生 tombstone（ Deletes 只来自 trash），pull 侧已有本地文件不受影响；设备逐台升级后自愈，无数据丢失。

## 验证

- 单元：`TestBuildManifestV2KeepsConflictCopiesLocal`（v2 manifest 只含活笔记、EntryCount 一致、pattern 精确匹配/不误伤）。
- 诊断复现：temp/s3e2e/repro 全序列（push → 收敛 up_to_date → B 编辑 push → A pull 产生副本 → A 再 push）修复前 `manifest_identity_migration_required`，修复后全绿。
- 端到端：`TestSyncOutputRealDualDeviceRegression`（真实 MinIO S3 直连、manifest v2、CAS 条件写路径），经 `tools/testkit/syncrealevidence` 产出证据 `temp/integration-test-runs/20260822T094645Z-1210789`。
