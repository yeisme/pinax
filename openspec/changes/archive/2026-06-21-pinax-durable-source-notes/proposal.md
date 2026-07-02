## Why

Pinax 已经能创建、移动、标记、索引和查询 Markdown 笔记，但长期保存外部资料源时，用户和 agent 仍然容易把“临时摘录”当作“长期知识资产”。例如 `iptv-org/iptv` 这类 GitHub 资源笔记，如果只记录 URL 和简介，后续很难判断它是否仍可用、能否用于产品、有哪些风险、应该链接到哪些概念卡或项目卡。

长期资料源笔记需要稳定的创建模板、推荐路径、标签规范、复查字段、关系检查和安全整理流程。它们应该沉到 Pinax CLI 的模板、metadata、organize、index 和 graph 能力里，而不是只依赖某个 agent skill 的写作习惯。

## What Changes

- 新增长期外部资料源笔记类型的产品规格：以 `kind: source` 或兼容的 `kind: reference` + `source/*` tags 表达，不破坏现有 note frontmatter。
- 新增内置模板规划：`source.github` 用于 GitHub 仓库资料源，输出建议路径为 `sources/github/<slug>.md`。
- 扩展 metadata/organize 规划：能对已有外部 URL 笔记建议 `kind`、结构化 tags、目标路径、复查字段和关系检查，不自动重写正文。
- 固定长期资料源笔记的推荐正文结构：source facts、canonical URLs、use decision、risk/boundary、verification、related notes、next review。
- 固定 Pinax 可执行维护路径：snapshot → metadata/organize plan → human review → apply → index refresh → links/backlinks/orphans check。
- 预留一个薄 skill 作为 agent 审稿流程，但 skill 只指导如何调用 Pinax，不成为长期存储规则的唯一来源。

## Compatibility

本变更按增量演进设计：

- CLI 命令：复用现有 `note`、`template`、`metadata`、`organize`、`index`、`links/backlinks/orphans`；如需新增 `template` 名称或 `organize` 建议类型，均为 additive。
- CLI 输出：不得删除或重命名现有 JSON envelope、`--agent` key、错误码或命令名；只能新增可忽略字段和 facts。
- Note frontmatter：不得要求迁移既有 notes；新增推荐字段必须可选，旧笔记仍可索引、搜索和显示。
- Tags：新增建议 tag 词表，不改变 tag 校验规则。
- Storage：不得新增强制数据库迁移；若需要索引额外字段，必须是可重建 projection 的增量字段。

## Non-Goals

- 不实现网页爬虫、GitHub API 同步、自动 freshness 探测或实时链接健康监控。
- 不让 agent 手写 `.pinax/` 结构化资产。
- 不把所有 reference 笔记强制迁移成 `kind: source`。
- 不引入新的外部 provider、daemon、Web UI 或云端服务。
- 不让 organize 自动拆分正文、改写判断、删除笔记或创建大量概念卡。
