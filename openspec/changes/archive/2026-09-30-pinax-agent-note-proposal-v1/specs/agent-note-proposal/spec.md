## ADDED Requirements

### Requirement: Note-domain Agent writes SHALL go through proposals and the proof loop

Agents SHALL express note-domain changes as proposals validated by an application service and applied only through the existing proof loop (snapshot, apply, receipt). No transport SHALL expose a direct file-write path for Agent principals, and handwritten structured metadata SHALL be rejected.

#### Scenario: Low-risk proposal auto-applies with receipt
- **WHEN** an Agent principal proposes an inbox capture, journal append or note draft and the proposal passes validation
- **THEN** Pinax SHALL apply it through the proof loop with a snapshot and SHALL emit a redacted receipt
- **AND** the created file SHALL NOT overwrite an existing note.

#### Scenario: Handwritten metadata is rejected
- **WHEN** a proposal payload contains client-assembled structured metadata that did not pass the Pinax parser
- **THEN** Pinax SHALL reject the proposal fail-closed and SHALL leave the vault unchanged.

### Requirement: Approval SHALL be risk-tiered with an explicit auto-apply list

Note edit proposals SHALL require user confirmation. The auto-apply list SHALL be limited to inbox capture, journal append and note draft, and SHALL be enforced in code without a configuration override in this change. High-risk organize and repair flows SHALL remain on their existing plan approval path.

#### Scenario: Note edit waits for confirmation
- **WHEN** an Agent proposes a note edit
- **THEN** the proposal SHALL enter pending review and SHALL NOT apply without explicit approval
- **AND** approval SHALL require the confirmation flag and SHALL produce a receipt for both approve and reject outcomes.

#### Scenario: High-risk flow is unchanged
- **WHEN** an Agent-driven change requires organize or repair operations
- **THEN** it SHALL route through the existing saved-plan approval path
- **AND** this change SHALL NOT add any new auto-apply entry for it.

### Requirement: Note edit proposals SHALL pin the expected revision

A note edit proposal SHALL carry the target object id, the expected content revision, a structured change and a bounded rationale. When the observed revision has drifted, Pinax SHALL reject with a stable `plan_stale` code and a next-action message.

#### Scenario: Drift is rejected with a next action
- **WHEN** a note edit proposal targets a revision older than the vault's current revision
- **THEN** Pinax SHALL reject the proposal with `plan_stale`
- **AND** the error SHALL include a bounded next action directing the Agent to re-read the card and re-propose.

#### Scenario: Rationale is bounded
- **WHEN** a proposal carries a rationale longer than the documented limit
- **THEN** Pinax SHALL truncate or reject it per the contract without storing the full text in the receipt.

### Requirement: Review surfaces SHALL be bounded and mode-consistent

The proposal list and show commands SHALL present rationale, risk tier, target reference and a diff summary without full note bodies by default, and SHALL behave consistently across the four output modes.

#### Scenario: Proposal list is bounded
- **WHEN** a user lists pending proposals in any output mode
- **THEN** each entry SHALL show rationale, risk tier and target object id with diff line counts only
- **AND** full bodies SHALL NOT appear unless explicitly requested through the existing display boundary.

#### Scenario: Preview without confirmation flag
- **WHEN** an approve command runs without the confirmation flag
- **THEN** Pinax SHALL show a preview of the resulting apply and SHALL NOT write anything.

### Requirement: MCP proposal tools SHALL be capability-gated

Write proposal tools SHALL NOT be registered when the Agent proposal capability is disabled. Read-only proposal listing SHALL remain available. When enabled, proposing SHALL return the proposal id and approval facts without auto-applying medium-risk changes.

#### Scenario: Default server hides write proposal tools
- **WHEN** the MCP server runs with default configuration
- **THEN** the proposal write tools SHALL NOT appear in tools/list
- **AND** read-only proposal listing SHALL remain exposed.

#### Scenario: Enabled capability proposes without silent apply
- **WHEN** the capability is enabled and an Agent proposes a note edit
- **THEN** Pinax SHALL return the proposal id with approval-required facts
- **AND** it SHALL NOT apply the edit in the same call.

### Requirement: Proposal outcomes SHALL feed dogfood evidence without bodies

Proposal counters SHALL be emitted to the dogfood evidence profile covering proposed, auto-applied, approved, rejected and stale outcomes, without note bodies or secrets.

#### Scenario: Counters are emitted
- **WHEN** proposals transition through their lifecycle during an evidence run
- **THEN** the profile SHALL emit per-outcome counters
- **AND** the evidence SHALL NOT contain full note bodies, tokens or provider payloads.
