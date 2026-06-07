## Why

Pinax 已经具备本地 Markdown vault、note CRUD、索引、模板、doctor 和基础信息架构，但还没有形成一个日常可用的“笔记软件”闭环。现在应先补齐本地笔记软件本体，让用户可以捕获、组织、浏览、关联、回顾和迁移笔记，再讨论外部 provider、AI 自动化或云同步扩展。

## What Changes

- 增加 daily/inbox 工作流：支持创建或打开当天 daily note、快速捕获到 inbox、把 inbox 条目归档到项目/文件夹/用途分类。
- 增强组织维度：支持按 group/project、folder、kind、status、tag、日期范围浏览；提供 tag/folder/kind/group 的本地索引视图。
- 增强链接和反链：支持列出 note 的出链、反链、孤立 note 和未解析 wiki link，保持 Markdown link/body 为真源。
- 增加附件管理：支持把本地文件复制到 vault 内附件目录、生成 Markdown 引用、列出 note 附件和检测缺失附件。
- 增加保存视图：支持 CLI-authored saved view，用稳定过滤条件保存常用列表，例如 inbox、active project、reference library。
- 增加导入导出：支持从本地 Markdown 文件/目录导入到 vault，支持按过滤条件导出 Markdown bundle，不依赖外部平台。
- 增强索引投影：SQLite projection 记录 folder/kind/status/dates、link/backlink、attachment、saved view 所需字段。
- 增强本地检索：创建并维护 `.pinax/index.sqlite` 数据库 schema，支持按标题、正文、tag、group、folder、kind、status、日期和链接关系检索。
- 增加 agent 自动整理设计：agent 只能生成可审查 organize plan，不能直接改 note；apply 必须显式 `--yes` 且有 Git snapshot 保护。
- 保持输出合同：新增命令均支持中文默认输出、`--json`、`--agent`，诊断写 stderr，错误 code 稳定。
- 不做外部同步、Feishu/Notion 导入、AI 自动摘要、全文语义检索、多人协作、移动端 UI 或长期 daemon。

## Capabilities

### New Capabilities

- `notebook-workflows`: 本地笔记软件核心工作流，包括 daily/inbox、组织浏览、链接反链、附件、保存视图、导入导出。
- `notebook-index-search`: 本地 SQLite/GORM 索引、检索查询、索引诊断和 agent 可审查自动整理计划。

### Modified Capabilities

- `note-command-ux`: 扩展 note 命令过滤、维护和引用能力，使其覆盖新工作流的命令入口和输出事实。
- `pinax`: 在 Pinax 总体能力中明确“本地笔记软件核心”优先于外部 provider 扩展。

## Impact

- 影响命令入口：`cmd/pinax` 新增 `daily`、`inbox`、`tag/folder/kind/group` 视图命令或 note 子命令，增强 `note list/show/search`。
- 影响应用服务：`internal/app` 增加 daily/inbox、link/backlink、attachment、saved view、import/export 用例。
- 影响领域模型：`internal/domain` 增加保存视图、附件、链接诊断、导入导出结果等 projection 数据结构。
- 影响索引：`internal/index` 通过 GORM 扩展 note/link/tag/attachment/saved-view projection；不得在业务层硬编码 SQL。
- 影响输出：`internal/output` 继续从 projection 渲染，不新增命令层拼接机器输出。
- 测试需要覆盖 CLI e2e、fixture vault、fake filesystem、stdout/stderr 分离、dry-run 和边界路径。
