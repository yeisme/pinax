## ADDED Requirements

### Requirement: Release smoke SHALL verify the installed Pinax binary

Pinax SHALL provide a local release smoke path that verifies an installed or archive-extracted binary can run the first-user proof loop without using source-tree internals.

#### Scenario: Archive-installed binary runs minimal proof loop

- **WHEN** maintainers run the release smoke command against a release archive or locally built distribution binary
- **THEN** the smoke SHALL run `pinax version`, `pinax init`, `pinax note add`, and `pinax proof loop run --json` inside an isolated temporary directory
- **AND** it SHALL NOT require Go source paths, real provider credentials, user vaults, Cloud Sync, TaskBridge, MCP, dashboard, or a daemon.

#### Scenario: Release smoke failure is diagnosable

- **WHEN** the archive is missing, checksum validation fails, the binary is not executable, or proof-loop preview fails
- **THEN** the smoke command SHALL exit non-zero
- **AND** stderr SHALL include a redacted diagnostic and a concrete next action
- **AND** machine stdout SHALL NOT contain raw local paths, credentials, Authorization headers, cookies, provider payloads, or raw prompts.

