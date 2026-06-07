## Context

Pinax 的 vault 真源是 Markdown 文件，用户可以通过 Pinax 命令、普通编辑器、Git checkout、文件管理器或 import/export 操作修改这些文件。索引 projection 包含 note records、全文 token、双联边、附件、typed properties、saved view/query 结果依赖等；如果每次变化都全量 `index rebuild`，大 vault 的日常体验会变慢。如果只靠路径和 mtime，又会在 rename/move/delete 时产生陈旧索引、断链误判和数据库视图脏结果。

本设计定义一个统一的增量维护层：Pinax 负责把文件生命周期变化转换为结构化 index events，经过 coalescing、identity reconciliation、affected projection 计算和单 writer 事务提交，保持索引快速且可恢复。

## Goals / Non-Goals

**Goals:**

- 初次索引允许较慢；后续 note 内容变更、改名、移动、删除、恢复只更新受影响 projection。
- 稳定 note identity：优先 `note_id`，不把路径当成笔记身份。
- 区分强证据事件和推断事件：Pinax 命令知道 old/new path；外部编辑只能通过 hash、note_id、Git diff 和文件事实推断。
- 支持 rename/move reconciliation：更新 path、folder、links、backlinks、properties、FTS rows、saved query dependency facts。
- 支持 delete/tombstone：删除 projection、更新入边状态、保留短期 tombstone 以识别 restore 或 move。
- 使用 Go 并发做扫描/解析/比对，SQLite/GORM 写入保持单 writer + transaction。
- 失败时标记 stale/partial 并给出 repair/rebuild next action，不返回假 fresh。

**Non-Goals:**

- 不实现长期后台 daemon 或持续 filesystem watcher；后续 watcher 可以复用本事件模型。
- 不自动修改用户 Markdown 正文以修复链接或 metadata。
- 不依赖真实 Git 工作树状态才能运行；Git evidence 是可用时的增强信号。
- 不在命令层硬编码 SQL 或直接操作 index 表。

## Decisions

### 1. 文件生命周期事件是增量维护入口

统一事件类型：

```text
IndexEvent
  event_id
  seq
  epoch
  kind: note_created|note_changed|note_renamed|note_moved|note_deleted|note_restored|rebuild_requested|sync_requested
  source: pinax_command|external_scan|git_diff|import|metadata_apply|organize_apply|repair_apply
  old_path
  new_path
  note_id
  old_hash
  new_hash
  evidence[]
  emitted_at
```

Pinax 命令路径产生强事件。例如 `note rename` 必须携带 old_path/new_path/note_id；`note delete` 必须携带 path/note_id/trash_path。外部编辑由 `index sync` 或 `index status --refresh` 扫描产生推断事件。

理由：事件让 note mutation、import、organize、repair、metadata apply 和 future watcher 复用同一 index updater。

### 2. Note identity 优先级固定

身份识别优先级：

1. `note_id` frontmatter 精确匹配。
2. Pinax 命令事件携带的 old_path/new_path 和 note_id。
3. tombstone 中的 note_id/hash/path 证据。
4. Git rename/move diff evidence。
5. 内容 hash 完全匹配。
6. 标题 + 大小 + mtime 接近 + path 相似度作为弱候选。

弱候选只能产生 `move_candidate` 或 `rename_candidate` consistency issue，不能自动更新 identity。歧义时保持 index stale/partial，并建议 `pinax index repair` 或 `pinax index rebuild`。

### 3. Path 变化不等于内容变化

rename/move 的最小更新：

```text
known move/rename with same note_id + same content hash:
  update notes.path/folder/group-derived fields
  update FTS path field if indexed
  update property values for file.path/file.name/file.folder
  update link_edges source_path for outgoing edges
  recompute markdown relative links from moved note only if relative target base changed
  recompute incoming edges whose target matched old/new path
```

如果只是 title/frontmatter 改名但 path 不变，则重算 title/aliases/properties/FTS title 和引用 title/alias 的边。如果 path 和内容同时变化，按 changed + moved 组合处理。

### 4. 删除使用 tombstone 短期记忆

删除 note 时写入 tombstone projection：

```text
IndexTombstone
  note_id
  old_path
  old_hash
  title
  deleted_at
  source
  evidence[]
  expires_at
```

删除事务：

- 删除该 note 的 text/token/property/FTS/attachment/dimension rows。
- 删除该 note 发出的 link_edges。
- 将指向该 note 的 incoming edges 重算为 broken、ambiguous 或 unresolved。
- saved views/query 不保存结果快照，因此只需后续查询反映当前 projection。

如果之后出现同 note_id 或同 hash 文件，可识别为 restore/move 并清理 tombstone。

### 5. 外部编辑通过 sync reconcile，不默认全量 rebuild

`pinax index sync --vault ./my-notes --json` 执行：

1. 扫描 vault Markdown files，构建当前 file facts：path、size、mtime、hash、note_id、title optional。
2. 读取 indexed file facts 和 tombstones。
3. diff：created、changed、deleted、same、possible_move。
4. 强匹配直接转事件；弱匹配生成 consistency issue。
5. 批量处理事件，输出 updated/skipped/candidates/stale facts。

扫描可并发；解析 note_id/title 可以按需进行，先用 path/size/mtime/hash 快速过滤。大 vault 下 hash 可分层：mtime/size 未变先跳过，变更候选再读文件 hash。

### 6. Affected projection 计算要显式

每类事件的受影响范围：

```text
note_changed:
  self: note/text/token/property/attachment/outgoing links/FTS
  affected: incoming links that target changed note_id/title/alias/path

note_renamed title/frontmatter:
  self: note/title/properties/FTS
  affected: links by old_title/new_title/aliases

note_moved path:
  self: path/folder/system properties/outgoing relative links
  affected: links by old_path/new_path

note_deleted:
  self: remove all projection rows
  affected: incoming edges and orphan/backlink counts
```

这些规则应编码为小的 pure-ish planner：`PlanIndexUpdate(event, currentFacts) -> IndexWriteBatch`，便于测试 full rebuild 与 incremental 最终结果一致。

### 7. 并发模型：多 worker 计算，单 writer 提交

```text
event channel -> coalescer -> reconcile planner -> parse workers -> affected planner -> write batch channel -> single writer
```

- `epoch`：full rebuild 或 repair 开始时递增，旧 worker 结果提交前丢弃。
- `atomic` counters：queued、coalesced、parsed、planned、committed、skipped、failed、epoch。
- `context.Context`：命令取消时停止扫描和 worker。
- SQLite 写入：单 writer + GORM transaction；FTS5 raw SQL 仅在 index repository 受控边界。

### 8. CLI 输出必须暴露 index_update facts

任何会改 vault 的命令如果触发索引更新，应在 projection facts 中暴露：

```text
index_update=committed|skipped|queued|failed|stale
index_event=note_moved
index_status=fresh|partial|stale
affected_notes=N
affected_links=N
```

如果命令不能等待增量提交，可以返回 `queued` 和 next action `pinax index status --refresh`；但普通 note mutation 首选同步提交小批量增量，保证后续查询变快且正确。

## Risks / Trade-offs

- 外部 rename 推断错误 -> 只自动处理 note_id/hash 强匹配；弱匹配进入 repair plan。
- hash 扫描成本高 -> mtime/size 快速过滤，候选才 hash；大 vault benchmark 验证。
- 多事件顺序混乱 -> coalescer 按 path/note_id 合并，并以 seq/epoch 防旧结果提交。
- relative Markdown link 在 move 后语义变化 -> moved note 的相对 Markdown links 必须重算；正文不自动改写。
- 增量 planner 漏依赖 -> full rebuild vs incremental equivalence test 作为回归门禁。

## Migration Plan

1. 扩展 index meta 和 NoteRecord file facts，旧 index 返回 stale。
2. 先实现 Pinax 命令强事件增量：create/rename/move/delete/edit。
3. 再实现 `index sync` 外部变化 reconciliation。
4. 最后把 database view/property、双联、attachments、search token 统一接入 affected projection planner。
5. 回滚时删除 index.sqlite 并全量 rebuild；Markdown 真源不迁移。

## Open Questions

- tombstone 保留多久？建议默认 30 天或最多 N 条，由 config 后续控制。
- 是否需要 `index watch`？本 change 不实现 daemon，但保留 event model 给后续 watcher。
- 外部编辑缺失 note_id 的 Markdown 是否自动补 metadata？建议不自动补，进入 `metadata plan/apply`。
