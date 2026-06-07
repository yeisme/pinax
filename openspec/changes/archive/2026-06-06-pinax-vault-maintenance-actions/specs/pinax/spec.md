## ADDED Requirements

### Requirement: Pinax exposes local vault repair commands
Pinax SHALL expose repair plan and apply commands as local Markdown vault management capabilities.

#### Scenario: Help includes repair commands
- **WHEN** a user runs `pinax repair --help`
- **THEN** the command list SHALL include `plan` and `apply`
- **AND** help text SHALL describe local Markdown vault maintenance rather than provider automation.

#### Scenario: Repair commands follow output contract
- **WHEN** `pinax repair plan` or `pinax repair apply` is run with default human output, `--json`, or `--agent`
- **THEN** Pinax SHALL render all modes from one command projection
- **AND** machine-readable stdout SHALL contain only the selected format.
