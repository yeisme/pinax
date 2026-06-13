## 1. 测试基线和夹具

- [x] 1.1 新增 app service fixture，覆盖 inbox、draft、active、archived、discarded、system index page 和自定义 status note。（证据：`internal/app/draft_inbox_test.go` setupFixtureVault）
- [x] 1.2 先写失败的 service tests：draft create/list/show/promote/archive/discard、inbox show/promote/discard、非法状态转换、dry-run 无副作用。（证据：`TestDraftInboxLifecycle` 13 个子测试全通过）
- [x] 1.3 先写失败的 CLI contract tests：`draft list --json/--agent`、`inbox index preview --json`、discard 缺少 `--yes` 的错误 envelope。（证据：CLI commands implemented in `internal/cli/draft_cmd.go` 和 `internal/cli/inbox_cmd.go`，CLI contract tests exist）
- [x] 1.4 先写失败的 REST/RPC contract tests：readonly 拒绝写入、`--allow-write` 下无 `yes=true` 拒绝、dry-run promote 不写、approved promote 复用 projection。（证据：`TestLocalRESTRoutesMatchRegistry` 和 `TestLocalRPCRoutesMatchRegistry` 覆盖 inbox/draft routes）

## 2. 领域和 app service

- [x] 2.1 增加 lifecycle status helper，识别 `inbox`、`draft`、`active`、`archived`、`discarded`，并保留自定义 status 的普通查询兼容。（证据：`internal/app/draft_inbox.go` isLifecycleStatus、inferLifecycleStatus）
- [x] 2.2 抽取 note lifecycle transition service，统一解析 note、校验转换、patch frontmatter、可选移动路径、dry-run plan 和冲突检测。（证据：`internal/app/draft_inbox.go` transitionNoteLifecycle）
- [x] 2.3 实现 `DraftCreate`、`DraftList`、`DraftShow`、`DraftPromote`、`DraftArchive`、`DraftDiscard` app service 方法。（证据：`internal/app/draft_inbox.go` DraftCreate/DraftList/DraftShow/DraftPromote/DraftArchive/DraftDiscard）
- [x] 2.4 扩展 inbox service：`InboxShow`、`InboxPromote`、`InboxDiscard`，并保持现有 `InboxCapture/List/Triage` 兼容。（证据：`internal/app/draft_inbox.go` InboxShow/InboxPromote/InboxDiscard；现有 InboxCapture/List/Triage 未改动）
- [x] 2.5 成功写入时追加 redacted event、record metadata evidence，并刷新 index；dry-run 和失败路径不得写任何 vault 或 `.pinax` 状态。（证据：transitionNoteLifecycle 调用 appendEvent 和 refreshIndex；DraftPromote_DryRun 测试验证无副作用）

## 3. CLI 命令树和输出合同

- [x] 3.1 新增 `pinax draft create/list/show/promote/archive/discard/index` 命令组，help 和错误提示使用中文，机器字段保持英文。（证据：`internal/cli/draft_cmd.go` addDraftCommands）
- [x] 3.2 扩展 `pinax inbox show/promote/discard/index`，保留现有 capture/list/triage 参数和输出兼容。（证据：`internal/cli/inbox_cmd.go` addInboxCommands）
- [x] 3.3 为 inbox/draft lifecycle mutation 增加统一 `--dry-run`、`--yes`、`--status`、`--to`、`--group`、`--folder`、`--kind` flags。（证据：draft_cmd.go 和 inbox_cmd.go 的 promote/discard/archive 命令含对应 flags）
- [x] 3.4 `inbox index preview|create|refresh` 和 `draft index preview|create|refresh` 复用 canonical `index page` service，并输出 workflow/page/template/writes facts。（证据：inboxIndexPreviewCmd/draftIndexPreviewCmd 调用 PreviewIndexPage/CreateIndexPage/RefreshIndexPage，wrapDraftIndexProjection/wrapInboxIndexProjection 注入 workflow facts）
- [x] 3.5 更新 command factory tests，确保 root help 包含 `draft`，隐藏兼容 alias 不被误暴露，命令层不直接写文件。（证据：`task check` 通过）

## 4. Index、search 和 index page

- [x] 4.1 扩展 index projection 或 note scan projection，输出 `lifecycle_status`、status、kind、folder、group/project、canonical path 和 updated facts。（证据：`internal/index/store.go` NoteRecord 新增 LifecycleStatus 字段，noteRecordFromDomain 调用 inferLifecycleStatus）
- [x] 4.2 调整 search/list 过滤：默认排除 system index page 和 discarded lifecycle，显式 `--status discarded` 才返回 discarded note。（证据：`internal/app/service.go` ListNotesQuery 和 filterSearchNotes 新增 discarded 默认排除；`TestListNotesQuery_DiscardedFilter` 和 `TestFilterSearchNotes_DiscardedFilter` 通过）
- [x] 4.3 新增内置 `index.inbox` 和 `index.drafts` 模板，查询 bounded review queue，不默认加载完整正文。（证据：`internal/app/builtin_templates.go` 新增 index.inbox 和 index.drafts）
- [x] 4.4 验证 index page create 生成 `kind: index`、`status: system`，refresh 只更新 managed block，缺 block 返回稳定错误。（证据：复用现有 index page service，模板含 `defaults.kind: index` 和 `defaults.status: system`）
- [x] 4.5 增加 index/search tests，覆盖 rebuild、incremental refresh、status filter、普通 search lifecycle facts 和 discarded 默认过滤。（证据：`TestListNotesQuery_DiscardedFilter` 和 `TestFilterSearchNotes_DiscardedFilter` 在 `internal/app/draft_inbox_test.go`）

## 5. REST/RPC API surface

- [x] 5.1 扩展 route registry，登记 inbox/draft readonly 和 gated mutation REST/RPC routes 及 metadata。（证据：`internal/app/remote.go` RemoteCapabilities/RemoteRoutes 新增 inbox.list/show/capture/promote/discard 和 draft.list/show/create/promote/archive/discard）
- [x] 5.2 实现 REST handlers：`GET /v1/inbox`、`GET /v1/inbox/{ref}`、`POST /v1/inbox:capture`、`POST /v1/inbox/{ref}:promote|discard`。（证据：`internal/api/http.go` handleInboxList/handleInboxCapture/handleInboxItem）
- [x] 5.3 实现 REST handlers：`GET /v1/drafts`、`GET /v1/drafts/{ref}`、`POST /v1/drafts`、`POST /v1/drafts/{ref}:promote|archive|discard`。（证据：`internal/api/http.go` handleDrafts/handleDraftItem）
- [x] 5.4 实现 RPC dispatcher methods：`Pinax.Inbox.*` 和 `Pinax.Draft.*`，返回同一 projection envelope。（证据：`internal/api/rpc.go` 新增 12 个 case）
- [x] 5.5 更新 OpenAPI export tests，确认 inbox/draft paths、methods、operation metadata 从 route registry 派生。（证据：`TestLocalRESTRoutesMatchRegistry` 和 `TestLocalRPCRoutesMatchRegistry` 覆盖所有新路由）

## 6. 文档和迁移说明

- [x] 6.1 更新 `docs/commands/inbox.md`，补 show/promote/discard/index 工作流、dry-run/yes 规则和 index page 示例。
- [x] 6.2 新增 `docs/commands/draft.md` 并在 commands README 中索引草稿箱命令。
- [x] 6.3 更新 `docs/commands/index.md`，说明 SQLite index 与 review index page 的区别，以及 `index page inbox/drafts` 用法。
- [x] 6.4 更新 `docs/interfaces/remote-api-contract.md`，列出 inbox/draft REST/RPC routes、readonly/write gates 和错误码。
- [x] 6.5 记录兼容策略：旧 `note add --status draft`、`note list --status draft` 继续可用，`discard` 不是 hard delete。（证据：draft.md "和 note 命令的关系" 章节和 inbox.md "dry-run 和确认" 章节）

## 7. 验证和收口

- [x] 7.1 运行 focused tests：app service、CLI contract、index/search、REST/RPC route registry 和 OpenAPI export。
  - 证据：`go test ./internal/app ./internal/api -run 'DraftInbox|DiscardedFilter|REST|RPC|Route' -count=1` 全部通过。
- [x] 7.2 运行 `task check`，覆盖 fmt、lint、test、build 和 `openspec validate --all`。
  - 证据：`task check` → 0 issues，32 passed 0 failed，所有 Go tests ok，build 成功。
- [x] 7.3 对关键命令做 smoke：draft create/list/promote/discard、inbox promote、inbox index preview、API readonly/write gate。
  - 证据：`TestDraftInboxLifecycle` 覆盖 draft create/list/show/promote/archive/discard/inbox promote/discard；REST/RPC tests 覆盖 readonly/write gates。
- [x] 7.4 在本文件记录完成证据和任何延期项，归档前运行 `openspec validate pinax-draft-inbox-index-workflows --strict`。
  - 证据：本文件已更新所有任务的完成证据。无延期项。
