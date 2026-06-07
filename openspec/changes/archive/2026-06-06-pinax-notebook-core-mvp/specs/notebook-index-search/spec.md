## ADDED Requirements

### Requirement: Local index database is initialized and rebuilt through Pinax
Pinax SHALL create and maintain a local SQLite/GORM index projection for notebook search and organization without making the database the source of truth.

#### Scenario: Initialize index database
- **WHEN** a user runs `pinax index init --vault ./my-notes --json`
- **THEN** Pinax SHALL create `.pinax/index.sqlite` through the application service
- **AND** the database SHALL contain schema metadata for the supported index version
- **AND** stdout SHALL include index path, schema version, and status facts.

#### Scenario: Rebuild index with full note projection
- **WHEN** a user runs `pinax index rebuild --vault ./my-notes --json`
- **THEN** Pinax SHALL scan Markdown notes inside the vault boundary and rebuild note, text, tag, token, link, and attachment projections through GORM
- **AND** system index notes SHALL be marked so ordinary note statistics and orphan detection can exclude them.

#### Scenario: Index status reports freshness
- **WHEN** a user runs `pinax index status --vault ./my-notes --json`
- **THEN** Pinax SHALL report `fresh`, `stale`, `missing`, or `unreadable`
- **AND** stale results SHALL include evidence such as changed note path, modified time, size, or content hash.

### Requirement: Search uses local index with safe fallbacks
Pinax SHALL search the local notebook using the index when fresh and degrade to local scan or ripgrep fallback when needed.

#### Scenario: Search through fresh index
- **WHEN** a user runs `pinax search "认证" --tag auth --kind reference --limit 20 --vault ./my-notes --json`
- **THEN** Pinax SHALL query the local index projection first
- **AND** stdout SHALL include `engine=index`, `index_status=fresh`, total count, returned count, selected filters, result scores, matched fields, snippets, and note projections.

#### Scenario: Search with stale index warning
- **WHEN** the index is stale and a user runs `pinax search "认证" --allow-stale --vault ./my-notes --json`
- **THEN** Pinax SHALL return index results with status `partial`
- **AND** stdout SHALL include `index_status=stale` and an action recommending `pinax index rebuild`.

#### Scenario: Search falls back without index
- **WHEN** `.pinax/index.sqlite` is missing and a user runs `pinax search "认证" --vault ./my-notes --json`
- **THEN** Pinax SHALL use `rg` when available or in-process scan otherwise
- **AND** facts SHALL identify the fallback engine without requiring external network access.

### Requirement: Search filters cover notebook organization dimensions
Pinax SHALL let users combine full-text query with local notebook filters.

#### Scenario: Filter search by organization dimensions
- **WHEN** a user runs `pinax search "设计" --group work --folder architecture --kind reference --status active --vault ./my-notes --json`
- **THEN** Pinax SHALL return only matching notes
- **AND** JSON facts SHALL include stable keys for group, folder, kind, and status filters.

#### Scenario: Filter search by links and attachments
- **WHEN** a user runs `pinax search "" --link-target "Auth" --has-attachment --vault ./my-notes --json`
- **THEN** Pinax SHALL return notes with matching resolved or unresolved link targets and at least one attachment reference
- **AND** each result SHALL include link and attachment summary counts.

#### Scenario: Invalid search filter fails clearly
- **WHEN** a user runs `pinax search "x" --updated-after not-a-date --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `invalid_date_filter`
- **AND** no index database or Markdown file SHALL be modified.

### Requirement: Agent organize suggestions are reviewable plans
Pinax SHALL let agents generate local organize suggestions as reviewable plans rather than directly editing notes.

#### Scenario: Generate organize suggestions
- **WHEN** an agent runs `pinax organize suggest --vault ./my-notes --save --json`
- **THEN** Pinax SHALL read notes and index projection through application services
- **AND** it SHALL save `.pinax/organize-plans/<plan_id>.json` through the service
- **AND** the plan SHALL include operations with kind, mode, risk, path, target, reason, and evidence.

#### Scenario: Agent output exposes low-token organize facts
- **WHEN** an agent runs `pinax organize suggest --vault ./my-notes --agent`
- **THEN** stdout SHALL include stable key=value lines for plan id, operation counts, automatic count, manual review count, risk counts, and save path when present
- **AND** stdout SHALL NOT include localized prose, raw prompts, provider payloads, or secrets.

#### Scenario: Apply saved organize plan with snapshot protection
- **WHEN** a user runs `pinax organize apply --plan organize_123 --yes --snapshot-message "整理前快照" --vault ./my-notes --json`
- **THEN** Pinax SHALL ensure a Git snapshot exists or create one with the supplied message
- **AND** it SHALL apply only approved low-risk operations through the application service
- **AND** it SHALL refresh the local index after successful writes.

#### Scenario: Reject stale organize plan
- **WHEN** a saved organize plan source facts no longer match the current vault
- **AND** a user runs `pinax organize apply --plan organize_123 --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `plan_stale`
- **AND** stdout SHALL include an action recommending `pinax organize suggest --save`.

### Requirement: Organize suggestions are explainable and conservative
Pinax SHALL base automatic organization suggestions on local evidence and avoid high-risk mutations.

#### Scenario: Suggest metadata and path operations with evidence
- **WHEN** `pinax organize suggest --vault ./my-notes --json` analyzes notes with missing kind, missing status, or mismatched folder
- **THEN** suggested operations SHALL include evidence from title, tags, project metadata, current path, links, or saved views
- **AND** each operation SHALL be classified as automatic, manual_review, low, medium, or review risk.

#### Scenario: High-risk operations require manual review
- **WHEN** organize suggestion detects duplicate titles, possible merges, destructive deletes, body link rewrites, or broad folder moves
- **THEN** Pinax SHALL emit manual_review operations
- **AND** `organize apply` SHALL NOT perform those operations automatically.
