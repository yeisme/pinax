## 背景

harness-plugins 的 `dsh-pinax-pane-v1` 规格要求 Host 适配器从 Pinax 派生四类投影：「note list、backlink、graph 摘要与 history」。Pinax 侧 `pinax-dsh-pane-contract`（2026-08-21）只交付了 note list 快照（`internal/app/pane.go`）；backlink/graph/history 三个面尚无 owner 侧合同，Host 侧无法在不复制 Pinax 状态的情况下实现完整 Pane。

## 目标

- 在 `internal/app/pane.go` 增加三个有界投影组装器，全部复用 `pinax-dsh-pane-contract` 已固化的红线（`paneRefPattern` 白名单 + 扩展黑名单、无路径、无凭据、无正文）：
  - **Backlinks**：`AssemblePaneBacklinks(noteRef, projection)`——目标笔记的有界反向链接实体（ref/title/kind，不含正文与路径）。
  - **Graph 摘要**：`AssemblePaneGraphSummary(projection)`——节点/边计数、度数 top-k（k≤20，仅 ref+计数）、连通分量数；不输出路径。
  - **History 时间线**：`AssemblePaneHistory(projection)`——来自 record ledger 的有界修订事件（op/ref/revision/time），填入 envelope 既有的 `payload.timeline` 槽位。
- 三个面均产出 `pane.event.v1alpha1`（op 仍为 `snapshot`，payload 结构 additive），门控 action 与手写 metadata 拒绝复用现有合同。
- `dsh-pane` 证据 profile 扩展 extra checks（`backlinks_bounded`、`graph_summary_no_paths`、`history_timeline_bounded`）。
- `docs/interfaces/dsh-pane.md` 补三个面的接口文档。

## 非目标

- 不实现 push/事件流/cursor 续传（维持快照阶段语义，出现消费需求另立变更）。
- 不在 Pinax 内实现 Host 桥或视图注册（归属 harness-plugins）。
- 不改变现有 note.list 快照与 `note links/backlinks`、`graph`、`records` 命令的行为。
