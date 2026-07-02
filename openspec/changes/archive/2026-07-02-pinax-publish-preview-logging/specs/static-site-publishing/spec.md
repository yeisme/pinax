## ADDED Requirements

### Requirement: Publish preview commands expose additive live progress events

Pinax SHALL expose publish preview progress through additive human logs and `--events` NDJSON without changing existing JSON, agent, explain, or summary projection contracts.

#### Scenario: Build emits stage events
- **WHEN** 用户运行 `pinax publish build --profile public --target local --out ./dist/site --vault ./my-notes --events`
- **THEN** stdout SHALL contain NDJSON events with `start`, `plan_checked`, `renderer_started`, `renderer_completed`, `scan_completed`, `receipt_written`, and `end`
- **AND** every event SHALL include `spec_version`, `mode=events`, `command=publish.build`, `type`, `seq`, and `status`
- **AND** events SHALL NOT include vault absolute paths, private note bodies, tokens, Authorization headers, Cookie headers, provider payloads, raw prompts, hidden system prompts, or private tool arguments.

#### Scenario: Dev preview emits serve and watch events
- **WHEN** 用户运行 `pinax publish dev --profile public --out ./dist/site --host 127.0.0.1 --port 0 --once --vault ./my-notes --events`
- **THEN** stdout SHALL contain NDJSON events with `start`, build stage events, `serve_ready`, `smoke_completed`, and `end`
- **AND** the `serve_ready` event SHALL include a loopback `url` fact.

#### Scenario: Human preview logs go to stderr
- **WHEN** 用户运行 `pinax publish build --profile public --target local --out ./dist/site --vault ./my-notes` without a machine-output flag
- **THEN** stdout SHALL remain the normal human summary projection
- **AND** stderr SHALL include concise stage logs such as `plan_checked`, `renderer_started`, `scan_completed`, and `receipt_written`.

#### Scenario: Machine projection modes stay pure
- **WHEN** 用户 runs publish preview commands with `--json`, `--agent`, or `--explain`
- **THEN** stdout SHALL contain only that selected output mode
- **AND** stderr SHALL NOT include live progress logs unless a downstream external command emits redacted diagnostics.

#### Scenario: Preview approval emits approval event
- **WHEN** 用户运行 `pinax publish preview approve --profile public --out ./dist/site --vault ./my-notes --events`
- **THEN** stdout SHALL contain NDJSON events with `start`, `scan_completed`, `preview_approved`, and `end`
- **AND** the approval event SHALL include profile, target, selected count, scan finding count, output hash status, and receipt path without private content.
