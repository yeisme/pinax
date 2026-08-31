## ADDED Requirements

### Requirement: Pinax SHALL expose a versioned adapter capability descriptor

Every Agent adapter SHALL expose runtime ID, adapter version, supported contract versions, transports, read/write capabilities, approval requirements, maximum context size and degraded behavior through `pinax.agent_adapter.descriptor.v1`.

#### Scenario: Adapter capability is discovered
- **WHEN** a consumer lists registered Agent runtimes
- **THEN** Pinax SHALL return each adapter descriptor through machine-readable output
- **AND** it SHALL not expose credentials, raw provider configuration or private tool arguments.

### Requirement: Adapter conversion SHALL not alter core semantics

Adapters MAY render context and lifecycle events into runtime-native forms, but SHALL preserve common memory IDs, scopes, lifecycle states, source refs and approval results. Adapter-specific metadata SHALL remain optional.

#### Scenario: Codex and Ordo exchange a handoff
- **WHEN** a Ordo reference adapter produces a common handoff and a Codex reference adapter consumes it
- **THEN** both SHALL use the same handoff schema and source refs
- **AND** neither SHALL require a core schema change or runtime-specific memory ID.

### Requirement: Adapter failure SHALL be isolated

An adapter initialization, capability, rendering or transport failure SHALL return a stable degraded status and SHALL NOT corrupt the memory ledger, block other adapters or rewrite canonical lifecycle state.

#### Scenario: Codex adapter is unavailable
- **WHEN** the Codex adapter executable or configuration is unavailable
- **THEN** Pinax SHALL report the adapter as degraded with a recovery action
- **AND** generic CLI/MCP and Ordo adapter access SHALL remain operational.

### Requirement: Generic transport adapters SHALL share application services

CLI, MCP, REST/RPC and Go SDK SHALL call the same Agent memory application services. Transport handlers SHALL NOT directly read `.pinax/**`, execute raw business SQL or maintain a parallel lifecycle implementation.

#### Scenario: Transport parity is verified
- **WHEN** a fixture sends the same read-only context request through CLI, MCP, REST/RPC and Go SDK
- **THEN** all responses SHALL contain semantically equivalent core context data
- **AND** transport-specific wrappers SHALL be the only allowed differences.

### Requirement: Write capabilities SHALL be explicit and approval-aware

Adapters that expose proposal or feedback writes SHALL declare the capability and resulting side effect. No adapter SHALL expose direct confirmed-memory mutation as an unguarded tool.

#### Scenario: MCP server is read-only
- **WHEN** the MCP server starts without explicit write capability
- **THEN** `tools/list` SHALL expose only read-only Agent memory tools
- **AND** a proposal write call SHALL return `approval_required` or `capability_disabled` without writing state.
