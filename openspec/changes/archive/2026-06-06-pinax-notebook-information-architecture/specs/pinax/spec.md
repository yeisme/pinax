## MODIFIED Requirements

### Requirement: Pinax note command is ergonomic and backwards compatible
Pinax SHALL expose an ergonomic note command surface while preserving existing `note new`, `note list`, and `note show` behavior.

#### Scenario: Note help shows daily workflow commands
- **WHEN** a user runs `pinax note new --help`
- **THEN** help output SHALL include notebook information architecture flags such as `--group`, `--folder`, `--kind`, `--tags`, `--project`, `--dir`, and `--status`.
