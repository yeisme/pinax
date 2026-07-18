# pinax-performance-monitor-traces Specification

## Purpose
TBD - created by archiving change pinax-performance-monitor-traces. Update Purpose after archive.
## Requirements
### Requirement: Monitor runs are persisted as redacted performance traces

Pinax SHALL persist a monitor run for supported index, search, query, dataview, and database view operations under `.pinax/monitor/**` with schema version `pinax.monitor_run.v1`.

#### Scenario: Search creates a monitor run without raw query text

- **WHEN** a user runs `pinax search "secret query" --vault <vault> --json`
- **THEN** Pinax writes a monitor run for `note.search`
- **AND** the run contains step metrics for validation, note scan, index/native search, and related index steps when used
- **AND** the run records query length/hash only, not the raw query text.

### Requirement: Monitor CLI exposes runs, details, tail, summary, and management status

Pinax SHALL expose `pinax monitor runs`, `pinax monitor show`, `pinax monitor tail`, `pinax monitor summary`, and `pinax monitor manage` through the standard projection renderers.

#### Scenario: Agent reads monitor runs

- **WHEN** an agent runs `pinax monitor runs --vault <vault> --agent`
- **THEN** stdout contains stable key=value projection facts including `command=monitor.runs`, `status`, `fact.runs`, and `fact.schema_version`.

### Requirement: Activity includes monitor runs

Pinax SHALL expose monitor runs through activity source `monitor_runs`.

#### Scenario: Activity filters monitor runs

- **WHEN** a user runs `pinax activity list --vault <vault> --source monitor_runs --query note.search --json`
- **THEN** activity entries include matching monitor runs with `run_id`, duration, safe facts, and a next action to `pinax monitor show`.

### Requirement: Show commands complete monitor and activity identifiers

Pinax SHALL provide dynamic shell completion for show command identifiers that are backed by local readonly monitor and activity projections.

#### Scenario: Monitor show completes run ids

- **WHEN** a user asks the shell for `pinax __complete monitor show --vault <vault> ""`
- **THEN** completion includes recent monitor run ids with safe command/status descriptions
- **AND** completion returns the no-file-completion directive.

#### Scenario: Activity show completes event ids

- **WHEN** a user asks the shell for `pinax __complete activity show --vault <vault> ""`
- **THEN** completion includes recent activity event ids with safe source/kind/status descriptions
- **AND** completion returns the no-file-completion directive.

### Requirement: Workbench can read monitor traces through readonly API and RPC

Pinax SHALL expose readonly monitor list/show/summary surfaces through REST, RPC, and capability registry.

#### Scenario: REST client reads a monitor run

- **WHEN** a client sends `GET /v1/monitor/runs/{run_id}`
- **THEN** the response is a standard Pinax projection with command `monitor.show` and the monitor run under `data.run`.

#### Scenario: RPC client reads monitor summary

- **WHEN** a client calls `Pinax.Monitor.Summary`
- **THEN** the response is a standard Pinax projection with command `monitor.summary`.

