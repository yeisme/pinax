# agent-continuity-experience Specification

## Purpose
TBD - created by archiving change pinax-agent-continuity-experience. Update Purpose after archive.
## Requirements
### Requirement: Pinax SHALL expose an intent-level Agent continuity command
Pinax SHALL expose experimental `pinax continue` as an additive facade over the provider-neutral Agent memory runtime. It SHALL resolve an explicit principal and bounded workspace, project, session or task scope before retrieval and SHALL NOT replace or change existing `pinax agent context` behavior.

#### Scenario: A second Agent continues project work
- **WHEN** Agent B runs `pinax continue --principal agent:codex:local-primary --project pinax --task "prepare v0.2 release" --budget 6000 --vault ./my-notes --json` after Agent A created a valid handoff and approved memories
- **THEN** Pinax SHALL return a versioned Continuity Pack with objective, current state, decisions, preferences, open commitments, failed attempts, blockers, conflicts, source refs, freshness, body exposure, truncation and next commands
- **AND** the response SHALL identify the consumed handoff without returning a complete prior transcript.

#### Scenario: No handoff exists
- **WHEN** a valid principal requests continuity for an allowed scope with no matching handoff
- **THEN** Pinax SHALL degrade to context-only continuity
- **AND** it SHALL return `handoff_status=missing` with a concrete next action instead of failing the entire request.

### Requirement: Continuity compilation SHALL be permission-first, source-backed and bounded
Pinax SHALL apply principal permission, scope, lifecycle, conflict, source authority, freshness and deterministic budget limits before emitting Continuity Pack content. Similarity SHALL NOT override permission or source eligibility.

#### Scenario: Relevant memory is outside allowed scope
- **WHEN** a semantically relevant memory belongs to another project that the principal cannot read
- **THEN** Pinax SHALL exclude the memory before ranking and rendering
- **AND** output and metrics SHALL NOT reveal its content, title, source path or existence beyond an allowed aggregate denial fact.

#### Scenario: Context exceeds budget
- **WHEN** eligible continuity content exceeds the requested budget
- **THEN** Pinax SHALL truncate deterministically by section and priority
- **AND** it SHALL return omitted counts, `truncated=true` and drill-down commands without silently dropping conflict or source coverage warnings.

### Requirement: Continuity evidence SHALL distinguish verified, stale and unresolved sources
Every continuity item SHALL preserve memory/source identifiers and resolvability facts. Missing or drifted source evidence SHALL reduce trust state instead of being presented as verified fact.

#### Scenario: Source revision no longer resolves
- **WHEN** a recalled decision references a missing path or mismatched source revision
- **THEN** Pinax SHALL mark the item stale or unresolved
- **AND** it SHALL include a safe refresh, search or review command rather than fabricating replacement evidence.

### Requirement: Continuity surfaces SHALL remain additive and experimental
New CLI, JSON, agent, event and HTTP fields for continuity SHALL be marked experimental and versioned. Existing `agent`, `memory`, `brain`, `proof`, MCP, API and stored ledger consumers SHALL continue to work without adopting the new facade.

#### Scenario: Continuity capability is disabled
- **WHEN** the new capability registration is disabled or rolled back
- **THEN** existing commands and routes SHALL retain their documented behavior
- **AND** stored additive continuity data SHALL remain readable or safely ignored without destructive migration.

### Requirement: Pinax SHALL provide a five-minute cross-Agent proof flow
Pinax SHALL provide a fixture-backed integration entry point that exercises Agent A context, proposal and handoff, owner review and approval, and Agent B continuation with source and task assertions.

#### Scenario: Cross-Agent proof flow succeeds
- **WHEN** the integration flow runs against a temporary fixture vault and reference adapters
- **THEN** Agent B SHALL receive the approved decision, unresolved blocker, open commitment and resolvable sources required to continue
- **AND** the run SHALL write redacted evidence under `temp/integration-test-runs/<run-id>/` with the original exit code preserved.

### Requirement: Product validation SHALL gate scope expansion and maturity
Pinax SHALL keep the Agent Continuity facade experimental and under scope freeze until a real target-user cohort produces a reviewable Go, Iterate or Stop decision. Engineering completion, fixture success, downloads, command invocations or positive feature interest SHALL NOT independently satisfy this gate.

#### Scenario: A new adjacent capability is proposed before product validation
- **WHEN** dogfood, seven-day reuse analysis and the CEO decision receipt are incomplete
- **THEN** the change SHALL reject new top-level intent commands, provider categories, team collaboration, editor, publishing or automation-platform scope
- **AND** it MAY accept only safety, correctness, onboarding, source resolution, recovery and compatibility fixes that unblock the existing proof flow.

#### Scenario: Product owner evaluates the wedge
- **WHEN** the target-user observation window ends
- **THEN** the decision evidence SHALL report participant and run sample sizes, completion, seven-day reuse, continuation success, source resolvability, silent promotion, re-explanation change, willingness-to-pay signal, failure distribution and known bias
- **AND** the owner SHALL record exactly one outcome: `go`, `iterate` or `stop`, with a bounded next change or shutdown action.

#### Scenario: Evidence supports iteration but not expansion
- **WHEN** user value exists but failures are concentrated in no more than two correctable stages
- **THEN** the next change SHALL be limited to those named failure classes
- **AND** it SHALL NOT add unrelated knowledge-base capabilities as a substitute for fixing the observed problem.

