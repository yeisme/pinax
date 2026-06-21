# cli-output-contract Specification

## Purpose
TBD - created by archiving change pinax-cli-sharing-sync-theme-alignment. Update Purpose after archive.
## Requirements
### Requirement: Pinax human CLI output defaults to Chinese
Pinax SHALL render human-facing CLI summaries, help, validation messages and explain reports in Chinese by default for this subproject, while preserving stable English machine contracts.

#### Scenario: Default human command summary uses Chinese chrome
- **WHEN** a user runs a representative successful `pinax` command without `--json`, `--agent`, `--events`, or `--explain`
- **THEN** stdout SHALL use concise Chinese human prose for status, highlights, evidence, risks and next action labels
- **AND** stdout SHALL include at most one primary recommended next command when a next step is useful
- **AND** agents or scripts SHALL NOT be required to parse localized human prose.

#### Scenario: Machine output keeps English protocol fields
- **WHEN** a user runs a supported command with `--json`, `--agent`, or `--events`
- **THEN** protocol keys, facts, event fields, error codes, schema names and command ids SHALL remain stable English ASCII where applicable
- **AND** stdout SHALL contain only the selected machine format without extra human prose.

#### Scenario: Explain mode is Chinese and redacted
- **WHEN** a user requests `--explain` for a supported command
- **THEN** stdout SHALL use Chinese human review sections or equivalent localized labels
- **AND** stdout SHALL NOT include full chain-of-thought, raw prompts, hidden prompts, provider payloads, secrets, tokens, cookies, Authorization headers, private tool arguments, or model-internal reasoning.

### Requirement: Output language docs do not rename machine contracts
Pinax SHALL distinguish human chrome language from structured protocol language.

#### Scenario: Stable structured fields are not translated
- **GIVEN** a JSON envelope key, `--agent` key, event type, error code, schema field, command id, flag name, provider id, or persisted structured asset field has a stable English contract
- **WHEN** Pinax renders machine output, writes CLI-authored metadata, or documents the protocol
- **THEN** those fields SHALL remain English unless an explicit major contract migration is approved
- **AND** human-language localization SHALL apply only to prose, labels, help text, summaries and explanations.

