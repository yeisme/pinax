# 设计

## 判定规则（与 move 分支统一）

```
preserveConflict(op, localFileHash) =
    op.BaseRevision == ""                       // 无 base 信息（首次同步该对象）→ 保守保留
    || op.LocalRevision != op.BaseRevision       // planner 判定本地已偏离 base → 保留
    || localFileHash != lastSyncedLocalBlob      // plan 之后本地又被改（TOCTOU）→ 保留
```

三个条件全部为假（本地 == base == 上次同步状态）时：远端内容直接落盘，无副本、无 conflict 计数。

## 修改点

1. `internal/sync/planner.go`：`hasLocal && hasRemote` 分支构造 download_blob 时补 `LocalRevision: localEntry.RevisionID, BaseRevision: baseEntry.RevisionID`（沿用 `objectOperation` 或显式字段）；`localUnchanged` 判定复用 L201 的 `RevisionID/BlobID` 双比对。
2. `internal/app/cloud_sync.go` direct pull：`download_blob` 分支从 plan op 读取上述字段计算 preserveConflict（对齐 L319 move 分支）；apply 前读本地文件 hash 与本地 manifest entry BlobID 比对作为 TOCTOU 防线。本地 manifest 在 pull 上下文已加载（BuildPlan 输入），随 op 传递。
3. up-to-date pull fast path：apply 阶段 zero ops 且本地/远端 manifest 内容一致时，`result=up_to_date`（facts 与 sync_view 与 push 侧一致），`files_applied=0` 保留。

## 兼容性

- `download_blob` op 新增字段为 additive（plan JSON 增量字段，旧消费者忽略）。
- 行为变化仅限「本地未偏离 base」的 pull：从留副本改为不留。真冲突路径字节级行为不变。
- `sync_view` 中 `C` 分类计数随之只反映真冲突——与文档语义一致。

## 风险

- 误判「本地未改」会静默覆盖本地编辑。防线：planner 用 manifest 事实（非文件 mtime），apply 用内容 hash 二次确认；两条独立证据都指向未改才 fast-forward。
- base manifest 缺失（对象首同步）保守保留，与现状一致。

## 验证

- 单元：planner 分类（顺序编辑→download_blob 带 base=local；双方编辑→download_blob 带 base≠local）；apply 规则三条件真值表；TOCTOU 用例。
- file:// e2e：顺序编辑 pull 后无 `*.conflict.md`；双方编辑 pull 后恰好 1 份副本且内容为本地版。
- 真实 S3：扩展 `TestSyncOutputRealDualDeviceRegression`——双向编辑阶段断言零副本，冲突阶段断言恰好 1 副本；二次 pull 断言 `up_to_date`。
- 证据经 `syncrealevidence` 归档。
