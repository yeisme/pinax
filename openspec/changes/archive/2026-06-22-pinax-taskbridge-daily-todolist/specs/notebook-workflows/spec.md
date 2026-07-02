# notebook-workflows Delta

## ADDED Requirements

### Requirement: Daily journal template SHALL reserve planning managed content

Pinax SHALL provide a stable managed-block location for generated daily planning content while keeping user-authored daily note content editable.

#### Scenario: journal daily template includes planning block
- **WHEN** a user runs `pinax journal daily open --template journal.daily --vault ./my-notes --json`
- **THEN** a newly created daily note SHALL include `<!-- pinax:managed name=planning-daily -->`
- **AND** it SHALL still include the existing `daily-captures` managed block

#### Scenario: existing daily note receives planning block only on approval
- **GIVEN** today's daily note exists without `planning-daily`
- **WHEN** the user runs `pinax plan daily --vault ./my-notes --taskbridge --yes --json`
- **THEN** Pinax MAY append the `planning-daily` managed block to the daily note
- **AND** it SHALL preserve all existing user-authored content and other managed blocks

#### Scenario: invalid planning block fails closed
- **GIVEN** today's daily note has duplicate or unclosed `planning-daily` managed block markers
- **WHEN** the user runs `pinax plan daily --vault ./my-notes --taskbridge --yes --json`
- **THEN** Pinax SHALL refuse the write with `PLANNING_BLOCK_CONFLICT`
- **AND** it SHALL include a safe next action rather than guessing an insertion point

