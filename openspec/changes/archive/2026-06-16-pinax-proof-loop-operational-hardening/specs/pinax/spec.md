# Pinax Proof Loop Operational Hardening Specification

## ADDED Requirements

### Requirement: Pinax SHALL support reversible local proof-loop apply

Pinax SHALL provide a CLI-authored restore apply path so a bad local apply can be reverted from an existing snapshot/restore plan without direct file surgery by an agent.

#### Scenario: Restore apply refuses implicit writes

- **GIVEN** a restore plan exists for a vault snapshot
- **WHEN** the user runs restore apply without explicit approval
- **THEN** Pinax SHALL refuse to mutate the vault
- **AND** output SHALL include the exact approval flag or next command required.

#### Scenario: Restore apply restores local Markdown safely

- **GIVEN** a restore plan targets the current vault and a valid snapshot id
- **WHEN** the user runs `pinax version restore apply --yes --plan <path>` or the accepted equivalent command
- **THEN** Pinax SHALL restore local Markdown files from the snapshot plan
- **AND** SHALL write a restore receipt
- **AND** SHALL report `local_write=true` and `remote_write=false`
- **AND** SHALL NOT call provider, cloud sync or MCP write surfaces.

### Requirement: Pinax SHALL enforce shared projection redaction before rendering

Pinax SHALL apply one shared redaction gate to command projections before rendering default, `--json`, `--agent`, `--events`, `--explain` or evidence sidecars.

#### Scenario: Nested projection data is scanned

- **GIVEN** a command projection contains nested facts, actions, evidence, data, error or event payloads
- **WHEN** the projection is rendered
- **THEN** Pinax SHALL redact or reject forbidden protected content before stdout/stderr/evidence persistence
- **AND** forbidden content SHALL include note body sentinels, full body fields, Authorization headers, Bearer tokens, cookies, webhook URLs, provider payloads, raw prompts and hidden prompts.

### Requirement: Pinax SHALL expose a single agent-callable proof loop run command

Pinax SHALL provide one orchestration command for the local proof loop while preserving existing stage commands.

#### Scenario: Proof loop run defaults to preview mode

- **WHEN** an agent runs `pinax proof loop run --vault <vault>`
- **THEN** Pinax SHALL emit one bounded projection with `proof_loop_run_id`, ordered stage facts, evidence paths and next actions
- **AND** it SHALL NOT mutate the vault unless explicit apply flags are present.

#### Scenario: Proof loop run applies only approved safe operations

- **WHEN** an agent runs proof loop run with explicit apply approval
- **THEN** Pinax SHALL take a fresh snapshot before applying allowed repair or organize operations
- **AND** manual-review-only operations SHALL remain next actions instead of being auto-applied.

### Requirement: Proof-loop output contracts SHALL cover all rendering modes

Pinax SHALL contract-test proof-loop stage commands, proof-loop run and restore apply across default, `--json`, `--agent`, `--events` and `--explain` modes.

#### Scenario: Machine modes stay bounded and parseable

- **WHEN** proof-loop commands render machine output
- **THEN** `--json` SHALL emit one valid envelope
- **AND** `--agent` SHALL emit stable key=value facts
- **AND** `--events` SHALL emit start/end NDJSON events
- **AND** no mode SHALL leak note body, token, Authorization header, cookie, raw prompt, hidden prompt or provider payload.

#### Scenario: Explain mode is evidence summary, not chain-of-thought

- **WHEN** proof-loop commands render `--explain`
- **THEN** the output SHALL include conclusion, evidence, confidence or risk where applicable and next action
- **AND** it SHALL NOT include full chain-of-thought, raw prompts or hidden system prompts.
