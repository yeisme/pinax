# english-cli-output-contract Delta Spec

## ADDED Requirements

### Requirement: Pinax defaults to English user-visible CLI chrome

Pinax SHALL render CLI chrome in English by default for human-facing command surfaces owned by `cli/pinax`.

#### Scenario: Default command summary uses English chrome

- **WHEN** a user runs a representative successful `pinax` command without `--json`, `--agent`, `--events`, or `--explain`
- **THEN** stdout SHALL use English section labels and prose for status, highlights, facts, evidence, risks, and next action labels
- **AND** stdout SHALL include at most one primary recommended next command when a next step is useful
- **AND** stdout SHALL NOT require agents or scripts to parse localized human prose.

#### Scenario: Help and validation errors use English chrome

- **WHEN** a user runs `pinax --help`, command-specific help, an unknown command, or a validation-failure path
- **THEN** help text, usage text, examples, suggestions, error messages, and correction hints SHALL be English
- **AND** every suggested command SHALL be a real user-runnable `pinax ...` command.

#### Scenario: Explain mode is English and redacted

- **WHEN** a user requests `--explain` for a supported command
- **THEN** stdout SHALL use English review sections such as conclusion, evidence, confidence, risks, tradeoffs, and recommended next step
- **AND** stdout SHALL NOT include full chain-of-thought, raw prompts, hidden prompts, provider payloads, secrets, tokens, cookies, Authorization headers, private tool arguments, or model-internal reasoning.

### Requirement: Pinax preserves domain content language

Pinax SHALL distinguish CLI chrome from data and SHALL NOT blindly translate non-English domain content.

#### Scenario: User-authored content remains unchanged

- **GIVEN** a note body, template body, title, tag, folder name, quoted source, or imported document contains non-English text
- **WHEN** Pinax renders or returns that content
- **THEN** the content SHALL retain its original language and bytes except for existing domain-normalization rules
- **AND** English-only enforcement SHALL apply to surrounding CLI labels, summaries, errors, and actions, not to the user-authored data.

#### Scenario: Provider and third-party payload fields remain stable

- **GIVEN** a provider response, third-party API field, schema field, enum value, event type, command id, flag name, JSON envelope key, or `--agent` key already has a stable machine contract
- **WHEN** Pinax renders machine output or records structured data
- **THEN** those fields SHALL remain stable unless a major output-contract version migration is explicitly introduced
- **AND** the English migration SHALL NOT rename machine fields only for prose consistency.

### Requirement: Machine output remains parseable, stable, and prose-free

Pinax SHALL keep machine-readable output independent from human-language text.

#### Scenario: JSON output is a single envelope

- **WHEN** a user runs a supported command with `--json`
- **THEN** stdout SHALL contain exactly one valid JSON object
- **AND** the object SHALL include `spec_version`, `mode=json`, `command`, and `status`
- **AND** stdout SHALL NOT include ANSI, progress logs, banners, tables, human-only suggestions, or localized prose outside JSON string fields that are explicitly part of the envelope.

#### Scenario: Agent output is stable key=value

- **WHEN** a user runs a supported command with `--agent`
- **THEN** stdout SHALL contain stable ASCII key=value lines
- **AND** stdout SHALL include `spec_version`, `mode=agent`, `command`, and `status`
- **AND** stdout SHALL NOT include ANSI, tables, raw debug dumps, localized prose, raw prompts, hidden prompts, provider payloads, private tool arguments, or chain-of-thought.

#### Scenario: Events output is NDJSON only

- **WHEN** a user runs a supported long-running command with `--events`
- **THEN** stdout SHALL be newline-delimited JSON events
- **AND** the stream SHALL start with a `start` event and end with an `end` or `error` event
- **AND** diagnostics and progress text SHALL go to stderr or structured event fields, not mixed prose on stdout.

### Requirement: Contract tests guard English output and redaction

Pinax SHALL include automated tests that prevent regressions in English CLI chrome, machine parseability, stdout/stderr separation, and redaction.

#### Scenario: Focused output tests fail on localized CLI chrome

- **WHEN** the focused CLI output test suite runs
- **THEN** representative default summaries, help text, validation errors, stderr diagnostics, and explain reports SHALL be checked for English CLI chrome
- **AND** intentional non-English data SHALL be covered by an explicit allowlist or fixture classification.

#### Scenario: Machine-mode tests parse output

- **WHEN** the focused machine-output tests run
- **THEN** tests SHALL parse `--json` as JSON envelopes
- **AND** tests SHALL parse `--agent` as key=value lines
- **AND** tests SHALL parse `--events` as NDJSON for commands that support event streams
- **AND** tests SHALL fail if ANSI, logs, table decoration, or human prose leaks into machine stdout.

#### Scenario: Redaction tests cover all output surfaces

- **WHEN** stdout, stderr, events, traces, snapshots, sidecars, fixtures, or integration evidence are generated
- **THEN** tests SHALL reject secrets, tokens, Authorization headers, cookies, raw prompts, hidden system prompts, unredacted provider payloads, private tool arguments, and full chain-of-thought
- **AND** evidence metadata SHALL be generated by project-owned commands or test runners rather than hand-written by agents.
