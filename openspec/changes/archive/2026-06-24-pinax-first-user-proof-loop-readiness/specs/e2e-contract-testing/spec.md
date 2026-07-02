## ADDED Requirements

### Requirement: Proof loop readiness SHALL have command-level e2e coverage

Pinax SHALL verify the first-user proof loop through command-level e2e tests using the owning project's existing Go/testscript stack.

#### Scenario: Preview e2e covers read-only proof loop

- **WHEN** the proof-loop preview e2e runs against the deterministic demo vault
- **THEN** it SHALL assert JSON envelope validity, `proof_loop_run_id`, stage facts, next actions, and `local_write=false`
- **AND** it SHALL fail if preview writes Markdown, `.pinax` apply assets, Git state, provider state, or remote state.

#### Scenario: Apply e2e covers plan and snapshot gates

- **WHEN** the apply e2e runs against the deterministic demo vault
- **THEN** it SHALL save a repair plan, create a version snapshot, apply only approved low-risk operations, and assert receipt facts
- **AND** it SHALL reject stale plans and missing snapshot paths with stable machine-readable error codes.

#### Scenario: Restore e2e covers controlled rollback

- **WHEN** the restore e2e runs after a proof-loop apply
- **THEN** it SHALL generate a restore plan, apply that plan through the CLI/application service, and assert `local_write=true` and `remote_write=false`
- **AND** it SHALL verify stale restore plans fail safely.

### Requirement: Proof loop outputs SHALL be recursively redaction-tested

Pinax SHALL recursively scan proof-loop output and evidence surfaces for body leaks and sensitive values.

#### Scenario: Machine outputs contain no bounded-projection body leak

- **WHEN** proof-loop commands run in `--json`, `--agent`, or `--events` modes without an explicit body-display command
- **THEN** stdout SHALL NOT contain non-empty `body`, `note_body`, or `raw_body` fields at any nesting depth
- **AND** stdout SHALL NOT contain body sentinel text from fixture notes.

#### Scenario: Evidence surfaces contain no forbidden sensitive values

- **WHEN** tests inspect stdout, stderr, saved plans, receipts, events, snapshots, restore evidence, integration evidence, and fixtures
- **THEN** those surfaces SHALL NOT contain Authorization headers, Bearer tokens, API keys, raw prompts, provider payloads, hidden system prompts, private tool arguments, or complete chain-of-thought.

