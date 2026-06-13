# configurable-output-rendering Specification

## Purpose
TBD - created by archiving change pinax-configurable-output-rendering. Update Purpose after archive.
## Requirements
### Requirement: Pinax renders human output with configurable themes

Pinax SHALL render default human output using named theme roles that can be selected or overridden without changing machine output contracts, and the human-facing output chrome SHALL be English by default.

#### Scenario: Default theme renders human summary

- **WHEN** a user runs a successful default-mode command in a color-capable terminal
- **THEN** Pinax SHALL render status, facts, table rules, paths, errors, and next actions using the configured theme roles
- **AND** section labels, fact labels, status prose, error prose, and next-action labels SHALL be English
- **AND** the output SHALL remain readable when color is disabled.

#### Scenario: Theme selection can come from config or flag

- **WHEN** project config sets `output.theme=mono` and the user passes `--theme high-contrast`
- **THEN** default human output SHALL use the high-contrast theme for that command invocation.

#### Scenario: Machine outputs ignore human theme

- **WHEN** a user runs a command with `--json`, `--agent`, or `--events`
- **THEN** stdout SHALL contain only the selected machine format
- **AND** it SHALL NOT include ANSI escape sequences, table decoration intended only for humans, Markdown renderer styling, or English/localized human prose outside the selected machine contract.

### Requirement: Pinax supports custom role-based colors

Pinax SHALL allow users to define custom colors by semantic output role rather than by command-specific renderer internals.

#### Scenario: Custom theme maps role names to colors

- **WHEN** config sets `output.theme=custom` and defines `themes.custom.success`, `themes.custom.danger`, `themes.custom.rule`, and `themes.custom.path`
- **THEN** default human output SHALL use those colors for matching roles.

#### Scenario: Invalid custom color fails clearly

- **WHEN** config defines an invalid color value such as `themes.custom.success=greenish`
- **THEN** Pinax SHALL fail configuration validation with a stable error code and an English correction hint.

#### Scenario: Missing custom role falls back safely

- **WHEN** `output.theme=custom` omits an optional role color
- **THEN** Pinax SHALL fall back to the built-in `pinax` role value for that role
- **AND** it SHALL NOT fail only because an optional role is missing.

### Requirement: Pinax renders Markdown documents with Glamour in default human mode

Pinax SHALL use the Glow/Glamour Markdown rendering component for note and template bodies in default human output while preserving raw Markdown data in machine modes.

#### Scenario: Note show renders Markdown for humans

- **WHEN** a user runs `pinax note show note_123 --vault ./my-notes` in default mode
- **THEN** stdout SHALL include a concise English metadata summary followed by a rendered Markdown document body
- **AND** the rendered body SHALL honor configured width, color mode, theme, and Markdown style.

#### Scenario: Template render displays rendered Markdown document

- **WHEN** a user runs `pinax template render meeting --title "Client meeting" --vault ./my-notes`
- **THEN** stdout SHALL include rendered Markdown content for human reading
- **AND** `--json` SHALL still return the unstyled body under the JSON envelope data field.

#### Scenario: Markdown rendering is disabled by config

- **WHEN** config sets `output.markdown.enabled=false`
- **THEN** default human output for note and template bodies SHALL fall back to plain readable text without Glamour styling.

### Requirement: Pinax keeps output width and color deterministic
Pinax SHALL derive render width and color behavior from explicit options, configuration, terminal state, and environment variables in a deterministic order.

#### Scenario: Explicit width overrides automatic detection
- **WHEN** a user passes `--width 100`
- **THEN** summary tables and Markdown rendering SHALL use width 100 for that invocation.

#### Scenario: Auto color respects terminal capability
- **WHEN** `output.color=auto` and stdout is not a terminal
- **THEN** default human output SHALL render without ANSI escape sequences.

#### Scenario: Explicit color always overrides non-terminal default
- **WHEN** stdout is not a terminal and the user passes `--color always`
- **THEN** default human output MAY include ANSI color
- **AND** machine output modes SHALL still remain ANSI-free.

### Requirement: Pinax does not introduce implicit TUI behavior for output beautification
Pinax SHALL improve default stdout rendering without making ordinary read commands enter an implicit full-screen TUI.

#### Scenario: Journal show remains stdout-rendered by default
- **WHEN** a user runs `pinax daily show --vault ./my-notes` in a terminal
- **THEN** Pinax SHALL render the note body to stdout using the configured human output renderer
- **AND** it SHALL NOT enter a full-screen Bubble Tea pager unless a future explicit pager option is selected.

#### Scenario: Pager behavior is explicit and deferred
- **WHEN** config contains `output.markdown.pager=never`
- **THEN** Pinax SHALL NOT invoke `$PAGER`, Bubble Tea, or any external paging process for Markdown output.

### Requirement: Preview commands render Markdown bodies in default human mode

Pinax SHALL route preview command bodies through the shared summary Markdown renderer in default human mode while preserving raw body data in machine modes.

#### Scenario: Template preview renders body for humans

- **WHEN** a user runs `pinax template preview meeting --title "Client meeting" --tags meeting,client --vault ./my-notes`
- **THEN** stdout SHALL include a concise English metadata summary, visible tags, and the rendered template body
- **AND** stdout SHALL remain readable when Markdown styling or color is disabled.

#### Scenario: Template preview JSON remains raw and structured

- **WHEN** a user runs `pinax template preview meeting --title "Client meeting" --tags meeting,client --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope with template facts and raw body data
- **AND** stdout SHALL NOT contain ANSI escape sequences, summary table decoration, Markdown renderer styling, or human prose outside JSON.

### Requirement: Dimension summaries include plain-text visualization

Pinax SHALL render dimension lists in default human mode with count, percentage, and plain-text heat visualization derived from the same structured dimension count data.

#### Scenario: Dimension list renders visual columns

- **WHEN** a default-mode dimension list projection contains values and counts
- **THEN** stdout SHALL render a table with English column labels for value, count, percentage, and heat
- **AND** the heat visualization SHALL use plain text that does not require color.

#### Scenario: Machine modes omit visualization prose

- **WHEN** the same dimension list projection is rendered with `--agent`, `--json`, or `--events`
- **THEN** stdout SHALL preserve stable machine fields and item counts
- **AND** stdout SHALL NOT include human-only visualization labels such as `Percentage`, `Heat`, `占比`, or `热度` outside structured data requested by that mode.

