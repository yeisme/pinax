## MODIFIED Requirements

### Requirement: Daily workflow supports review and capture
Pinax SHALL provide local daily, weekly, and monthly journal workflows without requiring external services, and SHALL create new journal notes from inspectable journal templates.

#### Scenario: Open or create today's daily note from journal template
- **WHEN** a user runs `pinax journal daily open --template journal.daily --vault ./my-notes --json`
- **THEN** Pinax SHALL create `daily/YYYY-MM-DD.md` if missing through the application service
- **AND** the created note SHALL use the resolved `journal.daily` template body, defaults, and safe output path pattern
- **AND** stdout SHALL contain one JSON envelope with command `journal.daily.open`, daily note path, date, template name, and editor facts when an editor is executed.

#### Scenario: Existing daily note is not rewritten
- **GIVEN** `daily/YYYY-MM-DD.md` already exists
- **WHEN** a user runs `pinax journal daily open --template journal.daily --vault ./my-notes --json`
- **THEN** Pinax SHALL return the existing note projection
- **AND** it SHALL NOT rewrite the note body, frontmatter, managed blocks, `.pinax` render artifacts, provider state, or Git state.

#### Scenario: Legacy notes daily note remains readable
- **GIVEN** `notes/daily/YYYY-MM-DD.md` exists from an older vault layout and `daily/YYYY-MM-DD.md` does not exist
- **WHEN** a user runs `pinax journal daily open --template journal.daily --vault ./my-notes --json`
- **THEN** Pinax SHALL return the legacy daily note projection or a compatibility projection with stable legacy path facts
- **AND** it SHALL NOT silently move, copy, rewrite, or delete the legacy note.

#### Scenario: Daily capture index updates managed block only
- **GIVEN** today's daily note contains `<!-- pinax:managed name=daily-captures -->` and `<!-- /pinax:managed -->`
- **WHEN** Pinax records a new captured note that should appear in today's capture index
- **THEN** Pinax SHALL update only the `daily-captures` managed block through the application service
- **AND** it SHALL preserve all user-authored Markdown outside that block.

#### Scenario: Daily capture index refuses ambiguous legacy note
- **GIVEN** today's daily note does not contain a `daily-captures` managed block
- **WHEN** Pinax attempts to append a daily capture index entry
- **THEN** Pinax SHALL fail or return a partial projection with stable error code `managed_block_missing`
- **AND** it SHALL include a next action that recommends creating or upgrading the daily note with `journal.daily`
- **AND** it SHALL NOT guess an insertion point or rewrite the note body.

#### Scenario: Weekly and monthly journals use period templates
- **WHEN** a user runs `pinax journal weekly open --template journal.weekly --vault ./my-notes --json` or `pinax journal monthly open --template journal.monthly --vault ./my-notes --json`
- **THEN** Pinax SHALL create the corresponding `weekly/YYYY-Www.md` or `monthly/YYYY-MM.md` note when missing
- **AND** the created note SHALL use the selected journal template and period-specific context facts.

## ADDED Requirements

### Requirement: Journal templates are inspectable workflow templates
Pinax SHALL expose built-in journal templates as first-class templates that users and agents can inspect, preview, override, and use for journal creation.

#### Scenario: Inspect daily journal template
- **WHEN** a user runs `pinax template inspect journal.daily --vault ./my-notes --json`
- **THEN** Pinax SHALL report template kind `journal_template`, output path pattern, required variables, default note metadata, query count, managed block names, and whether the template is refreshable
- **AND** stdout SHALL contain one JSON envelope without leaking secrets, raw provider payloads, raw prompts, or hidden system instructions.

#### Scenario: Preview daily journal template without writing
- **WHEN** a user runs `pinax template preview journal.daily --vault ./my-notes --json`
- **THEN** Pinax SHALL render the template using safe example context and bounded query results when available
- **AND** it SHALL NOT create or modify notes, `.pinax` structured assets, Git state, provider state, or remote services.

#### Scenario: User template override wins over built-in template
- **GIVEN** `.pinax/templates/journal.daily.md` exists inside the vault
- **WHEN** a user runs `pinax journal daily open --template journal.daily --vault ./my-notes --json`
- **THEN** Pinax SHALL use the vault-local template file instead of the built-in fallback
- **AND** it SHALL validate the template output path remains inside the vault content boundary before writing
- **AND** it SHALL reject paths that are absolute, contain `..`, or target reserved directories such as `.pinax/`, `.git/`, `attachments/`, `temp/`, `dist/`, `node_modules/`, or `vendor/`.

### Requirement: Default note creation uses vault root content layout
Pinax SHALL create new user notes in the vault root content layout by default while preserving compatibility with existing `notes/` paths.

#### Scenario: Default note add writes to vault root
- **WHEN** a user runs `pinax note add "demo" --vault ./my-notes --json`
- **THEN** Pinax SHALL create a registered Pinax Markdown note at `demo.md` or an equivalent safe root-level slug path
- **AND** it SHALL NOT place the note under `notes/` unless the user explicitly chooses a `notes/` legacy folder or project prefix.

#### Scenario: Note add with directory writes under root-relative folder
- **WHEN** a user runs `pinax note add "idea" --dir inbox --vault ./my-notes --json`
- **THEN** Pinax SHALL create the note under `inbox/` inside the vault content boundary
- **AND** it SHALL reject reserved directories such as `.pinax`, `.git`, `attachments`, `temp`, `dist`, `node_modules`, and `vendor`.

#### Scenario: Legacy notes path remains referenceable
- **GIVEN** an existing registered note lives at `notes/work/design.md`
- **WHEN** a user runs `pinax note show notes/work/design.md --vault ./my-notes --json`
- **THEN** Pinax SHALL resolve and show that legacy note
- **AND** new default note creation SHALL still prefer root-relative paths.

### Requirement: Built-in note templates cover common workflows
Pinax SHALL provide task-oriented built-in note templates that create useful notes with minimal required variables.

#### Scenario: Quick note template creates root note
- **WHEN** a user runs `pinax note add "Demo" --template note.quick --vault ./my-notes --json`
- **THEN** Pinax SHALL create a registered note at `demo.md` or an equivalent safe root-level slug path
- **AND** the note SHALL include a useful title and minimal editable body without requiring custom variables.

#### Scenario: Inbox capture template creates triageable note
- **WHEN** a user runs `pinax note add "Later idea" --template inbox.capture --vault ./my-notes --json`
- **THEN** Pinax SHALL create a registered note under `inbox/`
- **AND** the frontmatter SHALL classify it as `kind: inbox` and `status: inbox`
- **AND** stdout SHALL include a next action for inbox triage or template preview.

#### Scenario: Meeting template includes action section
- **WHEN** a user runs `pinax note add "客户同步" --template meeting.notes --var participants=Acme --vault ./my-notes --json`
- **THEN** Pinax SHALL create a registered note under `meetings/`
- **AND** the body SHALL include sections for conclusion, discussion, action items, and links
- **AND** the created note SHALL be searchable as `kind=meeting`.

#### Scenario: Decision template creates decision record
- **WHEN** a user runs `pinax note add "选择同步策略" --template decision.record --vault ./my-notes --json`
- **THEN** Pinax SHALL create a registered note under `decisions/`
- **AND** the body SHALL include background, options, decision, impact, and review date sections
- **AND** the created note SHALL be searchable as `kind=decision`.

#### Scenario: Project brief template creates project note
- **WHEN** a user runs `pinax note add "Pinax 模板体验" --template project.brief --vault ./my-notes --json`
- **THEN** Pinax SHALL create a registered note under `projects/`
- **AND** the body SHALL include goal, current status, milestones, decisions, meetings, and risks sections
- **AND** the created note SHALL be searchable as `kind=project`.

#### Scenario: Learning and research templates create focused notes
- **WHEN** a user runs `pinax note add "Go Template" --template learning.video --var url=https://go.dev --vault ./my-notes --json` or `pinax note add "本地优先架构" --template research.topic --vault ./my-notes --json`
- **THEN** Pinax SHALL create registered notes under `learning/` or `research/`
- **AND** each template SHALL include sections that support later review, evidence capture, conclusions, and next steps.

### Requirement: Template recommendation helps users choose templates
Pinax SHALL let users discover templates by intent, use case, pack, and starter status without requiring memorized template names.

#### Scenario: List starter templates first
- **WHEN** a user runs `pinax template list --pack starter --vault ./my-notes --json`
- **THEN** Pinax SHALL list starter templates such as `note.quick`, `inbox.capture`, `meeting.notes`, `decision.record`, and `project.brief`
- **AND** each item SHALL include template name, kind, source, use cases, output path pattern, starter status, and one recommended next action.

#### Scenario: Recommend template by intent
- **WHEN** a user runs `pinax template recommend --intent meeting --vault ./my-notes --json`
- **THEN** Pinax SHALL return `meeting.notes` as the primary recommendation or an equivalent meeting template
- **AND** it SHALL include at most three alternatives
- **AND** it SHALL NOT call external providers, execute templates, execute SQL, write `.pinax` state, write Markdown, or access the network.

#### Scenario: Recommendation falls back to capture templates
- **WHEN** a user runs `pinax template recommend --intent unknown-intent --vault ./my-notes --json`
- **THEN** Pinax SHALL return a conservative fallback such as `note.quick` or `inbox.capture`
- **AND** it SHALL include a real command the user can run next.
