## Context

Pinax 已经具备三个基础块：`pinax inbox capture/list/triage` 可快速捕获和整理，`pinax note add/list/show` 支持 `status`、`kind`、`folder` 等 metadata，`pinax index page preview|create|refresh` 能用模板和托管区块生成系统索引页。当前缺口是草稿箱没有一等工作流，inbox 缺少 show/promote/discard 和 review index page，远程 REST/RPC 也缺少等价入口。

本设计继续坚持 Pinax 的本地优先边界：Markdown note 是真源，SQLite/GORM index 是可重建 projection，`.pinax/` 下的结构化资产只能由 CLI/service 写入。agent 和远程客户端必须调用 Pinax 命令或 API，不直接 `mkdir`、移动文件、改 frontmatter 或手写 `.pinax/*.json`。

## Goals / Non-Goals

**Goals:**

- 把 inbox、draft、active、archived、discarded 建模为可管理 lifecycle/status，并提供明确状态转换。
- 增加 `pinax draft` 一等命令组，补齐 `pinax inbox show/promote/discard/index`。
- 让本地 CLI、REST、RPC 都调用同一 app service 和 projection，不维护另一套远程业务模型。
- 复用 index page 模板机制实现 inbox/draft review page，例如 `index.inbox` 和 `index.drafts`。
- 保持 output contract：人类输出中文，`--json` envelope、`--agent` key=value、错误码和 facts 使用稳定英文。

**Non-Goals:**

- 不把 Pinax 变成长生命周期云笔记后端；API 仍是本机 localhost projection adapter。
- 不引入新的数据库真源或任务队列；index 仍可删除重建。
- 不让 `discard` 做 hard delete；真实删除继续走 `pinax note delete --yes`。
- 不把所有自定义 `status` 收紧成枚举；本变更只约束 inbox/draft lifecycle commands 管理的状态转换。

## Decisions

### 1. lifecycle 使用 frontmatter `status`，不新增并行字段

状态机只对管理状态生效：

```text
inbox   -> draft | active | archived | discarded
draft   -> active | archived | discarded
active  -> archived
archived/discarded -> 只能通过显式 restore 类能力回到 active，首版不实现
```

普通 note 可以继续拥有 `todo`、`blocked`、`done` 等自定义 status；但 `pinax inbox` 和 `pinax draft` 命令只接受上述 lifecycle 目标。这样能复用现有 list/search/index 字段，也避免新 frontmatter 字段和旧 vault 迁移。

备选方案是新增 `lifecycle_status` frontmatter。它能让业务状态和生命周期分离，但会立刻带来双字段同步、旧 note 兼容和 query 复杂度。本阶段只在 projection 中输出 `lifecycle_status`，来源由 `status` 推导。

### 2. 草稿箱是一等命令，但底层仍是 note service

新增命令组：

- `pinax draft create <title>`：默认写到 `drafts/`，设置 `status: draft`，保留用户指定 `kind`。
- `pinax draft list`：等价于 `note list --status draft --sort updated` 的工作流投影。
- `pinax draft show <note>`：复用 note display，默认 bounded rendered/source 视图。
- `pinax draft promote <note> --status active --folder <folder> --kind <kind>`：更新 metadata，必要时移动出 `drafts/`。
- `pinax draft archive <note>`：设置 `status: archived`。
- `pinax draft discard <note>`：设置 `status: discarded`，不 hard delete。

`pinax inbox` 保留 `capture/list/triage`，补 `show/promote/discard/index`。`triage` 继续表达“整理到项目/文件夹/kind/status”，`promote` 表达简单状态推进，适合远程 agent 和快捷键。

### 3. `index inbox` 落到 review index page，而不是 SQLite index 命令

SQLite/GORM index 的入口仍是 `pinax index refresh|rebuild|status`。用户说的 `index inbox` 更接近“生成收件箱索引页”，因此设计为：

- 主路径：`pinax index page preview inbox --template index.inbox`、`create`、`refresh`。
- 工作流别名：`pinax inbox index preview|create|refresh`，固定 name 为 `inbox`，模板默认 `index.inbox`。
- 草稿别名：`pinax draft index preview|create|refresh`，固定 name 为 `drafts`，模板默认 `index.drafts`。

这些命令只写 `kind: index`、`status: system` 的系统 index page，并只刷新托管区块。普通 search、orphan 和 stats 继续排除 system index page。

### 4. REST/RPC 是 projection adapter，不直接读写 vault

新增远程 surface 只调用 app service：

```text
GET  /v1/inbox
GET  /v1/inbox/{ref}
POST /v1/inbox:capture
POST /v1/inbox/{ref}:promote
POST /v1/inbox/{ref}:discard

GET  /v1/drafts
GET  /v1/drafts/{ref}
POST /v1/drafts
POST /v1/drafts/{ref}:promote
POST /v1/drafts/{ref}:archive
POST /v1/drafts/{ref}:discard

RPC Pinax.Inbox.List/Show/Capture/Promote/Discard
RPC Pinax.Draft.List/Show/Create/Promote/Archive/Discard
```

默认 readonly server 只允许 GET 和 readonly RPC。写操作必须在 `pinax api serve --allow-write` 下运行，并且非 dry-run mutation 必须带 `yes=true`；否则返回 `write_disabled` 或 `approval_required`，且不写 Markdown、`.pinax`、index、Git 或 provider 状态。

### 5. index projection 保留管理视图所需 facts

index rebuild/refresh 需要保留每篇 note 的 `status`、推导出的 `lifecycle_status`、`kind`、`folder`、`project/group`、`updated_at`、`note_id` 和 canonical path。普通 search/list 不静默隐藏 inbox/draft，但必须标注 lifecycle facts；`discarded` 默认从普通 search/list 排除，只能显式 `--status discarded` 或专用命令查看。

### 6. 写入统一走 service transition helper

实现时应提取窄 helper，例如 `transitionNoteLifecycle(req)`：负责解析 note、校验状态转换、patch frontmatter、可选 move、append record event、append `.pinax/events`、refresh index、返回 projection。CLI、REST 和 RPC 共享它，避免 `inbox triage`、`draft promote`、远程 handler 各写一套逻辑。

## Risks / Trade-offs

- 状态含义和用户自定义 status 混用 -> 只约束 workflow 命令的目标值，普通 note/search 继续保留自定义 status。
- `discard` 被误解为删除 -> 输出、docs 和 facts 必须明确 `writes=true`、`deleted=false`、`status=discarded`，并给出 `note delete` 作为真实删除下一步。
- inbox/draft 被普通 search 搜出导致噪声 -> projection 标注 lifecycle，专用命令提供 focused queue；后续可在 saved view 或配置层增加默认过滤策略。
- index page 模板查询过重 -> 模板 query 必须 bounded，默认 limit 小，正文不默认加载；需要更多内容时用 `note show`。
- 远程写入面扩大 -> 继续依赖 `--allow-write`、`yes=true`、dry-run、route registry metadata、projection envelope 和 redaction contract。
- 与正在进行的 API contract hardening 变更重叠 -> 新路由必须复用同一 route registry、OpenAPI exporter 和 REST/RPC contract tests，不新增平行注册机制。

## Migration Plan

1. 先实现 service helper 和 CLI commands，保持 `note add --status draft`、`note list --status draft` 等旧路径可用。
2. 增加内置 `index.inbox`、`index.drafts` 模板和 index page aliases；旧 `index page` 命令不改语义。
3. 扩展 route registry、REST handler、RPC dispatcher 和 OpenAPI schema，默认 readonly 行为不变。
4. 更新 docs 和 tests 后运行 `task check`；归档前验证 `openspec validate --all`。

## Open Questions

- 是否需要首版实现 `restore`，把 `discarded` 或 `archived` 回到 `draft/active`。当前建议延期，避免误恢复。
- `draft promote` 默认移动目标是 vault root、原目录，还是用户显式 `--folder`。当前建议没有目标时保留路径，只更新 status，降低意外路径变化。
- 是否把 inbox/draft 默认排除出 `search`。当前建议不隐藏普通 Markdown，只排除 system/discarded。
