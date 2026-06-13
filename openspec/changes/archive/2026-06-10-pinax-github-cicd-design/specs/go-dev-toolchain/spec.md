# go-dev-toolchain Delta

## ADDED Requirements

### Requirement: Pinax GitHub CI mirrors the local quality gate

Pinax SHALL provide a GitHub Actions CI workflow for repository changes that touch the Pinax subproject or its workflow definition, and the workflow SHALL run from the Pinax subproject working directory using the same quality gate expected locally.

#### Scenario: CI runs the Pinax check gate for subproject changes

- **GIVEN** a push or pull request changes files under `cli/pinax/**` or the Pinax CI workflow file
- **WHEN** the Pinax CI workflow runs
- **THEN** it SHALL set up Go, Task, golangci-lint, and OpenSpec dependencies
- **AND** it SHALL run the Pinax quality gate that covers formatting, linting, tests, build, and `openspec validate --all`
- **AND** it SHALL NOT require provider credentials, user vaults, or network calls to external note providers.

#### Scenario: CI scope stays inside the Pinax subproject

- **GIVEN** the workflow is triggered from the repository root
- **WHEN** it runs Pinax commands
- **THEN** command execution SHALL use `cli/pinax` as the working directory
- **AND** generated build artifacts, coverage files, and test evidence SHALL NOT be committed by the workflow.

### Requirement: Pinax release workflow uses Pinax tags

Pinax SHALL provide a GitHub Actions release workflow that is triggered only by Pinax release tags and builds release artifacts through the Pinax Go CLI release toolchain.

#### Scenario: Release workflow triggers on Pinax semantic tags

- **GIVEN** a tag matching `pinax/v*.*.*` is pushed
- **WHEN** the Pinax release workflow runs
- **THEN** it SHALL set up the Go release environment
- **AND** it SHALL run a release check or packaging step for the Pinax CLI
- **AND** it SHALL keep non-Pinax tags from triggering Pinax release packaging.

#### Scenario: Release workflow separates validation from publishing

- **GIVEN** the release workflow is being changed or reviewed
- **WHEN** validation is required without publishing a real release
- **THEN** the workflow SHALL support a check or snapshot path that validates release configuration without publishing artifacts
- **AND** real publishing SHALL require the Pinax release tag trigger.
