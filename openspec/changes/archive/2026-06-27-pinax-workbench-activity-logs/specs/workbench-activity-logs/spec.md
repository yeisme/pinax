# workbench-activity-logs Delta Spec

## ADDED Requirements

### Requirement: Workbench activity logs provide a unified readonly query projection

Pinax SHALL expose a readonly activity/log query projection that normalizes existing vault events, sync runs, sync daemon events, API audit entries, and record ledger events without changing their stored schemas.

#### Scenario: list returns normalized recent activity

- **GIVEN** a vault contains one or more supported activity sources
- **WHEN** the user runs `pinax activity list --vault <vault> --json`
- **THEN** stdout SHALL be one Pinax JSON projection envelope with `command=activity.list`
- **AND** `data.entries` SHALL contain normalized `pinax.activity_event.v1` entries sorted newest first
- **AND** each entry SHALL include `event_id`, `source`, `kind`, `summary`, and `ts` when the source contains a timestamp.

#### Scenario: query filters are stable and redacted

- **WHEN** the user passes `--source`, `--query`, `--status`, `--since`, `--until`, or `--object`
- **THEN** Pinax SHALL apply those filters to normalized safe fields only
- **AND** it SHALL NOT search or emit note body, Authorization headers, tokens, raw prompts, provider payloads, private tool arguments, or hidden system prompts.

#### Scenario: optional source corruption is partial

- **GIVEN** a supported optional activity source contains a corrupt JSONL line
- **WHEN** the user runs `pinax activity list --vault <vault> --json`
- **THEN** Pinax SHALL return a valid projection with `status=partial`
- **AND** `data.warnings` SHALL identify the source and line number without echoing sensitive raw payload.

### Requirement: Workbench activity logs are available through CLI, REST, RPC, and capability discovery

Pinax SHALL make the activity projection available to CLI users and readonly clients through capability registry, REST, and RPC.

#### Scenario: CLI exposes activity commands in every output mode

- **WHEN** the user runs `pinax activity sources|list|show|tail|manage`
- **THEN** each command SHALL render from one projection through default, `--json`, `--agent`, `--events`, and `--explain` modes
- **AND** machine outputs SHALL contain no ANSI or human-only progress text.

#### Scenario: API route discovery lists activity capability

- **WHEN** the user runs `pinax api routes --vault <vault> --json`
- **THEN** the route list SHALL include readonly Workbench Activity capabilities for list and show
- **AND** those capabilities SHALL include REST path or RPC method, `body_allowed=false`, and no write approval requirement.

#### Scenario: clients do not read vault internals directly

- **WHEN** a REST or RPC client requests activity entries
- **THEN** the handler SHALL call the application service projection
- **AND** it SHALL NOT directly parse `.pinax/**`, record ledger files, sync receipts, API audit files, or SQLite data.

### Requirement: Activity management is advisory in v1

Pinax SHALL report activity source health and safe maintenance guidance without deleting or mutating activity logs in v1.

#### Scenario: manage returns source status and safe actions

- **WHEN** the user runs `pinax activity manage --vault <vault> --json`
- **THEN** Pinax SHALL return source availability, counts, warnings, estimated sizes, and safe next actions
- **AND** any prune action for sync receipts SHALL point to the existing `pinax sync logs prune` command.

#### Scenario: immutable evidence sources are not pruned by activity manage

- **WHEN** activity management inspects API audit, record ledger, or vault event sources
- **THEN** it SHALL NOT delete, truncate, rewrite, or compact those sources.
