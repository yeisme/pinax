## ADDED Requirements

### Requirement: Repair plans convert doctor issues into reviewable operations
Pinax SHALL convert vault health issues into a repair plan without modifying Markdown files by default.

#### Scenario: Generate repair plan as JSON
- **WHEN** a user runs `pinax repair plan --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope for `repair.plan`
- **AND** the envelope SHALL include schema version, plan id, source doctor facts, operations, risk levels, skipped issues, scan duration, and next actions
- **AND** Markdown files, `.pinax/` assets, Git state, provider state, and remote services SHALL remain unchanged unless `--save` is provided.

#### Scenario: Save repair plan through CLI service
- **WHEN** a user runs `pinax repair plan --vault ./my-notes --save --json`
- **THEN** Pinax SHALL write `.pinax/repair-plans/<plan_id>.json` through the application service
- **AND** the plan SHALL include `schema_version=pinax.repair_plan.v1`, issue snapshot, operations, status, expiry, and redacted evidence
- **AND** stdout SHALL contain the saved plan path.

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
Pinax SHALL distinguish safe automatic operations from manual review recommendations.

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

### Requirement: Repair plans detect stale inputs
Pinax SHALL reject repair apply when the saved plan no longer matches the vault facts it was generated from.

#### Scenario: Plan becomes stale after note changes
- **WHEN** a note included in a repair plan changes after plan creation
- **AND** the user runs `pinax repair apply --vault ./my-notes --plan <plan_id> --yes --json`
- **THEN** Pinax SHALL fail with stable error code `plan_stale`
- **AND** the projection SHALL include a runnable `pinax repair plan --vault ./my-notes --save` next action.
