## Context

Pinax 当前已经具备基础本地笔记能力，`note links`、`note backlinks`、`note orphans` 和 `index rebuild` 中也已经有 link projection 的雏形。但现状仍偏“命令可用”：服务层临时扫描和索引层解析规则并不完全一致，`LinkRecord` 缺少行号、别名、heading、解析证据和歧义状态，反链语义也没有覆盖同名标题、note id、路径和 wiki alias 的完整解析矩阵。

双联必须成为 Pinax 本地 Markdown vault 的核心能力。Markdown 文件仍是真源，SQLite/GORM 只保存可重建 projection；CLI、dashboard、doctor、repair、organize 和只读 MCP 都应从同一个关系图服务读取结果，避免每个入口各自解析 Markdown。

## Goals / Non-Goals

**Goals:**

- 建立统一 `NoteLinkGraphService`：从 Markdown note body 和 frontmatter 构建出链、反链、断链、歧义链接和孤立分类。
- 支持便携 Markdown 语法：`[[Title]]`、`[[Title|Alias]]`、`[[Title#Heading]]`、`[text](relative-note.md)`、`[text](relative-note.md#heading)`。
- 支持稳定解析顺序：note id、vault-relative path、exact title、case-insensitive unique title、alias/title fallback；歧义时不猜测。
- 使用 GORM index projection 加速查询，但 index 缺失或 stale 时可本地扫描降级，并在输出 facts 中暴露 `engine` 和 `index_status`。
- 建立增量索引路径：初次 `index rebuild` 做全量扫描，后续 note 创建、编辑、移动、删除只更新受影响 note 的 projection 和相关 link resolve 结果。
- 充分利用 Go 并发、channel 事件、`context.Context` 取消和轻量 atomic runtime counters；SQLite 写入保持单 writer 和事务边界。
- 让 CLI 输出遵守一个 projection 多 renderer：默认中文摘要、`--json` envelope、`--agent` key=value、`--explain` 说明证据和风险。
- 将断链修复、歧义消解、正文链接重写放进 `repair plan` 或 `organize suggest` 的 manual review 操作，不自动修改正文。

**Non-Goals:**

- 不实现图谱可视化 UI、实时 watcher、daemon 或自动后台索引。
- 不引入云端图数据库、向量数据库、FTS5 强依赖或外部 provider。
- 不在 MVP 自动创建语义链接、自动改写正文 wiki link、自动合并重复标题。
- 不要求用户安装 Obsidian，也不把 Obsidian 私有配置作为 Pinax 真源。

## Decisions

### 1. 双联图从统一关系图服务生成

新增或重构为 `internal/app` 编排、`internal/index` projection、`internal/domain` projection 类型的三层形态：

```text
cmd/pinax note links/backlinks/orphans
          │
          ▼
internal/app NoteLinkGraphService
          │
          ├─ fresh index -> internal/index LinkGraphRepository
          └─ missing/stale -> scan Markdown vault fallback
          │
          ▼
domain.NoteGraphProjection -> internal/output renderers
```

理由：CLI、doctor、repair、organize、dashboard 和 MCP 都需要相同关系事实。当前服务层扫描和 index 层解析各自维护正则，容易出现 “search 查到的 link 与 note backlinks 不一致”。统一服务后，fallback 和 index 查询可以共享 parser/normalizer。

备选方案是所有命令直接扫描 Markdown。它实现简单，但大 vault 中反链查询退化明显，也无法为 search/filter/doctor 复用 projection。

### 2. Markdown 真源，SQLite/GORM 只做 projection

`LinkRecord` 应扩展为可重建关系边，不保存业务真源：

```text
LinkRecord
  id
  source_path       // note path
  source_note_id
  source_title
  target_raw        // 原始目标，不含展示 alias
  target_text       // 归一化目标
  target_alias      // wiki alias 或 markdown label
  target_heading    // #heading 片段
  target_path
  target_note_id
  target_title
  kind              // wiki|markdown
  status            // resolved|broken|ambiguous|external|ignored
  line              // 1-based line number
  evidence          // 简短稳定证据，可为空
```

GORM repository 负责写入和查询。应用层不得硬编码 SQL。`index rebuild` 在事务内重建 note、tag、link、attachment、dimension projection；失败时保留旧 projection 或返回 `index_rebuild_failed`，不得留下半截新 schema 被当作 fresh。

### 3. 初次全量重建，后续增量更新

索引生命周期分为两条路径：

```text
初次/强制 rebuild
  scan vault -> parse workers -> resolver maps -> resolve workers -> single writer transaction

后续增量 update
  file event -> debounce/coalesce -> hash check -> parse one note -> resolve affected edges -> single writer transaction
```

全量重建只在 `.pinax/index.sqlite` 缺失、schema stale、用户显式 `pinax index rebuild` 或损坏恢复时运行。普通 note 写入由 app service 产出 `IndexEvent`，进入有界事件队列：

```text
IndexEvent
  seq          // atomic monotonic sequence
  epoch        // 当前索引世代，用于丢弃旧 rebuild 结果
  kind         // note_changed|note_moved|note_deleted|rebuild_requested
  old_path
  path
  content_hash
  emitted_at
```

增量处理步骤：

1. `IndexCoordinator` 合并同一路径的短时间重复事件，避免编辑器保存风暴导致重复解析。
2. 读取 note 文件并计算 hash；若 hash、path、mtime、size 与 `NoteRecord` 一致，则跳过写入。
3. 并发 parser worker 只解析变化 note，产出 note metadata、aliases、raw links、terms 和 dimensions。
4. 使用当前 resolver maps 解析变化 note 的出链。
5. 找出受影响的入链：查询 `link_edges` 中 `target_key_norm`、`target_note_id`、`target_path` 或 ambiguous candidate 命中新 title/path/alias/note_id 的边，只重算这些 source note 的 link status。
6. 单 writer 在一个事务内删除变化 note 的旧 projection、写入新 projection、更新受影响 source note 的 link edges、更新 FTS rows 和 index meta。
7. 事务提交后更新内存 resolver snapshot 和 atomic counters；失败时保留旧 projection，并将 `index_status` 标记为 stale 或 partial。

删除和移动走专门路径：

- 删除 note：删除该 note 的 note/text/tag/token/dimension/FTS projection；将指向它的 `resolved` link edges 重算为 `broken` 或 `ambiguous`；删除该 note 发出的 edges。
- 移动 note：更新 path 和 path-derived dimensions；重算指向旧 path、新 path、title、alias 的 edges；如果正文未变，不重建 terms。
- 标题或 alias 变更：重算 resolver maps，并只重算引用旧 title/alias 或新 title/alias 的 source notes。

理由：大多数日常操作只改一两篇 note。反链和搜索应该在首次建立 index 后保持毫秒级响应，不能每次查询或每次编辑都扫描整个 vault。

### 4. Go 并发用于计算，SQLite 保持单 writer

运行时结构：

```text
IndexRuntime
  events: chan IndexEvent          // 有界队列，提供 backpressure
  parseJobs: chan NoteParseJob
  parseResults: chan NoteProjection
  resolveJobs: chan ResolveJob
  writeBatches: chan IndexWriteBatch

  epoch: atomic.Uint64             // rebuild 世代
  queued: atomic.Int64             // 待处理事件
  parsed: atomic.Int64             // 已解析 note
  indexed: atomic.Int64            // 已提交 note
  failed: atomic.Int64             // 失败事件
```

并发规则：

- parser workers 数量默认 `runtime.NumCPU()`，用于文件读取、Markdown/AST 解析、term extraction。
- resolver workers 数量默认 `max(2, runtime.NumCPU()/2)`，用于只读 map lookup 和 edge classification。
- writer 永远 1 个 goroutine。SQLite WAL 可以提高读写并发，但多 writer 只会增加锁等待和失败重试。
- `epoch` 用于取消旧任务：worker 产出结果时如果 `job.epoch != runtime.epoch.Load()`，直接丢弃，不允许旧 rebuild 覆盖新 projection。
- atomic 只用于 counters、epoch、fast cancel flag；复杂状态如 `fresh/stale/rebuilding/partial` 由 coordinator 单 owner 管理并写入 `index_meta`。

核心算法保持清晰：

```text
parse(note) -> NoteProjection
buildResolver(notes, aliases) -> ResolverSnapshot
resolve(rawLinks, snapshot) -> LinkEdges
diff(oldProjection, newProjection) -> IndexWriteBatch
commit(batch) -> transaction
```

这些函数应尽量保持纯函数或窄副作用，便于单元测试和 benchmark。Repository 层负责 GORM batch insert/delete/update；FTS5 virtual table 如需 raw SQL，必须集中在 `internal/index` 的受控 repository 例外中。

### 5. 解析规则显式排序，歧义不猜测

关系解析采用以下优先级：

1. `note_id` 精确匹配，例如未来支持 `[[note:note_123]]` 时直接解析。
2. vault-relative Markdown path 精确匹配，例如 `notes/a.md` 或 `../a.md` 归一化后仍在 vault 内。
3. exact title 匹配。
4. case-insensitive unique title 匹配。
5. alias/frontmatter title alias 匹配；若多个候选则 `ambiguous`。

`[[Title|Alias]]` 的 `Alias` 只用于展示，不参与优先解析；`[[Title#Heading]]` 先解析 note，再保存 heading。外部 URL、mailto、纯 `#heading`、非 Markdown 附件引用不进入 note link graph，可进入 attachment 或 ignored link evidence。

理由：双联必须可预测。遇到同名笔记时自动选一个会破坏 vault 可迁移性，也会让 agent 误读上下文。

### 6. CLI 命令保持当前入口，增加过滤和机器 facts

保留现有入口：

```text
pinax note links <note>
pinax note backlinks <note>
pinax note orphans
pinax search "" --link-target <target>
```

后续增强 flags：

```text
pinax note links <note> --broken-only --kind wiki --include-ignored
pinax note backlinks <note> --include-broken --limit 50
pinax note orphans --mode full|no-incoming|no-outgoing --exclude-kind index
```

稳定 facts 至少包含：`path`、`note_id`、`links`、`backlinks`、`resolved`、`broken`、`ambiguous`、`ignored`、`orphans`、`engine`、`index_status`。错误码包括：`note_ref_ambiguous`、`note_ref_not_found`、`link_target_ambiguous`、`index_unreadable`、`invalid_link_filter`。

### 7. 断链和正文重写进入 review plan

Pinax 可以检测并解释：

- `broken_link`: 找不到目标 note。
- `ambiguous_link`: 目标标题或 alias 对应多个 note。
- `orphan_note`: 无入边且无出边。
- `missing_backlink_context`: 某 note 被引用但缺少足够上下文，作为低优先级建议。

但 Pinax 不自动改写正文链接。`repair plan` 和 `organize suggest` 可以生成 manual review operation：

```text
kind: link_resolution|link_rewrite|orphan_review
mode: manual_review
risk: review
path, target, reason, evidence[]
```

理由：正文链接重写属于用户内容变更，不是机器 metadata。自动改写会破坏用户原始语境和 Git diff 可读性。

### 8. MCP 只读暴露关系上下文

只读 MCP surface 可以新增或扩展工具：

```text
pinax.note.links
pinax.note.backlinks
pinax.note.context
pinax.vault.graph_summary
```

这些工具必须路由到同一 app service，只返回低 token 关系事实和必要 note projection，不写 Markdown、`.pinax/`、Git 或 provider 状态。写入建议只返回人类可运行的 CLI next action。

### 9. 测试优先覆盖行为和性能预算

测试 fixture 应覆盖：

- wiki title、alias、heading、大小写唯一标题。
- Markdown relative path、heading、外部 URL ignored。
- 同名标题歧义、断链、孤立笔记、系统 index note 排除。
- fresh index 查询和 missing/stale scan fallback 的同结果性。
- 初次 rebuild 和后续增量 update 的同结果性；增量 update 不扫描无关 note。
- 并发 parser/resolver 的 race test；旧 epoch 结果不得提交。
- `--json`/`--agent` stdout 合同、默认中文摘要、stderr 诊断分离。
- `repair plan` 对 link rewrite 只生成 manual review。
- benchmark 覆盖 `index rebuild`、单 note update、backlinks、search link target 和 FTS search。

## Risks / Trade-offs

- 解析规则过宽导致误把附件当 note link -> 首版只把 `.md` relative link 和 wiki link 纳入 note graph，其它进入 attachment/ignored evidence。
- 大 vault 扫描 fallback 慢 -> fresh index 优先；fallback 输出 `engine=scan`，并给出 `pinax index rebuild` action。
- 增量事件丢失导致 projection stale -> 每次查询前 `index status` 可用 hash/mtime/size 快速抽查；app service 写入后同步发事件并可在命令内等待增量提交。
- 并发 worker 提交过期结果 -> 所有 job 携带 epoch；writer 提交前再次校验 epoch。
- SQLite writer 锁竞争 -> 单 writer + batch transaction；读查询使用 WAL，失败时报告 retryable index error 而不是静默 fallback 写入。
- GORM schema 扩展影响已有 index.sqlite -> index 是 projection，可通过 `index rebuild` 重建；schema mismatch 报 stale，不尝试业务层热迁移。
- 同名笔记让反链结果不完整 -> 标记 `ambiguous` 并输出候选；不自动猜测。
- agent 过度依赖关系图自动整理 -> 只读查询和 manual review plan 分离，禁止 MCP 直接写 vault。

## Migration Plan

1. 先补 domain projection 和 parser tests，统一服务层与 index 层解析逻辑。
2. 扩展 `LinkRecord` schema version，`index status` 对旧 schema 返回 stale，提示 `pinax index rebuild`。
3. 先实现全量 rebuild 的正确性，再接入增量 update；增量 update 必须和 rebuild 结果做 fixture diff。
4. 保持现有 `note links/backlinks/orphans` 命令兼容，新增字段只作为 JSON/agent optional facts；默认中文输出可增加断链/歧义摘要。
5. 将 doctor/repair/organize 逐步改用统一 `NoteLinkGraphService`，删除重复临时解析逻辑。
6. 回滚时删除 `.pinax/index.sqlite` 并重新运行旧版 `index rebuild` 即可恢复；Markdown note body 不需要迁移。

## Open Questions

- 是否立即支持 frontmatter `aliases`？建议设计预留，首版只读取 `title`，实现阶段视已有 metadata 结构决定。
- `[[note:note_123]]` 是否作为正式语法？建议先不写入用户正文，但解析器可以把它作为 future-compatible 目标类型。
- `note graph` 是否需要作为独立命令？建议先用 `note links/backlinks/orphans` 和 MCP context 满足 MVP，图谱导出作为后续增强。
