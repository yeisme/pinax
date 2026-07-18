## ADDED Requirements

### Requirement: Agent plans bind mutations to canonical object identities
Every Agent-originated mutation plan SHALL resolve target objects to canonical IDs and record the expected current revision and path before approval. Path-only write plans SHALL be rejected for already managed objects.

#### Scenario: Object moves after plan creation
- **WHEN** an Agent creates a metadata or organization plan and the target object moves before apply
- **THEN** apply SHALL resolve the same object by ID, compare expected revision and locator facts, and either safely rebase the locator or reject the stale plan without mutating another object at the old path.

#### Scenario: Agent proposes a new note
- **WHEN** an Agent proposes creation of a new note
- **THEN** the approved application service SHALL allocate the UUID once, write ledger/frontmatter/index facts with that value and return it in the receipt; the Agent SHALL NOT supply arbitrary machine metadata files.

### Requirement: Proof receipts connect identity, revision and sync evidence
Successful Agent writes SHALL emit redacted receipts containing command, object ID, before and after revision evidence, ledger sequence, snapshot reference, changed paths and sync readiness without including raw private bodies or credentials.

#### Scenario: Approved Tag update completes
- **WHEN** an Agent applies an approved Tag change to a managed note
- **THEN** the receipt SHALL identify the note by canonical object ID, show the current path and record version, and provide a real next command for sync or restore where applicable.
