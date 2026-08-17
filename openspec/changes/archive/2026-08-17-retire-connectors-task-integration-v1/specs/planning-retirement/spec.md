# Pinax 本地规划退役规范

## REMOVED Requirements

### Requirement: External task runtime integration

Pinax SHALL NOT invoke `connectors task` or `taskbridge`, probe either executable, parse their envelopes, or expose `--taskbridge`.

#### Scenario: ordinary planning is local

- **WHEN** the user runs `pinax plan daily|weekly|monthly --dry-run`
- **THEN** Pinax SHALL use only local vault and project-board data
- **AND** it SHALL NOT require an external task executable or provider credential

### Requirement: External task-owned planning assets

Pinax SHALL NOT generate `connectors.*` or `taskbridge.*` planning schemas, task-runtime source facts, or external task execute commands.

#### Scenario: local action draft

- **WHEN** the user runs `pinax plan actions --from daily --save`
- **THEN** Pinax SHALL write a Pinax-owned planning draft
- **AND** it SHALL not execute or delegate the draft to an external task runtime

## MODIFIED Requirements

### Requirement: Local daily task review

Pinax SHALL retain daily project-board task review through the `daily-task-review` managed block.

#### Scenario: review requires explicit write approval

- **WHEN** the user runs `pinax plan daily --task-review` without `--yes`
- **THEN** Pinax SHALL preview the replacement
- **AND** SHALL leave the vault unchanged

- **WHEN** the user runs `pinax plan daily --task-review --yes`
- **THEN** Pinax SHALL update only the local `daily-task-review` managed block
