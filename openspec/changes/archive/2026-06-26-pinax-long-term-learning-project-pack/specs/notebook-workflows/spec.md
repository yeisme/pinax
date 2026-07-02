## ADDED Requirements

### Requirement: Built-in templates cover learning workflows
Pinax SHALL provide executable built-in note templates for long-term learning projects while keeping templates local-only and safe.

#### Scenario: Generic learning templates are available
- **WHEN** the user runs `pinax template recommend --intent "术语" --vault ./my-notes --json`
- **THEN** Pinax SHALL recommend a learning template such as `learning.term`
- **AND** the template SHALL be executable by `pinax note add <title> --template learning.term --vault ./my-notes --json`.

#### Scenario: Stock learning templates preserve safety boundary
- **WHEN** the user creates a note with `pinax note add "K线基础" --template learning.stock.indicator --vault ./my-notes --json`
- **THEN** the note body SHALL frame the content as learning, historical review, simulation, or risk-rule documentation
- **AND** it SHALL NOT claim to provide investment advice, buy/sell recommendations, guaranteed returns, or automated trading decisions.
