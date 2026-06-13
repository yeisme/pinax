## ADDED Requirements

### Requirement: Preview commands render Markdown bodies in default human mode
Pinax SHALL route preview command bodies through the shared summary Markdown renderer in default human mode while preserving raw body data in machine modes.

#### Scenario: Template preview renders body for humans
- **WHEN** a user runs `pinax template preview meeting --title "客户会议" --tags meeting,client --vault ./my-notes`
- **THEN** stdout SHALL include a concise Chinese metadata summary, visible tags, and the rendered template body
- **AND** stdout SHALL remain readable when Markdown styling or color is disabled.

#### Scenario: Template preview JSON remains raw and structured
- **WHEN** a user runs `pinax template preview meeting --title "客户会议" --tags meeting,client --vault ./my-notes --json`
- **THEN** stdout SHALL contain exactly one JSON envelope with template facts and raw body data
- **AND** stdout SHALL NOT contain ANSI escape sequences, summary table decoration, or Markdown renderer styling outside JSON.

### Requirement: Dimension summaries include plain-text visualization
Pinax SHALL render dimension lists in default human mode with count, percentage, and plain-text heat visualization derived from the same structured dimension count data.

#### Scenario: Dimension list renders visual columns
- **WHEN** a default-mode dimension list projection contains values and counts
- **THEN** stdout SHALL render a table with value, count, percentage, and heat columns
- **AND** the heat visualization SHALL use plain text that does not require color.

#### Scenario: Machine modes omit visualization prose
- **WHEN** the same dimension list projection is rendered with `--agent`, `--json`, or `--events`
- **THEN** stdout SHALL preserve stable machine fields and item counts
- **AND** stdout SHALL NOT include localized human-only visualization labels such as `占比` or `热度` outside structured data requested by that mode.
