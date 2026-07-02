# 任务

## 0. 全局约束

- Owner: `cli/pinax`。
- 合同策略: 只做 additive change；不得删除、重命名或重定义现有命令、flag、JSON envelope 顶层字段、`--agent` key、API route、RPC method、`.pinax/**` registry key 或 index schema。
- 写入边界: Markdown 正文可以由用户编辑；`.pinax/**` structured assets、events、receipts、sync state、database schema/view registry、task adoption ledger 必须由 CLI/application service 写入。
- 复杂逻辑注释: 新增或修改 parser、state transition、path boundary、task adoption、managed block rewrite、protocol adapter、query planner、redaction 和非显然 fixture 时写中文注释说明不变量。
- 集成证据: 新增或扩展 integration/component/e2e 入口时，证据写入 `temp/integration-test-runs/<run-id>/`，至少包含 `summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json` 和 `artifacts/`，并脱敏 token、Authorization、raw prompt、provider payload、hidden system prompt、private tool arguments 和完整 chain-of-thought。
- 完成门禁: 每个阶段运行 focused tests；收口运行 `task check`、`task test:integration`、`openspec validate pinax-unified-vault-workspace-database --strict`、`openspec validate --all --strict`。

## P0: 收口现有工作面

- [x] **0.1 审计当前 active OpenSpec 与未提交改动**
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: none
  - Scope: 对齐当前 `pinax-client-cli-parity-realtime-sync`、已完成但未 archive 的 changes、dirty worktree 和已有 project/database/graph/task 代码；不修改代码。
  - Files: `openspec/changes/pinax-client-cli-parity-realtime-sync/*`、`openspec/specs/project-board-workspace/spec.md`、`openspec/specs/pinax-dataview-database/spec.md`、`git status --short` 输出。
  - Acceptance: 形成实现前依赖清单，明确本变更哪些任务依赖现有 change 完成、哪些可以并行。
  - Validation command: `openspec list && git status --short && openspec validate pinax-client-cli-parity-realtime-sync --strict`
  - Expected result: OpenSpec validation 通过；若存在 dirty files，记录哪些与本计划相关，不回滚用户改动。
  - Failure re-check: 如果 active change validation 失败，先修 active change 或把本计划依赖标记为 blocked，不要在新任务中重复实现同一合同。
  - Evidence: 2026-06-25 运行 `openspec validate pinax-client-cli-parity-realtime-sync --strict`，退出码 0；运行 `git diff --stat` 与 `git diff --name-only`，确认 dirty worktree 覆盖 remote parity、workspace/project board、sync daemon、skills/docs 和测试文件。本变更可并行推进 0.2 覆盖基线；后续 remote capability 补齐仍依赖 `pinax-client-cli-parity-realtime-sync` 的任务 3-7。

- [x] **0.2 生成 CLI tree 与能力覆盖基线**
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: 0.1
  - Scope: 在现有 remote parity 任务基础上输出命令覆盖矩阵，按 `remote_supported`、`local_only`、`unsupported` 分类 project/task/database/graph/publish/plugin/sync 相关命令。
  - Files: `internal/app/remote.go`、`internal/api/rpc.go`、`internal/api/http.go`、`internal/cli/root.go`、`cmd/pinax/*_test.go`。
  - Acceptance: 每个用户可见命令都有分类；`local_only` 命令有原因；未注册 remote command 不会 fallback 本地执行。
  - Validation command: `go test ./internal/app ./internal/api ./internal/cli ./cmd/pinax -run 'Remote|Capability|CommandParity|Route' -count=1`
  - Expected result: 覆盖矩阵测试通过，输出不含 secret、note body 或 provider payload。
  - Failure re-check: 如果分类需要新增字段，只能新增 optional 字段；不得改变现有 `remote_command_unsupported` 语义。
  - Evidence: 2026-06-25 新增 `RemoteCommandCoverage` 覆盖矩阵和 `TestRemoteCommandCoverageClassifiesEveryVisibleRunnableCommand`，遍历 Cobra 可见 runnable commands 并分类为 `remote_supported`、`local_only` 或 `unsupported`；`local_only`/`unsupported` 均带 reason，`remote_supported` 带 RPC method。先运行 `go test ./internal/cli -run TestRemoteCommandCoverageClassifiesEveryVisibleRunnableCommand -count=1` 观察缺 API 的 RED 失败，再补实现并通过；运行 `go test ./internal/app ./internal/api ./internal/cli ./cmd/pinax -run 'Remote|Capability|CommandParity|Route' -count=1`，退出码 0。

## P1: 统一 Vault Workspace 模型

- [x] **1.1 定义 workspace aggregate 与路径边界**
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 0.1
  - Scope: 定义 `VaultWorkspace`、`WorkspaceProjectRef`、`WorkspaceSubprojectRef`、`WorkspaceCollectionRef`、`WorkspaceViewRef` 等 domain 类型；集中实现 vault-relative path、reserved directory、slug 和 path traversal 校验。
  - Files: `internal/domain/types.go`、`internal/domain/project_board.go`、`internal/app/project_workspace.go`、`internal/app/service.go`、`internal/app/project_workspace_test.go`。
  - Acceptance: 合法 project/subproject/collection/view ref 可序列化；拒绝空 slug、`..`、绝对路径、`.pinax`、`.git`、`temp`、`dist`、`node_modules`、`vendor` 和 vault 外路径。
  - Validation command: `go test ./internal/domain ./internal/app -run 'Workspace|ProjectRef|Subproject|Collection|Path|Reserved' -count=1`
  - Expected result: domain/app tests 通过，旧 project board tests 不需要改语义。
  - Failure re-check: 如果需要改旧 project 字段，先补 deprecation/compatibility plan；默认只新增类型和 optional 字段。
  - Evidence: 2026-06-25 检查 `internal/domain/project_board.go`、`internal/app/project_workspace.go`、`internal/app/project_board.go` 与相关 tests，确认已有 `ProjectWorkspace` aggregate、workspace path、subproject registry ref 和 project board workspace 投影；`validateSubprojectSlug` 拒绝空/非法 slug 与 `temp`、`dist`、`node_modules`、`vendor`，workspace path 通过 `validateProjectPrefix` 保持 vault-relative。运行 `go test ./internal/domain ./internal/app -run 'Workspace|ProjectRef|Subproject|Collection|Path|Reserved' -count=1`，退出码 0。

- [x] **1.2 实现 CLI-authored workspace registry**
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 1.1
  - Scope: 通过 application service 读写 `.pinax/workspaces/current.json`、`.pinax/project-workspaces/<project>/<subproject>.json` 和 workspace event；`vault validate` 校验 schema、path、project ref、redaction。
  - Files: `internal/app/project_workspace.go`、`internal/app/service.go`、`internal/domain/types.go`、`internal/cli/vault_cmd.go`、`cmd/pinax/vault_project_storage_command_test.go`。
  - Acceptance: `pinax project subproject create` 或后续 workspace 命令创建 registry；agent 不需要手写 `.pinax/**`；invalid registry 返回稳定错误码。
  - Validation command: `go test ./internal/app ./cmd/pinax -run 'WorkspaceRegistry|ProjectWorkspace|VaultValidate|Redaction' -count=1`
  - Expected result: registry 创建、读取、validate 和 redaction tests 通过。
  - Failure re-check: 如果测试 fixture 必须准备异常 registry，可手写 fixture 输入；official write path 必须走 CLI/service。
  - Evidence: 2026-06-25 在 `TestProjectSubprojectWorkspaceCLI` 先补 RED 断言，要求 `pinax project subproject create` 写入 `.pinax/workspaces/current.json`，首次运行 `go test ./cmd/pinax -run TestProjectSubprojectWorkspaceCLI -count=1` 因文件不存在失败；随后新增 `domain.CurrentWorkspace` 与 `saveCurrentWorkspace`，通过 app service/`writeJSONAsset` 同步写 `.pinax/project-workspaces/<project>/<subproject>.json` 和 `.pinax/workspaces/current.json`。重跑 `go test ./cmd/pinax -run TestProjectSubprojectWorkspaceCLI -count=1`，退出码 0；运行 `go test ./internal/app ./cmd/pinax -run 'WorkspaceRegistry|ProjectWorkspace|VaultValidate|Redaction' -count=1`，退出码 0。

- [x] **1.3 增加 workspace projection 与输出合同**
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 1.2
  - Scope: 在 shared projection 中新增 optional `data.workspace`、`facts.workspace.*`、`fact.workspace.*`；CLI human/json/agent/events/explain 复用同一 projection。
  - Files: `internal/output/render.go`、`internal/domain/types.go`、`cmd/pinax/cli_output_contract_test.go`、`cmd/pinax/vault_project_storage_command_test.go`。
  - Acceptance: `--json` stdout 是单个 envelope；`--agent` 是 low-token key=value；默认 human 输出不暴露 body；events 是 NDJSON。
  - Validation command: `go test ./cmd/pinax ./internal/output -run 'WorkspaceOutput|CLIOutput|Agent|Events|JSON' -count=1`
  - Expected result: 输出合同 tests 通过，旧 envelope 顶层字段不变。
  - Failure re-check: 重命名 `fact.project`、`fact.subproject` 或旧 board key 属于 breaking，必须改为新增 optional key。
  - Evidence: 2026-06-25 在 `TestProjectSubprojectWorkspaceCLI` 先补 RED 断言，要求 `project subproject show --agent` 输出 `fact.workspace.project`、`fact.workspace.subproject`、`fact.workspace.path`，首次运行 `go test ./cmd/pinax -run TestProjectSubprojectWorkspaceCLI -count=1` 因缺 `fact.workspace.*` 失败；随后在 `projectWorkspaceProjection` 中新增 optional workspace facts，保留旧 `project`、`subproject`、`workspace_path` key。重跑 `go test ./cmd/pinax -run TestProjectSubprojectWorkspaceCLI -count=1`，退出码 0；运行 `go test ./cmd/pinax ./internal/output -run 'WorkspaceOutput|CLIOutput|Agent|Events|JSON' -count=1`，退出码 0。

- [x] **1.4 补齐 Project Manager 子项目路径注释与语义目录**
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 1.3
  - Scope: Project Manager/OD/dashboard/client 对子项目创建、预览、详情和空状态展示 vault-local 路径语义；假设 vault 为 `~/data/yeisme-notes` 时，子项目 full path 必须落在 `~/data/yeisme-notes/` 下，等于 `vault_root + workspace_path`。新建子项目默认目录使用 `charter`、`inbox`、`sources`、`runs`、`outputs`、`retros`、`tool-candidates`，不得默认创建 `00-`、`10-` 等数字前缀目录。该任务只解释和投影 vault-local workspace，不创建 Yeisme monorepo subproject、Git submodule、独立 remote、`AGENTS.md`、`CLAUDE.md` 或开发工具链。
  - Files: `internal/app/project_workspace.go`、`internal/domain/types.go`、`internal/output/render.go`、`cmd/pinax/vault_project_storage_command_test.go`、`docs/commands/project.md`、`openspec/changes/pinax-unified-vault-workspace-database/design.md`、`openspec/changes/pinax-unified-vault-workspace-database/specs/project-board-workspace/spec.md`。
  - Acceptance: `project subproject create/show --json` 和 `--agent` 保留旧 `workspace_path`，并新增 optional `vault_root`、`workspace.full_path` 或等价 bounded facts；OD/文档展示 `Vault root`、`Workspace path`、`Full path preview` 注释；`.pinax/project-workspaces/<project>/<subproject>.json` 被解释为 registry metadata，用户内容在 `workspace_path` 指向的 vault 目录中；新建 workspace 和 learning pack 都写语义目录，旧数字目录不被删除或强制迁移。
  - Validation command: `go test ./cmd/pinax ./internal/app ./internal/output ./tests/e2e -run 'ProjectSubproject|WorkspacePath|ProjectManager|ProjectLearning|ProjectBoard|CLIOutput|Agent|JSON' -count=1`
  - Expected result: Project Manager 子项目路径注释和语义目录 tests 通过；旧 `fact.workspace.path`、`workspace_path` 和 registry 可读性不变；输出与 docs 不再推荐 `00-`/`10-` 默认目录。
  - Failure re-check: 如果 full path 暴露涉及用户 home path，machine 输出必须保持 bounded；不得把 full path 写入公开 fixture 中的真实用户名或泄露 provider/config path；不得把旧数字目录当成唯一支持结构。
  - Evidence: 2026-06-26 在 `TestProjectSubprojectWorkspaceCLI` 先补 RED 断言，要求 `project subproject create/show --json` 和 `--agent` 暴露 `vault_root` 与 `workspace.full_path`，human summary 显示 `Vault root`、`Workspace path`、`Full path preview`，同时确认 `.pinax/project-workspaces/<project>/<subproject>.json` 不持久化绝对路径。首轮 `go test ./cmd/pinax -run TestProjectSubprojectWorkspaceCLI -count=1` 因缺路径 facts 失败；随后在 `projectWorkspaceProjection` 中增加 projection-only 路径 facts/data，并在 summary renderer 中补人类可读标签。重跑 `go test ./cmd/pinax -run TestProjectSubprojectWorkspaceCLI -count=1` 通过；运行 `go test ./cmd/pinax ./internal/app ./internal/output ./tests/e2e -run 'ProjectSubproject|WorkspacePath|ProjectManager|ProjectLearning|ProjectBoard|CLIOutput|Agent|JSON' -count=1`，退出码 0。

## P2: Todo Kanban 与 Daily Review

- [x] **2.1 建立 task source 分类和 adoption plan**
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 1.1
  - Scope: 将 task 分为 `managed`、`adopted`、`inferred`；新增 adopt plan/apply service，生成 task adoption ledger，不直接修改未确认 checklist。
  - Files: `internal/domain/project_board.go`、`internal/app/project_board.go`、`internal/app/taskbridge_planning.go`、`internal/index/query_sources.go`、`internal/app/project_board_test.go`。
  - Acceptance: inferred checklist 只读；`pinax task adopt --plan` 不写入；`pinax task adopt --yes` 才生成 managed task metadata；adoption evidence 可审计。
  - Validation command: `go test ./internal/app ./internal/index -run 'TaskSource|Checklist|Adopt|Managed|Inferred' -count=1`
  - Expected result: task source tests 通过，未 adopt checklist move/archive 返回 `task_unmanaged` 或 `project_item_unmanaged`。
  - Failure re-check: 不允许用字符串替换直接改任意 Markdown checklist 行作为 adoption apply。
  - Evidence: 2026-06-25 在 `TestProjectBoardInferredChecklistTasksRequireAdoption` 先补 RED，确认普通 Markdown checklist 进入 board 时是 `source_kind=inline_task`、`source_status=inferred`、`writable=false`，对 inferred task 执行 `ProjectItemMove` 返回 `task_unmanaged`，`TaskAdopt` plan 不写 ledger，apply 才写 `.pinax/task-adoptions/<task_id>.json`。首次运行 `go test ./internal/app ./internal/index -run 'TaskSource|Checklist|Adopt|Managed|Inferred' -count=1` 因缺 `SourceStatus`/`TaskAdopt` 失败；补 `BoardItem.source_status`、`TaskAdoption`、checklist source scan、task adoption service 和 ledger 后通过。随后新增 `pinax task adopt <item> --plan|--yes` CLI，先运行 `go test ./cmd/pinax -run TestTaskAdoptCLIPlansAndWritesLedger -count=1` 观察 `unknown command "task"` RED 失败，再接入 `internal/cli/task_cmd.go` 并通过。最终运行 `go test ./internal/app ./internal/index -run 'TaskSource|Checklist|Adopt|Managed|Inferred' -count=1`、`go test ./cmd/pinax ./internal/cli -run 'TaskAdopt|RemoteCommandCoverage|RootHelp' -count=1` 和 `openspec validate pinax-unified-vault-workspace-database --strict`，退出码均为 0。

- [x] **2.2 强化 project board saved views**
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 1.3, 2.1
  - Scope: 支持 board view 保存 filter/sort/group/display 规则；project/subproject/collection 可以拥有多个 board view，如 `active`、`blocked`、`weekly-review`。
  - Files: `internal/app/project_board.go`、`internal/domain/database.go`、`internal/cli/project_cmd.go`、`cmd/pinax/project_board_command_test.go` 或现有 project board tests。
  - Acceptance: `pinax project board view save <project> <view> --subproject <slug> --columns inbox,next,doing,blocked,review,done --vault <vault> --json` 创建 CLI-authored view；show/render 读取当前 task 投影，不保存结果快照。
  - Validation command: `go test ./cmd/pinax ./internal/app -run 'ProjectBoardView|SavedView|TaskView|Subproject' -count=1`
  - Expected result: saved board view tests 通过；旧 `project board show` 无 view 时行为不变。
  - Failure re-check: 如果命令名与现有 `database view` 冲突，保留现有命令并用新子命令 additive 扩展。
  - Evidence: 2026-06-25 在 `TestProjectBoardViewSaveAndShowCLI` 先补 RED，要求 `pinax project board view save research active --columns next,doing --group column --sort priority --display card --vault <vault> --json` 写入 `.pinax/project-board-views/research/active.json`，且 registry 只保存 view 配置、不保存 item 结果行；首次运行 `go test ./cmd/pinax -run TestProjectBoardViewSaveAndShowCLI -count=1` 因缺 board view 子命令/flags 失败。随后新增 `domain.ProjectBoardView`、`ProjectBoardViewSave`、`project board view save`、`project board show --view`，并让 saved view 使用 strict columns，避免不在 view columns 中的 item 被 fallback 计入其它列。重跑 `go test ./cmd/pinax -run TestProjectBoardViewSaveAndShowCLI -count=1` 通过；运行 `go test ./cmd/pinax ./internal/app -run 'ProjectBoardView|SavedView|TaskView|Subproject' -count=1`、`go test ./internal/domain ./internal/app ./internal/cli ./cmd/pinax -run 'ProjectBoard|TaskAdopt|CLIOutput|Agent|JSON' -count=1` 和 `openspec validate pinax-unified-vault-workspace-database --strict`，退出码均为 0。

- [x] **2.3 增加 daily task review managed block**
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 2.1, 2.2
  - Scope: daily note 中只更新 `pinax:managed name=daily-task-review` block；没有 marker 时只输出 plan 和 next action。
  - Files: `internal/app/taskbridge_planning.go`、`internal/app/builtin_templates.go`、`internal/cli/plan_cmd.go`、`cmd/pinax/note_record_command_test.go`、`tests/e2e/testdata/project_board/scripts/project_board_workspace.txt`。
  - Acceptance: daily review 汇总 today/overdue/blocked/review tasks；写入需 `--yes`；非 managed 区域不被修改。
  - Validation command: `go test ./internal/app ./cmd/pinax ./tests/e2e -run 'DailyTaskReview|ManagedBlock|ProjectBoard' -count=1`
  - Expected result: managed block tests 通过；没有 marker 返回 `managed_block_missing`。
  - Failure re-check: 写入前必须有 snapshot 或 record evidence；失败不得留下半更新正文。
  - Evidence: 2026-06-25 在 `TestPlanDailyTaskReviewRequiresManagedBlockAndYes` 先补 RED，要求 `PlanDaily(TaskReview=true)` 在缺 `pinax:managed name=daily-task-review` 时返回 `managed_block_missing` 且不修改 daily note；有 marker 但无 `--yes` 只输出 plan；带 `--yes` 仅替换该 managed block 并保留用户正文。随后新增 `daily-task-review` 内置 daily template block、`pinax plan daily --task-review`、today/overdue/blocked/review 汇总和 e2e project board 脚本覆盖。运行 `go test ./internal/app ./cmd/pinax ./tests/e2e -run 'DailyTaskReview|ManagedBlock|ProjectBoard' -count=1`，退出码 0；运行 `openspec validate pinax-unified-vault-workspace-database --strict`，退出码 0。

## P3: Notion 风格 Database v2

- [x] **3.1 扩展 property schema 与类型校验**
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 1.1
  - Scope: 支持 `text`、`number`、`checkbox`、`date`、`select`、`multi_select`、`url`、`email`、`person_text`、`relation`、`rollup`、`formula` 的本地安全子集；schema registry 通过 CLI/service 写入。
  - Files: `internal/domain/database.go`、`internal/index/property.go`、`internal/app/query.go`、`internal/cli/search_database_cmd.go`、`cmd/pinax/search_database_command_test.go`。
  - Acceptance: schema infer/set/list/show 输出 typed facts；非法 property 值返回 warnings 或 stable errors，不 panic。
  - Validation command: `go test ./internal/app ./internal/index ./cmd/pinax -run 'DatabaseSchema|Property|Type|Relation|Rollup|Formula' -count=1`
  - Expected result: property schema tests 通过；旧 v1/v2/v3 view 继续可读。
  - Failure re-check: formula 不允许文件、网络、环境、provider payload、secret、raw prompt 或 full body 访问。
  - Evidence: 2026-06-25 在 `TestDatabaseSchemaV2TypedOverridesListShowAndValidation` 和 `TestDatabaseSchemaAndViewRegistryV2CLI` 先补 RED，要求 `database schema set` 支持 `checkbox`、`multi_select`、`url` 等新增类型，unsupported type 返回 `property_type_unsupported` 且不写 registry，`schema list/show` 输出 typed facts，现有非法值以 `validation_status=warnings` 和 `invalid_values` 呈现。随后新增 `PropertySchemaOverrideRegistry`、Notion 风格 additive property types、schema override 合并写入、现有值校验 warning、CLI `database schema list/show` 和新 type completion。运行 `go test ./internal/app ./internal/index ./cmd/pinax -run 'DatabaseSchema|Property|Type|Relation|Rollup|Formula' -count=1`、`go test ./internal/app ./internal/app/searchops ./internal/index ./cmd/pinax -run 'Query|Dataview|DatabaseSchema|DatabaseView|Property|Type|Relation|Rollup|Formula|CLIOutput|Agent|JSON' -count=1` 和 `openspec validate pinax-unified-vault-workspace-database --strict`，退出码均为 0。

- [x] **3.2 实现 table/board/list/calendar view render 合同**
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 2.2, 3.1
  - Scope: `pinax database view save/render` 支持 `--display table|board|list|calendar`；board view 可复用 task/project board 投影，calendar view 使用 date property。
  - Files: `internal/app/searchops/query.go`、`internal/app/query.go`、`internal/cli/search_database_cmd.go`、`internal/output/render.go`、`tests/e2e/testdata/dataview_database/scripts/dataview_database.txt`。
  - Acceptance: view 保存 query 和 display options，不保存结果 rows；render 输出 bounded rows/cards/events；`--json`/`--agent` 合同稳定。
  - Validation command: `go test ./cmd/pinax ./internal/app ./tests/e2e -run 'DatabaseView|Render|Table|Board|List|Calendar' -count=1`
  - Expected result: view render tests 通过；缺少 calendar field 返回稳定 error 和 next action。
  - Failure re-check: 不允许在 render 时写 Markdown 或 `.pinax/**`，除非命令是显式 save/apply。
  - Evidence: 2026-06-25 在 `TestDatabaseViewDisplayRenderContracts` 和 `TestDatabaseViewDisplayRenderCLI` 先补 RED，要求 `database view save --display table|board|list|calendar` 只保存 view 配置不保存 rows，`database view render` 输出 `display`、row count、board columns 或 calendar events；calendar view 缺 `--calendar-field` 返回 `calendar_field_required` 和 next action。随后新增 `domain.DatabaseViewRender`、`RenderDatabaseView`、table/list/board/calendar render 转换、`--display` flag 和 dataview e2e 覆盖。运行 `go test ./cmd/pinax ./internal/app ./tests/e2e -run 'DatabaseView|Render|Table|Board|List|Calendar' -count=1`、`go test ./internal/app ./internal/app/searchops ./internal/index ./cmd/pinax ./tests/e2e -run 'Query|Dataview|DatabaseSchema|DatabaseView|Property|Type|Relation|Rollup|Formula|CLIOutput|Agent|JSON|Render|Table|Board|List|Calendar' -count=1` 和 `openspec validate pinax-unified-vault-workspace-database --strict`，退出码均为 0。

- [x] **3.2a 实现 SavedView-as-Tab 与 Markdown 多 tab 渲染**
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 3.2
  - Scope: 将一个 saved database view 定义为一个可发现 tab；支持 Markdown `pinax-database-view <name>` fence 按文档顺序组成多 tab 页面；`pinax-sql` 和 `pinax-dataview` fence 继续作为临时单查询块兼容。`--kind` 保持旧兼容，`--display` 是 query-backed database view 的 canonical display 参数。
  - Files: `internal/app/query.go`、`internal/app/service.go`、`internal/cli/search_database_cmd.go`、`internal/domain/types.go`、`internal/output/render.go`、`cmd/pinax/search_database_command_test.go`、`cmd/pinax/note_record_command_test.go`、`tests/e2e/testdata/dataview_database/scripts/dataview_database.txt`。
  - Acceptance: saved view registry 只新增 optional `display.*` tab metadata，不保存 result rows；`database view render` 保留旧 `facts.view`、`facts.rows`、`facts.columns` 并新增 optional `data.database_view`、`data.database_tab`、`fact.database.*`、`fact.database_tab.*`；`note show --view rendered` 对多个 saved view fence 返回 bounded tab projection；缺失 view 返回 `database_tab_view_not_found` 且不改写 note body。
  - Validation command: `go test ./cmd/pinax ./internal/app ./tests/e2e -run 'DatabaseView|Render|Table|Board|List|Calendar|Tab|Rendered' -count=1`
  - Expected result: database view render、Markdown 多 tab render、JSON/agent contract tests 通过；临时 `render --display/--group-by/--calendar-field/--board-column` 不写回 registry。
  - Failure re-check: 不允许直接解析或手写 `.pinax/views.json` 作为 UI 数据源；dashboard/client/API/MCP 只能消费 app service projection。
  - Evidence: 2026-06-25 新增 `domain.DatabaseTab` optional projection，并让 `database view render` 保留旧 `view`、`render` data/facts，同时新增 `data.database_view`、`data.database_tab`、`fact.database.*`、`fact.database_tab.*`；saved view registry 仅保存 `display.mode/tab` 配置，不保存 rows。新增 `pinax-database-view <name>` Markdown fence 渲染，`note show --view rendered` 支持按文档顺序输出多个 saved view tabs，JSON data 带 `database_tabs`，缺失 saved view 返回 `database_tab_view_not_found` 且不修改 note body；`pinax-sql`/`pinax-dataview` fence 兼容路径保留。新增 app/CLI/e2e 覆盖 `TestDatabaseViewRenderAddsTabProjectionContract`、`TestNoteRenderedDatabaseViewTabsCLI`、agent facts 断言和 dataview database testscript 多 tab 场景。运行 `go test ./cmd/pinax ./internal/app ./tests/e2e -run 'DatabaseView|Render|Table|Board|List|Calendar|Tab|Rendered' -count=1` 和 `openspec validate pinax-unified-vault-workspace-database --strict`，退出码均为 0。

- [x] **3.3 实现 relation-lite 与 rollup-lite 查询**
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 3.1, 3.2
  - Scope: relation 只引用 vault 内 note/task/project/view；rollup 支持 count、min、max、latest、status_summary；输出带 bounded explanation。
  - Files: `internal/app/searchops/query.go`、`internal/index/model/records.go`、`internal/index/query_sources.go`、`internal/app/searchops/query_test.go`。
  - Acceptance: relation missing/ambiguous 返回 stable facts；rollup 不读取完整 note body；query plan 报告 selected sources。
  - Validation command: `go test ./internal/app/searchops ./internal/index -run 'Relation|Rollup|QueryPlan|Ambiguous' -count=1`
  - Expected result: relation/rollup tests 通过，`--explain` 不含 chain-of-thought。
  - Failure re-check: 如果需要新增 index table，使用 GORM/GORM Gen repository；不得在 app/handler 里硬编码 SQL。
  - Evidence: 2026-06-25 在 `TestExecuteRelationSourceAndRollupLiteAggregates` 先补 RED，要求 `FROM relations` 输出 vault 内 wikilink relation facts，resolved/broken/ambiguous 状态稳定且不泄漏 note body；`LATEST()` 和 `STATUS_SUMMARY()` rollup 聚合可按 status 分组或全局汇总。随后新增 `QuerySourceRelations`、`ExtractRelationRows`、查询执行 source 路由、`LATEST` 与 `STATUS_SUMMARY` aggregate。运行 `go test ./internal/app/searchops ./internal/index -run 'Relation|Rollup|QueryPlan|Ambiguous' -count=1`、`go test ./internal/app ./internal/app/searchops ./internal/index ./cmd/pinax -run 'Query|Dataview|DatabaseSchema|DatabaseView|Relation|Rollup|QueryPlan|Ambiguous|Property|Type' -count=1` 和 `openspec validate pinax-unified-vault-workspace-database --strict`，退出码均为 0。

## P4: Obsidian 兼容能力矩阵

- [x] **4.1 固化 wikilink/backlink/graph 兼容包**
  - Owner: `cli/pinax`
  - Lane: D
  - Depends on: 1.3
  - Scope: 将 wikilink、backlink、orphan、ambiguous link、graph facts 和 repair plan 纳入统一 compatibility matrix；确保 Obsidian 常见 alias、heading、block ref 能 bounded 解析。
  - Files: `internal/notelinks/`、`internal/app/linkgraph.go`、`internal/app/linkgraph_test.go`、`internal/cli/collection_graph_cmd.go`、`openspec/specs/note-bidirectional-links/spec.md`。
  - Acceptance: `pinax note backlinks`、`pinax note links`、`pinax graph show` 或现有 graph 命令输出 stable facts；repair 只 plan，不静默改正文。
  - Validation command: `go test ./internal/notelinks ./internal/app ./cmd/pinax -run 'WikiLink|Backlink|Graph|Ambiguous|Repair' -count=1`
  - Expected result: graph/link tests 通过，无 full body 泄漏。
  - Failure re-check: ambiguous link 不得自动选候选；必须返回 candidate facts 和 next action。
  - Evidence: 2026-06-25 在 `TestLinkGraphCompatibilityMatrixAndRepairPlanFacts` 先补 RED，要求 wikilink alias、heading、broken、ambiguous、backlink、graph summary 都输出 compatibility matrix facts，ambiguous/broken link 只指向 `pinax repair plan`，不自动选候选或改正文。随后新增 `addLinkCompatibilityFacts`，统一注入 enhanced/fallback note links、backlinks 和 graph summary。运行 `go test ./internal/notelinks ./internal/app ./cmd/pinax -run 'WikiLink|Backlink|Graph|Ambiguous|Repair' -count=1`、`go test ./internal/app ./internal/app/searchops ./internal/index ./cmd/pinax -run 'WikiLink|Backlink|Graph|Ambiguous|Repair|Relation|Rollup|Query|Search|Resolver' -count=1` 和 `openspec validate pinax-unified-vault-workspace-database --strict`，退出码均为 0。

- [x] **4.2 完成 assets/templates/properties/daily notes 兼容包**
  - Owner: `cli/pinax`
  - Lane: D
  - Depends on: 2.3, 3.1
  - Scope: 对齐 Obsidian 常见 vault 使用：properties schema、daily notes、template preview/override、attachment missing/orphan doctor、asset manifest。
  - Files: `internal/app/builtin_templates.go`、`internal/app/builtin_templates_test.go`、`internal/assets/`、`internal/cli/template_cmd.go`、`docs/commands/template.md`、`docs/commands/asset.md`。
  - Acceptance: 用户可以在 Obsidian 编辑正文后运行 Pinax doctor/repair/organize；Pinax 只改 managed metadata 或经批准的 repair plan。
  - Validation command: `go test ./internal/app ./cmd/pinax ./tests/e2e -run 'Template|Daily|Asset|Property|Doctor|Repair' -count=1`
  - Expected result: template/asset/property workflow tests 通过。
  - Failure re-check: 不允许把 Obsidian 插件生成的未知 frontmatter 当作 Pinax-owned 字段强制重写。
  - Evidence: 2026-06-25 新增 `TestNotePropertyPreservesObsidianPluginFrontmatterCLI`，覆盖 `note property set/remove` 只改目标 property 和 Pinax 时间戳，保留 `cssclasses`、插件状态字段、自定义 frontmatter 与 Obsidian 编辑正文；新增 `TestBuiltInDailyTemplateObsidianCompatibilityBlocks`，锁定 `journal.daily` 的 daily path pattern 与 `planning-daily`、`daily-task-review`、`daily-captures` managed blocks。更新 `template` CLI help 与 `docs/commands/template.md`、`docs/commands/asset.md`，明确 template preview/inspect 只读、daily task review 只替换 managed block、asset missing/orphan/repair plan 不改正文或删除文件、asset manifest 由 CLI/service 写入。运行 `go test ./internal/app ./cmd/pinax ./tests/e2e -run 'Template|Daily|Asset|Property|Doctor|Repair' -count=1` 和 `openspec validate pinax-unified-vault-workspace-database --strict`，退出码均为 0。

- [x] **4.3 形成 Obsidian import/doctor/publish smoke flow**
  - Owner: `cli/pinax`
  - Lane: D
  - Depends on: 4.1, 4.2
  - Scope: testscript 构造 Obsidian-style vault fixture，覆盖 wikilinks、properties、daily、attachments、templates、dataview block、canvas placeholder 文件和 plugin metadata ignore。
  - Files: `tests/e2e/testdata/obsidian_compat/`、`tests/e2e/*_test.go`、`internal/vaultignore/`、`docs/commands/vault.md`。
  - Acceptance: `vault doctor`、`index refresh`、`note backlinks`、`database view render`、`publish plan --dry-run` 在 fixture 上通过；未知 Obsidian plugin metadata 不导致 corruption。
  - Validation command: `go test ./tests/e2e -run 'ObsidianCompat|VaultDoctor|PublishPlan' -count=1`
  - Expected result: e2e 通过，publish dry-run 不写远端。
  - Failure re-check: `.obsidian/**` 默认只读/忽略；不得把 Obsidian config 写入 `.pinax/**` registry。
  - Evidence: 2026-06-25 新增 `TestObsidianCompat` 与 `tests/e2e/testdata/obsidian_compat/scripts/obsidian_compat.txt`，构造 Obsidian-style vault fixture，覆盖 `.obsidian` plugin config/ignored Markdown、wikilink/backlink、properties/dataview render、daily managed block、attachments missing/orphan、canvas placeholder、repair/organize plan 和 `publish plan`。首轮 RED 暴露默认 `.pinaxignore` 缺 `.obsidian/`，随后在 `vaultignore.DefaultPinaxIgnore()` 中加入 `.obsidian/` 并补单元断言；smoke 断言 `.obsidian` plugin sentinel、canvas sentinel 和 plugin Markdown 不出现在 index/doctor/publish 输出，`organize suggest` 不纳入 `.obsidian`、canvas 或 Obsidian template 文件。运行 `go test ./tests/e2e -run 'ObsidianCompat|VaultDoctor|PublishPlan' -count=1`、`go test ./internal/vaultignore -count=1` 和 `openspec validate pinax-unified-vault-workspace-database --strict`，退出码均为 0。

## P5: API/MCP/Dashboard 与最终收口

- [x] **5.1 注册 workspace/task/database capabilities**
  - Owner: `cli/pinax`
  - Lane: E
  - Depends on: 1.3, 2.2, 3.2a
  - Scope: 为 workspace show/list、task view/adopt plan、database view render/database tab projection、graph read 注册 REST/RPC capabilities；MCP/dashboard 默认只读，dashboard/client 只消费 shared projection，不直接解析 `.pinax/**` registry 或 Markdown fences。
  - Files: `internal/api/http.go`、`internal/api/rpc.go`、`internal/dashboard/server.go`、`internal/mcpserver/`、`internal/app/remote.go`。
  - Acceptance: `pinax api routes --vault <vault> --json` 可发现新增能力；database view render 返回与 CLI JSON 相同的 bounded tab projection；readonly server 拒绝写；Remote API Mode unsupported 不 fallback；dashboard active tab selection 保持 client-local，不新增持久 layout registry。
  - Validation command: `go test ./internal/api ./internal/dashboard ./internal/mcpserver ./cmd/pinax -run 'Workspace|Task|Database|Tab|Capability|Route|RPC|MCP|Readonly' -count=1`
  - Expected result: API/RPC/MCP/dashboard projection tests 通过；REST/RPC/MCP/dashboard 不出现与 CLI 不一致的 database tab 字段命名。
  - Failure re-check: REST/RPC handler 不得直接读 Markdown、`.pinax/**`、GORM repository 或 Git；必须走 app service。
  - Evidence: 2026-06-25 新增 `database.view.render`、`task.adopt.plan`、`graph.summary` additive remote capabilities 和 REST/RPC routes；`pinax api routes --json` 可发现 `/v1/database/views/{name}:render`、`/v1/tasks/{item}:adopt-plan`、`/v1/graph/summary` 与对应 RPC methods。HTTP/RPC/MCP/dashboard 均通过 app service 调用 `RenderDatabaseView`、`TaskAdopt(Yes:false)` 和 `GraphSummaryProjection`，REST/RPC route registry tests 覆盖代表性 fixture；dashboard 新增 `/api/database-tabs/<view>` 只读 endpoint，返回 shared `data.database_view`/`data.database_tab`，不新增持久 layout registry；CLI remote mode 新增 `database view render` RPC 映射，unsupported command 仍返回 `remote_command_unsupported` 且不 fallback。先运行 focused RED 观察缺 route/tool/remote mapping 失败；实现后运行 `go test ./internal/api ./internal/dashboard ./internal/mcpserver ./cmd/pinax -run 'Workspace|Task|Database|Tab|Capability|Route|RPC|MCP|Readonly' -count=1` 和 `openspec validate pinax-unified-vault-workspace-database --strict`，退出码均为 0。

- [x] **5.2 增加全流程 e2e 与 integration evidence**
  - Owner: `cli/pinax`
  - Lane: E
  - Depends on: 2.3, 3.3, 4.3, 5.1
  - Scope: 一个 fixture 覆盖统一 workspace、subproject、managed/adopted/inferred task、database table/board/calendar、wikilinks/backlinks、assets、daily review、API readonly、MCP readonly 和 publish dry-run。
  - Files: `tests/e2e/testdata/unified_workspace/`、`tests/e2e/*_test.go`、`internal/testkit/integrationevidence/main.go`、`Taskfile.yml`。
  - Acceptance: `task test:integration` 生成证据；失败也保留 evidence；证据扫描无 secret/body/provider/raw prompt 命中。
  - Validation command: `task test:integration && find temp/integration-test-runs -maxdepth 2 -type f | sort | tail -40`
  - Expected result: integration tests pass，最新 run 目录包含 `summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json`、`artifacts/`。
  - Failure re-check: 若 evidence 中出现 token、Authorization、raw prompt、provider payload、hidden prompt、private tool arguments 或完整 note body sentinel，先修 redaction，再重新运行。
  - Evidence: 2026-06-25 新增 `TestUnifiedWorkspace` 与 `tests/e2e/testdata/unified_workspace/scripts/unified_workspace.txt`，一个 testscript fixture 覆盖 unified workspace/subproject、managed/adopted/inferred task、daily task review managed block、database table/board/calendar views、Markdown saved-view tabs、wikilinks/backlinks、asset missing/orphan、API route discovery、MCP stdio readonly database view render、Remote API unsupported no fallback 和 publish plan。扩展 `internal/testkit/integrationevidence` command，纳入 `TestUnifiedWorkspace`、`TestObsidianCompat`、5.1 API/RPC/MCP/dashboard readonly projection tests，并记录 `unified_workspace`、`obsidian_compat`、`api_readonly_capabilities`、`dashboard_database_tab`、`mcp_database_view` checks。首轮 `task test:integration` 失败于 dataview testscript 反引号断言，仍生成失败证据 `temp/integration-test-runs/20260625T172856Z-4110732/`；修复后运行 `task test:integration` 通过，生成 `temp/integration-test-runs/20260625T172955Z-4122915/`，其中包含 `summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json`、`artifacts/README.txt`。运行 `find temp/integration-test-runs -maxdepth 2 -type f | sort | tail -40` 和 `find temp/integration-test-runs/20260625T172955Z-4122915 -maxdepth 2 -print | sort` 确认文件结构；运行 `grep -R -n -E 'Authorization: Bearer|raw prompt|provider payload|hidden system prompt|private tool arguments|full chain-of-thought|EVIDENCE_|secret-token|fake-token' temp/integration-test-runs/20260625T172955Z-4122915 || true` 无命中。

- [x] **5.3 更新文档和最终门禁**
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: 5.2
  - Scope: 更新 Pinax 子项目文档地图、命令手册和产品定位，说明统一 workspace、Kanban、database、Obsidian compatibility、API/MCP/dashboard 边界和 Cloud Sync/Remote API 区别。
  - Files: `README.zh-CN.md`、`README.md`、`docs/README.zh-CN.md`、`docs/README.md`、`docs/overview/product-positioning.md`、`docs/commands/project.md`、`docs/commands/database.md`、`docs/commands/vault.md`、`docs/interfaces/client-cli-parity-and-sync.md`。
  - Acceptance: 文档命令均为真实 `pinax ...` 命令；不展示 agent-only wrapper；中文人类文档清楚标注 Preview/Experimental/Mature。
  - Validation command: `task check && openspec validate pinax-unified-vault-workspace-database --strict && openspec validate --all --strict`
  - Expected result: 全量门禁通过；OpenSpec active change 可进入实现 closeout 或 archive 准备。
  - Failure re-check: 如果 lint/test 有既有无关失败，记录具体失败、相关性和 focused tests 证据；不要降低合同测试。
  - Evidence: 2026-06-25 更新 `README.zh-CN.md`、`README.md`、`docs/README.zh-CN.md`、`docs/README.md`、`docs/overview/product-positioning.md`、`docs/commands/project.md`、`docs/commands/database.md`、`docs/commands/vault.md`、`docs/interfaces/client-cli-parity-and-sync.md`，同步 unified workspace、task adoption、database saved-view tabs、Obsidian compatibility、API/MCP/dashboard readonly boundaries、Remote API Mode 与 Cloud Sync 区分，并用真实 `pinax ...` 命令示例。首次运行 `task check` 失败于既有 lint 死代码 `internal/index/store.go` 中未使用 `notePathByTitle*` helper；确认仅定义无引用且已由 `notelinks.ResolverSnapshot` 路径替代后删除死代码并重跑。最终运行 `task check`、`openspec validate pinax-unified-vault-workspace-database --strict`、`openspec validate --all --strict`，退出码均为 0。
