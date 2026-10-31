# daily-interaction-orchestration Specification

## ADDED Requirements

### Requirement: Existing projections expose an additive daily interaction view

Pinax SHALL optionally attach `pinax.daily_interaction.v1` to capture, retrieval, review, planning, organization, continuity, checkpoint and proof-loop projections. Existing envelope fields, command names and facts SHALL remain compatible.

#### Scenario: Read-only retrieval

- **WHEN** a user runs `pinax search "release workflow" --vault ./my-notes --json`
- **THEN** the response SHALL expose `phase=retrieve`, `read_only=true`, `writes=false`, `remote_effect=false`, and one bounded preview action when a note identity is available
- **AND** it SHALL not expose note body, raw prompt, credentials or absolute vault paths in the interaction view.

#### Scenario: Capture transitions to review

- **WHEN** `pinax inbox capture "Idea" --body "text" --vault ./my-notes --json` succeeds
- **THEN** the interaction view SHALL expose `phase=capture`, `writes=true`, and the next action `pinax inbox review --limit 5 --json`
- **AND** it SHALL preserve the returned note identity/readback facts and SHALL not create a second note.

### Requirement: State SHALL distinguish partial, blocked, approval and unknown

The composer SHALL derive state from the existing projection status, stable error codes, warnings, source coverage and freshness facts. It SHALL never turn stale, missing, conflict, approval or unknown results into `ready`.

#### Scenario: Stale review source

- **WHEN** a projection reports stale or partial source evidence
- **THEN** interaction state SHALL be `partial` with a warning code and a real recovery action
- **AND** the original trustworthy facts SHALL remain visible.

#### Scenario: Unknown operation

- **WHEN** a command returns `reconcile_required`, `operation_unknown` or `unknown_accept`
- **THEN** interaction state SHALL be `unknown`
- **AND** the recommended action SHALL query the original operation identity before any retry.

### Requirement: Writes and remote effects SHALL remain explicit

The interaction view SHALL derive `writes` from existing local write facts and `remote_effect` from existing remote write facts. It SHALL not authorize or execute a write.

#### Scenario: Protected organization apply

- **WHEN** `pinax organize apply --plan <plan-id> --yes --vault ./my-notes --json` is rejected as `plan_stale` or `approval_required`
- **THEN** interaction state SHALL be `blocked` or `approval_required` as appropriate
- **AND** snapshot, revision, receipt and stale-plan behavior SHALL remain owned by the existing application service.

### Requirement: Operator Skill SHALL be provider-neutral and fail closed

The operator Skill SHALL call real Pinax commands, render Pinax-returned facts and actions, and preserve vault/scope boundaries. It SHALL not inspect private Pinax state, scan all vaults, guess a project, confirm durable memory, or infer user outcome feedback.

#### Scenario: Missing binding

- **WHEN** `pinax continue --json` reports a missing or ambiguous binding
- **THEN** the Skill SHALL present the returned bind/status/rebind action or one bounded question
- **AND** it SHALL not select another vault or fabricate continuity.

### Requirement: Local and real-use evidence remain separate

The change SHALL record local contract/build evidence separately from the two-week real-vault observation. Passing tests SHALL NOT close adoption, continuity dogfood, sync, or external client gates.

#### Scenario: Local checks pass without real use

- **WHEN** focused tests and `task check` pass before a two-week real-vault observation
- **THEN** the engineering gate MAY be marked complete
- **AND** the real-use gate SHALL remain `pending` or `unknown`.
