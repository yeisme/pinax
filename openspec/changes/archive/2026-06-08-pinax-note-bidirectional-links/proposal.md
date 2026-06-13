## Why

Pinax 已有基础 `note links/backlinks/orphans` 命令和索引中的 link projection，但“双联”还没有成为稳定产品能力：链接解析、同名歧义、断链诊断、索引一致性和 agent/MCP 可消费输出缺少统一设计。现在需要把它补成可迁移 Markdown vault 的核心关系图能力，而不是停留在临时扫描命令。

## What Changes

- 完善本地双联能力：支持 wiki link、Markdown note link、标题/路径/note_id 解析、别名展示、heading 片段归一化和断链状态。
- 强化反链和孤立笔记语义：反链从同一 link graph projection 派生，孤立笔记排除系统索引页，并区分无出链、无入链、完全孤立。
- 强化 SQLite/GORM 索引 projection：link record 记录 source、target、resolved path、kind、line、alias、heading、broken 和 ambiguity evidence，索引仍是可重建 projection。
- 完善 CLI 输出合同：`note links`、`note backlinks`、`note orphans`、`search --link-target` 在默认中文、`--json`、`--agent` 下使用同一 projection，稳定暴露 count、resolved、broken、ambiguous、orphan facts。
- 增加维护闭环：`doctor` / `repair plan` / `organize suggest` 对断链、歧义链接和孤立笔记只生成可审查建议，不自动改写正文链接。
- 扩展只读 agent/MCP 查询面：允许 agent 读取某笔记的出链、反链、断链和局部关系上下文，但不通过 MCP 直接写 vault。

## Capabilities

### New Capabilities
- `note-bidirectional-links`: 定义 Pinax 本地 Markdown 双联图谱、解析规则、关系查询、诊断和只读 agent surface。

### Modified Capabilities
- `notebook-workflows`: 将已有 links/backlinks/orphans 行为从基础检查升级为完整双联工作流，补充断链、歧义和孤立分类。
- `notebook-index-search`: 明确 link graph projection 的 GORM schema、fresh/stale 行为和 `--link-target` 查询语义。
- `note-command-ux`: 补充关系命令的 flags、输出 facts、错误码和同名候选行为。

## Impact

- 影响 CLI：`cmd/pinax` 的 `note links`、`note backlinks`、`note orphans`、`search --link-target`，后续可增加 `note graph` 或 `note links --broken-only` 等 flags。
- 影响 app/domain：`internal/app` 需要统一 link graph service；`internal/domain` 需要稳定 `NoteLink`、`NoteGraph`、`BrokenLink`、`OrphanNote` projection。
- 影响索引：`internal/index` 的 `LinkRecord` 需要迁移或重建字段；通过 GORM repository 维护，不让业务层硬编码 SQL。
- 影响输出：`internal/output` 需要覆盖 summary/json/agent/explain 的统一 projection 和 stdout/stderr 分离测试。
- 影响健康与整理：`doctor`、`repair plan`、`organize suggest` 需要把 link evidence 纳入计划，但正文重写保持 manual review。
- 不影响 provider、云同步、飞书/Notion 导入、长期 daemon 或真实网络访问。
