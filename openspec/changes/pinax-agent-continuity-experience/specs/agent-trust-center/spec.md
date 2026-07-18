## ADDED Requirements

### Requirement: Dashboard SHALL expose a read-only Agent Trust Center
The localhost Dashboard SHALL expose additive Agent Trust Center sections and JSON projections for continuity, Memory Inbox, Agent activity, proof receipts, restore hints, adapter health and trust metrics. All routes SHALL be loopback-oriented and read-only.

#### Scenario: User opens the Trust Center
- **WHEN** the user runs `pinax vault dashboard --vault ./my-notes --port 4180`
- **THEN** the page SHALL show bounded recent Agent activity, pending review counts, context source coverage, proposal decisions, reversible receipts and adapter status
- **AND** it SHALL not display full note bodies or create a browser-owned write path.

#### Scenario: Browser sends a write request
- **WHEN** a client sends POST, PUT, PATCH or DELETE to a Trust Center route
- **THEN** Pinax SHALL return `405`
- **AND** no Markdown, memory, receipt, Git, sync, provider or remote state SHALL change.

### Requirement: Agent activity SHALL explain reads and writes without exposing private payloads
Activity projection SHALL identify principal, capability, bounded scope, action kind, status, timestamp, affected object counts, proposal or receipt references and safe next commands. It SHALL NOT persist or emit raw model prompts, complete transcripts or provider payloads.

#### Scenario: Agent generated a memory proposal
- **WHEN** the activity feed includes a proposal event
- **THEN** the Trust Center SHALL show who proposed it, its scope, risk, source coverage and review status
- **AND** it SHALL link to a copyable `pinax review` command rather than a fake approval button.

### Requirement: Trust metrics SHALL be local, redacted and rebuildable
Pinax SHALL aggregate local continuity metrics from receipts and lifecycle events, including context reuse, source resolvability, proposal acceptance, handoff continuation, silent promotion and stale/conflict resolution. Metrics SHALL be rebuildable projections and SHALL NOT be synced as private model state by default.

#### Scenario: Metrics are rebuilt
- **WHEN** the local metrics projection is missing or corrupt
- **THEN** Pinax SHALL rebuild it from eligible redacted events or report unavailable status
- **AND** core memory, continuity and proof workflows SHALL remain operational.

#### Scenario: Silent promotion is detected
- **WHEN** a memory reaches confirmed state without an allowed owner/service approval receipt
- **THEN** the metric SHALL record a silent-promotion violation
- **AND** the Trust Center SHALL surface a high-severity diagnostic and safe investigation command.

### Requirement: Product evidence metrics SHALL expose denominator and cohort quality
Trust metrics used for product decisions SHALL distinguish unique participants from runs, include the observation window and failure categories, and SHALL NOT count repeated internal runs as additional target users.

#### Scenario: One participant runs continuity repeatedly
- **WHEN** the same redacted participant ID completes multiple runs during the observation window
- **THEN** run-level metrics SHALL include every eligible run while participant-level completion and reuse metrics SHALL count that participant once
- **AND** the projection SHALL expose both denominators.

#### Scenario: Product decision data is incomplete
- **WHEN** seven-day reuse, failure classification, source coverage or sample size is unavailable
- **THEN** the Trust Center SHALL mark the decision dataset incomplete
- **AND** it SHALL NOT emit a maturity recommendation based only on command volume or successful fixture runs.

### Requirement: Trust Center SHALL degrade by section
A failure in one upstream projection SHALL NOT blank the complete dashboard. Each failed section SHALL provide a bounded error and a real diagnostic command while unaffected sections remain available.

#### Scenario: Adapter health projection fails
- **WHEN** adapter capability discovery returns an error
- **THEN** the Trust Center SHALL mark the adapter section degraded
- **AND** Memory Inbox, receipts and local continuity facts SHALL remain visible.

### Requirement: Trust Center contracts SHALL remain additive
New routes and fields SHALL be versioned and optional for existing dashboard and API consumers. Existing dashboard routes and visibility behavior SHALL remain unchanged.

#### Scenario: Old dashboard client ignores Trust Center fields
- **WHEN** an existing client consumes the previous dashboard projection shape
- **THEN** additive Trust Center fields SHALL be safely ignorable
- **AND** no existing required field, method, path or status semantic SHALL be removed or repurposed.
