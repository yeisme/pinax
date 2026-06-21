# prompt-asset-vault Specification

## Purpose
TBD - created by archiving change pinax-prompt-asset-vault. Update Purpose after archive.
## Requirements
### Requirement: Pinax SHALL persist prompt assets as local-first knowledge assets

Pinax SHALL provide a prompt asset vault for `yeisme.prompt_asset.v1` records with version, lifecycle, permission, source refs, variable schema, prompt template, constraints, and review guidance.

#### Scenario: Create valid prompt asset
- **GIVEN** a valid prompt asset file with required fields
- **WHEN** the user runs `pinax prompt import --from <file> --json`
- **THEN** Pinax SHALL persist the asset and return its `prompt_asset_id`, lifecycle, permission, and resolve action.

#### Scenario: Reject incomplete prompt asset
- **GIVEN** a prompt asset file missing `permission` or `prompt_template`
- **WHEN** the user imports it
- **THEN** Pinax SHALL reject it with a structured validation error
- **AND** no partial prompt asset SHALL be written.

### Requirement: Pinax SHALL resolve `pinax://prompt/<id>` through CLI-owned contracts

Other Yeisme projects SHALL resolve prompt assets through Pinax-owned CLI or API contracts, not by reading Pinax SQLite or vault metadata directly.

#### Scenario: Resolve prompt asset URI
- **GIVEN** `pinax://prompt/novel_character_portrait_v1` exists
- **WHEN** the user runs `pinax prompt resolve pinax://prompt/novel_character_portrait_v1 --agent`
- **THEN** Pinax SHALL emit low-token key=value output with prompt asset ID, lifecycle, permission, and next action
- **AND** it SHALL not reveal raw local paths by default.

### Requirement: Pinax SHALL own prompt lifecycle updates

Pinax SHALL decide lifecycle transitions for prompt assets. Imported usage feedback MAY inform lifecycle changes, but external projects SHALL NOT mutate Pinax lifecycle state directly.

#### Scenario: Import accepted Eikona feedback
- **GIVEN** an Eikona feedback record references a known prompt asset and an accepted artifact
- **WHEN** the user runs `pinax prompt feedback import --from <file> --json`
- **THEN** Pinax SHALL persist feedback once
- **AND** it SHALL expose a lifecycle recommendation or explicit lifecycle update action owned by Pinax.

### Requirement: Prompt asset outputs SHALL be redacted and agent-safe

Pinax SHALL preserve stdout/stderr separation and shall not emit secrets, raw provider payloads, hidden system prompts, private tool arguments, or full chain-of-thought in prompt asset machine output or integration evidence.

#### Scenario: Agent output is bounded
- **WHEN** the user runs `pinax prompt show <id> --agent`
- **THEN** output SHALL include only decision-essential keys
- **AND** prompt body and local paths SHALL be omitted unless an explicit full or reveal mode is requested.

