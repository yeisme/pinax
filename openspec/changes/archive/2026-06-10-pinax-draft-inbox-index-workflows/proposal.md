## Why

Pinax 现在已有 `inbox capture/list/triage`、note `status` 和本地 index projection，但草稿箱还只是普通 status 值，缺少一等命令、远程 API 和 review/index page 机制。用户和远程 agent 需要用统一的 `pinax` 入口管理临时内容、草稿、收件箱和可视化索引页，而不是直接改 Markdown 路径或 `.pinax` 结构化资产。

## What Changes

- 新增草稿箱工作流设计：`pinax draft list/show/create/promote/archive/discard` 作为一等入口，底层仍写 Pinax Markdown note metadata/path，并通过 app service 刷新索引。
- 扩展 inbox 工作流：补齐 `show`、`promote`、`discard`、`index preview|create|refresh` 等 review-oriented 操作，让 inbox 项可以被预览、推进到 draft/active，或安全丢弃。
- 明确 lifecycle/status 规则：`inbox`、`draft`、`active`、`archived`、`discarded` 是 CLI/API 认可的可管理状态；跨状态转换必须经过 Pinax service，不能由 agent 直接手写 frontmatter 或移动文件。
- 统一本地和远程操作面：API server 暴露 readonly 查询和 gated write RPC/REST 操作，写操作仍受 `api serve --allow-write`、dry-run/yes、vault boundary、事件和索引刷新约束。
- 设计 inbox/draft review index page：复用现有 `index page` 模板和托管区块能力，支持 `index.inbox`、`index.drafts` 或工作流别名生成可刷新导航页。
- 调整搜索和列表默认可见性：普通 search/list 默认不把 system index page 当普通笔记；inbox/draft 默认可通过专用命令和显式状态过滤查询，并在普通结果中稳定标注 lifecycle/status。

## Capabilities

### New Capabilities

<!-- 无新增独立能力；本变更扩展现有 notebook workflow、index/search 和 CLI/API surface。 -->

### Modified Capabilities

- `notebook-workflows`: 扩展 inbox，并新增 draft review lifecycle 的需求。
- `notebook-index-search`: 明确 inbox/draft 与 system index page 的 projection、搜索过滤和 review index page 行为。
- `cli-tree-ux`: 增加草稿箱和 inbox/draft index page 工作流在命令树中的位置。
- `project-board-workspace`: 复用现有 REST/RPC projection adapter 规范，增加 inbox/draft 远程查询与 gated mutation 路由。

## Impact

- CLI：`internal/cli/inbox_cmd.go`、新增或扩展 draft command factory、`internal/cli/index_cmd.go` 的 workflow alias/help。
- App service：`internal/app` 中新增 draft/inbox review use case、状态转换校验、event append、record metadata event 和 index refresh 调用。
- Index/search：`internal/index` 和 search/list projection 需要保留 `lifecycle_status`、`status`、`kind`、review queue facts、index page system classification。
- API：`internal/cli/api_cmd.go` 和 API handler 增加 inbox/draft readonly 查询与 gated write 操作，保持 JSON envelope、错误码和 stdout/stderr 分离合同。
- Docs/tests：补充 `docs/commands/inbox.md`、新增 draft 命令文档、index page 文档示例，以及 testscript/contract tests。
