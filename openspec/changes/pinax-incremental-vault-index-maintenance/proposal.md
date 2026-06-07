## Why

Pinax 的索引、双联图、数据库视图和搜索都依赖 Markdown vault projection；但用户会用编辑器、Git、文件管理器或 Pinax 命令移动、改名、删除和修改 Markdown 文件。需要一套统一的增量索引维护设计，让首次全量构建后，后续文件生命周期变化只更新受影响 projection，而不是反复全量扫描。

## What Changes

- 新增 vault file lifecycle 增量索引能力：检测 note created、changed、renamed、moved、deleted、restored 和 external edit。
- 定义稳定 note identity 策略：优先 frontmatter `note_id`，其次 index fingerprint 仅用于 rename/move 猜测，不把路径当唯一身份。
- 新增 index event queue 和 coalescing：Pinax 命令写入直接产生结构化事件，外部编辑通过 `index sync/status` 扫描差异补事件。
- 新增 rename/move reconciliation：通过 note_id、content hash、previous path、Git diff hints 和 title/mtime/size 证据识别移动与改名，更新 path projection 和受影响 links/properties/search rows。
- 新增 delete/tombstone 策略：删除 note 时清理出边和 property/FTS rows，并将入边重算为 broken/ambiguous；trash/restore 保留可审计证据。
- 新增增量 query/search 维护：note 内容变更只重建该 note 的 text/token/property/link projection；title/alias/path 变更只重算相关候选边和依赖 view/query facts。
- 新增一致性和恢复策略：incremental update 失败时标记 stale/partial，提供 `pinax index repair` 或 `pinax index rebuild` next action。

## Capabilities

### New Capabilities
- `incremental-vault-index-maintenance`: 定义 Markdown 文件生命周期、note identity、增量事件、rename/move/delete reconciliation、受影响 projection 更新、staleness 和恢复行为。

### Modified Capabilities
- `notebook-index-search`: 增强 index status/rebuild/search，加入增量维护、文件移动改名检测、delete/tombstone 和 partial/stale 恢复语义。
- `note-command-ux`: 让 note rename/move/delete/edit 命令产出稳定增量索引事件和输出 facts，避免命令成功但索引落后。
- `vault-maintenance-actions`: 将索引不一致、疑似外部移动、冲突 rename、orphan tombstone 等情况纳入 repair plan。

## Impact

- 影响 `internal/app`：note mutation、metadata apply、organize apply、import/export 和 repair apply 后需要触发 index events 或同步增量更新。
- 影响 `internal/index`：新增 event/coordinator、file fingerprint、note identity map、tombstone、affected projection updater 和 stale/partial 状态。
- 影响 `internal/domain`：新增 vault file event、index update result、rename candidate、tombstone 和 consistency issue projection。
- 影响 CLI：新增或增强 `pinax index sync`、`pinax index repair`、`pinax index status --explain`，并让 note mutation 输出 index update facts。
- 影响测试：需要 rename/move/delete/external edit fixtures、concurrency/race tests、full-vs-incremental equivalence tests 和 benchmark。
- 不实现长期 daemon/watcher；本变更只定义 CLI 驱动和显式 sync 的增量维护。后续 watcher 可复用同一 event service。
