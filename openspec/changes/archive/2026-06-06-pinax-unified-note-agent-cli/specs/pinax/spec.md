## ADDED Requirements

### Requirement: Pinax has a working development base

Pinax SHALL provide a minimal Go/Cobra development base before non-trivial product implementation starts.

#### Scenario: running bootstrap checks
- **GIVEN** the developer is in `cli/pinax`
- **WHEN** they run `task check`
- **THEN** tests, build, and OpenSpec validation SHALL pass without external provider credentials

#### Scenario: running checks without Taskfile
- **GIVEN** the developer is in `cli/pinax` and Taskfile is not installed
- **WHEN** they run `gofmt -w cmd internal`, `go test ./...`, `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`, and `openspec validate --all`
- **THEN** the same local quality gate SHALL pass without external provider credentials

### Requirement: Pinax implementation work is OpenSpec-gated

Business capability implementation SHALL be tracked by Pinax subproject OpenSpec changes.

#### Scenario: adding a business feature
- **GIVEN** a developer wants to implement vault, provider, sync, briefing, MCP, delivery, or feedback behavior
- **WHEN** code changes are planned
- **THEN** a `pinax-*` OpenSpec change SHALL describe proposal, design, tasks, validation commands, and failure re-checks before implementation proceeds

### Requirement: Pinax exposes a Go development task surface

Pinax SHALL provide a Taskfile-based development task surface that maps to direct Go and OpenSpec commands.

#### Scenario: building through Taskfile
- **GIVEN** the developer is in `cli/pinax`
- **WHEN** they run `task build`
- **THEN** Pinax SHALL produce `dist/pinax`
- **AND** the task SHALL verify Go formatting before building

#### Scenario: listing local tasks
- **GIVEN** the developer is in `cli/pinax`
- **WHEN** they run `task --list`
- **THEN** the output SHALL include at least `build`, `test`, `fmt`, `fmt-check`, `openspec`, `check`, and `clean`
