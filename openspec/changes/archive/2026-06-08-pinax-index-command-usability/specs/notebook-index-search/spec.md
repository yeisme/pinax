## ADDED Requirements

### Requirement: Index commands guide local maintenance decisions
Pinax SHALL make `pinax index` a decision-oriented maintenance surface that explains the current index state, affected workflows, and the safest next command without requiring users to infer state transitions from implementation details.

#### Scenario: Default index command summarizes status
- **WHEN** a user runs `pinax index --vault ./my-notes`
- **THEN** Pinax SHALL render a concise Chinese summary containing the index status, index path, note count when available, freshness evidence, affected workflows, and one recommended next command
- **AND** it SHALL NOT write `.pinax/index.sqlite`, Markdown files, event files, Git state, provider state, or remote services.

#### Scenario: Default index command preserves machine contracts
- **WHEN** a user runs `pinax index --vault ./my-notes --json` or `pinax index --vault ./my-notes --agent`
- **THEN** Pinax SHALL emit the same command projection contract as an index summary command with stable English keys including `index_status`, `path`, `schema_version`, `notes`, `recommended_action`, and `writes=false`
- **AND** localized Chinese labels SHALL NOT appear in `--agent` keys or JSON field names.

#### Scenario: Missing index recommends bounded recovery
- **WHEN** the index database is missing and a user runs `pinax index --vault ./my-notes`
- **THEN** Pinax SHALL recommend `pinax index refresh --vault ./my-notes` for ordinary recovery when the vault size is within the lazy refresh budget
- **AND** it SHALL recommend `pinax index rebuild --vault ./my-notes` when a full rebuild is required.

### Requirement: Index refresh is the default low-cost maintenance action
Pinax SHALL provide `pinax index refresh` as the preferred low-cost maintenance command for reconciling the local index projection when the vault can be repaired incrementally.

#### Scenario: Refresh skips unchanged notes
- **WHEN** a user runs `pinax index refresh --vault ./my-notes --json`
- **THEN** Pinax SHALL scan registered Pinax note facts, skip unchanged notes using ledger sequence, content hash, modified time, size, schema version, and projection row evidence where available
- **AND** stdout SHALL include stable facts for scanned notes, changed notes, skipped notes, indexed notes, deleted rows, failed rows, batch count, duration, and final `index_status`.

#### Scenario: Refresh creates missing index safely
- **WHEN** `.pinax/index.sqlite` is missing and a user runs `pinax index refresh --vault ./my-notes --json`
- **THEN** Pinax SHALL create the index database through the application service and index registered Pinax notes only
- **AND** unmanaged Markdown files without Pinax frontmatter SHALL remain excluded.

#### Scenario: Refresh reports partial failures without hiding them
- **WHEN** one or more notes cannot be parsed or indexed during `pinax index refresh`
- **THEN** Pinax SHALL return `status=partial`
- **AND** stdout SHALL include failed count, redacted evidence, affected paths, and next actions for `pinax index doctor` or `pinax index rebuild`.

### Requirement: Index doctor explains freshness and integrity problems
Pinax SHALL provide `pinax index doctor` to diagnose index availability, schema compatibility, freshness, row consistency, and projection health without mutating vault content.

#### Scenario: Doctor diagnoses stale index
- **WHEN** registered note facts differ from indexed facts and a user runs `pinax index doctor --vault ./my-notes --json`
- **THEN** Pinax SHALL report `status=partial`, issue counts grouped by code and severity, stale evidence, affected paths, and a recommended action
- **AND** it SHALL NOT modify the index database unless the user explicitly chooses a repair or refresh command.

#### Scenario: Doctor diagnoses unreadable index
- **WHEN** `.pinax/index.sqlite` exists but cannot be opened or migrated
- **AND** a user runs `pinax index doctor --vault ./my-notes --json`
- **THEN** Pinax SHALL report stable issue code `index_unreadable`
- **AND** it SHALL include a safe next action for `pinax index repair --kind recreate` or `pinax index rebuild` without printing raw stack traces or secrets.

#### Scenario: Doctor emits explainable human output
- **WHEN** a user runs `pinax index doctor --vault ./my-notes`
- **THEN** Pinax SHALL render Chinese sections for 状态, 问题, 证据, 影响, and 推荐下一步
- **AND** machine keys such as `schema_version` or `index_status` SHALL be localized in the default human output.

### Requirement: Index repair is bounded to projection-safe operations
Pinax SHALL provide index repair operations only for projection-safe maintenance and SHALL avoid changing Markdown note bodies, record ledger assets, Git state, provider state, or remote services.

#### Scenario: Repair previews projection-safe operations
- **WHEN** a user runs `pinax index repair --vault ./my-notes --dry-run --json`
- **THEN** Pinax SHALL return a repair preview with operation kind, mode, risk, target path, reason, and evidence
- **AND** `writes=false` SHALL be present in facts.

#### Scenario: Repair requires explicit approval for writes
- **WHEN** a user runs `pinax index repair --vault ./my-notes --kind recreate --json` without `--yes`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** no index database, Markdown file, event file, Git state, provider state, or remote service SHALL be modified.

#### Scenario: Repair recreates corrupt projection only after approval
- **WHEN** `pinax index doctor` reports a corrupt or unreadable projection
- **AND** a user runs `pinax index repair --vault ./my-notes --kind recreate --yes --json`
- **THEN** Pinax SHALL move or remove only the local index projection according to the selected repair policy, rebuild registered Pinax notes, and report final `index_status=fresh` when successful
- **AND** stdout SHALL include evidence for the old projection handling and the rebuilt index path.

### Requirement: Index output remains one projection across modes
Pinax SHALL render index summary, status, refresh, doctor, repair, sync, and rebuild output from a single command projection per command.

#### Scenario: Index commands support structured modes
- **WHEN** a user runs any index maintenance command with `--json`, `--agent`, or `--explain`
- **THEN** Pinax SHALL emit valid mode-specific output from the same projection
- **AND** `--json` stdout SHALL contain JSON only, `--agent` stdout SHALL contain stable key=value lines, and `--explain` SHALL be a Chinese reviewable summary with evidence references.

#### Scenario: Index events stream stays structured
- **WHEN** a user runs a long-running index command with `--events`
- **THEN** Pinax SHALL emit NDJSON start/progress/end or error events with monotonic sequence numbers
- **AND** progress events SHALL include bounded counts without writing ANSI, localized prose, or debug logs to stdout.
