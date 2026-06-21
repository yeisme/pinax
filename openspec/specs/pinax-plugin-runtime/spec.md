# pinax-plugin-runtime Specification

## Purpose
TBD - created by archiving change pinax-dynamic-plugin-runtime. Update Purpose after archive.
## Requirements
### Requirement: Plugin manifests are validated before installation
Pinax SHALL validate dynamic plugin manifests before installing or executing plugins.

#### Scenario: Validate a WASM plugin manifest
- **WHEN** a user runs `pinax plugin validate ./plugins/project-dashboard --json`
- **THEN** stdout SHALL contain one JSON envelope with command `plugin.validate`
- **AND** facts SHALL include plugin id, version, runtime kind, capability count, permission summary, digest status, and write status `false`
- **AND** no vault file, `.pinax` registry, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Reject secret-bearing manifest
- **WHEN** a plugin manifest contains an API token, Authorization header, Cookie value, webhook URL, or secret-like value
- **THEN** Pinax SHALL fail with stable error code `plugin_manifest_secret_rejected`
- **AND** stdout, stderr, fixtures, audit logs, and evidence SHALL NOT echo the raw secret value.

### Requirement: Plugin registry and lock files are CLI-authored
Pinax SHALL manage installed plugin registry and lock metadata through CLI/application services only.

#### Scenario: Install plugin without enabling it
- **WHEN** a user runs `pinax plugin install ./plugins/project-dashboard --scope vault --vault ./my-notes --json`
- **THEN** Pinax SHALL write `.pinax/plugins/registry.json` and `.pinax/plugins/plugin-lock.json` through the plugin service
- **AND** the installed plugin SHALL remain disabled until `pinax plugin enable <id> --yes` is run
- **AND** stdout SHALL include command `plugin.install`, plugin id, version, runtime kind, enabled status, and next action.

#### Scenario: User disables plugin after unsafe finding
- **WHEN** a user runs `pinax plugin disable project-dashboard --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL mark the plugin disabled in the registry through the plugin service
- **AND** future hook dispatch and `plugin run` SHALL refuse execution with `plugin_disabled` until re-enabled.

### Requirement: Plugin execution is permission-scoped and bounded
Pinax SHALL execute plugins only after checking trust, enabled state, capability, permissions, and resource budgets.

#### Scenario: WASM runtime fails closed when no engine is configured
- **GIVEN** plugin `project-dashboard` is installed, enabled, and granted `projection.read`
- **WHEN** a user runs `pinax plugin run project-dashboard render_dashboard --vault ./my-notes --dry-run --json`
- **THEN** Pinax SHALL return stable error code `plugin_runner_unavailable` when no real WASM engine is configured
- **AND** stdout SHALL contain one JSON envelope with command `plugin.run`, runtime facts, and write status `false`
- **AND** Pinax SHALL NOT pass raw note bodies, full environment variables, Authorization headers, cookies, provider payloads, hidden prompts, or private tool arguments to the runtime boundary by default.

#### Scenario: Refuse ungranted permission
- **WHEN** a plugin capability requests `note.body.read` but the user has not granted it
- **THEN** Pinax SHALL fail with stable error code `plugin_permission_denied`
- **AND** the runner SHALL NOT be started.

#### Scenario: Runner timeout is bounded
- **WHEN** a plugin exceeds its configured timeout or output byte budget
- **THEN** Pinax SHALL terminate or abandon the runner
- **AND** it SHALL return `plugin_budget_exceeded` without leaking raw partial output.

### Requirement: JS and Python plugins run as external trusted runners
Pinax SHALL support JavaScript and Python plugins through explicit external runners rather than embedding language VMs in the Go CLI.

#### Scenario: Missing JavaScript runner
- **WHEN** a JavaScript plugin requires `node` and the runner is not available
- **THEN** `pinax plugin doctor --vault ./my-notes --json` SHALL report `plugin_runner_unavailable`
- **AND** it SHALL recommend installing or configuring the runner without modifying plugin state.

#### Scenario: Python plugin receives limited environment
- **WHEN** a Python plugin is executed
- **THEN** Pinax SHALL set cwd to a Pinax-managed temp directory and pass only allowlisted environment variables
- **AND** the plugin input SHALL arrive through the Plugin RPC envelope rather than command-line arguments containing raw note data or secrets.

### Requirement: Plugin writes are action plans, not direct vault mutation
Pinax SHALL prevent plugins from directly mutating vault files, `.pinax` metadata, Git state, provider state, or remote services.

#### Scenario: Plugin returns action plan
- **WHEN** a plugin returns `action_plan` for note metadata or rendered artifact changes
- **THEN** Pinax SHALL validate the action kinds against the plugin capability and user permission
- **AND** it SHALL return or save a reviewable plan instead of applying writes during `plugin run`
- **AND** applying the plan SHALL require existing Pinax approval, snapshot, record ledger, index update, and evidence gates.

#### Scenario: Plugin attempts direct file write
- **WHEN** a plugin declares or attempts host filesystem write outside its temp directory
- **THEN** Pinax SHALL fail with `plugin_filesystem_denied`
- **AND** no vault file or structured asset SHALL be modified.

### Requirement: Plugin audit evidence is redacted
Pinax SHALL audit plugin lifecycle and execution without persisting sensitive payloads.

#### Scenario: Audit plugin execution
- **WHEN** a plugin execution finishes or fails
- **THEN** Pinax SHALL append a redacted event to `.pinax/events/plugin-audit.jsonl`
- **AND** the event SHALL include plugin id, version, runtime kind, capability id, permission grant ids, input hash, output hash, duration, status, and error code when present
- **AND** it SHALL NOT include raw note bodies, provider payloads, raw prompts, Authorization headers, cookies, tokens, webhook URLs, environment values, or full chain-of-thought.

