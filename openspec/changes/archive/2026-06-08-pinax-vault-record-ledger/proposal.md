## Why

“Markdown 是真源”如果理解为所有事实都来自可随意编辑的 Markdown，会让 Pinax 在身份、变更记录、数据库视图、双联和 agent 写入上缺少约束：用户或工具一改文件，note identity、属性类型、链接语义和审计事实都可能漂移。Pinax 需要把 Markdown 的便携性和系统级记录的可审计性拆开：Markdown 继续承载用户正文，CLI-authored record ledger 承载身份、生命周期、结构约束和变更证据。

## What Changes

- 明确新的真源边界：Markdown 是正文和用户可读内容真源；record ledger 是 note identity、lifecycle、schema、event、tombstone、sync/repair evidence 的机器真源。
- 新增 record ledger 能力：通过 CLI/service 写入 `.pinax/records/` 或 `.pinax/ledger.sqlite`/JSONL，记录 note 创建、移动、改名、删除、恢复、metadata 变更、索引事件和 schema 变更。
- 新增 note identity registry：每个 note 的 `note_id`、创建时间、当前路径、历史路径、record version、状态、content hash 和 lifecycle 状态由 ledger 维护，Markdown frontmatter 是便携镜像和校验点。
- 新增约束与修复机制：当 Markdown 文件缺失 metadata、note_id 冲突、路径漂移、schema 类型冲突或外部编辑破坏结构时，Pinax 生成 repair/metadata plan，不让 agent 直接手写机器 metadata。
- 新增 append-only event/audit 语义：所有 CLI-authored 结构化变更写入事件证据，支持回放、校验和恢复 index projection。
- 新增 version evidence 语义：record event、index batch 和 search result 关联 Git HEAD、dirty 状态、diff summary、content object id 或外部版本后端 revision，使检索结果可以解释“命中来自哪个版本”。
- 新增版本后端抽象：MVP 集成 Git snapshot/diff evidence，不强制把完整 diff 存入 ledger；后续可接入 Jujutsu、Pijul、Restic、SQLite artifact store、CAS/object store 或其他二进制项目管理/版本控制后端。
- 新增性能和并发边界：ledger 单写入者保证事件顺序，扫描/解析/哈希并行，SQLite projection 批量写入，查询优先读 projection，首次重建可慢但增量 refresh 必须跳过未变更内容。
- 新增内存/时间/磁盘取舍：支持 memory budget、bounded worker、checkpoint、index epoch、ledger seq、changed/skipped 诊断和 benchmark/race/profile 门禁。
- 保留 Markdown 便携性：用户仍可用普通编辑器编辑 note body；Pinax 不把正文锁进私有数据库，也不要求外部编辑器理解 ledger。

## Capabilities

### New Capabilities
- `vault-record-ledger`: 定义 Pinax 的 record ledger、note identity registry、append-only lifecycle events、Markdown mirror 校验、外部编辑 reconciliation 和 repair 边界。

### Modified Capabilities
- `pinax`: 修改“Markdown vault 真源”表述，明确 Markdown 只是真源的一部分，机器可读身份和结构化记录由 CLI-authored assets 管理。
- `notebook-index-search`: 索引从 ledger + Markdown 重建，index stale/repair 需要校验 record ledger 与 Markdown mirror 的一致性。
- `note-command-ux`: note create/rename/move/delete/edit 必须维护 ledger record 和 frontmatter mirror，输出 record/index facts。
- `vault-maintenance-actions`: doctor/repair 需要覆盖 ledger 缺失、record/frontmatter 不一致、note_id 冲突、orphan record、外部编辑破坏约束等问题。

## Impact

- 影响 `internal/domain`：新增 `NoteRecord`、`RecordEvent`、`LedgerState`、`RecordInvariant`、`RecordRepairOperation` projection。
- 影响 `internal/domain`：新增 `VersionEvidence`、`VersionBackend`、`ContentRevision`、`IndexSnapshot`，用于描述 Git HEAD、dirty diff、content object id 和外部版本后端 revision。
- 影响 `internal/app`：note mutation、metadata、organize、repair、import/export、index sync 都必须通过 ledger service 维护记录、事件和版本证据。
- 影响 `internal/index`：index projection 以 ledger identity 为主，Markdown path/hash 为内容事实，不再把路径当身份；refresh 必须基于 ledger seq、version evidence、mtime/size/hash 增量跳过未变更 note。
- 影响 Git adapter/service：提供 HEAD、worktree status、diff summary、snapshot commit、file blob id 和 restore/read-at-revision 能力；不能在命令层解析复杂 Git porcelain。
- 影响 CLI：新增或增强 `pinax record status`、`pinax record repair`、`pinax record history`、`pinax metadata plan/apply`，并在 note mutation 输出 record facts。
- 影响测试/性能门禁：新增大 vault fixture、benchmark、race test、内存预算测试和 index/record diagnostics。
- 影响结构化资产：新增 `.pinax/records/**` 或 `.pinax/ledger.*` 资产，只能由 CLI/service 创建和修改。
- 不实现私有块编辑器、不废弃 Markdown、不把正文迁入数据库、不实现长期 daemon。
