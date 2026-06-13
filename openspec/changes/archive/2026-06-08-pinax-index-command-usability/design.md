## Context

Pinax 的索引是 `.pinax/index.sqlite` 中的本地 SQLite/GORM projection，Markdown vault 和 CLI-authored record assets 才是事实来源。当前 `pinax index` 只有 `init`、`status`、`sync`、`rebuild` 四个子命令；它们对熟悉实现的人够用，但对普通用户和 agent 来说缺少“下一步应该做什么”的决策层。

现有实现边界已经基本正确：`internal/cli/index_cmd.go` 负责 Cobra 接线，`internal/app/service.go` 负责 use case/projection，`internal/index` 负责 GORM schema、Inspect、Sync、Rebuild 和 Search。这个变更应继续沿用这些边界，不把本地化输出塞进业务层，也不让命令层直接操作 SQLite 文件。

## Goals / Non-Goals

**Goals:**

- 让 `pinax index` 成为可直接运行的状态摘要入口，输出当前状态、影响范围和推荐下一步。
- 将常规维护路径明确为 `status -> refresh -> doctor -> repair/rebuild`。
- 增加 `index refresh`、`index doctor`、`index repair` 的设计合同，使用户能先低成本修复，再升级到重建。
- 保持 `--json`、`--agent`、`--events`、`--explain` 的一套 projection 多 renderer 输出模型。
- 将 repair 限定在可重建 projection 层，不修改 Markdown 正文、record ledger、Git 状态或 provider 状态。

**Non-Goals:**

- 不把 SQLite 索引提升为真源。
- 不引入长期 daemon、后台 watcher 或 Air 热加载入口。
- 不依赖真实公网、provider token、用户全局配置或真实用户 vault。
- 不删除 `index init/status/sync/rebuild`，不改已有机器命令名。
- 不在本变更中实现复杂历史版本索引快照；version-aware search 继续沿用既有规格分阶段推进。

## Decisions

1. `pinax index` 默认运行状态摘要，而不是只显示 help。
   - 原因：index 是运维入口，用户运行裸命令通常是在问“现在怎样、下一步做什么”。默认摘要能减少查 help 的跳转。
   - 备选：保持裸命令显示 help。该方案不破坏旧行为，但继续无法解决用户决策成本。

2. 将 `refresh` 作为默认推荐修复动作，`rebuild` 作为全量重置动作。
   - 原因：多数 stale/missing 行为可通过扫描注册笔记并补齐 projection 解决；直接推荐 rebuild 会增加不必要成本，也隐藏增量路径的价值。
   - 备选：统一推荐 rebuild。实现简单，但对大 vault 不友好，也无法推动后续增量诊断能力。

3. `doctor` 只诊断和解释，不隐式写入。
   - 原因：doctor 应可安全地嵌入 CI、agent 检查和用户排障流程；写入动作必须由 `refresh`、`repair` 或 `rebuild` 显式触发。
   - 备选：doctor 自动修复轻微问题。该方案表面顺手，但会破坏 dry-run/readonly 直觉。

4. `repair` 只处理 projection-safe 操作，并默认需要 `--dry-run` 或 `--yes` 明确意图。
   - 原因：索引可重建，repair 可以移动/重建索引文件，但不应越界修 Markdown、record ledger 或 Git。显式 approval 避免误删本地 projection 证据。
   - 备选：复用 `repair plan` 处理所有 index 问题。该方案一致性强，但对“索引文件损坏、重建 projection”这类常规维护过重。

5. 状态诊断数据由 `internal/index` 返回结构体，application service 负责投影成 CLI projection。
   - 原因：`internal/index` 知道 schema、row、hash 和 GORM 细节；`internal/app` 知道命令、next action、错误码和输出合同。两者分开可避免 renderer 或 Cobra 层理解 SQLite。
   - 备选：在 service 中直接检查 SQLite 文件和表。短期少文件，但会让 index 包的职责被绕开。

6. 事件流先作为长命令输出模式设计，不引入后台任务。
   - 原因：Pinax 是短生命周期 CLI。`--events` 可以为 rebuild/refresh 暴露进度，但命令结束后不留下 daemon。
   - 备选：增加 watcher/daemon 自动维护索引。超出当前项目定位，也会引入生命周期和锁管理复杂度。

## Proposed Shape

命令树：

```text
pinax index                     # 状态摘要和推荐下一步，readonly
pinax index status              # 机器友好的轻量状态
pinax index refresh             # 低成本增量维护，可创建缺失索引
pinax index doctor              # 深诊断，readonly
pinax index repair              # projection-safe 修复，默认 dry-run/需 --yes
pinax index sync                # 保留兼容的外部变更同步入口
pinax index rebuild             # 全量重建
pinax index init                # 保留显式初始化入口
```

建议的 service 结构：

```text
internal/app
  IndexSummary(ctx, VaultRequest) (domain.Projection, error)
  IndexRefresh(ctx, IndexRefreshRequest) (domain.Projection, error)
  IndexDoctor(ctx, VaultRequest) (domain.Projection, error)
  IndexRepair(ctx, IndexRepairRequest) (domain.Projection, error)

internal/index
  Inspect(root, notes) (Status, error)
  Diagnose(root, notes) (DoctorReport, error)
  Refresh(root, notes, RefreshOptions) (RefreshResult, error)
  Repair(root, notes, RepairOptions) (RepairResult, error)
```

关键 projection facts：

- 通用：`index_status`、`path`、`schema_version`、`notes`、`writes`、`recommended_action`。
- refresh：`scanned`、`changed`、`skipped`、`indexed`、`deleted`、`failed`、`batches`、`duration_ms`、`index_status`。
- doctor：`issues.total`、`issues.error`、`issues.warning`、`issue_codes`、`recommended_action`。
- repair：`dry_run`、`writes`、`operations`、`risk.low`、`risk.review`、`index_status`。

默认 human summary 使用中文标签；`--json` 和 `--agent` 保持英文 key。

## Risks / Trade-offs

- `refresh` 与现有 `sync` 语义可能混淆。→ help 中明确：`refresh` 是常规低成本维护；`sync` 保留为外部文件变化 reconcile 的兼容/专项入口，后续可在实现中复用同一底层增量引擎。
- 裸 `pinax index` 行为从 help 变为状态摘要可能影响少量用户习惯。→ 仍保留 `pinax index --help`；默认摘要必须 readonly，且机器模式给稳定 command 名。
- repair 处理索引文件可能丢失损坏现场证据。→ 默认先 dry-run；`--yes` 时将旧 projection 移到 `.pinax/index-backups/` 或记录处理证据，再重建。
- 增量 refresh 判断如果只靠 mtime/size 可能漏变更。→ 优先使用 ledger sequence/content hash/schema evidence；mtime/size 只作为候选过滤，必要时再 hash。
- 大 vault refresh/rebuild 可能较慢。→ 输出 progress facts/events，批处理写入，控制 worker 并发，保留 rebuild 作为显式重操作。

## Migration Plan

1. 先补 CLI contract tests：`pinax index` 默认摘要、`index --help` 工作流、`--json`/`--agent` key 稳定。
2. 在 `internal/index` 增加诊断和 refresh result 类型，优先复用现有 Inspect/Sync/Rebuild 逻辑。
3. 在 `internal/app` 增加 IndexSummary/Refresh/Doctor/Repair projection，所有 next action 使用主路径。
4. 在 `internal/cli/index_cmd.go` 接入默认 RunE、新子命令和中文 help 示例。
5. 增加 testscript/e2e 覆盖 missing/stale/fresh/unreadable/dry-run/approval_required 场景。
6. 运行 `task check`，并在 `tasks.md` 记录验证证据。
7. 归档时同步 `openspec/specs/notebook-index-search` 和 `openspec/specs/cli-tree-ux`。

## Open Questions

- `index sync` 是否在后续版本中作为 `index refresh --external-changes` 的 alias 隐藏？本变更先保持可见，避免打断已有脚本。
- repair 旧索引处理策略应默认 `backup` 还是 `remove`？建议默认 backup，并提供 `--discard-corrupt` 作为显式删除选项。
- `refresh` 是否默认允许 lazy rebuild 超过当前 search lazy budget？建议不允许，超预算时推荐显式 `rebuild`。
