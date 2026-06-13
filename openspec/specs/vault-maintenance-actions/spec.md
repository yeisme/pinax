# vault-maintenance-actions Specification

## Purpose

描述 Pinax 将 vault health issue 转换为可审查、可保存、可回滚的本地 repair plan，并在显式审批和 Git snapshot 保护下应用低风险维护动作。
## Requirements
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

### Requirement: Repair apply is explicit and Git protected
Pinax SHALL apply repair operations only after explicit approval and Git snapshot protection.

#### Scenario: Refuse apply without approval
- **WHEN** a user runs `pinax repair apply --vault ./my-notes --plan <plan_id> --json`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** no Markdown files, `.pinax/` assets, Git state, provider state, or remote services SHALL be modified.

#### Scenario: Refuse apply without snapshot
- **WHEN** a user runs `pinax repair apply --vault ./my-notes --plan <plan_id> --yes --json` without recent Pinax Git snapshot evidence
- **THEN** Pinax SHALL fail with stable error code `snapshot_required`
- **AND** the projection SHALL include a runnable `pinax git snapshot` next action.

#### Scenario: Apply low-risk repair operations
- **WHEN** a saved repair plan contains low-risk operations and the user runs `pinax repair apply --vault ./my-notes --plan <plan_id> --yes --json` after snapshot protection
- **THEN** Pinax SHALL apply only approved low-risk operations inside the vault boundary
- **AND** it SHALL append redacted event evidence
- **AND** stdout SHALL contain operation results, skipped operations, and next actions.

### Requirement: Repair plans separate automatic and manual actions
Pinax SHALL distinguish safe automatic operations from manual review recommendations, and SHALL treat note identity conflicts, lifecycle ambiguity, and destructive Markdown changes as manual review unless an operation has deterministic ledger evidence.

#### Scenario: Duplicate titles require manual review
- **WHEN** doctor reports duplicate titles
- **THEN** `pinax repair plan` SHALL create manual-review operations rather than automatically merging, deleting, or renaming notes.

#### Scenario: Empty notes are not deleted automatically
- **WHEN** doctor reports an empty note
- **THEN** `pinax repair plan` SHALL NOT create an automatic delete operation
- **AND** it SHALL return a manual-review next action for the affected note.

#### Scenario: Index repair can be automatic
- **WHEN** doctor reports `index_stale` or `index_missing`
- **THEN** `pinax repair plan` MAY include an automatic `index_rebuild` operation
- **AND** `repair apply` SHALL route through the same index service as `pinax index rebuild`.

#### Scenario: Low-risk ledger mirror repair can be automatic
- **GIVEN** the ledger record is valid and a Markdown frontmatter mirror is missing a Pinax-managed field
- **WHEN** doctor reports `record_frontmatter_missing`
- **THEN** `pinax repair plan` MAY include an automatic mirror repair operation
- **AND** `repair apply` SHALL require explicit approval and Git snapshot protection before writing the Markdown file.

#### Scenario: Note id conflicts require manual review
- **WHEN** doctor reports `note_id_conflict` across multiple Markdown files or ledger records
- **THEN** `pinax repair plan` SHALL create manual-review operations
- **AND** it SHALL NOT automatically reassign note ids, merge records, delete files, or rewrite frontmatter.

### Requirement: Repair plans detect stale inputs
Pinax SHALL reject repair apply when the saved plan no longer matches the vault facts it was generated from.

#### Scenario: Plan becomes stale after note changes
- **WHEN** a note included in a repair plan changes after plan creation
- **AND** the user runs `pinax repair apply --vault ./my-notes --plan <plan_id> --yes --json`
- **THEN** Pinax SHALL fail with stable error code `plan_stale`
- **AND** the projection SHALL include a runnable `pinax repair plan --vault ./my-notes --save` next action.

### Requirement: Record repair apply preserves ledger invariants
Pinax SHALL apply record repair operations only through the record ledger service and SHALL preserve sequence, schema version, idempotency, and lifecycle transition rules.

#### Scenario: Apply record-only repair
- **WHEN** a saved repair plan contains an approved record-only operation and the user runs `pinax repair apply --vault ./my-notes --plan <plan_id> --yes --json` after snapshot protection
- **THEN** Pinax SHALL append record repair events through the ledger service
- **AND** it SHALL update registry projections without directly hand-writing record JSON from the command layer.

#### Scenario: Reject stale record repair plan
- **GIVEN** the record ledger sequence or Markdown content hash differs from the saved repair plan evidence
- **WHEN** a user runs `pinax repair apply --vault ./my-notes --plan <plan_id> --yes --json`
- **THEN** Pinax SHALL fail with stable error code `plan_stale`
- **AND** it SHALL include a next action to regenerate the repair plan.

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

### Requirement: Maintenance plans include version and asset repair operations
Pinax SHALL include version evidence, asset manifest consistency, asset link consistency, and index object lookup issues in repair planning.

#### Scenario: Plan asset consistency repair
- **WHEN** `pinax repair plan --vault ./my-notes --json` detects missing asset files, changed content hashes, orphan asset manifest entries, or dangling note asset links
- **THEN** stdout SHALL include repair operations with asset id, path, issue code, risk, evidence, and required approval level.

#### Scenario: Plan version snapshot before risky repair
- **WHEN** a repair operation would move, remove, restore, or rewrite a note or asset file
- **THEN** Pinax SHALL require version snapshot protection
- **AND** next actions SHALL recommend `pinax version snapshot`, not `pinax git snapshot`.

### Requirement: Asset and version apply operations are protected
Pinax SHALL require explicit approval and snapshot evidence before applying high-risk asset or version repair operations.

#### Scenario: Reject asset repair apply without snapshot
- **WHEN** a saved repair plan contains asset move/remove/restore operations
- **AND** a user runs `pinax repair apply --vault ./my-notes --plan <plan_id> --yes --json` without recent version snapshot evidence
- **THEN** Pinax SHALL fail with stable error code `snapshot_required`
- **AND** stdout SHALL include a runnable `pinax version snapshot` next action.

#### Scenario: Projection-only repair remains low risk
- **WHEN** a saved repair plan contains only index projection rebuild or stale row cleanup operations
- **THEN** Pinax MAY apply them without modifying Markdown or asset files
- **AND** stdout SHALL state that no version snapshot was required because only rebuildable projection state changed.

