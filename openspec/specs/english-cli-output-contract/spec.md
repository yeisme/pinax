# cli-output-language-contract Specification

## Purpose
TBD - created by archiving change english-cli-output-contract. Update Purpose after archive.
## Requirements
### Requirement: Pinax defaults to Chinese user-visible CLI chrome

Pinax SHALL render CLI chrome in Chinese by default for human-facing command surfaces owned by `cli/pinax`, while preserving English machine protocol fields.

#### Scenario: Default command summary uses Chinese chrome

- **WHEN** a user runs a representative successful `pinax` command without `--json`, `--agent`, `--events`, or `--explain`
- **THEN** stdout SHALL use Chinese section labels and prose for status, highlights, facts, evidence, risks, and next action labels
- **AND** stdout SHALL include at most one primary recommended next command when a next step is useful
- **AND** stdout SHALL NOT require agents or scripts to parse localized human prose.

#### Scenario: Help and validation errors use Chinese chrome

- **WHEN** a user runs `pinax --help`, command-specific help, an unknown command, or a validation-failure path
- **THEN** help text, usage text, examples, suggestions, error messages, and correction hints SHALL be Chinese where they are human prose
- **AND** every suggested command SHALL be a real user-runnable `pinax ...` command.

#### Scenario: Explain mode is Chinese and redacted

- **WHEN** a user requests `--explain` for a supported command
- **THEN** stdout SHALL use Chinese review sections such as conclusion, evidence, confidence, risks, tradeoffs, and recommended next step, or equivalent localized labels
- **AND** stdout SHALL NOT include full chain-of-thought, raw prompts, hidden prompts, provider payloads, secrets, tokens, cookies, Authorization headers, private tool arguments, or model-internal reasoning.

### Requirement: Pinax preserves domain content language

Pinax SHALL distinguish CLI chrome from data and SHALL NOT blindly translate non-English domain content.

#### Scenario: User-authored content remains unchanged

- **GIVEN** a note body, template body, title, tag, folder name, quoted source, or imported document contains non-English text
- **WHEN** Pinax renders or returns that content
- **THEN** the content SHALL retain its original language and bytes except for existing domain-normalization rules
- **AND** Chinese human chrome SHALL apply to surrounding CLI labels, summaries, errors, and actions, not to the user-authored data.

#### Scenario: Provider and third-party payload fields remain stable

- **GIVEN** a provider response, third-party API field, schema field, enum value, event type, command id, flag name, JSON envelope key, or `--agent` key already has a stable machine contract
- **WHEN** Pinax renders machine output or records structured data
- **THEN** those fields SHALL remain stable unless a major output-contract version migration is explicitly introduced
- **AND** human-language localization SHALL NOT rename machine fields only for prose consistency.

### Requirement: Machine output remains parseable, stable, and prose-free

Pinax SHALL keep machine-readable output independent from human-language text.

#### Scenario: JSON output is a single envelope

- **WHEN** a user runs a supported command with `--json`
- **THEN** stdout SHALL contain exactly one valid JSON object
- **AND** the object SHALL include `spec_version`, `mode=json`, `command`, and `status`
- **AND** stdout SHALL NOT include ANSI, progress logs, banners, tables, human-only suggestions, or localized prose outside JSON string fields that are explicitly part of the envelope.

#### Scenario: Agent output is stable key=value

- **WHEN** a user runs a supported command with `--agent`
- **THEN** stdout SHALL contain stable ASCII key=value lines
- **AND** stdout SHALL include `spec_version`, `mode=agent`, `command`, and `status`
- **AND** stdout SHALL NOT include ANSI, tables, raw debug dumps, localized prose, raw prompts, hidden prompts, provider payloads, private tool arguments, or chain-of-thought.

#### Scenario: Events output is NDJSON only

- **WHEN** a user runs a supported long-running command with `--events`
- **THEN** stdout SHALL be newline-delimited JSON events
- **AND** the stream SHALL start with a `start` event and end with an `end` or `error` event
- **AND** diagnostics and progress text SHALL go to stderr or structured event fields, not mixed prose on stdout.

### Requirement: Contract tests guard human output language and redaction

Pinax SHALL include automated tests that prevent regressions in Chinese CLI chrome, machine parseability, stdout/stderr separation, and redaction.

#### Scenario: Focused output tests fail on wrong-language CLI chrome

- **WHEN** the focused CLI output test suite runs
- **THEN** representative default summaries, help text, validation errors, stderr diagnostics, and explain reports SHALL be checked for Chinese CLI chrome
- **AND** intentional non-English data SHALL be covered by an explicit allowlist or fixture classification.

#### Scenario: Machine-mode tests parse output

- **WHEN** the focused machine-output tests run
- **THEN** tests SHALL parse `--json` as JSON envelopes
- **AND** tests SHALL parse `--agent` as key=value lines
- **AND** tests SHALL parse `--events` as NDJSON for commands that support event streams
- **AND** tests SHALL fail if ANSI, logs, table decoration, or human prose leaks into machine stdout.

#### Scenario: Redaction tests cover all output surfaces

- **WHEN** stdout, stderr, events, traces, snapshots, sidecars, fixtures, or integration evidence are generated
- **THEN** tests SHALL reject secrets, tokens, Authorization headers, cookies, raw prompts, hidden system prompts, unredacted provider payloads, private tool arguments, and full chain-of-thought
- **AND** evidence metadata SHALL be generated by project-owned commands or test runners rather than hand-written by agents.

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

