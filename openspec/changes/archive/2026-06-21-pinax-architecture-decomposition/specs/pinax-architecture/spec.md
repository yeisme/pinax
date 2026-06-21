## ADDED Requirements

### Requirement: Pinax SHALL keep CLI access behind the app Service facade

Pinax SHALL keep `app.Service` as the CLI-facing application facade. CLI packages and command entrypoints SHALL dispatch through the facade rather than importing app capability packages directly.

#### Scenario: CLI imports stay facade-only
- **WHEN** architecture guard tests inspect imports under `internal/cli` and `cmd/pinax`
- **THEN** they SHALL reject direct imports of `internal/app/noteops`, `internal/app/searchops`, `internal/app/vaultops`, `internal/app/templateops`, `internal/app/syncops`, `internal/app/versionops`, `internal/app/briefingops`, or `internal/app/planningops`
- **AND** existing CLI behavior SHALL continue through `internal/app` facade methods.

### Requirement: Pinax SHALL assign app use cases to capability-owned packages

Pinax SHALL organize new or moved app-layer business logic under documented capability packages with clear ownership, dependency rules, and focused test entrypoints.

#### Scenario: Capability packages declare ownership
- **WHEN** architecture guard tests inspect app capability package directories
- **THEN** each required capability package SHALL contain a `doc.go` file
- **AND** the package documentation SHALL name the command family, responsibility, prohibited dependencies, and focused test entrypoint.

#### Scenario: Capability packages avoid rendering dependencies
- **WHEN** architecture guard tests inspect imports under `internal/app/*ops`
- **THEN** capability packages SHALL NOT import `internal/cli`
- **AND** they SHALL NOT import `internal/output`
- **AND** they SHALL return domain results or projection inputs to the facade instead of rendering stdout/stderr.

### Requirement: Pinax SHALL preserve external contracts during architecture decomposition

Architecture decomposition SHALL preserve released external surfaces while allowing internal package reshaping with in-repo caller updates.

#### Scenario: External CLI contracts remain stable
- **WHEN** command builders or tests are split by family
- **THEN** existing command names, flags, default output behavior, `--json` envelopes, `--agent` keys, and `--events` types SHALL remain compatible
- **AND** any discovered breaking change SHALL stop implementation until this OpenSpec records migration, deprecation window, consumer updates, and rollback.

#### Scenario: Internal app API changes update callers together
- **WHEN** an `internal/app` request type, method, or package-level symbol changes
- **THEN** every in-repo caller and focused test SHALL be updated in the same implementation slice
- **AND** `app.Service` SHALL keep the CLI-facing behavior compatible for that slice.

### Requirement: Pinax SHALL split large command tests by command family without changing behavior

Pinax SHALL move broad command tests into command-family test files and keep shared helpers in explicit testkit files.

#### Scenario: Command test split is mechanical
- **WHEN** command tests are moved out of `cmd/pinax/main_test.go`
- **THEN** test expectations SHALL remain equivalent
- **AND** shared helpers SHALL live in focused files such as `cli_testkit_test.go`
- **AND** `go test ./cmd/pinax -count=1` SHALL pass before the split is considered complete.
