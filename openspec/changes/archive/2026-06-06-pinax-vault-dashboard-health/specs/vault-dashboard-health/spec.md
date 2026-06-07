## ADDED Requirements

### Requirement: Vault statistics are available from the CLI
Pinax SHALL compute local Markdown vault statistics without requiring network access, provider credentials, or cloud services.

#### Scenario: Render human vault statistics
- **WHEN** a user runs `pinax stats --vault ./my-notes`
- **THEN** stdout SHALL contain a concise Chinese summary of note count, tag count, directory distribution, frontmatter coverage, recent update activity, and index status
- **AND** diagnostics SHALL go to stderr.

#### Scenario: Render JSON vault statistics
- **WHEN** a user runs `pinax stats --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope for `stats`
- **AND** the envelope SHALL include schema version, command name, vault facts, metric values, scan duration, index status, and next actions
- **AND** stdout SHALL NOT contain human prose outside the JSON envelope.

#### Scenario: Statistics degrade when index is missing
- **WHEN** a vault has Markdown notes but no `.pinax/index.sqlite`
- **THEN** `pinax stats --vault ./my-notes --json` SHALL still compute Markdown scan metrics
- **AND** the projection SHALL report `index_status=missing` with a runnable `pinax index rebuild` next action.

### Requirement: Vault health checks identify actionable note issues
Pinax SHALL audit local Markdown notes for maintainability issues and return stable issue codes, severities, evidence, and next actions.

#### Scenario: Detect common note health issues
- **WHEN** a user runs `pinax doctor --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope for `doctor`
- **AND** the envelope SHALL include issues for missing title, missing tags, missing Pinax metadata, duplicate title, empty note, stale note, orphan note, path anomaly, and stale index when those facts are present
- **AND** each issue SHALL include `issue_code`, `severity`, affected note path or note id when available, evidence, and next actions.

#### Scenario: Doctor is read-only by default
- **WHEN** a user runs `pinax doctor --vault ./my-notes`
- **THEN** Pinax SHALL NOT modify Markdown files, `.pinax/` assets, Git state, provider state, or remote services
- **AND** suggested fixes SHALL be returned as runnable commands rather than applied changes.

#### Scenario: Agent output is stable for automation
- **WHEN** a user runs `pinax doctor --vault ./my-notes --agent`
- **THEN** stdout SHALL contain stable key-value records or the project-approved agent format
- **AND** it SHALL include total issue counts by severity, machine-readable issue codes, and next actions
- **AND** diagnostics SHALL go to stderr.

### Requirement: Dashboard exposes a readonly local vault view
Pinax SHALL provide a local dashboard that visualizes vault statistics and health without writing to the vault.

#### Scenario: Start dashboard on localhost
- **WHEN** a user runs `pinax dashboard --vault ./my-notes --port 0`
- **THEN** Pinax SHALL start an HTTP server bound to localhost only
- **AND** stderr SHALL show the local URL
- **AND** the server SHALL expose readonly views for statistics, health issues, recent activity, and index status.

#### Scenario: Dashboard data reuses application projections
- **WHEN** the dashboard serves its data endpoints
- **THEN** it SHALL call the same application services used by `pinax stats` and `pinax doctor`
- **AND** it SHALL NOT scan the vault through duplicated command-layer or UI-layer business logic.

#### Scenario: Dashboard does not expose sensitive data
- **WHEN** the dashboard reads `.pinax/` state or event evidence
- **THEN** rendered HTML and JSON data SHALL NOT include provider tokens, webhook URLs, cookies, Authorization headers, raw provider payloads, or unredacted traces.

### Requirement: Vault analytics respect path and output boundaries
Pinax SHALL keep vault analytics inside the configured vault root and SHALL render outputs through the shared output contract.

#### Scenario: Reject paths outside the vault
- **WHEN** a vault note, link, or dashboard request attempts to resolve outside the vault root
- **THEN** Pinax SHALL reject the access with a stable error code
- **AND** it SHALL NOT read or render files outside the vault.

#### Scenario: Structured outputs keep stdout clean
- **WHEN** `pinax stats`, `pinax doctor`, or dashboard data export is run with `--json` or `--agent`
- **THEN** stdout SHALL contain only the selected machine-readable format
- **AND** diagnostics, dashboard listening URLs, and warnings SHALL go to stderr.
