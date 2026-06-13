## MODIFIED Requirements

### Requirement: Repair plans convert doctor issues into reviewable operations
Pinax SHALL convert vault health issues into a repair plan without modifying Markdown files by default. Repair planning SHALL include record ledger issues such as missing records, missing files, frontmatter mirror conflicts, note id conflicts, schema type conflicts, tombstones, and event replay failures.

#### Scenario: Generate repair plan as JSON
- **WHEN** a user runs `pinax repair plan --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope for `repair.plan`
- **AND** the envelope SHALL include schema version, plan id, source doctor facts, record ledger facts, operations, risk levels, skipped issues, scan duration, and next actions
- **AND** Markdown files, `.pinax/` assets, Git state, provider state, and remote services SHALL remain unchanged unless `--save` is provided.

#### Scenario: Save repair plan through CLI service
- **WHEN** a user runs `pinax repair plan --vault ./my-notes --save --json`
- **THEN** Pinax SHALL write `.pinax/repair-plans/<plan_id>.json` through the application service
- **AND** the plan SHALL include `schema_version=pinax.repair_plan.v1`, issue snapshot, record issue snapshot, operations, status, expiry, and redacted evidence
- **AND** stdout SHALL contain the saved plan path.

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

## ADDED Requirements

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
