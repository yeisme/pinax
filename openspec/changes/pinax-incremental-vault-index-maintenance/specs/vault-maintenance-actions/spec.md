## MODIFIED Requirements

### Requirement: Repair plans convert doctor issues into reviewable operations
Pinax SHALL convert safe vault health and index consistency issues into reviewable repair plans without requiring agents to hand-write structured metadata.

#### Scenario: Create repair plan from doctor issues
- **WHEN** a user runs `pinax repair plan --vault ./my-notes --save --json`
- **THEN** Pinax SHALL create `.pinax/repair-plans/<plan_id>.json` through the application service
- **AND** the plan SHALL include operations derived from current vault health, metadata, index freshness, and index consistency evidence.

#### Scenario: Apply repair plan with approval
- **WHEN** a user runs `pinax repair apply --plan repair_123 --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL apply only approved low-risk operations through the application service
- **AND** it SHALL append redacted event evidence.

#### Scenario: Repair plan handles stale index facts
- **WHEN** doctor finds index stale, partial, missing, stale path rows, orphan tombstones, or ambiguous external move candidates
- **THEN** `pinax repair plan` SHALL create index sync, index rebuild, tombstone cleanup, or manual review operations as appropriate
- **AND** it SHALL NOT rewrite Markdown files automatically.

#### Scenario: Ambiguous move candidate requires manual review
- **WHEN** index reconciliation finds multiple possible source files for an external move or rename
- **THEN** `pinax repair plan` SHALL create a manual-review operation with candidates and evidence
- **AND** `repair apply` SHALL NOT choose one candidate automatically.

### Requirement: Repair applies safe index-only operations
Pinax SHALL allow safe index-only repair operations while preserving Markdown as the source of truth.

#### Scenario: Apply index sync operation
- **WHEN** a saved repair plan contains an index sync or stale path cleanup operation
- **AND** the user runs `pinax repair apply --plan repair_123 --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL update only `.pinax/index.sqlite` and CLI-authored event evidence
- **AND** it SHALL NOT modify Markdown note bodies.

#### Scenario: Reject stale repair plan after file changes
- **WHEN** a saved repair plan source facts no longer match the current vault
- **AND** a user runs `pinax repair apply --plan repair_123 --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `plan_stale`
- **AND** stdout SHALL include an action recommending a new repair plan or index sync.
