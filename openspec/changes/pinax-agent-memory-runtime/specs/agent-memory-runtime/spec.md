## ADDED Requirements

### Requirement: Pinax SHALL expose a provider-neutral Agent memory runtime

Pinax SHALL expose versioned principal, scope, memory, source, context, proposal, handoff and feedback contracts that do not depend on one Agent runtime or model provider. The runtime SHALL be available through shared application services and additive CLI, MCP, REST/RPC and Go SDK adapters.

#### Scenario: Generic context request succeeds
- **WHEN** a registered Agent principal requests context for an allowed project scope with a bounded budget
- **THEN** Pinax SHALL return `pinax.agent_context.pack.v1` containing eligible memories, conflicts, source refs, freshness and next actions
- **AND** the response SHALL declare body exposure and truncation without returning full private note bodies.

#### Scenario: Runtime-specific fields remain optional
- **WHEN** a Codex or Ordo adapter adds runtime metadata to a request
- **THEN** the core SHALL process known common fields and optional adapter extensions
- **AND** runtime metadata SHALL NOT change memory identity or lifecycle semantics.

### Requirement: Pinax SHALL own canonical memory lifecycle and scope

Pinax SHALL persist memory state with stable ID, kind, scope, status, confidence, source refs, creator principal, timestamps, optional expiration, supersession and conflict facts. Supported lifecycle SHALL include proposed, confirmed, rejected, superseded, expired and conflicted.

#### Scenario: Existing memory row remains readable
- **GIVEN** a vault contains an existing fact, decision, event or task row created by `pinax memory capture`
- **WHEN** the Agent memory runtime reads the ledger after additive migration
- **THEN** it SHALL expose the row through the common memory view without changing its ID or content
- **AND** the existing `pinax memory` command SHALL continue to read it.

#### Scenario: Scope prevents cross-project recall
- **WHEN** a principal requests project A context and a confirmed memory belongs only to project B
- **THEN** Pinax SHALL exclude that memory unless explicit scope inheritance or permission allows it
- **AND** it SHALL not expose private source metadata in the exclusion result.

### Requirement: Context compilation SHALL be deterministic, explainable and bounded

The context compiler SHALL apply permission, scope, lifecycle, source visibility, exact entity and project match, source authority, confidence, freshness, task fitness, existing semantic/keyword projections, conflict grouping and budget in a documented order.

#### Scenario: Context exceeds budget
- **WHEN** eligible results exceed `max_items` or `max_chars`
- **THEN** Pinax SHALL return a deterministic subset with `truncated=true`
- **AND** it SHALL provide ranking reasons and safe next commands for drill-down.

#### Scenario: Conflicting claims are recalled
- **WHEN** two confirmed records in the same scope conflict and both retain valid evidence
- **THEN** Pinax SHALL return a conflict group containing both records
- **AND** ranking SHALL NOT silently present one as uncontested truth.

### Requirement: Agent memory writes SHALL be proposal-first

Agent-originated changes SHALL create proposals by default. Proposal review SHALL validate principal capability, scope, source, duplicates, conflicts and policy before any confirmed lifecycle mutation.

#### Scenario: Agent submits unsourced preference
- **WHEN** an Agent proposes an owner-level preference without explicit user confirmation or source evidence
- **THEN** Pinax SHALL save it as proposed or return `approval_required`
- **AND** default confirmed recall SHALL exclude it.

#### Scenario: Owner approves a proposal
- **WHEN** the owner approves a valid proposal with `--yes`
- **THEN** Pinax SHALL persist the confirmed memory and source relations in one service-owned transaction
- **AND** it SHALL write a redacted receipt containing proposal ID, memory ID and lifecycle transition.

### Requirement: Handoff and feedback SHALL be bounded and separately governed

Pinax SHALL store bounded handoff packages and recall feedback without treating either as automatically confirmed memory. Handoff SHALL carry objective, state, decisions, completed work, blockers, verification, follow-ups and source refs.

#### Scenario: Cross-Agent handoff is consumed
- **WHEN** one registered Agent creates a handoff and another allowed Agent reads it
- **THEN** Pinax SHALL return the same common handoff semantics through the consumer adapter
- **AND** it SHALL not require or expose the producer's complete transcript.

#### Scenario: Agent marks recalled memory irrelevant
- **WHEN** an Agent submits `irrelevant` feedback for one recalled memory
- **THEN** Pinax SHALL append feedback evidence for future ranking and maintenance
- **AND** it SHALL not silently change the confirmed memory content or status.

### Requirement: Existing memory and brain surfaces SHALL remain compatible

The new Agent memory runtime SHALL be additive. Existing `pinax memory`, `pinax brain`, `pinax.brain.*` MCP tools, Local API routes and stored ledger rows SHALL remain available during this change.

#### Scenario: Old and new read paths coexist
- **WHEN** the same confirmed memory is queried through `pinax memory recall` and `pinax agent memory recall`
- **THEN** both commands SHALL resolve the same underlying record ID and source evidence
- **AND** differences SHALL be limited to the documented projection envelope for each command.

#### Scenario: Runtime is disabled
- **WHEN** the experimental Agent runtime capability is disabled or rolled back
- **THEN** existing memory and brain commands SHALL continue to operate
- **AND** additive schema objects SHALL not be destructively removed.
