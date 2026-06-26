# Pinax Project Workspace v2 任务

## 分组说明

- **Lane A: 合同与模型**，定义兼容数据结构、registry 和 path 规则。
- **Lane B: CLI 命令**，实现 subproject、board、item 用户入口。
- **Lane C: API/RPC**，实现 local projection adapter 和 route registry。
- **Lane D: 输出美化与合同测试**，拆成 D0a/D0b/D1a/D1b/D2/D3/D4，分别覆盖 demo fixture、golden 样本、专用 renderer、compact/empty/truncation、`--json`、`--agent`、`--events` 合同。
- **Lane E: E2E、证据和 fixture**，覆盖真实 project management 工作流。
- **Lane F: 文档和最终门禁**，更新命令手册并跑全量验证。

## Lane A: 合同与模型

- [x] **A1. 定义 ProjectWorkspace domain 类型**
  - Owner: `cli/pinax`
  - Scope: 在 `internal/domain` 增加 additive 类型：ProjectWorkspace/Subproject、ProjectWorkspaceRegistry、ProjectWorkspaceTemplate、ProjectWorkspacePathSet，不修改现有 Project 字段语义。
  - Depends on: none
  - Parallel lane: A
  - Acceptance: 新类型支持 project、subproject、title、template、workspace_path、directories、created_at、updated_at、status。
  - Validation command: `go test ./internal/domain -run 'Project|Workspace|Subproject' -count=1`
  - Expected result: domain tests pass, existing project tests不需要改语义。
  - Failure re-check: 如果需要改现有 Project JSON 字段，先改 OpenSpec 记录 deprecation；默认只 additive。

- [x] **A2. 增加 workspace path 规则和 reserved directory 校验**
  - Owner: `cli/pinax`
  - Scope: 在 app/service 或 capability 包中实现 `notes/projects/<project>/<subproject>/` 默认路径、slug 校验、reserved directory 拒绝和 path traversal 防护。
  - Depends on: A1
  - Parallel lane: A
  - Acceptance: 拒绝空 slug、`..`、绝对路径、`.pinax`、`.git`、`temp`、`dist`、`node_modules`、`vendor`。
  - Validation command: `go test ./internal/app -run 'ProjectWorkspace|Subproject|Path|Reserved' -count=1`
  - Expected result: 合法 slug 通过，危险路径稳定失败。
  - Failure re-check: 不允许 command 层单独拼路径，边界校验必须在 app service 可复用。

- [x] **A3. 增加 CLI-authored workspace registry**
  - Owner: `cli/pinax`
  - Scope: 通过 app service 读写 `.pinax/project-workspaces/<project>/<subproject>.json`，记录 schema、project、subproject、template、directories、status、evidence refs。
  - Depends on: A1, A2
  - Parallel lane: A
  - Acceptance: registry 只由 CLI/service 创建修改；`vault validate` 能检查 schema、project ref、path boundary、redaction。
  - Validation command: `go test ./internal/app ./cmd/pinax -run 'ProjectWorkspace|VaultValidate' -count=1`
  - Expected result: create/show/list/validate 的结构化资产测试通过。
  - Failure re-check: agent 不得手写 registry；测试 fixture 可准备输入，但 official flow 必须走命令。

- [x] **A4. 扩展 NoteDisplay/ProjectItem optional 字段**
  - Owner: `cli/pinax`
  - Scope: 在不破坏现有字段的前提下，为 board item/note display 增加 optional `subproject`、`labels`、`milestone`、`priority`、`due_at`、`blocked_by`、`workspace_path`。
  - Depends on: A1
  - Parallel lane: A
  - Acceptance: 旧 JSON consumers 仍可读取原字段，新字段为空时省略或为空值。
  - Validation command: `go test ./cmd/pinax ./internal/output -run 'ProjectBoard|NoteDisplay|Compatibility' -count=1`
  - Expected result: 现有 project board contract tests 不破坏。
  - Failure re-check: 不允许重命名现有 `project`、`board_column`、`status`、`tags` 字段。

## Lane B: CLI 命令

- [x] **B1. 实现 `project subproject create/list/show`**
  - Owner: `cli/pinax`
  - Scope: 在 `internal/cli` 和 `cmd/pinax` 入口添加 `pinax project subproject create|list|show`，调用 app service，不直接写文件。
  - Depends on: A2, A3
  - Parallel lane: B
  - Acceptance: create 创建标准目录和 registry；list/show 返回 projection；默认不需要 TaskBridge、Cloud 或 Git remote。
  - Validation command: `go test ./cmd/pinax -run 'ProjectSubproject' -count=1`
  - Expected result: CLI tests 覆盖 success、duplicate、missing project、unsafe slug。
  - Failure re-check: 如果父 project 不存在，返回 `project_not_found` 和 `pinax project list --vault <vault>` next action。

- [x] **B2. 扩展 `project board configure/show --subproject`**
  - Owner: `cli/pinax`
  - Scope: 为现有 board 命令新增 optional `--subproject`，读取 subproject scoped config，缺省时保持 project-wide 行为。
  - Depends on: A3, A4
  - Parallel lane: B
  - Acceptance: `pinax project board show research --vault <vault>` 行为不变；`--subproject stock-learning` 只显示 scoped items。
  - Validation command: `go test ./cmd/pinax ./internal/app -run 'ProjectBoard|Subproject' -count=1`
  - Expected result: project-wide 和 subproject board tests 都通过。
  - Failure re-check: 不允许把旧 board config 自动迁移成 subproject config。

- [x] **B3. 扩展 `project item add` 字段**
  - Owner: `cli/pinax`
  - Scope: 为 `pinax project item add` 增加 optional `--subproject`、`--labels`、`--milestone`、`--priority`、`--due-at`、`--blocked-by`。
  - Depends on: A4, B2
  - Parallel lane: B
  - Acceptance: item 写入 Markdown/frontmatter 或 managed block；输出包含 item_id、project、subproject、column、labels、next action。
  - Validation command: `go test ./cmd/pinax ./internal/app -run 'ProjectItemAdd|Subproject|Labels|Milestone' -count=1`
  - Expected result: add item 成功且 board show 能读回。
  - Failure re-check: `--labels` 等字段解析失败时返回稳定错误，不写半成品 note。

- [x] **B4. 扩展 `project item move/archive` 安全门禁**
  - Owner: `cli/pinax`
  - Scope: 支持 subproject item move/archive，保持 unmanaged checklist 拒绝，高风险 archive/batch 继续要求 `--yes` 和 snapshot gate。
  - Depends on: B3
  - Parallel lane: B
  - Acceptance: managed item 可移动；unmanaged inferred item 返回 `project_item_unmanaged`；archive 无 `--yes` 返回 `approval_required`。
  - Validation command: `go test ./cmd/pinax ./internal/app -run 'ProjectItemMove|Archive|Approval|Snapshot' -count=1`
  - Expected result: 安全门禁测试通过。
  - Failure re-check: 不允许直接改任意 Markdown checklist 行作为 move 实现。

- [x] **B5. 增加项目管理补全和 help 文案**
  - Owner: `cli/pinax`
  - Scope: 为 project/subproject/board/item 新 flags 增加 help、completion 和命令示例，长 flag 优先。
  - Depends on: B1, B2, B3
  - Parallel lane: B
  - Acceptance: help 展示真实命令，短 flag 不抢占小写命名空间。
  - Validation command: `go test ./cmd/pinax -run 'Completion|Help|Project' -count=1`
  - Expected result: help/completion tests 通过。
  - Failure re-check: 不在 help 中展示不存在的 GitHub/Gitea sync 能力。

## Lane C: API/RPC

- [x] **C1. 扩展 API route registry**
  - Owner: `cli/pinax`
  - Scope: 注册 project/subproject/board/item read routes 和 controlled write plan routes，包含 route_id、method、path、command、capability_id、schema_version、readonly、body_allowed、approval_required、snapshot_required、errors。
  - Depends on: A3, A4
  - Parallel lane: C
  - Acceptance: `pinax api routes --vault <vault> --json` 能列出新增 routes，旧 routes 仍存在。
  - Validation command: `go test ./internal/httpapi ./cmd/pinax -run 'ApiRoutes|Project|Subproject|Board' -count=1`
  - Expected result: route registry contract tests 通过。
  - Failure re-check: 不允许 REST handler 与 OpenAPI schema 各维护一份不一致 route 表。

- [x] **C2. 实现 readonly REST project workspace endpoints**
  - Owner: `cli/pinax`
  - Scope: 增加 `GET /v1/projects`、`GET /v1/projects/{project}`、`GET /v1/projects/{project}/subprojects`、`GET /v1/projects/{project}/subprojects/{subproject}`、`GET /v1/project-items/{item_id}`。
  - Depends on: C1, B1, B3
  - Parallel lane: C
  - Acceptance: handler 只做参数解析和 projection serialization，业务走 app service。
  - Validation command: `go test ./internal/httpapi -run 'ProjectWorkspace|Subproject|ProjectItem' -count=1`
  - Expected result: REST read endpoints 返回 projection envelope。
  - Failure re-check: handler 不得直接读 Markdown、`.pinax`、GORM 或 Git。

- [x] **C3. 扩展 board REST endpoint 支持 optional subproject**
  - Owner: `cli/pinax`
  - Scope: 让 `GET /v1/projects/{project}/board?subproject=<slug>&note_display=card` 复用 CLI board projection。
  - Depends on: C1, B2
  - Parallel lane: C
  - Acceptance: 不带 subproject 行为与现有 endpoint 一致；带 subproject 返回 scoped board。
  - Validation command: `go test ./internal/httpapi ./internal/dashboard -run 'ProjectBoard|Subproject' -count=1`
  - Expected result: REST/dashboard board tests 通过且不泄漏 body。
  - Failure re-check: 不允许把 `subproject` 作为必填参数。

- [x] **C4. 实现 controlled write plan REST endpoints**
  - Owner: `cli/pinax`
  - Scope: 为 create subproject、add/move/archive item 增加 POST endpoints，默认 readonly server 返回 `write_disabled`；`--allow-write` 后仍需要 `yes=true`。
  - Depends on: C1, B1, B3, B4
  - Parallel lane: C
  - Acceptance: write route 走 app service；高风险动作无 snapshot 返回 `snapshot_required`。
  - Validation command: `go test ./internal/httpapi -run 'ProjectWrite|AllowWrite|Approval|Snapshot' -count=1`
  - Expected result: write gates contract tests 通过。
  - Failure re-check: 不允许 API 直接执行未确认写入。

- [x] **C5. 增加 RPC 方法和 remote API mode 支持**
  - Owner: `cli/pinax`
  - Scope: 增加 `Pinax.Project.Subproject.List/Show/CreatePlan`、`Pinax.ProjectBoard.Show` optional subproject、`Pinax.ProjectItem.AddPlan/MovePlan/ArchivePlan`，并让 supported CLI remote mode 可转发 read paths。
  - Depends on: C1, C2, C3
  - Parallel lane: C
  - Acceptance: RPC 返回与 CLI/REST 同 schema projection；unsupported write 在 remote mode 不会静默 fallback 本地。
  - Validation command: `go test ./internal/httpapi ./cmd/pinax -run 'RPC|RemoteAPI|Project' -count=1`
  - Expected result: RPC/remote mode tests 通过。
  - Failure re-check: 不允许 RPC invent 单独 response shape。

## Lane D: 输出美化与合同测试

- [x] **D0a. 建立 CLI 看板 demo fixture 数据**
  - Owner: `cli/pinax`
  - Scope: 增加固定 demo 数据，覆盖 project `research`、subproject `stock-learning`、workspace path `notes/projects/research/stock-learning`、7 个标准目录、6 个 columns、至少 8 个 items、blocked/review/priority/due/labels/milestone/blocked_by 字段。
  - Depends on: B2, B3
  - Parallel lane: D
  - Acceptance: demo 使用真实 `pinax project ...` 命令创建，不依赖用户真实 `yeisme-notes`，不手写 `.pinax` structured assets；普通 note 正文 fixture 不包含 token、provider payload 或 raw prompt。
  - Validation command: `go test ./cmd/pinax ./tests/e2e -run 'ProjectBoardDemo|ProjectWorkspaceDemo' -count=1`
  - Expected result: demo fixture 能稳定生成 project/subproject/workspace/board/items projection。
  - Failure re-check: 如果 demo 需要测试 setup 直接写 Markdown，可以只写普通 note 正文；registry、board config、events 必须由 CLI/service 生成。

- [x] **D0b. 固定 CLI 看板 golden 输出样本**
  - Owner: `cli/pinax`
  - Scope: 为 demo fixture 固定默认 human、`--compact`、empty board、`--json`、`--agent`、`--events` golden expectations，包含 `Project:`、`Path:`、`Structure:`、`Board:`、分栏、risks 和 next action。
  - Depends on: D0a
  - Parallel lane: D
  - Acceptance: golden 样本使用真实用户可运行命令生成；默认输出不是 JSON dump；空看板有明确 empty state 和 add-item next action。
  - Validation command: `go test ./cmd/pinax ./tests/e2e -run 'ProjectBoardDemoGolden|ProjectWorkspaceDemoGolden' -count=1`
  - Expected result: 所有 demo output golden tests 通过，失败时 diff 能指出缺失 section 或字段。
  - Failure re-check: 不允许靠放宽 golden 规避结构缺失；先修 projection 或 renderer。

- [x] **D1a. 增加 `project.board.show` 专用 human summary renderer**
  - Owner: `cli/pinax`
  - Scope: 在 `internal/output` 为 `project.board.show` 增加专用 summary renderer，从 shared projection 读取 `data.board` 和 `data.workspace`，渲染分栏摘要；command 层只调用 service 和 output renderer。
  - Depends on: D0b
  - Parallel lane: D
  - Acceptance: 默认输出展示 `Project:`、`Path:`、`Structure:`、`Board:`、milestone/priority 汇总、Inbox/Next/Doing/Blocked/Review 分组、Risks、Recommended next step；`Done` 默认只展示计数。
  - Validation command: `go test ./cmd/pinax ./internal/output -run 'ProjectBoardHuman|Summary' -count=1`
  - Expected result: golden/default output tests 通过。
  - Failure re-check: scripts 不应解析 human summary；机器合同必须在 JSON/agent。

- [x] **D1b. 固定 board summary 截断、compact 和空状态规则**
  - Owner: `cli/pinax`
  - Scope: 增加 `--compact` 入口和 renderer 规则；每个展开列最多显示 5 条 item，超出显示 `... N more, use --json for full list`；空 board 显示 `No project items yet.` 和 add-item next action。
  - Depends on: D1a
  - Parallel lane: D
  - Acceptance: compact 输出包含 project/subproject、path、board counts、top item、risk counts、one next command；默认输出在长列下不无限增长。
  - Validation command: `go test ./cmd/pinax ./internal/output -run 'ProjectBoardCompact|ProjectBoardEmpty|ProjectBoardTruncate' -count=1`
  - Expected result: compact、empty、truncation tests 全部通过且无 ANSI 泄漏到机器模式。
  - Failure re-check: 不新增多套 `--layout` 系统；v1 只支持默认分栏和 `--compact`。

- [x] **D2. 固定 `--json` envelope 和 data schema**
  - Owner: `cli/pinax`
  - Scope: 为 subproject、board、item 输出增加 JSON contract tests，断言顶层 envelope 和 demo 中的 `data.workspace`、`data.board`、`data.project`、`data.subproject`、`data.columns`、`data.items`、`actions`；所有新增字段 optional additive。
  - Depends on: D0b
  - Parallel lane: D
  - Acceptance: stdout 只含一个 JSON object；错误也是 failed envelope；`data.workspace` 是 optional additive 字段，包含相对 vault path 和 bounded directory status；JSON 不含 Authorization、Bearer、api_key、secret、raw prompt、provider payload、hidden prompt 或 full body sentinel。
  - Validation command: `go test ./cmd/pinax -run 'ProjectWorkspaceJSON|ProjectBoardJSON|ProjectItemJSON' -count=1`
  - Expected result: JSON contract tests 通过。
  - Failure re-check: 不允许在 command 层手拼和 renderer 不一致的 JSON。

- [x] **D3. 固定 `--agent` key=value 合同**
  - Owner: `cli/pinax`
  - Scope: 为 subproject、board、item 输出增加 agent contract tests，稳定 keys：`fact.project`、`fact.subproject`、`fact.workspace_path`、`fact.column.<name>`、`fact.items.total`、`fact.item.top.*`、`fact.risk.*`、`action.<name>`。
  - Depends on: D0b
  - Parallel lane: D
  - Acceptance: agent stdout 无中文 prose、ANSI、raw body、provider payload 或 secret sentinel；每个 `fact.*` key 都来自 projection facts/data，不解析 human summary。
  - Validation command: `go test ./cmd/pinax -run 'ProjectWorkspaceAgent|ProjectBoardAgent|ProjectItemAgent' -count=1`
  - Expected result: 每行都是可解析 key=value。
  - Failure re-check: 重命名 agent key 视为 breaking；只能新增 optional key。

- [x] **D4. 增加 `--events` 生命周期事件**
  - Owner: `cli/pinax`
  - Scope: 为 create/list/show/add/move/archive 增加 start/end/error NDJSON event tests，包含 spec_version、mode、seq、command、project、subproject、status，并为 board show 增加来自 projection 的 `board.summary` demo event。
  - Depends on: D0b, B4
  - Parallel lane: D
  - Acceptance: stdout 是 NDJSON；diagnostics 在 stderr；`board.summary` 包含 project、subproject、workspace_path、items、blocked、review；counts 与 `--json` projection facts 一致；事件不含 note body、token、Authorization、Bearer、api_key、secret、raw prompt 或 provider payload。
  - Validation command: `go test ./cmd/pinax -run 'ProjectWorkspaceEvents|ProjectBoardEvents' -count=1`
  - Expected result: events contract tests 通过。
  - Failure re-check: 不允许 progress/log 混入 events stdout。

## Lane E: E2E、证据和 fixture

- [x] **E1. 增加项目管理 fixture vault**
  - Owner: `cli/pinax`
  - Scope: 创建 testscript fixture，包含 project `research`、subproject `stock-learning`、章程、sources、runs、outputs、retros、tool-candidates 和多列 item。
  - Depends on: B1, B2, B3
  - Parallel lane: E
  - Acceptance: fixture 不含真实用户路径、token、provider payload；issue/item 数据 deterministic。
  - Validation command: `go test ./tests/e2e -run 'ProjectWorkspaceFixture' -count=1`
  - Expected result: fixture tests 通过。
  - Failure re-check: 不允许测试依赖默认用户 vault `yeisme-notes`。

- [x] **E2. 增加完整 CLI 工作流 e2e**
  - Owner: `cli/pinax`
  - Scope: testscript 覆盖 `project create`、`subproject create`、`note add --subproject`、`board configure`、`item add`、`item move`、`board show`。
  - Depends on: E1, D2, D3
  - Parallel lane: E
  - Acceptance: 默认、`--json`、`--agent` 输出均可验证；写入只在显式命令发生。
  - Validation command: `go test ./tests/e2e -run 'ProjectWorkspaceWorkflow' -count=1`
  - Expected result: CLI e2e 通过。
  - Failure re-check: 若命令顺序依赖 index，必须显式调用 `pinax index refresh --vault <vault>` 或在 projection 报告 stale。

- [x] **E3. 增加 API/RPC e2e**
  - Owner: `cli/pinax`
  - Scope: 启动 `pinax api serve --vault <vault> --port 0 --no-auth`，验证 read endpoints、route discovery、OpenAPI export、readonly write rejection。
  - Depends on: C2, C3, C4, C5
  - Parallel lane: E
  - Acceptance: API 返回 projection envelope；readonly server write 返回 `write_disabled`。
  - Validation command: `go test ./tests/e2e ./internal/httpapi -run 'ProjectWorkspaceAPI|ProjectBoardAPI' -count=1`
  - Expected result: API/RPC e2e 通过。
  - Failure re-check: 不允许测试依赖固定端口。

- [x] **E4. 接入 integration evidence**
  - Owner: `cli/pinax`
  - Scope: 确保 `task test:integration` 包含 project workspace workflow，写入 `temp/integration-test-runs/<run-id>/` 标准证据。
  - Depends on: E2, E3, D2, D3, D4
  - Parallel lane: E
  - Acceptance: evidence 包含 `summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json`、`artifacts/`，并 redacted。
  - Validation command: `task test:integration`
  - Expected result: integration 通过且 latest evidence summary `project=cli/pinax`、`redaction.applied=true`。
  - Failure re-check: 失败路径也必须保留 evidence 和原始 exit code。

## Lane F: 文档和最终门禁

- [x] **F1. 更新 project 命令文档**
  - Owner: `cli/pinax`
  - Scope: 更新 `docs/commands/project.md`，说明 project、subproject、board、item、labels、milestone、API 边界和不适用场景。
  - Depends on: B1, B2, B3, C1
  - Parallel lane: F
  - Acceptance: 文档使用真实命令，保持 CLI help/output 示例英文稳定，说明文字中文优先。
  - Validation command: `rg -n 'subproject|project board|project item|pinax project' docs/commands/project.md`
  - Expected result: 文档覆盖新工作流。
  - Failure re-check: 不写 GitHub/Gitea sync 承诺。

- [x] **F2. 更新 Remote API contract 文档**
  - Owner: `cli/pinax`
  - Scope: 更新 `docs/interfaces/remote-api-contract.md`，记录新增 REST/RPC routes、write gates、route registry 和 OpenAPI schema 要求。
  - Depends on: C1, C2, C3, C4, C5
  - Parallel lane: F
  - Acceptance: 文档和 `pinax api routes --json` 输出一致。
  - Validation command: `go test ./internal/httpapi -run 'ApiSchema|Routes|Project' -count=1`
  - Expected result: API schema/route tests 通过。
  - Failure re-check: 不维护第二套手写 schema。

- [x] **F3. 更新 README/commands 索引**
  - Owner: `cli/pinax`
  - Scope: 在 README 或 commands index 中把 Project Workspace 作为本地工作流入口，保持 Proof Loop 主线优先。
  - Depends on: F1
  - Parallel lane: F
  - Acceptance: Project Workspace 被介绍为高级本地项目管理能力，不抢首次用户 proof loop 首屏。
  - Validation command: `rg -n 'Project Workspace|project workspace|pinax project' README.md README.zh-CN.md docs/commands/README.md`
  - Expected result: 文档入口可发现。
  - Failure re-check: 不把它描述成远端 issue tracker。

- [x] **F4. OpenSpec 和 focused gates**
  - Owner: `cli/pinax`
  - Scope: 运行 change strict validate、全量 OpenSpec 和 focused tests。
  - Depends on: all A-E tasks
  - Parallel lane: sequential
  - Acceptance: OpenSpec 和 focused tests 通过。
  - Validation command: `openspec validate pinax-project-workspace-hierarchy-api-cli-polish --strict && openspec validate --all --strict && go test ./cmd/pinax ./internal/app ./internal/httpapi ./internal/output ./tests/e2e -run 'Project|Board|Subproject|Workspace|API|Agent|JSON|Events' -count=1`
  - Expected result: 命令 exit 0。
  - Failure re-check: 先修源头，不删合同断言。

- [x] **F5. 全量门禁和归档准备**
  - Owner: `cli/pinax`
  - Scope: 运行 Pinax 标准门禁，记录 integration evidence，确认可归档。
  - Depends on: F4
  - Parallel lane: sequential
  - Acceptance: `task check` 和 `task test:integration` 通过，tasks 全部完成。
  - Validation command: `task check && task test:integration && find temp/integration-test-runs -maxdepth 2 -type f | sort | tail -50`
  - Expected result: 全量通过且 evidence 完整。
  - Failure re-check: 如果 active changes 之间有冲突，先合并/拆分 owner，不在归档时隐瞒 skipped tasks。

## 验证记录

- 2026-06-24：RED `go test ./internal/api -run 'TestLocalAPIProjectBoardMatchesProjectionEnvelope|TestLocalAPINoteReadAndProjectItemWritePlan' -count=1`，失败于 `/v1/projects` redirect 和 `GET /v1/project-items/{item}` 405，证明缺少只读 REST paths。
- 2026-06-24：GREEN `go test ./internal/api -run 'TestLocalAPIProjectBoardMatchesProjectionEnvelope|TestLocalAPINoteReadAndProjectItemWritePlan|TestLocalRESTMethodAndRouteErrorsUseProjectionEnvelope' -count=1` 通过。
- 2026-06-24：`go test ./internal/app -run 'TestRemoteCapabilitiesExposeProjectionCommandsAndGates|TestAPISchemaExportUsesRegisteredRESTMethods|TestAPISchemaExportMatchesRemoteRouteRegistry' -count=1` 通过。
- 2026-06-24：`go test ./cmd/pinax ./internal/app ./internal/api ./internal/cli ./internal/output ./tests/e2e -run 'Project|Board|Subproject|Workspace|API|Agent|JSON|Events|TestProjectBoardWorkspace' -count=1` 通过。
- 2026-06-24：`task test:integration` 通过，生成 `temp/integration-test-runs/20260624T085054Z-2600598/`，包含 `summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json` 和 `artifacts/`；`summary.json` 记录 `project=cli/pinax`、`exit_code=0`、`redaction.applied=true`。
- 2026-06-24：`task check` 通过，覆盖 `golangci-lint fmt --diff`、`golangci-lint run`、`go test ./...`、`go build`、`openspec validate --all` 和 LanceDB sidecar protocol。
