## ADDED Requirements

### Requirement: Pinax SHALL expose a unified Memory Inbox projection
Pinax SHALL expose experimental `pinax review` and a shared Memory Inbox projection that aggregates pending proposals, conflicts, duplicates, stale or expired candidates and reversible receipts without creating a second canonical memory store.

#### Scenario: Owner reviews pending items
- **WHEN** the owner runs `pinax review --project pinax --vault ./my-notes --json`
- **THEN** Pinax SHALL return inbox items classified as new fact, preference, decision, lesson, commitment, conflict, duplicate or stale
- **AND** each item SHALL include reason codes, source coverage, scope, confidence, expiry, risk, suggested action and stable references.

### Requirement: Memory Inbox classification SHALL be deterministic and explainable
Inbox category, risk and suggested action SHALL be derived from versioned rules over proposal, lifecycle, source, conflict, feedback and receipt facts. The same eligible state SHALL produce semantically equivalent output across CLI and HTTP projections.

#### Scenario: Candidate duplicates confirmed memory
- **WHEN** a pending proposal matches a confirmed memory within the same scope and source authority
- **THEN** Pinax SHALL classify it as duplicate or possible duplicate with reason codes
- **AND** it SHALL NOT silently confirm, merge or delete either record.

#### Scenario: Candidate conflicts with an existing decision
- **WHEN** a pending decision contradicts a confirmed decision in an overlapping scope
- **THEN** Pinax SHALL classify the item as conflict with both source sets and lifecycle facts
- **AND** bulk approval SHALL be disabled for that item.

### Requirement: Review mutations SHALL reuse lifecycle and proof services
Any approve, reject, supersede, expire or restore action initiated from `pinax review` SHALL call existing application services and SHALL enforce their capability, stale-plan, snapshot, receipt, confirmation and restore rules. Dashboard projections SHALL remain read-only.

#### Scenario: Owner approves a low-risk sourced proposal
- **WHEN** the owner explicitly approves a current low-risk proposal with required confirmation
- **THEN** Pinax SHALL persist the lifecycle transition through the canonical service
- **AND** it SHALL return a redacted receipt and restore or supersession hint without writing directly from the CLI handler.

#### Scenario: Proposal changed after review
- **WHEN** the proposal content, source revision, scope or conflict state changed after the inbox item was rendered
- **THEN** Pinax SHALL reject the action as stale
- **AND** it SHALL require the owner to refresh review state before applying.

### Requirement: High-risk inbox items SHALL require individual review
Deletion, destructive rewrite, conflict resolution, cross-scope promotion, bulk supersession and restore SHALL NOT be approved through an undifferentiated bulk action.

#### Scenario: Bulk approval contains a conflict item
- **WHEN** a bulk approval request includes at least one conflict or high-risk item
- **THEN** Pinax SHALL exclude or reject the high-risk item with a stable reason
- **AND** low-risk items SHALL only proceed if the command explicitly supports partial success and reports per-item receipts.

### Requirement: Inbox output SHALL preserve privacy boundaries
Memory Inbox output SHALL NOT include full note bodies, complete transcripts, raw prompts, provider payloads, Authorization headers, cookies, credentials, hidden system prompts, private tool arguments or complete chain-of-thought.

#### Scenario: Proposal source contains private body content
- **WHEN** an inbox item references a private note
- **THEN** Pinax SHALL emit bounded source metadata and approved preview fields only
- **AND** it SHALL declare body exposure without leaking the private body.
