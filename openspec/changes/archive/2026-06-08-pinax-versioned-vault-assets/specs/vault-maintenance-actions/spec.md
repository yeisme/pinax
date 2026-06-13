## ADDED Requirements

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

