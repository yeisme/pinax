## ADDED Requirements

### Requirement: Project workspace commands SHALL follow one projection across output modes

Pinax project workspace, board, and item commands SHALL render human, agent, JSON, events, and explain outputs from one shared projection.

#### Scenario: JSON output is a single envelope
- **WHEN** the user runs any project workspace command with `--json`
- **THEN** stdout SHALL contain exactly one JSON object
- **AND** it SHALL include stable top-level fields `spec_version`, `mode`, `command`, `status`, `facts`, `data`, `actions`, `evidence`, and `error` when applicable
- **AND** diagnostics SHALL go to stderr.

#### Scenario: Agent output uses stable key-value facts
- **WHEN** the user runs `pinax project board show research --subproject stock-learning --vault yeisme-notes --agent`
- **THEN** stdout SHALL include `spec_version=1.0`, `mode=agent`, `command=project.board.show`, `status=success`, `fact.project=research`, `fact.subproject=stock-learning`, `fact.workspace_path`, column count keys such as `fact.column.next`, aggregate keys such as `fact.items.total`, top item keys such as `fact.item.top.id`, `fact.item.top.priority`, and risk keys such as `fact.risk.blocked` and `fact.risk.review`
- **AND** runnable next steps SHALL use `action.<name>` keys when available
- **AND** it SHALL NOT include localized prose, ANSI control codes, full note bodies, raw prompts, provider payloads, Authorization headers, cookies, or tokens.

#### Scenario: Human output is readable project management summary
- **WHEN** the user runs `pinax project board show research --subproject stock-learning --vault yeisme-notes`
- **THEN** default stdout SHALL summarize project/subproject, workspace path, standard directory structure status, board column counts, important items, risk counts, and one recommended next command
- **AND** scripts and agents SHALL NOT need to parse the localized human summary.

#### Scenario: Workspace structure is visible in default board output
- **GIVEN** project `research` has subproject `stock-learning` with standard workspace directories
- **WHEN** the user runs `pinax project board show research --subproject stock-learning --vault <demo-vault>`
- **THEN** stdout SHALL include `Path: notes/projects/research/stock-learning`
- **AND** stdout SHALL include one `Structure:` line with `00-charter`, `10-inbox`, `20-sources`, `30-runs`, `40-outputs`, `50-retros`, and `90-tool-candidates` in that order
- **AND** each directory SHALL show a bounded status such as `ok` or `missing` rather than listing arbitrary file contents.

#### Scenario: Board demo output has stable human sections
- **GIVEN** the fixed project workspace demo has project `research`, subproject `stock-learning`, columns `inbox,next,doing,blocked,review,done`, and at least one blocked item
- **WHEN** the user runs `pinax project board show research --subproject stock-learning --vault <demo-vault>`
- **THEN** stdout SHALL include `Project: research / stock-learning`, `Path: notes/projects/research/stock-learning`, one `Structure:` line, one `Board:` count line, `Next`, `Doing`, `Blocked`, `Review`, `Risks`, and `Recommended next step` sections
- **AND** stdout SHALL NOT include JSON braces, full note bodies, Authorization headers, cookies, tokens, raw prompts, provider payloads, or ANSI control codes when color is disabled.

#### Scenario: Long board columns are truncated in default human output
- **GIVEN** the fixed project workspace demo has more than five `next` items
- **WHEN** the user runs `pinax project board show research --subproject stock-learning --vault <demo-vault>`
- **THEN** the `Next` section SHALL display at most five item lines
- **AND** it SHALL include a line matching `... N more, use --json for full list`
- **AND** the full item list SHALL remain available in `--json` output.

#### Scenario: Empty board output has a useful next action
- **GIVEN** project `research` has subproject `empty-demo` with no items
- **WHEN** the user runs `pinax project board show research --subproject empty-demo --vault <demo-vault>`
- **THEN** stdout SHALL say that no project items exist yet
- **AND** it SHALL include a runnable `pinax project item add research "<title>" --subproject empty-demo --column next --vault <demo-vault> --json` next action.

#### Scenario: Compact board output remains scannable
- **WHEN** the user runs `pinax project board show research --subproject stock-learning --compact --vault <demo-vault>`
- **THEN** stdout SHALL fit the same board facts into a short summary with project/subproject, column counts, top item, risk counts, and one next command
- **AND** machine consumers SHALL still use `--json` or `--agent` instead of parsing compact human text.

#### Scenario: Events output is redacted NDJSON
- **WHEN** the user runs a project workspace command with `--events`
- **THEN** stdout SHALL contain NDJSON start/end/error events with monotonic sequence numbers when ordering matters
- **AND** event payloads SHALL include project, optional subproject, command, status, and redacted evidence refs
- **AND** event payloads SHALL NOT include full note bodies or secret values.

#### Scenario: Board events include summary event
- **WHEN** the user runs `pinax project board show research --subproject stock-learning --vault <demo-vault> --events`
- **THEN** stdout SHALL include a `start` event, a `board.summary` event with project, subproject, workspace path, item count, blocked count, and review count, and an `end` event
- **AND** logs and diagnostics SHALL go to stderr rather than events stdout.

#### Scenario: Board events are rendered from projection facts
- **WHEN** the user runs `pinax project board show research --subproject stock-learning --vault <demo-vault> --events`
- **THEN** the `board.summary` event counts SHALL match the `--json` projection facts for items, blocked, and review
- **AND** the event stream SHALL NOT be assembled by parsing default human summary text.

#### Scenario: Board JSON includes additive workspace payload
- **WHEN** the user runs `pinax project board show research --subproject stock-learning --vault <demo-vault> --json`
- **THEN** stdout SHALL include `data.workspace.project`, `data.workspace.subproject`, `data.workspace.path`, and `data.workspace.directories`
- **AND** `data.workspace.path` SHALL be relative to the vault rather than an absolute local user path
- **AND** each `data.workspace.directories[]` entry SHALL include `name`, `path`, and a bounded `status` value such as `ok` or `missing`
- **AND** existing consumers MAY ignore `data.workspace` without losing previously released project board fields.

### Requirement: Project workspace output changes SHALL be additive

Pinax SHALL preserve existing project board and item output contracts while adding subproject and project-management fields.

#### Scenario: Existing project board consumers remain compatible
- **WHEN** existing consumers parse `pinax project board show research --vault yeisme-notes --json`
- **THEN** previously released fields SHALL remain present and keep their meaning
- **AND** new fields such as `subproject`, `labels`, `milestone`, `priority`, `due_at`, `blocked_by`, `workspace_path`, and `data.workspace` SHALL be optional additive fields.
