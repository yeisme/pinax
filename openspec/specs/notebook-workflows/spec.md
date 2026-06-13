# notebook-workflows Specification

## Purpose

定义 Pinax 本地笔记软件工作流：daily/inbox、组织浏览、链接/附件、saved views 和 Markdown import/export。所有能力都以本地 vault 为真源，不依赖外部服务。
## Requirements
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

### Requirement: Inbox workflow supports fast capture and triage
Pinax SHALL provide an inbox workflow for quick capture and later organization.

#### Scenario: Capture inbox note
- **WHEN** a user runs `pinax inbox capture "想法" --body "正文" --tags inbox --vault ./my-notes --json`
- **THEN** Pinax SHALL create a note under `notes/inbox/`
- **AND** the note frontmatter SHALL include `kind: inbox` and `status: inbox`
- **AND** the created note SHALL be added to the daily index and local index.

#### Scenario: List inbox notes
- **WHEN** a user runs `pinax inbox list --vault ./my-notes --json`
- **THEN** Pinax SHALL list notes with inbox status or inbox kind
- **AND** the JSON facts SHALL include total and returned counts.

#### Scenario: Triage inbox note into project folder
- **WHEN** a user runs `pinax inbox triage note_123 --group work --folder ideas --kind reference --status active --vault ./my-notes --json`
- **THEN** Pinax SHALL update the note frontmatter and move it into the selected group/folder path through the application service
- **AND** it SHALL fail with `note_path_conflict` if the target path already exists.

### Requirement: Notebook organization views are discoverable
Pinax SHALL expose local organization dimensions as first-class readable views under `pinax note` while preserving old root dimension commands as compatibility aliases.

#### Scenario: List tags with counts
- **WHEN** a user runs `pinax note tags --vault ./my-notes --json`
- **THEN** Pinax SHALL return tags and note counts from the current vault index or scan fallback
- **AND** stdout SHALL contain no human prose outside the JSON envelope.

#### Scenario: List folders with counts
- **WHEN** a user runs `pinax note folders --vault ./my-notes --json`
- **THEN** Pinax SHALL return vault-relative note folders and counts
- **AND** it SHALL NOT include `.pinax`, `.git`, `dist`, or paths outside the vault.

#### Scenario: List kinds and groups
- **WHEN** a user runs `pinax note kinds --vault ./my-notes --json` or `pinax note groups --vault ./my-notes --json`
- **THEN** Pinax SHALL return kind or group values with counts
- **AND** missing values SHALL be represented with a stable empty bucket fact rather than crashing.

#### Scenario: Root dimension aliases remain compatible
- **WHEN** a user runs `pinax tag list --vault ./my-notes --json`, `pinax folder list --vault ./my-notes --json`, `pinax kind list --vault ./my-notes --json`, or `pinax group list --vault ./my-notes --json`
- **THEN** Pinax SHALL preserve backwards-compatible behavior and machine output fields
- **AND** those root aliases MAY be hidden from primary root help.

### Requirement: Links and backlinks are inspectable
Pinax SHALL let users inspect note links, backlinks, orphan notes, unresolved references, ambiguous references, and local bidirectional graph facts from local Markdown content.

#### Scenario: Show note outgoing links
- **WHEN** a user runs `pinax note links note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL return wiki links and Markdown note links found in the note body
- **AND** each link SHALL include source path, target text, link kind, resolved target path when available, broken status, ambiguous status, alias when available, heading when available, and line number when available.

#### Scenario: Show note backlinks
- **WHEN** a user runs `pinax note backlinks note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL return notes that link to the target note by note id, vault-relative path, exact title, unique case-insensitive title, or wiki reference
- **AND** it SHALL include stable facts for backlink count, resolved count, broken count, ambiguous count, and unresolved count.

#### Scenario: Show ambiguous backlink candidates
- **WHEN** a target reference could match multiple notes
- **AND** a user runs `pinax note backlinks <target> --vault ./my-notes --json`
- **THEN** Pinax SHALL fail or return partial graph facts with stable error code `note_ref_ambiguous` or `link_target_ambiguous`
- **AND** the projection SHALL include candidate paths or note ids without selecting one automatically.

#### Scenario: List orphan notes
- **WHEN** a user runs `pinax note orphans --vault ./my-notes --json`
- **THEN** Pinax SHALL list notes with no incoming and no outgoing note links by default
- **AND** system index notes SHALL NOT be counted as ordinary orphans.

#### Scenario: Classify partial orphans
- **WHEN** a user runs `pinax note orphans --mode no-incoming --vault ./my-notes --json` or `pinax note orphans --mode no-outgoing --vault ./my-notes --json`
- **THEN** Pinax SHALL list notes matching the selected orphan class
- **AND** stdout facts SHALL include the selected mode and returned count.

### Requirement: Attachments are managed inside the vault
Pinax SHALL let users attach local files to notes while keeping attachments inside the vault boundary.

#### Scenario: Attach local file to note
- **WHEN** a user runs `pinax note attach note_123 ./diagram.png --vault ./my-notes --json`
- **THEN** Pinax SHALL copy the file into a vault attachment directory
- **AND** it SHALL append or return a Markdown reference to the attachment
- **AND** stdout SHALL include source path, vault-relative attachment path, and note path without leaking external secrets.

#### Scenario: List note attachments
- **WHEN** a user runs `pinax note attachments note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL return attachment references found in the note body
- **AND** each item SHALL include whether the target file exists inside the vault.

#### Scenario: Reject attachment outside allowed source path when missing
- **WHEN** a user runs `pinax note attach note_123 ./missing.png --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `attachment_source_missing`
- **AND** no note body or vault attachment file SHALL be modified.

### Requirement: Saved views store reusable local filters
Pinax SHALL let users save and reuse common note list filters and database-style local views through CLI-authored structured assets.

#### Scenario: Save a note view
- **WHEN** a user runs `pinax view save active-work --group work --status active --kind reference --sort updated --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update `.pinax/views.json` through the application service
- **AND** the saved view SHALL store filters rather than note result snapshots.

#### Scenario: Show a saved view
- **WHEN** a user runs `pinax view show active-work --vault ./my-notes --json`
- **THEN** Pinax SHALL resolve the saved filters and return current matching notes
- **AND** it SHALL report the saved view name and filter facts.

#### Scenario: Delete a saved view with approval
- **WHEN** a user runs `pinax view delete active-work --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL remove only that view from `.pinax/views.json`
- **AND** it SHALL NOT delete notes or attachments.

#### Scenario: Save a database table view
- **WHEN** a user runs `pinax view save active-projects --query 'SELECT title, status, due FROM notes WHERE tags CONTAINS "project" AND status = "active" ORDER BY due ASC LIMIT 50' --kind table --vault ./my-notes --json`
- **THEN** Pinax SHALL store a database-style view definition through the application service
- **AND** the saved view SHALL store query text, display kind, columns, limit, and display options rather than result rows.

#### Scenario: Show database table view through legacy view command
- **WHEN** a user runs `pinax view show active-projects --vault ./my-notes --json`
- **AND** the saved view is a database-style table view
- **THEN** Pinax SHALL execute the saved query against the current local index projection
- **AND** it SHALL return table columns, rows, filters, sorts, engine, index status, and pagination facts.

#### Scenario: Old filter-only views remain compatible
- **WHEN** `.pinax/views.json` contains an older filter-only saved view
- **AND** a user runs `pinax view show <name> --vault ./my-notes --json`
- **THEN** Pinax SHALL treat it as a database view with equivalent filters and default columns
- **AND** it SHALL NOT require the user to hand-edit the view registry.

### Requirement: Local import and export preserve Markdown portability
Pinax SHALL support local Markdown import and export without external provider dependencies.

#### Scenario: Dry run Markdown directory import
- **WHEN** a user runs `pinax import markdown ./source --group research --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL report planned note paths, conflicts, and skipped files
- **AND** it SHALL NOT write notes, `.pinax` receipts, Git state, or provider state.

#### Scenario: Apply Markdown import
- **WHEN** a user runs `pinax import markdown ./source --group research --conflict rename --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL copy Markdown files into the vault with Pinax frontmatter normalized when needed
- **AND** it SHALL record a redacted import receipt through the application service.

#### Scenario: Export Markdown bundle
- **WHEN** a user runs `pinax export markdown ./out --tag research --vault ./my-notes --json`
- **THEN** Pinax SHALL export matching Markdown notes and referenced attachments into the output directory
- **AND** it SHALL write an export receipt without storing provider credentials or raw external payloads.

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
Pinax SHALL provide task-oriented built-in note templates that create useful notes with minimal required variables, and SHALL apply v2 note template metadata through the note application service when creating notes.

#### Scenario: Quick note template creates root note
- **WHEN** a user runs `pinax note add "Demo" --template note.quick --vault ./my-notes --json`
- **THEN** Pinax SHALL create a registered note at `demo.md` or an equivalent safe root-level slug path
- **AND** the note SHALL include a useful title and minimal editable body without requiring custom variables
- **AND** template defaults such as `kind` and `status` SHALL be applied unless explicit CLI flags override them.

#### Scenario: Inbox capture template creates triageable note
- **WHEN** a user runs `pinax note add "Later idea" --template inbox.capture --vault ./my-notes --json`
- **THEN** Pinax SHALL create a registered note under `inbox/` using the template `output.path_pattern` or an equivalent safe root-relative content path
- **AND** the frontmatter SHALL classify it as `kind: inbox` and `status: inbox`
- **AND** stdout SHALL include a next action for inbox triage or template preview.

#### Scenario: Meeting template includes action section
- **WHEN** a user runs `pinax note add "客户同步" --template meeting.notes --var participants=Acme --vault ./my-notes --json`
- **THEN** Pinax SHALL create a registered note under `meetings/` using the template `output.path_pattern` unless an explicit destination flag overrides it
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

#### Scenario: Explicit note fields override template defaults
- **WHEN** a user runs `pinax note add "Later idea" --template inbox.capture --kind reference --status active --dir custom --vault ./my-notes --json`
- **THEN** Pinax SHALL prefer explicit CLI fields over template defaults and output path pattern
- **AND** stdout SHALL include facts showing the effective path, kind, status, and template name.

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

### Requirement: Draft workflow supports reviewable authoring

Pinax SHALL provide a draft workflow for user-authored notes that are not ready for ordinary active notebook surfaces, while keeping Markdown notes as the source of truth.

#### Scenario: Create draft note
- **WHEN** a user runs `pinax draft create "草稿想法" --body "先写一版" --vault ./my-notes --json`
- **THEN** Pinax SHALL create a registered Markdown note under `drafts/` or an equivalent safe draft folder through the application service
- **AND** the note frontmatter SHALL include `status: draft`
- **AND** Pinax SHALL NOT force `kind: draft` when the user or selected template provides another note kind.

#### Scenario: List draft notes
- **WHEN** a user runs `pinax draft list --vault ./my-notes --json`
- **THEN** Pinax SHALL list notes whose managed lifecycle status is `draft`
- **AND** the JSON envelope facts SHALL include total count, returned count, status filter, index status when available, and one next action for previewing or promoting a draft.

#### Scenario: Show draft note through bounded note display
- **WHEN** a user runs `pinax draft show note_123 --view rendered --vault ./my-notes --json`
- **THEN** Pinax SHALL return the same bounded note display projection used by `pinax note show`
- **AND** it SHALL include stable facts for note id, path, title, status, lifecycle status, view, and body exposure mode.

#### Scenario: Promote draft to active note
- **WHEN** a user runs `pinax draft promote note_123 --status active --folder research --kind reference --vault ./my-notes --json`
- **THEN** Pinax SHALL update the note frontmatter and optional path through the application service
- **AND** it SHALL append redacted event and record metadata evidence
- **AND** it SHALL refresh the local index after the successful write.

#### Scenario: Discard draft without hard delete
- **WHEN** a user runs `pinax draft discard note_123 --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL set the note lifecycle status to `discarded` through the application service
- **AND** it SHALL NOT hard delete the Markdown file or attachments
- **AND** stdout facts SHALL include `deleted=false` and a next action for `pinax note delete` if the user wants real deletion.

### Requirement: Inbox workflow supports review actions

Pinax SHALL extend inbox capture and triage with review actions that let users and remote clients inspect, promote, or discard inbox items without direct file manipulation.

#### Scenario: Show inbox item
- **WHEN** a user runs `pinax inbox show note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL return a bounded note display projection for the inbox item
- **AND** the projection SHALL include status `inbox`, lifecycle status `inbox`, path, title, tags, and recommended next actions.

#### Scenario: Promote inbox item to draft
- **WHEN** a user runs `pinax inbox promote note_123 --to draft --vault ./my-notes --json`
- **THEN** Pinax SHALL update only controlled metadata and optional safe path fields through the application service
- **AND** the resulting note SHALL have lifecycle status `draft`
- **AND** the local index SHALL be refreshed after the successful write.

#### Scenario: Promote inbox item to active note
- **WHEN** a user runs `pinax inbox promote note_123 --to active --group work --folder ideas --kind reference --vault ./my-notes --json`
- **THEN** Pinax SHALL set the note status to `active` and move it to the selected safe target path through the application service
- **AND** it SHALL fail with stable error code `note_path_conflict` if the target path already exists.

#### Scenario: Discard inbox item without deleting content
- **WHEN** a user runs `pinax inbox discard note_123 --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL set lifecycle status `discarded`
- **AND** it SHALL NOT hard delete Markdown, attachments, `.pinax` structured assets, Git state, provider state, or remote service state.

### Requirement: Review lifecycle transitions are service-owned

Pinax SHALL enforce inbox and draft lifecycle transitions through application services and SHALL reject direct or invalid workflow transitions.

#### Scenario: Invalid lifecycle transition is rejected
- **WHEN** a user runs `pinax draft promote note_123 --status inbox --vault ./my-notes --json`
- **THEN** Pinax SHALL fail with stable error code `invalid_lifecycle_transition`
- **AND** it SHALL NOT modify Markdown, `.pinax` assets, index projection, Git state, provider state, or remote services.

#### Scenario: Dry-run lifecycle transition has no side effects
- **WHEN** a user runs `pinax inbox promote note_123 --to active --dry-run --vault ./my-notes --json`
- **THEN** Pinax SHALL return a reviewable transition plan with planned status, planned path, risk, and required approval facts
- **AND** it SHALL NOT write Markdown, `.pinax` events, record metadata, index projection, Git state, provider state, or remote services.

#### Scenario: Successful lifecycle transition records evidence
- **WHEN** Pinax completes an approved inbox or draft lifecycle transition
- **THEN** it SHALL append a redacted event, append record metadata evidence when record ledger is available, and refresh the local index
- **AND** stdout SHALL include stable facts for old status, new status, path, writes, record event, and index update status.

