## 任务

- [x] 1. planner：`hasLocal && hasRemote` 分支为 `download_blob` 补 `LocalRevision`/`BaseRevision`（`localUnchanged` 复用既有双比对规则）。
- [x] 2. apply：`download_blob` 采用统一 preserveConflict 规则 + 本地内容 hash TOCTOU 防线；`move` 分支规则不变。
- [x] 3. up-to-date pull fast path：无操作且 manifest 一致时报 `result=up_to_date`，与 push 侧 facts/sync_view 对齐。
- [x] 4. 单元测试：planner 分类、apply 规则真值表、TOCTOU 用例。
- [x] 5. file:// e2e：顺序编辑零副本；双方编辑恰好 1 副本（扩展现有 `cloud_direct_two_device.txt`）。
- [x] 6. 真实 S3 回归扩展：双向阶段零副本、冲突阶段 1 副本、二次 pull `up_to_date`；经 `syncrealevidence` 归档证据。
- [x] 7. `docs/commands/sync.md` 冲突副本条件更新（仅本地偏离 base 时保留）；`task check` + `openspec validate --all` 全绿。

每个未完成任务都必须保留脱敏失败证据；正文 diff 不得进入 receipt、事件日志或对象存储。

证据（2026-08-23）：planner 单测 `internal/sync/planner_fast_forward_test.go`（v2 对象分支双证明/无证明/无 base 四类 + v1 路径分支 blob 证明）；file:// e2e 顺序编辑零副本 + 收敛 pull `up_to_date`；真实 S3（MinIO 直连）`TestSyncOutputRealDualDeviceRegression` 全序列通过（双向阶段零副本、冲突阶段恰好 1 副本、二次 push/pull 均 `up_to_date`），证据 `temp/integration-test-runs/20260823T135145Z-1082619`。实现说明：planner 显式 `FastForward`+`LocalBlobID` 字段（additive），apply 统一 preserveConflict 规则 + 内容 hash TOCTOU 防线；v1 路径分支的 download 本就 blob 证明，仅补标记。双方偏离 base 仍归 `revision_conflict`，副本行为不变。
