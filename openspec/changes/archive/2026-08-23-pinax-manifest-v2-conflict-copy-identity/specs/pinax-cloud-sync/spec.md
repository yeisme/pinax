## ADDED Requirements

### Requirement: 冲突副本 SHALL 保持本地

manifest v2 同步 SHALL 把 `*.<14位时间戳>.conflict.md` 冲突副本视为本地保留快照：`BuildManifestV2` SHALL NOT 将其写入远端 manifest，identity 审计 SHALL NOT 因其存在而判定 `duplicate_object_id`。

#### Scenario: 冲突后继续 push

- **WHEN** pull 在 v2 vault 中保留了冲突副本后设备再次 push
- **THEN** identity 审计 SHALL 保持 eligible
- **AND** push SHALL 正常提交，活笔记条目不受影响

#### Scenario: 副本不上行

- **WHEN** `BuildManifestV2` 构建远端 manifest
- **THEN** 冲突副本路径 SHALL NOT 出现在 entries 中
- **AND** `EntryCount` SHALL 等于 entries 数量

#### Scenario: v1 行为不变

- **WHEN** v1 manifest vault 产生冲突副本
- **THEN** 现有同步行为 SHALL 保持不变
