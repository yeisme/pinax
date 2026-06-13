## Context

思源没有直接把 Markdown 文件作为唯一核心，是因为它要控制块身份、引用、属性、事务和历史；这些能力依赖约束和记录，而普通 Markdown 文件天然缺少这些机制。Pinax 选择 local-first Markdown vault 是为了便携和可迁移，但如果把 Markdown 误认为所有机器事实的唯一真源，就会出现：路径变化破坏身份、frontmatter 被外部工具改坏、属性类型漂移、agent 无审计写入、索引无法判断哪个事实可信。

因此 Pinax 需要改成“双真源边界”：

```text
Markdown Content Source
  用户正文、可读标题、普通链接、可迁移内容

CLI-authored Record Source
  note identity、lifecycle、schema、events、tombstone、provider receipt、repair evidence、version evidence

SQLite/GORM Index Projection
  从 content source + record source + version evidence 重建的查询投影
```

## Goals / Non-Goals

**Goals:**

- 明确 Markdown 只承载正文和便携内容，不承担完整身份、审计、结构约束和变更记录。
- 建立 record ledger，记录 note 创建、移动、改名、删除、恢复、metadata/schema/index/provider 事件。
- 建立 note identity registry，稳定管理 `note_id`、当前路径、历史路径、状态、版本、hash、创建/更新时间。
- 建立 version evidence，使每个 record event、index batch 和 search hit 能关联 Git HEAD、dirty 状态、diff 摘要、file blob id 或外部版本后端 revision。
- 让 frontmatter 成为 ledger 的便携镜像和校验点，而不是唯一权威。
- 外部编辑破坏约束时，通过 doctor/repair/metadata plan 修复，不自动猜测或静默覆盖。
- 索引从 ledger + Markdown + version evidence 重建，查询结果能解释来源、一致性风险和版本上下文。

**Non-Goals:**

- 不把用户正文迁入私有数据库。
- 不实现思源式块编辑器或块级事务系统。
- 不禁止用户用普通编辑器改 Markdown。
- 不让 agent 手写 `.pinax` 结构化资产。
- 不在 MVP 做 CRDT、多端实时协作或云端 record service。
- 不默认把完整 Git diff 或大二进制 patch 存入索引；索引保存可追溯指针、摘要和内容 hash。

## Decisions

### 1. 区分 Content Source 和 Record Source

Pinax 的核心边界调整为：

| 事实类型 | 权威来源 | 说明 |
| --- | --- | --- |
| note body | Markdown file | 用户正文，以 Markdown 为真源 |
| note title display | Markdown/frontmatter + ledger mirror | 可被用户编辑，但 ledger 记录最近 CLI 确认值 |
| note_id | record ledger | frontmatter 是镜像，冲突时 ledger 优先 |
| current path | record ledger + filesystem reconciliation | filesystem 是事实输入，ledger 记录确认路径 |
| lifecycle status | record ledger | active/archived/deleted/trash/restored |
| schema/property type | CLI-authored schema record | frontmatter value 是内容，类型约束由 schema record 管 |
| index rows | SQLite projection | 可重建，不是真源 |
| provider receipt/sync state | CLI-authored records | 不进入正文 |
| version/revision facts | version evidence provider | Git HEAD、dirty 状态、file blob id、外部版本后端 revision |

理由：Markdown 适合人写，不适合表达系统约束。ledger 负责机器可验证事实，Markdown 负责可迁移内容。

### 2. Record ledger 使用 append-only events + materialized registry

建议资产形态：

```text
.pinax/records/events.jsonl       append-only event log
.pinax/records/notes.json         materialized note registry
.pinax/records/schemas.json       property/schema overrides
.pinax/records/tombstones.json    deleted/restored evidence
```

未来也可压缩为 `.pinax/ledger.sqlite`，但 MVP 用 JSON/JSONL 更容易审计和 Git diff。所有文件只能由 CLI/service 写入。

事件形状：

```json
{
  "schema_version": "pinax.record_event.v1",
  "event_id": "evt_...",
  "seq": 42,
  "kind": "note.moved",
  "note_id": "note_123",
  "actor": "user|agent|system",
  "source_command": "pinax note move",
  "before": {"path": "notes/a.md"},
  "after": {"path": "notes/archive/a.md"},
  "content_hash": "...",
  "version": {
    "backend": "git",
    "head": "abc123",
    "worktree": "dirty",
    "file_blob": "def456",
    "diff_id": "diff_...",
    "snapshot": "commit_or_none"
  },
  "created_at": "...",
  "evidence": ["git_head=...", "snapshot=..."]
}
```

Registry 是 events 的 materialized view，便于快速读取；如果 registry 损坏，可以从 events 重放。

### 2.1 Version evidence 作为事件证据，不作为正文存储

版本证据建议资产形态：

```text
.pinax/records/version.json       当前版本后端配置和 capability
.pinax/records/diffs/             可选的小型 diff summary 或外部 diff pointer
.pinax/snapshots/                 可选的非 Git backend snapshot receipt
```

MVP 版本后端优先级：

1. Git repository：读取 `HEAD`、branch、worktree status、file blob id、diff summary；在写操作前后可创建 Pinax snapshot commit。
2. No Git / detached vault：记录 content hash、mtime、size、ledger seq，提示用户可运行 `pinax git init` 或配置版本后端。
3. Future backend：通过 `VersionBackend` adapter 接入 Jujutsu、Pijul、Restic、CAS/object store、SQLite artifact store 或其他二进制项目管理系统。

关键原则：

- ledger event 保存 `version.backend`、`revision_id`、`head`、`worktree_state`、`file_blob_id`、`diff_summary_hash`、`snapshot_id`，不默认内嵌完整 diff。
- diff summary 用于解释和 stale 判断；完整 diff 通过 Git 或外部 backend 按需读取。
- 对二进制附件或大型文件，记录 object id、size、mime、backend revision 和 checksum；不把二进制内容放进 Markdown、ledger event 或 search index。
- 当用户要求“按版本检索”时，index 使用 revision 指针定位对应内容版本；如果后端不支持 read-at-revision，则返回 `version_read_unavailable`。

### 3. Frontmatter 是镜像，不是不可挑战权威

Pinax-managed frontmatter 字段：

```yaml
schema_version: pinax.note.v1
note_id: note_123
title: ...
created_at: ...
updated_at: ...
```

处理规则：

- CLI 创建 note 时同时写 ledger event 和 frontmatter mirror。
- 外部编辑改坏 `note_id` 时，不直接相信新值；doctor 报 `record_frontmatter_mismatch`。
- frontmatter 缺失时，metadata plan 可以根据 ledger 补镜像。
- ledger 缺失但 Markdown 有 frontmatter 时，record adopt plan 可以把它纳入 registry。

### 4. Record lifecycle 是状态机

```text
unregistered -> active -> archived -> trashed -> restored
                         -> deleted
active -> moved -> active
active -> renamed -> active
```

转换权限：

- CLI user command：可创建、移动、改名、归档、删除、恢复。
- agent/MCP：只读或生成 plan；没有 approval 不直接写 ledger。
- repair apply：只能应用 plan 中批准的 record 修复。
- external scan：只能产生 candidate issue，不能直接做弱推断转换。

### 5. Index 由 ledger + Markdown + version evidence 共同重建

重建输入：

```text
records/events.jsonl + records/notes.json + Markdown files + version evidence -> index.sqlite
```

如果 Markdown 文件存在但 registry 无记录，标记 `unregistered_note`。如果 registry 记录存在但文件缺失，标记 `missing_record_file` 或 tombstone。查询输出可以带 consistency facts，避免用户误以为投影完全可信。

index projection 建议新增这些字段：

| 字段 | 用途 |
| --- | --- |
| `ledger_seq` | 说明索引构建到哪个 record event |
| `index_epoch` | Go atomic epoch，用于并发读取判断快照是否一致 |
| `version_backend` | `git`、`none`、`jj`、`cas` 等 |
| `revision_id` | HEAD、snapshot id 或 backend revision |
| `worktree_state` | `clean`、`dirty`、`unknown` |
| `file_blob_id` | Git blob id 或 CAS object id |
| `diff_summary_hash` | dirty diff summary 的 hash，用于 stale 判断 |
| `content_hash` | 当前 Markdown 内容 hash |

检索输出必须能表达：这个结果来自哪个 `note_id`、哪个 `ledger_seq/index_epoch`、哪个 `revision_id/file_blob_id`，以及是否包含未提交改动。

### 7. VersionBackend adapter 边界

Pinax 不在命令层直接拼 Git 命令和 porcelain 解析。版本管理通过 adapter/service 聚合：

```go
type VersionBackend interface {
    Name() string
    Detect(ctx context.Context, vault string) (VersionCapabilities, error)
    Current(ctx context.Context, vault string) (VersionEvidence, error)
    FileRevision(ctx context.Context, vault string, path string) (FileRevision, error)
    DiffSummary(ctx context.Context, vault string, paths []string) (DiffSummary, error)
    Snapshot(ctx context.Context, vault string, message string) (SnapshotReceipt, error)
    ReadFileAt(ctx context.Context, vault string, path string, revision string) ([]byte, error)
}
```

Git backend 是 MVP 默认实现；其他 backend 必须满足相同证据合同，不能把私有二进制 state 写进 stdout、事件或 fixture。

### 8. Git diff 与索引的关系

Pinax 可以利用 Git diff，但不建议把 diff 当主索引：

- 当前搜索默认查当前工作区内容 projection。
- `--at HEAD` 或 `--revision <rev>` 查询历史版本时，通过 version backend 读取该版本内容，构建临时 projection 或使用已保存的 `IndexSnapshot`。
- `--include-dirty` 查询当前未提交改动时，结果标记 `worktree_state=dirty` 和 `diff_summary_hash`。
- `--changed-since <rev>` 查询变更范围时，先由 version backend 找 changed paths，再对这些 note 做增量解析和过滤。
- 大 vault 只保存 index snapshot metadata，不长期保存每个历史版本全文索引；需要历史全文索引时按 snapshot 策略增量构建。

### 9. 性能和并发架构取舍

Pinax 的性能策略不是“所有路径都最快”，而是按用户感知分层：

| 路径 | 性能目标 | 取舍 |
| --- | --- | --- |
| search/list/read | 低延迟、低内存、可并发读 | 主要读 SQLite projection 和小型 registry cache，不扫描全 vault |
| note mutation | 正确性优先，允许几十毫秒到数百毫秒写入成本 | 单写入者顺序化 ledger event、frontmatter、version evidence、index delta |
| status/doctor | 可流式、可取消、可降级 | 并发扫描文件，但 issue materialization 保持有序 |
| first adopt/rebuild | 可慢但必须可恢复、可观测 | 分批 checkpoint、progress event、失败后从 batch cursor 继续 |
| historical search | 按需构建，避免常驻历史全文索引 | 牺牲首次历史查询速度，换取磁盘和内存可控 |

推荐分层：

```text
CLI command
  -> app service
    -> single-writer mutation coordinator
      -> ledger event append
      -> registry materialize
      -> version evidence capture
      -> index delta enqueue

read path
  -> atomic snapshot pointer
  -> registry cache + SQLite projection
  -> optional fallback scan with bounded workers
```

并发原则：

- ledger event sequence 只能有一个写入者；不要用多个 goroutine 直接 append JSONL。
- 文件扫描、Markdown parse、hash、link extraction 可以并行；结果通过 bounded channel 汇聚到单写入者或 batched SQLite writer。
- SQLite 用 WAL；读多写少场景下允许并发读，但索引写入通过批量 transaction 串行提交。
- `sync/atomic` 只用于 `index_epoch`、`ledger_seq_seen`、cancel flag、metrics counter、snapshot pointer 这类简单值；复杂生命周期状态必须走 service/state machine。
- 所有 worker 接收 `context.Context`，命令取消时停止新任务、drain 必要结果、写入 checkpoint。
- 大文件、二进制附件、历史版本内容不进入内存全集；只流式 hash 和记录 evidence。

### 10. 内存、时间、磁盘预算

默认预算建议：

| 规模 | note 数 | 策略 |
| --- | --- | --- |
| small | `< 5k` | registry 可全量 cache，parse worker 默认 `min(GOMAXPROCS, 4)` |
| medium | `5k-50k` | registry 分页读取，SQLite 批量 upsert，snippet 延迟生成 |
| large | `> 50k` | mmap/stream scan、分片 batch、跳过历史全文缓存，强制 checkpoint |

内存取舍：

- 不把所有 Markdown 正文留在内存；parse 完立即释放，只保留 token、link、property、hash、snippet offset。
- registry hot cache 只保留 `note_id -> compact record`、`path -> note_id`、`title normalized -> candidates`。
- 全文检索优先 SQLite FTS5；倒排 token 表只保留 Pinax 需要的结构化 token 和 link/property 维度，避免重复存储完整正文。
- snippet 默认查询时从 FTS 或文件按 offset 生成，不在 index 中保存大量片段。
- 可配置 `--memory-budget` 或 config，例如 `low|normal|high`；低内存模式降低 worker 数、缩小 batch、关闭历史 snapshot cache。

时间取舍：

- 首次 rebuild 做完整 parse、hash、link extraction、schema inference；这是可接受的慢路径。
- 后续增量优先用 Git changed paths、ledger seq、mtime/size/content hash 三层判断，能跳过就跳过。
- schema/dataview 查询优先走 typed property projection；只有索引缺失或 stale 且用户允许 fallback 时才扫 Markdown。
- 大 batch 追求吞吐，小 batch 追求交互响应；CLI 交互命令默认小 batch，`index rebuild` 默认大 batch。

磁盘取舍：

- 当前 projection 常驻 `.pinax/index.sqlite`。
- record events append-only，registry 可 compact；compact 不删除审计事件。
- 历史 index snapshot 默认只存 metadata 和 changed note projection；完整历史全文索引需要显式配置。
- diff 只存 summary/hash/pointer；完整 diff 交给 Git 或版本后端。

### 11. 性能观测和回归门禁

设计期不承诺绝对数字，但实现必须提供可重复基准：

- fixture：1k、10k、50k notes；包含 tags、wiki links、frontmatter properties、attachments、renames、dirty Git changes。
- 指标：first rebuild wall time、incremental refresh wall time、search p50/p95、RSS/max heap、alloc/op、SQLite write batch time、worker queue depth、skipped unchanged count。
- 命令：`go test -bench ... -benchmem`、`go test -race ./...`、大 vault testscript smoke、可选 CPU/heap profile。
- 输出：`pinax index status --json` 和 `pinax doctor --json` 暴露 index epoch、ledger seq、last batch duration、changed/skipped counts、version backend latency summary。

### 6. Repair plan 是约束恢复入口

典型问题：

- `record_missing`: Markdown 有 note 但 ledger 无记录。
- `record_file_missing`: ledger 有 active note 但文件不在路径。
- `note_id_conflict`: 多个 Markdown 声称同一 note_id。
- `record_frontmatter_mismatch`: frontmatter 与 ledger 不一致。
- `schema_type_conflict`: property 值与 schema record 不兼容。
- `orphan_tombstone`: tombstone 过期或被恢复。

这些都进入 doctor/repair plan。只有低风险 record-only 修复可以自动 apply；正文或身份冲突需要 manual review。

## Risks / Trade-offs

- 复杂度上升 -> 先做 note-level ledger，不做 block-level ledger。
- 用户手改 `.pinax/records` -> validate/doctor 检测 schema/hash/seq 异常，必要时从 Git 或 Markdown 重新 adopt。
- ledger 与 Markdown 冲突 -> ledger 对机器身份优先，Markdown 对正文优先；冲突输出 repair plan。
- Git diff 噪音 -> events append-only 可定期 compact registry，但不删除审计证据。
- diff/版本证据泄露敏感内容 -> 默认只保存 hash、路径、状态和摘要；完整 diff 只在显式 explain/debug 或用户批准的版本后端读取。
- 二进制版本后端不可移植 -> 通过 backend capability 标记 `read_at_revision`、`snapshot`、`diff_summary`，检索不假设所有 backend 都支持历史读取。
- 历史全文索引膨胀 -> 默认只索引当前 projection，历史版本采用 lazy snapshot 或按需临时 projection。
- 过度并发导致内存尖峰 -> worker 数、batch size、queue capacity 受 memory budget 限制；低内存模式优先稳定而非吞吐。
- 单写入者成为瓶颈 -> 只串行 commit/sequence，parse/hash/extract 在写入前并行，SQLite upsert 使用批量事务降低提交次数。
- atomic 滥用破坏状态机 -> atomics 只用于快照指针和计数，不承载 note lifecycle 或 repair 状态。

## Migration Plan

1. `pinax record init --vault` 创建 records 目录和初始 registry。
2. `pinax record adopt --vault --plan` 扫描现有 Markdown，生成 adoption plan。
3. `pinax record adopt --apply --yes` 为已有 notes 创建 record events，不改正文；缺失 frontmatter 交给 metadata plan。
4. note create/rename/move/delete/edit 接入 ledger service。
5. Git backend 接入 version evidence，record event 和 index batch 记录 HEAD、dirty 状态、file blob id 和 diff summary hash。
6. index rebuild 改为从 ledger + Markdown + version evidence 构建 projection。
7. 增加 `pinax search --at HEAD`、`--revision <rev>`、`--changed-since <rev>` 的设计入口，先实现 capability 检测和错误输出，再扩展历史 projection。
8. 增加 performance fixture、benchmark、race test 和 index diagnostics，先建立基线再优化 worker、batch 和 cache 参数。

## Open Questions

- ledger 用 JSONL+JSON 还是 SQLite？建议 MVP 用 JSONL+JSON，后续大 vault 再考虑 ledger.sqlite。
- 是否为每个 note 建 `.pinax/records/notes/<note_id>.json`？建议先集中 registry，避免小文件过多。
- frontmatter mismatch 时是否允许自动回写？建议只在 metadata apply 且有 approval 时回写。
- 历史版本全文索引要不要常驻？建议 MVP 不常驻，只保存当前 projection 和 snapshot metadata；高阶版本检索再按需构建。
- 默认 memory budget 具体数值需要基准后确定；建议先按 worker/batch 控制，不硬编码绝对内存上限。
