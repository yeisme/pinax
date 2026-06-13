# Tasks: Pinax Project Board Workspace

## 任务原则

- Owner: `cli/pinax`。
- Scope: 只实现 Pinax 本地 project board workspace，不修改 TaskBridge provider 写回行为。
- 结构化资产必须由 CLI/application service 写入，不让 Agent 手写 `.pinax/project-boards/*.json`、snapshot、receipt 或 event JSONL。
- 新增复杂映射、状态机、协议转换、approval/snapshot gate 和非显然 fixture 时必须写简短中文注释。
- 修改 Go 代码后运行 `task check`；没有 `task` 时运行 `golangci-lint fmt --diff`、`golangci-lint run`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`、`openspec validate --all`。

## Lane A: Board Domain 和 Projection

- [x] 1.1 新增 project board domain types。
  - Owner: `cli/pinax`
  - Files: `internal/domain/types.go` 或新增 `internal/domain/project_board.go`
  - Depends on: none
  - Acceptance: `go test ./internal/domain -run TestProjectBoard -count=1` 通过；类型包含 board、column、item、source、warning、snapshot facts。
  - Failure re-check: 如果输出层需要重复构造 map，回到 domain 定义稳定 projection shape。

- [x] 1.2 新增 board projection builder。
  - Owner: `cli/pinax`
  - Files: 新增 `internal/app/project_board.go`，测试 `internal/app/project_board_test.go`
  - Depends on: 1.1
  - Acceptance: fixture notes 能映射到 `inbox|next|doing|blocked|review|done` 列；unknown status 进入 warning，不静默丢弃。
  - Failure re-check: 如果 mapping 逻辑分散在 command 层，迁回 app service。

- [x] 1.3 复用 index/query 数据源。
  - Owner: `cli/pinax`
  - Files: `internal/app/project_board.go`、`internal/index/*`、`internal/app/query.go`
  - Depends on: 1.2
  - Acceptance: board show 优先使用 fresh index；missing/stale 时降级扫描并给出 `pinax index rebuild --vault ./my-notes` next action。
  - Failure re-check: 不允许新增绕过 Pinax SQL/query service 的平行 query parser。

- [x] 1.4 新增共享 `NoteDisplay` 投影。
  - Owner: `cli/pinax`
  - Files: `internal/domain/project_board.go` 或 `internal/domain/note_display.go`、`internal/app/project_board.go`、`internal/app/service.go`
  - Depends on: 1.1, 1.2
  - Acceptance: `card|detail|context|body` 四种 display 层级都有测试；`card/detail/context` 不返回完整正文；`body` 只在显式 note read/show 请求下返回。
  - Failure re-check: 如果 board、dashboard、MCP 和 note read 各自拼字段，抽回共享 builder。

## Lane B: CLI 命令和写入门禁

- [x] 2.1 新增 `pinax project board show|plan|configure|export` 命令。
  - Owner: `cli/pinax`
  - Files: `cmd/pinax/main.go` 或命令拆分文件、`cmd/pinax/main_test.go`
  - Depends on: 1.2
  - Acceptance: `pinax project board show research --vault ./my-notes --json` 输出一个 JSON envelope；默认输出为中文摘要；`--note-display card|detail|context` 控制 board item 嵌入的 note display 层级。
  - Failure re-check: 如果 help 文案暗示远端 Todo 写回，改为本地 Markdown project workspace 语义。

- [x] 2.1b 扩展 `pinax note read/show` 的 display 参数。
  - Owner: `cli/pinax`
  - Files: `cmd/pinax/main.go` 或命令拆分文件、`cmd/pinax/main_test.go`、`internal/app/service.go`
  - Depends on: 1.4
  - Acceptance: `pinax note read note_123 --display card|detail|context|body --vault ./my-notes --json` 都从 `NoteDisplay` 投影渲染；默认人类模式保持现有 source/rendered 兼容。
  - Failure re-check: `--display body` 之外的模式不得在 `--agent`、dashboard 或 MCP 输出完整正文。

- [x] 2.2 新增 board registry service。
  - Owner: `cli/pinax`
  - Files: `internal/app/project_board.go`、`internal/domain/project_board.go`
  - Depends on: 2.1
  - Acceptance: `project board configure` 通过 service 写 `.pinax/project-boards/<slug>.json`，包含 `schema_version=pinax.project_board.v1` 并 append redacted event。
  - Failure re-check: 不允许 command 层直接拼 JSON 文件。

- [x] 2.3 新增 `pinax project item add|move|archive`。
  - Owner: `cli/pinax`
  - Files: `cmd/pinax/main.go`、`internal/app/project_board.go`、`internal/app/service.go`
  - Depends on: 1.2
  - Acceptance: item add 创建 Pinax-managed note 或 managed item block；move 更新 `board_column/status/updated_at`；archive 需要 `--yes`。
  - Failure re-check: 如果能改非 Pinax-managed inline task，必须拒绝并返回 manual review next action。

- [x] 2.4 加入 approval 和 Git snapshot guard。
  - Owner: `cli/pinax`
  - Files: `internal/app/project_board.go`、`internal/git/*`
  - Depends on: 2.3
  - Acceptance: 高风险 move/archive/batch apply 缺少 `--yes` 返回 `approval_required`；需要 snapshot 时返回 `snapshot_required` 和 runnable `pinax git snapshot` action。
  - Failure re-check: 验证 dry-run、show、plan 不写 Markdown、`.pinax`、Git 或远端状态。

## Lane C: 输出、Dashboard、MCP

- [x] 3.1 扩展 output renderer。
  - Owner: `cli/pinax`
  - Files: `internal/output/render.go`、`internal/output/render_test.go`
  - Depends on: 1.2
  - Acceptance: human、`--json`、`--agent`、`--events`、`--explain` 都从同一 projection 渲染；机器 stdout 不含本地化表格噪音；`NoteDisplay` fields 在 `--json` 和 `--agent` 中稳定。
  - Failure re-check: `--agent` 不输出 note body、长 Markdown、ANSI、Glamour 渲染文本或本地化段落。

- [x] 3.1b 定义 CLI 对外展示字段白名单。
  - Owner: `cli/pinax`
  - Files: `docs/interfaces/cli-output-contract.md`、`internal/output/render_test.go`、`internal/app/project_board_test.go`
  - Depends on: 1.4, 3.1
  - Acceptance: `note_id/title/path/project/kind/status/tags/updated_at/display/exposure/excerpt/board_column/links_count/backlinks_count/attachments_count/related_count/redaction_warnings` 有 contract tests；新增字段走可选字段。
  - Failure re-check: 输出不得包含 token、Authorization header、provider payload、raw prompt、隐藏系统提示、完整思维链或未请求的正文。

- [x] 3.2 新增只读 dashboard board API。
  - Owner: `cli/pinax`
  - Files: `internal/dashboard/server.go`、`internal/dashboard/server_test.go`
  - Depends on: 1.2, 3.1
  - Acceptance: GET board API 返回 project board JSON；POST/PUT/DELETE 返回 readonly error；HTML 页面只展示 CLI next action。
  - Failure re-check: API 不写 Markdown、`.pinax`、Git、TaskBridge 或远端 provider。

- [x] 3.3 新增只读 MCP board resource/tool。
  - Owner: `cli/pinax`
  - Files: `internal/mcpserver/server.go`、`internal/mcpserver/server_test.go`
  - Depends on: 1.2, 3.1
  - Acceptance: `pinax://project/{slug}/board` 和 `pinax.project.board` 返回 bounded board facts；不返回完整正文。
  - Failure re-check: write-like tool request 必须拒绝或给 CLI next action。

## Lane C2: REST/RPC Remote Surface

- [x] 3.4 新增 remote capability registry。
  - Owner: `cli/pinax`
  - Files: `internal/domain/remote.go`、`internal/app/remote.go`、`internal/app/remote_test.go`
  - Depends on: 1.4, 3.1
  - Acceptance: `api.capabilities` projection 返回 CLI/REST/RPC/MCP surfaces、command、schema version、readonly、body_allowed、approval_required、snapshot_required 和 stable errors。
  - Failure re-check: 如果 capability 信息散落在 handler、文档和测试里，改成单一 registry。

- [x] 3.5 设计本地 REST adapter。
  - Owner: `cli/pinax`
  - Files: `internal/api/http.go`、`internal/api/http_test.go` 或等价现有 package
  - Depends on: 3.4
  - Acceptance: REST endpoint 只做参数解析、鉴权占位、状态码映射和 projection JSON 序列化；`GET /v1/projects/{slug}/board` 与 CLI `project.board.show --json` 的 facts keys 对齐。
  - Failure re-check: REST handler 不直接读 `.pinax`、不直接解析 Markdown、不直接访问 GORM repository。

- [x] 3.6 设计本地 RPC adapter。
  - Owner: `cli/pinax`
  - Files: `internal/api/rpc.go`、`internal/api/rpc_test.go` 或 MCP 复用 adapter 文件
  - Depends on: 3.4
  - Acceptance: `Pinax.ProjectBoard.Show`、`Pinax.Note.Read`、`Pinax.ProjectItem.Plan` 返回同一 projection envelope；字段只增不改。
  - Failure re-check: RPC 不能维护与 REST/CLI 不同的响应结构。

- [x] 3.7 新增 `pinax api` 命令面。
  - Owner: `cli/pinax`
  - Files: `cmd/pinax/main.go` 或命令拆分文件、`cmd/pinax/main_test.go`
  - Depends on: 3.4, 3.5
  - Acceptance: `pinax api routes --vault ./my-notes --json` 和 `pinax api schema export --format openapi --vault ./my-notes --json` 从 registry 输出；`pinax api serve --readonly --port 0` 默认绑定 `127.0.0.1`。
  - Failure re-check: 非 loopback bind、CORS、TLS、token、多用户权限如果未设计，必须拒绝或标记 unsupported。

## Lane D: Planning 集成和测试证据

- [x] 4.1 让 daily/weekly planning 可读取 board snapshot。
  - Owner: `cli/pinax`
  - Files: `internal/app/*planning*` 或现有 planning service 文件
  - Depends on: 1.2
  - Acceptance: planning decision facts 包含 board blocked/next/doing counts 和 evidence refs；不自动把所有 board item 写入计划。
  - Failure re-check: 如果 TaskBridge unavailable，board 本地 projection 仍可工作。

- [x] 4.2 增加 testscript 覆盖完整 CLI workflow。
  - Owner: `cli/pinax`
  - Files: `tests/e2e/testdata/project_board/*`
  - Depends on: 2.1, 2.3, 3.1
  - Acceptance: 覆盖 board show/plan/save、item add/move/archive、note display card/detail/context/body、approval gate、snapshot guard、stdout/stderr 分离和 redaction。
  - Failure re-check: 测试不得依赖真实 TaskBridge、真实 provider token、用户 vault 或公网。

- [x] 4.2b 增加 REST/RPC component test evidence。
  - Owner: `cli/pinax`
  - Files: `tests/e2e/testdata/project_board_api/*`、`internal/testkit/integrationevidence/*` 或现有 evidence harness
  - Depends on: 3.5, 3.6, 3.7
  - Acceptance: `task test:integration` 或项目约定入口启动本地 API/RPC server，验证 capabilities、board、note display、write dry-run、approval_required、snapshot_required、redaction，并写入 `temp/integration-test-runs/<run-id>/` 证据。
  - Failure re-check: 失败测试也必须保留 evidence，并以原 exit code 退出；证据中不得包含 token、Authorization header、raw prompt、provider payload 或完整正文泄漏。

- [x] 4.3 更新 Pinax 文档。
  - Owner: `cli/pinax`
  - Files: `README.md`、`docs/README.md`、`docs/product/mvp-scope.md`、`docs/interfaces/cli-output-contract.md`、`docs/interfaces/remote-api-contract.md`、`docs/operations/local-development.md`
  - Depends on: 2.1, 3.1
  - Acceptance: 文档展示真实可运行命令；说明 board 是本地 project workspace，不是远端 Todo provider；REST/RPC 是本地 projection adapter，不是公网 hosted API。
  - Failure re-check: 不回填根 `docs/**`。

- [x] 4.4 运行质量门禁。
  - Owner: `cli/pinax`
  - Depends on: all implementation tasks
  - Acceptance: `task check` 通过；如果本机没有 `task`，fallback 命令全部通过并记录结果。
  - Failure re-check: OpenSpec 通过 `openspec validate --all`；失败时先修 spec，再复验。

## 验证证据

- 领域与 app：`go test ./internal/domain -run TestProjectBoard -count=1`；`go test ./internal/app -run 'TestProjectBoard|TestShowNoteProjectionDisplay|TestPlanWeeklyIncludesSavedProjectBoardSnapshot|TestProjectItemArchiveRequiresVersionSnapshot|TestProjectItemMoveDoneRequiresApprovalAndVersionSnapshot|TestValidateVaultChecksProjectBoardAssets|TestProjectItemMoveRefusesUnmanagedNote|TestRemoteCapabilitiesExposeProjectionCommandsAndGates' -count=1`。
- CLI workflow：`go test ./cmd/pinax -run TestProjectBoardAndNoteDisplayCLI -count=1`。
- dashboard/MCP/API/RPC：`go test ./internal/dashboard ./internal/mcpserver -run 'TestReadonlyDashboardServesProjectBoard|TestReadonlyMCPProjectBoardTool|TestReadonlyDashboardServesBoundedNoteDisplay|TestReadonlyMCPNoteReadUsesBoundedDisplay' -count=1`；`go test ./internal/api -run 'TestLocalAPIProjectBoardMatchesProjectionEnvelope|TestLocalAPINoteReadAndProjectItemWritePlan|TestLocalRPCProjectBoardNoteAndProjectItemPlan' -count=1`。
- process e2e：`go test ./tests/e2e -run TestProjectBoardWorkspace -count=1`。
- integration evidence：`task test:integration` 通过，最新证据目录 `temp/integration-test-runs/20260608T160040Z-2577515`。
- 全量回归：`go test ./...` 通过；`openspec validate --all` 通过，23 passed, 0 failed；最终 `task check` 通过，包含 fmt-check、lint、test、build 和 OpenSpec validate。
