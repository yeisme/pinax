## MODIFIED Requirements

### Requirement: Root help uses workflow groups

Pinax SHALL render root help as a scannable personal-local workflow map that shows only core commands by default, while keeping advanced commands executable and discoverable through `pinax commands`.

#### Scenario: Root help displays personal-local core commands

- **WHEN** a user runs `pinax --help`
- **THEN** stdout SHALL include English group headings for getting started, capture, retrieval, organization, and local safety workflows
- **AND** each visible core command SHALL appear under exactly one group
- **AND** the local safety group SHALL expose `backup` instead of requiring users to understand the lower-level `version` command
- **AND** advanced commands and compatibility-only aliases SHALL NOT appear in the root primary command list.

#### Scenario: Root help remains plain terminal text

- **WHEN** a user runs `pinax --help` in a plain terminal
- **THEN** stdout SHALL remain readable without ANSI color or box drawing characters
- **AND** command names, flag names, JSON fields, and protocol names SHALL remain stable English.

#### Scenario: Core help hides advanced global flags

- **WHEN** a user runs root help or help for a core command
- **THEN** help SHALL teach only the vault selector and shared output modes from the persistent flag set
- **AND** remote API credentials, remote URL, theme, width, color, and Markdown rendering flags SHALL NOT clutter the personal core help
- **AND** advanced command help SHALL continue to expose those existing flags without changing their executable behavior.

#### Scenario: Advanced commands remain discoverable

- **WHEN** a user needs commands outside the personal-local core
- **THEN** root help SHALL recommend the real command `pinax commands`
- **AND** `pinax commands` SHALL list advanced command paths without changing their executable behavior.
