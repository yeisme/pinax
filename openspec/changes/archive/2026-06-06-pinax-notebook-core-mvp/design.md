## Context

Pinax 当前已经有 Go/Cobra CLI、`internal/app` service、Markdown vault、SQLite/GORM index projection、模板、note CRUD、search、doctor、repair 和本地 dashboard。上一轮已经让 `note new` 支持 group/folder/kind，并自动维护 daily index 和本地索引。

短板是产品形态还没有形成笔记软件闭环：用户可以创建笔记，但缺少 inbox 捕获、daily note 入口、稳定组织视图、反链、附件、保存视图和导入导出。这个变更只补本地笔记软件核心，不触碰 provider、云同步、AI 总结或长期 daemon。

## Goals / Non-Goals

**Goals:**

- 建立 capture -> organize -> browse -> connect -> review -> migrate 的本地笔记闭环。
- 所有写入通过 Cobra 命令进入 `internal/app` service，不让命令层或 agent 手写 `.pinax` 结构化资产。
- 保持 Markdown 为真源：note body、frontmatter、wiki link、附件引用都可被普通编辑器读取。
- SQLite/GORM 只做本地 projection：用于列表、统计、反链、附件诊断和保存视图，不成为业务真源。
- 新命令全部遵守现有输出合同：默认中文摘要、`--json`、`--agent`，stderr 只放诊断。

**Non-Goals:**

- 不实现 Notion/Feishu/Obsidian 云端导入，不接真实公网 provider。
- 不实现 AI 自动分类、摘要、语义向量搜索或推荐。
- 不实现多人协作、实时同步、移动端 UI 或长期后台 watcher。
- 不把 daily index、saved view 或附件 metadata 设计成脱离 Markdown vault 的云端状态。

## Decisions

### 1. 命令形态先补本地工作流，再考虑扩展面

新增和增强命令建议采用：

```text
pinax daily open|show|append
pinax inbox capture|list|triage
pinax note links|backlinks|attachments
pinax view save|list|show|delete
pinax import markdown
pinax export markdown
pinax tag list
pinax folder list
pinax kind list
pinax group list
```

理由：这些命令覆盖笔记软件核心操作，且都能在本地 vault 内完成。备选方案是把所有能力塞进 `note` 子命令，但 daily/inbox/view 是用户一级工作流，单独入口更清晰。

### 2. `.pinax` 结构化资产只保存 CLI-authored 状态

新增结构化资产建议：

- `.pinax/views.json`：保存视图定义，例如过滤条件、排序、limit、显示列。
- `.pinax/imports.jsonl`：导入 receipt，记录来源路径、导入目标、冲突处理和时间。
- `.pinax/exports.jsonl`：导出 receipt，记录过滤条件、输出目录、文件数量和时间。

附件 metadata 不单独写 `.pinax`，默认从 Markdown body 引用和文件系统扫描投影得到。这样附件仍以 vault 文件和 Markdown 引用为真源。

### 3. Daily 和 Inbox 使用普通 Markdown note，而不是隐藏数据库记录

- daily note 路径：`notes/daily/YYYY-MM-DD.md`。
- inbox note 路径：`notes/inbox/YYYY-MM-DD.md` 或 capture 时创建独立 `notes/inbox/<slug>.md`，由实现 spike 决定默认。
- daily/index 类系统导航页不参与普通 note 统计；daily 正文笔记若 `kind: daily` 则参与列表和搜索。

理由：用户可以用编辑器直接修改 daily/inbox 内容。projection 可以区分系统 index 和用户 daily note，避免统计污染。

### 4. Link/backlink 由索引投影生成，但输出回到 note path/title

GORM projection 增强：

- note record：path、note_id、title、project/group、folder、kind、status、created_at、updated_at。
- link record：source path、target text、resolved target path、kind、broken flag。
- attachment record：note path、reference text、resolved file path、exists flag、media type。

解析仍使用 Markdown body，不修改用户正文。未解析 link/附件作为 doctor/links 命令的诊断结果，不自动重写。

### 5. Import/export 是本地文件操作，必须支持 dry-run 和冲突策略

导入默认只接受本地 Markdown 文件或目录；导出默认输出 Markdown bundle。所有跨 vault 边界写入必须校验路径。

冲突策略建议：

- `skip`：默认，目标存在则跳过。
- `rename`：自动加后缀。
- `overwrite`：必须显式 `--yes`。

### 6. 本地索引数据库是可重建 projection，不是真源

`.pinax/index.sqlite` 由 `pinax index init/rebuild` 和会改变 note 的 app service 维护。数据库可以删除并通过扫描 Markdown vault 重建；任何业务判断必须能从 Markdown 和 CLI-authored `.pinax` 资产恢复。

建议 GORM records：

```text
IndexMetaRecord
  key primary, value, updated_at

NoteRecord
  path primary, note_id unique, title, project, group, folder, kind, status,
  created_at, updated_at, modified_at, size_bytes, content_hash,
  has_frontmatter, is_system

NoteTextRecord
  note_path primary, title_text, body_text, excerpt, word_count

TagRecord
  note_path, tag, source(frontmatter|inline)

SearchTokenRecord
  token, note_path, field(title|tag|body|path), count, weight

LinkRecord
  note_path, target_text, target_path, kind(wiki|markdown), line, broken

AttachmentRecord
  note_path, reference_text, target_path, media_type, exists, line

SavedViewRecord 或 views projection
  name, filters_hash, updated_at
```

约束：

- repository 使用 GORM API；普通业务层不硬编码 SQL。
- SQLite FTS5 暂不作为 MVP 默认依赖；如后续引入 virtual table，只能在 index repository 的受控 raw SQL 例外中实现，并补迁移/兼容测试。
- `index rebuild` 使用临时表或事务边界保证失败时不会留下半截 projection；最小实现可先在单事务内清空并重建。
- `index status` 应报告 schema version、note count、last built time、stale/missing/fresh，以及触发 stale 的证据。

### 7. 检索采用 index-first，rg/scan fallback

检索路径：

```text
CLI search flags
      │
      ▼
SearchRequest(query, filters, limit, sort)
      │
      ▼
internal/app Search service
      │
      ├─ fresh index -> internal/index repository weighted search
      ├─ stale index + --allow-stale -> index search with stale warning
      ├─ missing index + rg available -> rg fallback
      └─ missing index + no rg -> scan fallback
```

查询语义首版保持简单：

- 普通 query 匹配 title、tags、body、path token。
- filter 支持 `--tag`、`--group`、`--folder`、`--kind`、`--status`、`--created-after`、`--updated-before`、`--link-target`、`--has-attachment`。
- 排序支持 `relevance`、`updated`、`created`、`title`、`path`。
- 输出 facts 必须包含 `engine`、`index_status`、`total`、`returned`、`sort`、filter keys；`data.results` 包含 score、matched_fields、snippet 和 note projection。

这样 agent 可以稳定读取结果，而不是解析中文摘要或 `rg` 输出。

### 8. Agent 自动整理是 plan，不是直接修改

“自动整理”分两层：

1. `pinax organize suggest --save --json`：只读 vault 和 index，生成 `.pinax/organize-plans/<plan_id>.json`。
2. `pinax organize apply --plan <plan_id> --yes --snapshot-message "整理前快照"`：在 Git snapshot 保护后应用低风险操作。

Plan schema 建议：

```text
OrganizePlan
  schema_version: pinax.organize_plan.v1
  plan_id, created_at, expires_at, vault_root, source_command
  source_facts: note count, index status, note hashes
  operations[]
    operation_id
    kind: move|tag_patch|kind_patch|status_patch|link_resolution|attachment_repair|manual_review
    mode: automatic|manual_review
    risk: low|medium|review
    path, target_path, before, after, reason, evidence[]
  status: planned|applied|expired
```

整理建议来源必须可解释：路径规则、frontmatter、tag 共现、标题关键词、链接关系、附件缺失、重复标题、stale/empty/orphan 诊断。不得把“模型觉得应该这样”作为唯一证据。后续接 AI 时也只能作为 signal，最终 plan 仍需本地规则和证据落地。

### 9. 自动整理应用必须有状态和幂等边界

- `organize apply` 必须要求 `--yes`。
- 如果 plan 已过期或 source facts 与当前 vault 不一致，返回 `plan_stale`，要求重新 suggest。
- 低风险操作可以自动应用：tag/status/kind patch、移动到明确 group/folder、index rebuild。
- 中高风险或不可逆操作进入 `manual_review`：删除、覆盖、合并重复 note、重写正文链接、大量批量移动。
- 每个 operation 应有 operation id；重复 apply 已完成 operation 时返回当前结果，不重复写。
- 所有写入通过 app service，写入后 append redacted event，并刷新 index projection。

## Risks / Trade-offs

- 范围过大导致一次变更难落地 -> 按 tasks 拆成 daily/inbox、组织视图、links/backlinks、attachments、views、import/export 六个可独立验证阶段。
- daily/inbox 与普通 note 统计语义混淆 -> 在 projection 中明确 `kind=index` 系统页跳过普通统计，用户 daily note 仍可搜索。
- Markdown link/附件解析不完整 -> 首版支持 wiki link、Markdown link/image 和相对路径；复杂嵌套语法作为后续增强。
- saved view 成为另一个真源 -> views 只保存过滤条件，不保存 note 结果；每次 show 都重新查询当前 vault。
- import/export 误写用户文件 -> 默认 dry-run 友好、路径边界校验、覆盖需要 `--yes`，并记录 receipt。

## Migration Plan

1. 新增命令和 projection 时保持现有 `note new/list/show/search` 行为不变。
2. 扩展 GORM projection 后，`pinax index rebuild` 重建即可迁移，不需要用户手写迁移文件。
3. 对已有 vault：缺少 `.pinax/views.json`、receipt 文件时只在相关写命令执行时由 service 创建。
4. 失败回滚：删除新增 `.pinax/views.json` 和 receipts 不影响 Markdown note 真源；重新运行 `index rebuild` 可恢复 projection。

## Open Questions

- inbox capture 默认是追加到当天 inbox note，还是每条 capture 创建独立 note？建议先实现两种模式，默认独立 note，`--append` 追加当天 inbox。
- daily `open` 是否必须调用编辑器？建议 `daily show` 只读，`daily open` 才执行 editor。
- export bundle 是否需要生成 manifest？建议首版生成 `manifest.json`，但它属于导出产物，不进 vault 真源。
