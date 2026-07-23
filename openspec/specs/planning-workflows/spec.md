# planning-workflows Specification

## Purpose
TBD - created by archiving change pinax-taskbridge-planning-workflows. Update Purpose after archive.
## Requirements
### Requirement: Pinax treats TaskBridge as the task execution control plane

Pinax SHALL 通过 Connectors 提供的稳定 TaskBridge CLI 合同消费任务事实，且 SHALL NOT 直接读取 Connectors/TaskBridge 本地存储、Provider token 或 Provider API。Pinax SHALL 优先调用 `connectors task agent ...`；仅当 `connectors` 可执行文件不存在时，MAY 在兼容窗口内回退到独立 `taskbridge agent ...`。

#### Scenario: probing TaskBridge capabilities
- **GIVEN** Pinax vault 已存在且 Connectors 已安装
- **WHEN** 用户运行 `pinax plan snapshot --vault ./my-notes --taskbridge --json`
- **THEN** Pinax SHALL 通过 CLI-backed adapter 调用 `connectors task agent` 能力和任务事实命令
- **AND** stdout SHALL 包含 Pinax JSON envelope、能力事实、警告和下一步操作
- **AND** Pinax SHALL NOT 读取 Connectors/TaskBridge store 文件或 Provider credential 文件

#### Scenario: compatibility fallback
- **GIVEN** `connectors` 不可用但兼容 `taskbridge` 可执行文件存在
- **WHEN** 用户运行 `pinax plan daily --vault ./my-notes --taskbridge --dry-run --json`
- **THEN** Pinax MAY 调用 `taskbridge agent today` 并解析相同的 `taskbridge.*.v1` 合同
- **AND** 新生成的下一步执行命令 SHALL 仍然指向 `connectors task agent execute`

#### Scenario: TaskBridge is unavailable
- **GIVEN** `connectors` 与兼容 `taskbridge` 均不可用，或首选运行时返回不支持的 schema
- **WHEN** 用户运行 `pinax plan daily --vault ./my-notes --taskbridge --dry-run --json`
- **THEN** Pinax SHALL 以 `TASKBRIDGE_UNAVAILABLE` 或 `TASKBRIDGE_CONTRACT_UNSUPPORTED` 失败或降级
- **AND** it SHALL NOT 写入 Markdown、`.pinax` planning assets、Git state、任务运行时状态或远端 Provider 状态

### Requirement: Planning snapshots are CLI-authored and redacted

Pinax SHALL persist normalized planning snapshots through application services when a command explicitly saves or applies planning state.

#### Scenario: saving a TaskBridge planning snapshot
- **GIVEN** TaskBridge returns a valid agent result for today's tasks
- **WHEN** the user runs `pinax plan snapshot --vault ./my-notes --taskbridge --json`
- **THEN** Pinax SHALL write `.pinax/planning/snapshots/<snapshot_id>.json` through the planning snapshot service
- **AND** the snapshot SHALL include schema version, source, captured time, normalized counts, risk summary, evidence refs, and next actions
- **AND** it SHALL NOT include provider tokens, Authorization headers, raw prompts, hidden system prompts, raw provider payloads, tool private parameters, or complete chain-of-thought

#### Scenario: validating planning assets
- **GIVEN** planning snapshots, decisions, action drafts, receipts, or event records exist
- **WHEN** the user runs `pinax validate --vault ./my-notes --json`
- **THEN** Pinax SHALL validate schema versions, required fields, enum values, redaction rules, path boundaries, and note references
- **AND** invalid assets SHALL return stable machine-readable error codes

### Requirement: Daily planning writes managed Markdown blocks only with approval

Pinax SHALL generate daily plans from TaskBridge task facts and vault context, and SHALL only write managed daily note blocks when explicitly approved.

#### Scenario: dry-run daily plan
- **GIVEN** a Pinax vault and available TaskBridge task facts
- **WHEN** the user runs `pinax plan daily --vault ./my-notes --taskbridge --dry-run --json`
- **THEN** Pinax SHALL return a decision preview with selected commitments, deferred candidates, risks, evidence refs, target daily note path, and recommended next action
- **AND** it SHALL NOT modify Markdown notes, `.pinax` planning assets, Git state, TaskBridge state, or remote Provider state

#### Scenario: applying daily plan
- **GIVEN** the daily planning preview has no managed block conflict
- **WHEN** the user runs `pinax plan daily --vault ./my-notes --taskbridge --yes`
- **THEN** Pinax SHALL create or update today's daily note through the journal and planning services
- **AND** it SHALL write only the `pinax:plan daily` managed block
- **AND** it SHALL preserve user-authored content outside the managed block
- **AND** it SHALL append redacted event evidence through the event service

#### Scenario: refusing managed block conflict
- **GIVEN** a user manually edited the existing `pinax:plan daily` managed block after the last Pinax write
- **WHEN** the user runs `pinax plan daily --vault ./my-notes --taskbridge --yes --json`
- **THEN** Pinax SHALL refuse the write with `PLANNING_BLOCK_CONFLICT`
- **AND** it SHALL include a safe next action rather than overwriting user edits

### Requirement: Weekly and monthly planning connect goals to execution without becoming Todo storage

Pinax SHALL use weekly and monthly planning notes to connect long-term goal notes, project notes, historical daily notes, and TaskBridge project/task facts.

#### Scenario: generating weekly plan
- **GIVEN** a vault contains daily notes, goal notes, project notes, and TaskBridge task facts
- **WHEN** the user runs `pinax plan weekly --vault ./my-notes --taskbridge --dry-run --json`
- **THEN** Pinax SHALL return this week's commitments, inherited unfinished work, project risks, goal alignment evidence, and suggested follow-up actions
- **AND** it SHALL NOT create or modify remote Todo tasks

#### Scenario: generating monthly plan
- **GIVEN** a vault contains monthly notes, weekly notes, active goal notes, and TaskBridge project facts
- **WHEN** the user runs `pinax plan monthly --vault ./my-notes --taskbridge --dry-run --json`
- **THEN** Pinax SHALL return monthly themes, active goals, project portfolio risks, suggested freezes or cuts, and recommended weekly focus areas
- **AND** long-term goals SHALL remain Markdown notes rather than being automatically expanded into far-future Todo tasks

### Requirement: Plan actions are TaskBridge action drafts, not direct writes

Pinax SHALL generate TaskBridge-compatible action drafts for user review and SHALL NOT execute task writes itself.

#### Scenario: generating action draft dry-run
- **GIVEN** a daily or weekly planning decision recommends deferring, decomposing, or reviewing tasks
- **WHEN** the user runs `pinax plan actions --vault ./my-notes --from daily --dry-run --json`
- **THEN** Pinax SHALL return a `taskbridge.actions.v1` preview with action ids, task ids, reasons, and confirmation requirements
- **AND** it SHALL include a runnable next action using `taskbridge agent execute --action-file <path> --dry-run` only when a file is saved

#### Scenario: saving action draft
- **GIVEN** the user wants to review actions outside Pinax
- **WHEN** the user runs `pinax plan actions --vault ./my-notes --from weekly --save --json`
- **THEN** Pinax SHALL write `.pinax/planning/actions/<action_id>.json` through the planning service
- **AND** the action draft SHALL include source decision id, snapshot id, created time, and redacted evidence refs
- **AND** Pinax SHALL NOT call `taskbridge agent execute --confirm`

### Requirement: Planning commands follow the AI-native CLI output contract

Pinax planning commands SHALL render human and machine outputs from one command projection.

#### Scenario: machine output mode
- **GIVEN** a planning command supports `--json`, `--agent`, `--events`, or `--explain`
- **WHEN** a machine output mode is selected
- **THEN** stdout SHALL contain only the selected machine format
- **AND** progress, diagnostics, external command stderr, and non-structured errors SHALL go to stderr
- **AND** errors SHALL include stable status and error code fields such as `TASKBRIDGE_UNAVAILABLE`, `TASKBRIDGE_CONTRACT_UNSUPPORTED`, `PLANNING_BLOCK_CONFLICT`, `APPROVAL_REQUIRED`, or `ACTION_DRAFT_INVALID`

#### Scenario: default human output
- **GIVEN** the user runs `pinax plan daily --vault ./my-notes --taskbridge`
- **WHEN** the command completes
- **THEN** default output SHALL be a concise Chinese summary
- **AND** it SHALL include period, selected commitment count, risks, evidence source, write status, and one recommended next action
- **AND** agents SHALL NOT need to parse localized text

### Requirement: Planning has fixture-first tests

Planning workflows SHALL be testable without real Todo provider credentials, real TaskBridge stores, remote networks, or the user's vault.

#### Scenario: testing planning commands
- **GIVEN** planning commands are implemented
- **WHEN** tests are added
- **THEN** command e2e tests SHOULD use `github.com/rogpeppe/go-internal/testscript`
- **AND** tests SHALL use fake TaskBridge executables, fixture TaskBridge JSON envelopes, temporary vaults, temporary Git repositories, and redaction assertions
- **AND** tests SHALL cover dry-run/yes gates, snapshot creation, managed block conflict, daily/weekly/monthly decisions, action draft generation, stdout/stderr separation, and unsupported TaskBridge schema handling

#### Scenario: requiring comments for non-obvious logic
- **GIVEN** future implementation touches capacity scoring, rolling commitment inheritance, project risk detection, TaskBridge protocol conversion, managed Markdown patching, action draft generation, or non-obvious fixtures
- **WHEN** code is added or changed
- **THEN** implementation tasks SHALL require succinct Chinese comments explaining the non-obvious decision or recovery boundary

### Requirement: Daily TaskBridge planning SHALL write a timestamped Markdown todolist through Pinax

Pinax SHALL 让 `pinax plan daily --taskbridge` 通过 Connectors TaskBridge 兼容合同消费每日任务事实，并在明确批准时把带时间戳的 Markdown todolist 写入当天日记的 Pinax managed block。

#### Scenario: TaskBridge daily dry-run is read-only
- **GIVEN** Pinax vault 与有效的 `connectors task agent today` 响应
- **WHEN** 用户运行 `pinax plan daily --vault ./my-notes --taskbridge --dry-run --json`
- **THEN** stdout SHALL 包含一个 Pinax JSON envelope，带有 `source=taskbridge`、`captured_at`、选中承诺数量、目标日记路径和建议下一步
- **AND** Pinax SHALL NOT 修改 Markdown、`.pinax` planning assets、Git state、任务运行时状态、Provider 状态或远端服务

#### Scenario: approved daily plan writes managed block
- **GIVEN** 当日日记不存在或具有有效的 `planning-daily` managed block
- **WHEN** 用户运行 `pinax plan daily --vault ./my-notes --taskbridge --yes`
- **THEN** Pinax SHALL 通过 journal 和 planning services 创建或更新 `daily/YYYY-MM-DD.md`
- **AND** it SHALL 只在 `<!-- pinax:managed name=planning-daily -->` 与 `<!-- /pinax:managed -->` 内写入 Markdown todolist
- **AND** managed block SHALL 包含 `Captured at: <RFC3339 UTC>`
- **AND** Pinax SHALL 保留 managed block 外的用户内容

#### Scenario: TaskBridge adapter failure is safe
- **GIVEN** 首选任务运行时不可用、返回无效 JSON 或返回不支持的 schema
- **WHEN** 用户运行 `pinax plan daily --vault ./my-notes --taskbridge --yes --json`
- **THEN** Pinax SHALL 返回 `TASKBRIDGE_UNAVAILABLE` 或 `TASKBRIDGE_CONTRACT_UNSUPPORTED`
- **AND** it SHALL NOT 写入 Markdown、`.pinax` planning assets、Git state、任务运行时状态、Provider 状态或远端服务

#### Scenario: daily planning actions remain drafts
- **GIVEN** TaskBridge 兼容任务事实产生了 deferred candidates
- **WHEN** 用户运行 `pinax plan actions --vault ./my-notes --from daily --taskbridge --save --json`
- **THEN** Pinax SHALL 在 `.pinax/planning/actions/` 下写入 `taskbridge.actions.v1` draft
- **AND** 下一步 SHALL 为 `connectors task agent execute --action-file <path> --dry-run`
- **AND** Pinax SHALL NOT 调用 `connectors task agent execute --confirm`

