# KB decouple deltas for release agent interface convergence

## MODIFIED Requirements

### Requirement: Release gate SHALL validate docs, contracts, tests, build, and OpenSpec

Pinax release convergence SHALL NOT be considered complete until project-level validation proves the CLI-first agent path and derived surfaces work together.

#### Scenario: Running the release quality gate

- **GIVEN** implementation tasks for this change are complete
- **WHEN** maintainers run `task check` from `cli/pinax`
- **THEN** formatting, lint, tests, build, and `openspec validate --all` SHALL pass
- **AND** failures SHALL be fixed in the owning lane rather than documented around.

#### Scenario: Running OpenSpec strict validation

- **WHEN** maintainers run `openspec validate pinax-release-agent-interface-convergence --strict`
- **THEN** this change SHALL validate without missing proposal, design, tasks, spec scenarios, or malformed requirement structure.
