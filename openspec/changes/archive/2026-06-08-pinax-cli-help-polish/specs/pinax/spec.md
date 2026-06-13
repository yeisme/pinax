## MODIFIED Requirements

### Requirement: Pinax exposes note vault management analytics
Pinax SHALL treat stats, doctor, validate, and dashboard as first-class local Markdown vault management capabilities under the `vault` command namespace while preserving existing root compatibility aliases.

#### Scenario: Vault namespace includes management commands
- **WHEN** a user runs `pinax vault --help`
- **THEN** the command list SHALL include `stats`, `validate`, `doctor`, and `dashboard`
- **AND** their help text SHALL describe local Markdown vault management rather than agent platform or provider automation behavior.

#### Scenario: Root help hides vault compatibility aliases
- **WHEN** a user runs `pinax --help`
- **THEN** root aliases `stats`, `validate`, `doctor`, and `dashboard` SHALL NOT appear in the primary command list
- **AND** the root help SHALL include `vault` as the primary local vault management entry.

#### Scenario: Analytics commands require no provider credentials
- **WHEN** a user runs `pinax vault stats`, `pinax vault doctor`, or `pinax vault dashboard` against a valid local vault
- **THEN** Pinax SHALL NOT require firecrawl, agent-browser, Lark, Notion, Pinax Cloud, provider tokens, or external network access.

#### Scenario: Analytics aliases remain compatible
- **WHEN** a user runs existing commands `pinax stats`, `pinax doctor`, or `pinax dashboard` against a valid local vault
- **THEN** Pinax SHALL preserve backwards-compatible behavior and machine output fields
- **AND** root aliases MAY be hidden from primary help.

#### Scenario: Analytics commands follow the CLI output contract
- **WHEN** `pinax vault stats` or `pinax vault doctor` is run with default human output, `--json`, or `--agent`
- **THEN** Pinax SHALL render all modes from one command projection
- **AND** machine-readable stdout SHALL be stable enough for scripts and agents.
