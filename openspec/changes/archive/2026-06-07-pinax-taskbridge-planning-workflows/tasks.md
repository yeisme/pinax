# Tasks: Pinax TaskBridge Planning Workflows

## 使用规则

- Owner: `cli/pinax`。
- 本 change 把 Pinax 扩展为个人计划和知识操作系统的计划工作流，不实现 Todo Provider，不直接写远端任务平台。
- TaskBridge 是任务执行控制面；Pinax 只能通过 TaskBridge CLI 稳定输出读取任务事实，或生成 action file 草稿交给 TaskBridge 执行。
- 机器可读资产必须由 CLI/service 写入；不得让 Agent 手写 planning snapshot、decision、action draft、receipt、event JSONL 或 `.pinax` metadata。
- CLI 输出遵守 AI-native CLI 输出合同：默认中文摘要，机器模式保持稳定英文字段。
- 新增或修改复杂启发式、状态机、错误恢复、TaskBridge 协议转换、managed block patch、边界判断和非显然测试夹具时，必须补简短中文注释。
- 每个完成项需要追加 `Evidence:`，记录命令、退出码、关键结论和失败复验。

## 1. OpenSpec 计划完整性

- [x] 1.1 创建 `pinax-taskbridge-planning-workflows` change 骨架。
  - Owner: `cli/pinax`
  - Scope: 通过 OpenSpec CLI 创建 `openspec/changes/pinax-taskbridge-planning-workflows/`。
  - Depends on: none
  - Lane: A
  - Acceptance:
    ```bash
    cd cli/pinax
    test -f openspec/changes/pinax-taskbridge-planning-workflows/.openspec.yaml
    ```
    预期结果：文件存在。
  - Failure re-check: 如果缺少 `.openspec.yaml`，重新运行 `openspec new change pinax-taskbridge-planning-workflows`。
  - Evidence: 2026-06-06 已运行 `openspec new change pinax-taskbridge-planning-workflows`，退出码 0。

- [x] 1.2 补齐 proposal、design、tasks 和 spec。
  - Owner: `cli/pinax`
  - Scope: 写明 Pinax 计划记忆系统、TaskBridge CLI adapter、daily/weekly/monthly/action workflows、数据合同、输出合同和验收场景。
  - Depends on: 1.1
  - Lane: A
  - Acceptance:
    ```bash
    cd cli/pinax
    find openspec/changes/pinax-taskbridge-planning-workflows -maxdepth 4 -type f | sort
    rg -n "pinax plan|TaskBridge|planning snapshot|Mermaid|action file" openspec/changes/pinax-taskbridge-planning-workflows
    ```
    预期结果：看到 `proposal.md`、`design.md`、`tasks.md`、`specs/planning-workflows/spec.md`，并命中关键设计词。
  - Failure re-check: 如果没有 Mermaid 图、没有 TaskBridge adapter 边界或没有输出合同，补齐后重跑。
  - Evidence: 2026-06-06 已补齐本 change 正文文件。

## 2. Planning Domain 和 Snapshot

- [x] 2.1 增加 planning domain 类型。
  - Owner: `cli/pinax`
  - Scope: 在 `internal/domain` 增加 `PlanningPeriod`、`PlanningSnapshot`、`PlanningDecision`、`PlanningCommitment`、`PlanningRisk`、`PlanningEvidenceRef`、`PlanningActionDraft`。
  - Depends on: 1.2
  - Lane: B
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/domain -run Planning -count=1
    ```
    预期结果：period 枚举、risk code、decision JSON 字段、evidence ref 和 action draft 校验测试通过。
  - Failure re-check: 如果 domain 类型无法表达 daily/weekly/monthly/action 四类输出，先补类型再实现 app service。

- [x] 2.2 增加 CLI-authored planning snapshot service。
  - Owner: `cli/pinax`
  - Scope: 在 `internal/app` 增加 `.pinax/planning/snapshots/<snapshot_id>.json` 写入和读取服务，记录脱敏任务事实、source schema、captured_at、risk summary 和 next action。
  - Depends on: 2.1
  - Lane: B
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'PlanningSnapshot|PlanningSnapshotRedaction|PlanningSnapshotPathBoundary' -count=1
    ```
    预期结果：snapshot 由 service 写入 vault 边界内，schema version、redaction、路径边界和读取测试通过。
  - Failure re-check: 如果测试需要手写 `.pinax/planning/*.json` 作为主要流程，改为通过 service 创建 fixture。

## 3. TaskBridge CLI Adapter

- [x] 3.1 实现 TaskBridge executable facade 和 capability probe。
  - Owner: `cli/pinax`
  - Scope: 新增 `internal/taskbridge` 或等价 adapter，执行 `taskbridge agent capabilities`，支持 fake executable，返回稳定 capability projection。
  - Depends on: 2.1
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/taskbridge ./internal/app -run 'TaskBridgeCapabilities|TaskBridgeUnavailable|TaskBridgeRedaction' -count=1
    ```
    预期结果：真实 executable 缺失返回 `TASKBRIDGE_UNAVAILABLE`，fake executable 测试通过，stderr/raw payload 被脱敏。
  - Failure re-check: 如果 adapter 读取 TaskBridge store 或 token 文件，移除直接读取路径并补禁止断言。

- [x] 3.2 实现 TaskBridge today/next/review 归一化。
  - Owner: `cli/pinax`
  - Scope: 解析 `taskbridge agent today` 和可用的 `next/review` JSON 输出，归一化为 planning snapshot facts；缺少字段时降级为 warning。
  - Depends on: 3.1, 2.2
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/taskbridge ./internal/app -run 'TaskBridgeToday|TaskBridgeNext|TaskBridgeContractUnsupported|PlanningSnapshotFromTaskBridge' -count=1
    ```
    预期结果：支持正常输出、缺字段、旧 schema、错误 envelope 和 stderr 噪声场景。
  - Failure re-check: 如果旧 schema 被静默当成成功，改为 `TASKBRIDGE_CONTRACT_UNSUPPORTED` 并给出 next action。

## 4. Planning Context 和 Plan Engine

- [x] 4.1 实现 vault planning context loader。
  - Owner: `cli/pinax`
  - Scope: 读取 daily/weekly/monthly notes、goal/project notes、Pinax project metadata、index/search/link context；index stale 时给出 `pinax index rebuild` next action。
  - Depends on: 2.1
  - Lane: D
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'PlanningContext|GoalNotes|ProjectNotes|PlanningContextIndexFallback' -count=1
    ```
    预期结果：能从 fixture vault 读取目标、项目、历史承诺和复盘事实，缺 index 不崩溃。
  - Failure re-check: 如果 loader 直接解析不属于 vault 的路径，修正路径边界后重跑。

- [x] 4.2 实现保守计划引擎。
  - Owner: `cli/pinax`
  - Scope: 基于 snapshot 和 context 生成 daily/weekly/monthly decision，覆盖容量、截止、项目连续性、inbox、长期目标解释、滚动继承和风险。
  - Depends on: 4.1, 3.2
  - Lane: D
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'DailyPlanningDecision|WeeklyPlanningDecision|MonthlyPlanningDecision|CapacityRisk|RollingCommitment' -count=1
    ```
    预期结果：每日最多 1-3 个深度承诺、逾期风险、项目风险、滚动继承和放弃建议测试通过。
  - Failure re-check: 如果计划引擎自动承诺所有任务或无解释地丢弃任务，修正启发式并补 evidence 断言。

## 5. Managed Block 和 Action Draft

- [x] 5.1 实现 Markdown managed planning block patcher。
  - Owner: `cli/pinax`
  - Scope: 在 daily/weekly/monthly note 中创建或更新 `<!-- pinax:plan ... -->` 区块，只替换受管理区块，不改区块外正文；source hash 冲突返回 `PLANNING_BLOCK_CONFLICT`。
  - Depends on: 4.2
  - Lane: E
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'PlanningManagedBlock|PlanningBlockConflict|PlanningBlockPreservesUserBody' -count=1
    ```
    预期结果：新增、更新、冲突拒绝和保留用户正文测试通过。
  - Failure re-check: 如果 `--dry-run` 写 Markdown 或区块外正文被改写，修正 patcher 和 approval gate。

- [x] 5.2 实现 TaskBridge action draft 生成。
  - Owner: `cli/pinax`
  - Scope: `plan actions` 根据 decision 生成 `taskbridge.actions.v1` 草稿，默认 dry-run；`--save` 时通过 service 写 `.pinax/planning/actions/<id>.json`。
  - Depends on: 4.2
  - Lane: E
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'PlanningActionDraft|TaskBridgeActionsSchema|ActionDraftSave|ActionDraftDryRun' -count=1
    ```
    预期结果：action file schema、requires_confirmation、source decision、dry-run 只读和保存 receipt 测试通过。
  - Failure re-check: 如果 Pinax 调用 `taskbridge agent execute --confirm`，删除执行路径并改为输出用户可运行命令。
  - Evidence: 2026-06-07 先运行 `go test ./internal/app -run 'PlanningActionDraft|TaskBridgeActionsSchema|ActionDraftSave|ActionDraftDryRun' -count=1`，退出码 1，失败于缺少 `buildPlanningActionDraft` 和 `source_snapshot` 字段，确认测试覆盖新增合同；补实现后重跑同一命令，退出码 0。随后运行 `go test ./cmd/pinax -run 'PlanningWorkflowsCLI|PlanningOutput|PlanningJSON|PlanningAgent|PlanningEvents|PlanningExplain|StdoutStderr' -count=1`，退出码 0。`rg -n "pinax\.planning\.actions\.v1|taskbridge\.actions\.v1|SourceSnapshot|requires_confirmation|source_snapshot" .` 显示旧 `pinax.planning.actions.v1` schema 已无残留，`taskbridge.actions.v1`、`source_snapshot` 和 `requires_confirmation` 出现在实现、测试和 spec 中。

## 6. Cobra 命令和输出合同

- [x] 6.1 增加 `pinax plan` 命令树。
  - Owner: `cli/pinax`
  - Scope: 在 `cmd/pinax` 增加 `plan daily/weekly/monthly/review/actions/snapshot`，支持 `--taskbridge`、`--dry-run`、`--yes`、`--save`、`--from`、`--period`。
  - Depends on: 5.1, 5.2
  - Lane: F
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./cmd/pinax -run 'PlanCommand|PlanDaily|PlanWeekly|PlanMonthly|PlanActions|PlanHelp' -count=1
    ```
    预期结果：help 展示计划命令；dry-run 不写；缺 `--yes` 返回 `APPROVAL_REQUIRED`。
  - Failure re-check: 如果 `pinax plan --help` 不展示子命令，检查 root command 注册。

- [x] 6.2 增加 planning 输出 contract tests。
  - Owner: `cli/pinax`
  - Scope: 覆盖默认中文摘要、`--json`、`--agent`、`--events`、`--explain`，确保 stdout/stderr 分离、机器输出无中文 prose/ANSI、错误 envelope 稳定。
  - Depends on: 6.1
  - Lane: F
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/output ./cmd/pinax -run 'PlanningOutput|PlanningJSON|PlanningAgent|PlanningEvents|PlanningExplain|StdoutStderr' -count=1
    ```
    预期结果：输出合同、错误码、redaction 和 explain 摘要测试通过。
  - Failure re-check: 如果 JSON stdout 混入日志、提示或 ANSI，修正 renderer 和 stderr 写入路径。

## 7. MCP、文档和质量门禁

- [x] 7.1 扩展只读 MCP planning resources。
  - Owner: `cli/pinax`
  - Scope: 在只读 MCP surface 暴露 `pinax://planning/latest`、`pinax://planning/snapshot/{id}`、`pinax.plan.context` 只读工具；不得写 Markdown、`.pinax`、Git 或 TaskBridge。
  - Depends on: 4.1, 4.2
  - Lane: G
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/mcpserver -run 'PlanningResource|PlanningReadonly|PlanningRejectWrite' -count=1
    ```
    预期结果：MCP 只读资源可列出，写请求拒绝。
  - Failure re-check: 如果 MCP 工具触发 managed block 写入或 action save，移除写路径。

- [x] 7.2 更新 Pinax README 和 docs。
  - Owner: `cli/pinax`
  - Scope: 更新 `README.md`、`docs/README.md` 和产品/接口文档，说明 Pinax 是个人计划和知识操作系统、TaskBridge 是任务执行控制面，并给出真实命令示例。
  - Depends on: 6.1
  - Lane: G
  - Acceptance:
    ```bash
    cd cli/pinax
    rg -n "pinax plan|TaskBridge|个人计划|任务执行控制面" README.md docs
    ```
    预期结果：文档展示用户可直接运行的真实命令，不包含 agent-only wrapper。
  - Failure re-check: 如果文档要求用户手写 `.pinax/*.json` 或直接读取 TaskBridge store，改成 CLI 命令流程。

- [x] 7.3 运行完成前质量门禁。
  - Owner: `cli/pinax`
  - Scope: 格式化、测试、构建和 OpenSpec 校验。
  - Depends on: 7.1, 7.2
  - Lane: sequential
  - Acceptance:
    ```bash
    cd cli/pinax
    task check
    openspec validate pinax-taskbridge-planning-workflows --strict
    ```
    预期结果：命令退出码 0。
  - Failure re-check: 如果没有安装 `task`，运行 `gofmt -w cmd internal && go test ./... && go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`，再重跑 OpenSpec 校验。
  - Evidence: 2026-06-07 运行 `openspec validate pinax-taskbridge-planning-workflows --strict`，退出码 0，输出 `Change 'pinax-taskbridge-planning-workflows' is valid`。首次运行 `task check` 时 Go 测试和 OpenSpec 全量校验通过，但 fmt-check 退出码 1；`gofmt -l cmd internal` 定位 `cmd/pinax/main.go`、`cmd/pinax/main_test.go`、`internal/app/linkgraph.go`、`internal/index/store.go`、`internal/mcpserver/server.go`，对这些文件运行 `gofmt -w` 后 `gofmt -l cmd internal` 无输出。随后重跑 `task check`，退出码 0，覆盖 `go test ./...`、`openspec validate --all`、fmt-check 和 `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`。
