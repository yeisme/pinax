# Proposal: Pinax Project Board Workspace

## 背景

Pinax 已经具备本地 Markdown vault、project metadata、saved views、typed database query、daily/weekly/monthly planning、organize/repair plan、只读 dashboard 和只读 MCP surface。现在的缺口是：用户知道一个项目有哪些相关笔记和计划，但很难像看板一样持续追踪“下一步是什么、卡在哪、哪些已经完成、哪些只是参考材料”。

现有 `pinax plan daily|weekly|monthly` 更偏时间维度，`pinax project create/list/switch` 更偏项目 metadata，`pinax database view` 更偏通用查询。需要一个项目工作区入口，把项目、计划、工作项、风险和证据连成可扫描的操作面。

本 change 设计 `pinax project board` 和 `pinax project item` 能力，让 Pinax 支持本地优先的项目看板，同时保持 Markdown vault 是真源、SQLite/GORM 是投影、TaskBridge 是任务执行控制面。

## Why

`plan project` 如果只输出一次性 Markdown 正文，很快会失去执行状态；如果直接写远端 Todo，又会越过 Pinax 的本地优先边界。项目看板把项目计划变成可重复查询、可审查、可保存证据的工作区投影，让用户和 agent 能看到项目当前状态，并把真实写入留给明确审批的 Pinax item 命令或 TaskBridge action draft。

## What Changes

- 新增 `pinax project board` 命令族，提供项目看板只读展示、计划快照、列配置和 Markdown 导出。
- 新增 `pinax project item` 命令族，提供受控的本地项目工作项创建、移动和归档。
- 新增 project board OpenSpec capability，定义 board projection、note display surface、CLI-authored board config、approval/snapshot gate、planning 集成、dashboard/MCP 只读面和 fixture-first tests。
- 新增 remote interface 设计：REST 和 RPC 都必须复用 application service projection，不维护平行业务模型。
- 扩展后续实现范围到 `cmd/pinax`、`internal/app`、`internal/domain`、`internal/output`、`internal/dashboard` 和 `internal/mcpserver`，但本 change 当前只提交设计。

## 用户问题

- 做项目计划时，用户需要同时打开 project note、daily note、TaskBridge 任务和搜索结果，认知负担高。
- `plan project` 类能力如果只生成一段正文，很快过期；如果直接写远端 Todo，又越过 Pinax 边界。
- Agent 需要稳定的项目上下文和下一步事实，但不能直接读 `.pinax` metadata 或猜测 Markdown 正文。
- 用户希望像看板一样扫列、拖阶段、看阻塞，但 Pinax 首期仍是 CLI-only，不能为了体验直接引入 Web 协作产品。
- 功能增加后如果 CLI、dashboard、MCP、REST 和 RPC 各自定义字段，远程接口会很快失控，客户端升级和兼容测试成本都会上升。

## 目标

- 新增项目工作区概念：一个 project 可以有 board layout、columns、work items、saved view 和 planning evidence。
- 新增 `pinax project board show|plan|save|export`：从 Markdown note、frontmatter、inline task、database query 和 TaskBridge snapshot 生成看板投影。
- 新增 `pinax project item add|move|archive`：把轻量工作项作为 Markdown note 或 managed block 维护，并通过 application service 写入。
- 增强笔记查看：为 `note read/show`、project board、dashboard 和 MCP 定义共享 `NoteDisplay` 投影，支持 `card`、`detail`、`context`、`body` 四种展示层级。
- 规范 CLI 对外展示的信息：默认摘要给人读，`--agent` 给低 token facts，`--json` 给 envelope，MCP/dashboard 只给 bounded facts；完整正文必须由显式 note read/show 请求取得。
- 规范 REST/RPC 暴露方式：REST 面向资源读取和少量 action endpoint，RPC 面向 agent/SDK 调用，但两者都返回同一 command projection envelope。
- 控制远程接口维护成本：所有远程 endpoint 都必须声明 capability id、schema version、projection command、权限、body exposure 和 contract test。
- 支持默认看板列：`inbox`、`next`、`doing`、`blocked`、`review`、`done`，允许每个 project 通过 CLI-authored metadata 覆盖。
- 让 daily/weekly planning 可以引用 project board snapshot，生成更好的承诺、风险和下一步建议。
- 让 dashboard 和 MCP 暴露只读 board summary，供人和 agent 查询。

一句话产品目标：

> Pinax project board 是本地 Markdown 项目的操作面：看清项目状态，保存审查证据，真正执行仍走明确审批。

## 非目标

- 不实现多人实时协作看板。
- 不实现浏览器拖拽 UI 作为 MVP 必需能力；dashboard 首期只读展示。
- 不让 Pinax 直接写 Todo provider；TaskBridge 写回仍通过 action draft 和 `taskbridge agent execute`。
- 不让 Agent 手写 `.pinax/projects.json`、`.pinax/project-boards/*.json`、snapshot、receipt 或 event JSONL。
- 不把所有 Markdown checklist 自动当成远端任务。
- 不引入长期 daemon、后台 watcher 或自动通知。
- 不把 REST/RPC 设计成独立云笔记 API，不允许它绕过 CLI/application service 写 vault。
- 不在 MVP 暴露公网默认监听；本地 API 默认只绑定 `127.0.0.1`，非 loopback 需要后续安全设计。

## MVP 范围

### 命令面

```bash
pinax project board show research --vault ./my-notes
pinax project board show research --note-display card --vault ./my-notes
pinax project board show research --vault ./my-notes --json
pinax project board plan research --vault ./my-notes --dry-run --json
pinax project board plan research --vault ./my-notes --save --json
pinax project board export research --format markdown --vault ./my-notes

pinax note read note_123 --display card --vault ./my-notes
pinax note read note_123 --display detail --with-context --vault ./my-notes --json
pinax note show note_123 --display body --vault ./my-notes

pinax api serve --vault ./my-notes --readonly --port 0
pinax api routes --vault ./my-notes --json
pinax api schema export --format openapi --vault ./my-notes --json

pinax project item add research "实现看板 projection" --column next --tags pinax,planning --vault ./my-notes --json
pinax project item move item_abc123 doing --vault ./my-notes --json
pinax project item move item_abc123 done --yes --snapshot-message "看板更新前快照" --vault ./my-notes --json
pinax project item archive item_abc123 --yes --vault ./my-notes --json
```

### 数据源

- Project metadata：`.pinax/projects.json` 中的 project slug、name、notes prefix 和 current pointer。
- Markdown notes：`project: <slug>`、`kind: project|task|decision|goal|reference`、`status`、`board_column`、`due`、`priority` 等 frontmatter。
- Inline task：受控解析 `- [ ]` 和 `- [x]` 行，首期只读入 projection，不自动重写任意正文。
- Database query：复用 Pinax SQL 和 typed property projection。
- TaskBridge snapshot：可选读取 planning snapshot 或 TaskBridge CLI facts；不可直接读 TaskBridge store。

### 输出

- 默认输出：中文项目看板摘要，列数量、阻塞项、近期 due、推荐下一步。
- Note display：`card` 只显示标题、路径、项目、状态、标签、更新时间和短摘；`detail` 增加链接/反链/附件/board facts；`context` 增加相关笔记摘要；`body` 才显示正文。
- `--json`：稳定 envelope，包含 board columns、items、source facts、warnings、note display payload 和 next actions。
- `--agent`：低 token facts，不输出完整正文，不输出本地化段落。
- `--explain`：中文可审查摘要，说明为何某项进入 blocked/next/review，或为何笔记摘要/上下文被选中。
- REST：`GET /v1/projects/{slug}/board`、`GET /v1/notes/{ref}`、`GET /v1/search` 等读取 endpoint 返回 JSON projection；写入类 endpoint 使用 `:plan` 或 action endpoint，默认 dry-run。
- RPC：`Pinax.ProjectBoard.Show`、`Pinax.Note.Read`、`Pinax.ProjectItem.MovePlan` 等方法返回同一 projection envelope；真实写入仍需要 approval 和 Git snapshot gate。

## Owner 和范围

- Owner: `cli/pinax`
- 主要实现路径：`cmd/pinax`、`internal/app`、`internal/domain`、`internal/output`、`internal/index`、`internal/dashboard`、`internal/mcpserver`、`internal/redaction`
- 可能新增路径：`internal/projectboard`
- OpenSpec owner：`cli/pinax/openspec/changes/pinax-project-board-workspace/`

## 风险

- 看板语义可能和 TaskBridge Todo 重叠。边界：Pinax 管 project memory 和 local workspace，TaskBridge 管 remote task execution。
- 自动从 Markdown checklist 推断工作项可能误伤。MVP 只读展示，写入只处理 Pinax 管理的 item note/frontmatter。
- 看板列变成任意 schema。MVP 先固定默认列和少量 CLI 覆盖，后续再扩展字段类型。
- Dashboard 容易诱导用户期待拖拽写入。MVP dashboard 只读，写入给出 CLI 命令。
