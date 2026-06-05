## ADDED Requirements

### Requirement: Pinax has a working development base

Pinax SHALL provide a minimal Go/Cobra development base before non-trivial product implementation starts.

#### Scenario: running bootstrap checks
- **GIVEN** the developer is in `cli/pinax`
- **WHEN** they run `go test ./...`, `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`, and `openspec validate --all`
- **THEN** tests, build, and OpenSpec validation SHALL pass without external provider credentials

### Requirement: Pinax implementation work is OpenSpec-gated

Business capability implementation SHALL be tracked by Pinax subproject OpenSpec changes.

#### Scenario: adding a business feature
- **GIVEN** a developer wants to implement vault, provider, sync, briefing, MCP, delivery, or feedback behavior
- **WHEN** code changes are planned
- **THEN** a `pinax-*` OpenSpec change SHALL describe proposal, design, tasks, validation commands, and failure re-checks before implementation proceeds

