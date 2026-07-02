# Pinax 统一 Vault 工作区、Todo Kanban 与数据库计划

## 为什么

Pinax 已经形成本地 Markdown vault、proof loop、project board、Dataview/database、Cloud Sync、dashboard/MCP/API 等底座。下一步如果直接追 Obsidian、Notion、Todoist 的完整功能清单，产品会变成大而散的笔记 App 复刻；这会削弱 Pinax 当前最强的差异化：让 agent 安全地操作用户真实本地知识库。

更好的方向是把 Obsidian、Notion 和 Todo/Kanban 能力收束到同一个本地优先工作区模型中：Markdown vault 仍是真源，SQLite/GORM 仍是可重建投影，`.pinax/**` 仍由 CLI/application service 写入，agent 写入仍经过 plan、snapshot、receipt、restore。这样用户能在一个 vault 下管理多个项目、任务、数据库视图、双链、模板、资产和发布/同步面，同时不会丢失本地可迁移和 agent-safe 的护城河。

## 做什么

本变更建立 Pinax 后续一组核心能力的正式 OpenSpec 计划：

1. **统一 Vault Workspace**：一个 vault 内承载多个 project、subproject、collection、database view、saved view、task view 和 publish/sync policy。
2. **Todo Kanban 强化**：区分 Pinax-managed task 与普通 Markdown checklist；支持 task adopt、move、archive、blocked/review/done、saved board view 和 daily task review。
3. **Notion 风格数据库 v2**：在本地 Markdown/metadata/index 投影上提供 property schema、table/board/calendar/list view、filter/sort/group、relation-lite 和 rollup-lite。
4. **Obsidian 兼容能力矩阵**：围绕 wikilink/backlink/graph、properties、daily notes、templates、assets、search/dataview、publish/sync、plugin manifest、vault doctor/repair/organize 补齐可验证能力。
5. **Agent-safe 客户端覆盖**：所有新增能力以 CLI/application service 为真源，REST/RPC/MCP/dashboard 只是 projection adapter；能力通过 capability registry 增量暴露。

## 不做什么

- 不在 Pinax CLI 子项目内实现完整富文本编辑器、Canvas 白板、移动端客户端或跨平台 GUI。
- 不把 Pinax Cloud 变成明文笔记托管平台、团队协作 workspace 或 Notion 式云数据库。
- 不引入通用远程 shell、任意命令执行 RPC、远端明文搜索或 agent 自动无门禁写入。
- 不把任意 Markdown checklist 当成可直接修改的任务源；未 adopt 的 checklist 默认只读展示。
- 不删除或重命名现有 CLI 命令、JSON envelope 字段、`--agent` key、API route、view registry 字段或数据库投影表。

## 兼容性策略

本计划只允许增量演进：

- CLI 只新增命令、flag、optional 输出字段和 optional `--agent` key。
- REST/RPC 只新增 route/method 或 optional request/response 字段。
- `.pinax/**` structured assets 只新增 schema version、optional keys 或新文件；旧 registry 继续可读。
- SQLite/GORM index 只新增表、nullable column、索引或 projection；不得 drop/rename/narrow 既有列。
- 若后续实现必须做 breaking change，必须在子任务中先补充迁移、deprecation window、rollback 和 consumer update 清单，再进入实现。

## 成功标准

- 用户能在一个 vault 下创建项目、子项目、任务、数据库视图和 Obsidian 风格知识结构，并通过一致的 projection 读取。
- `pinax project board show`、`pinax database view render`、`pinax note backlinks`、`pinax vault doctor` 等命令能复用同一 index/workspace/task/view 模型。
- agent 和客户端默认拿到 bounded context，不包含完整 note body、provider payload、secret、raw prompt 或完整 chain-of-thought。
- 所有写入都通过 CLI/application service，风险写入保留 approval、snapshot、receipt 和 restore 路径。
- 每个阶段都有 focused tests、e2e/testscript 或 integration evidence；完成前 `task check` 和 `openspec validate --all --strict` 通过。
