## ADDED Requirements

### Requirement: Pinax exposes note vault management analytics
Pinax SHALL treat stats, doctor, and dashboard as first-class local Markdown note CLI capabilities for managing a user's vault.

#### Scenario: Note CLI includes management commands
- **WHEN** a user runs `pinax --help`
- **THEN** the command list SHALL include `stats`, `doctor`, and `dashboard`
- **AND** their help text SHALL describe local Markdown vault management rather than agent platform or provider automation behavior.

#### Scenario: Analytics commands require no provider credentials
- **WHEN** a user runs `pinax stats`, `pinax doctor`, or `pinax dashboard` against a valid local vault
- **THEN** Pinax SHALL NOT require firecrawl, agent-browser, Lark, Notion, Pinax Cloud, provider tokens, or external network access.

#### Scenario: Analytics commands follow the CLI output contract
- **WHEN** `pinax stats` or `pinax doctor` is run with default human output, `--json`, or `--agent`
- **THEN** Pinax SHALL render all modes from one command projection
- **AND** machine-readable stdout SHALL be stable enough for scripts and agents.
