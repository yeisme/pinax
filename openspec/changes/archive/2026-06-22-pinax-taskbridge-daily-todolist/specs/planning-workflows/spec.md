# planning-workflows Delta

## ADDED Requirements

### Requirement: Daily TaskBridge planning SHALL write a timestamped Markdown todolist through Pinax

Pinax SHALL let `pinax plan daily --taskbridge` consume TaskBridge daily task facts and, when explicitly approved, write a timestamped Markdown todolist into today's daily note through a Pinax-managed block.

#### Scenario: TaskBridge daily dry-run is read-only
- **GIVEN** a Pinax vault and a valid `taskbridge agent today` response
- **WHEN** the user runs `pinax plan daily --vault ./my-notes --taskbridge --dry-run --json`
- **THEN** stdout SHALL contain one Pinax JSON envelope with `source=taskbridge`, `captured_at`, selected commitment count, target daily note path, and recommended next action
- **AND** Pinax SHALL NOT modify Markdown notes, `.pinax` planning assets, Git state, TaskBridge state, provider state, or remote services

#### Scenario: approved daily plan writes managed block
- **GIVEN** today's daily note is missing or has a valid `planning-daily` managed block
- **WHEN** the user runs `pinax plan daily --vault ./my-notes --taskbridge --yes`
- **THEN** Pinax SHALL create or update `daily/YYYY-MM-DD.md` through journal and planning services
- **AND** it SHALL write a Markdown todolist inside `<!-- pinax:managed name=planning-daily -->` and `<!-- /pinax:managed -->`
- **AND** the block SHALL include `Captured at: <RFC3339 UTC>`
- **AND** Pinax SHALL preserve user-authored content outside the managed block

#### Scenario: TaskBridge adapter failure is safe
- **GIVEN** `taskbridge` is unavailable, returns invalid JSON, or returns an unsupported schema
- **WHEN** the user runs `pinax plan daily --vault ./my-notes --taskbridge --yes --json`
- **THEN** Pinax SHALL return `TASKBRIDGE_UNAVAILABLE` or `TASKBRIDGE_CONTRACT_UNSUPPORTED`
- **AND** it SHALL NOT write Markdown notes, `.pinax` planning assets, Git state, TaskBridge state, provider state, or remote services

#### Scenario: daily planning actions remain drafts
- **GIVEN** a TaskBridge daily planning decision has deferred candidates
- **WHEN** the user runs `pinax plan actions --vault ./my-notes --from daily --taskbridge --save --json`
- **THEN** Pinax SHALL write a `taskbridge.actions.v1` draft under `.pinax/planning/actions/`
- **AND** the next action SHALL be `taskbridge agent execute --action-file <path> --dry-run`
- **AND** Pinax SHALL NOT call `taskbridge agent execute --confirm`

