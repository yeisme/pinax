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

#### Scenario: Wiki links preserve alias and heading
- **WHEN** a note contains `[[Title|Alias]]`, `[[Title#Heading]]`, or `[[Title#Heading|Alias]]`
- **THEN** `pinax note links <note> --vault ./my-notes --json` SHALL return wiki link edges
- **AND** each edge SHALL preserve raw target, normalized target, alias when present, heading when present, link kind, line number, and resolution status.

#### Scenario: Ambiguous wiki targets are not guessed
- **GIVEN** multiple notes can satisfy the same title, alias, or filename stem
- **WHEN** a note links to that target with `[[Target]]`
- **THEN** Pinax SHALL mark the link edge as `ambiguous`
- **AND** the edge SHALL include candidate paths or note ids without selecting one automatically.

#### Scenario: Non-note wiki embeds do not become broken note links
- **WHEN** a note contains `![[image.png]]` or wiki-style non-Markdown asset references
- **THEN** Pinax SHALL NOT count those references as broken note graph edges
- **AND** asset reference handling MAY report them through asset projections instead.

#### Scenario: Link repair remains reviewable
- **WHEN** `pinax repair plan --vault ./my-notes --json` detects a broken or ambiguous note link
- **THEN** the plan SHALL use `manual_review` operations such as `link_resolution` or `link_rewrite`
- **AND** Pinax SHALL NOT automatically rewrite the Markdown body.

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

#### Scenario: Parked idea seed template creates an idea note
- **WHEN** a user runs `pinax note add "某篇小说是怎么写成的" --template idea.research_seed --vault ./my-notes --json`
- **THEN** Pinax SHALL create a Markdown note under `ideas/research/`
- **AND** the note frontmatter SHALL include `kind: idea`, `status: parked`, and tags including `idea` and `research-seed`
- **AND** the template body SHALL use Chinese headings for trigger, value, questions, leads, and related notes without creating todo checkboxes.

#### Scenario: Sticky template creates a short inbox note
- **WHEN** a user runs `pinax note add "临时线索" --template sticky.capture --vault ./my-notes --json`
- **THEN** Pinax SHALL create a Markdown note under `inbox/sticky/`
- **AND** the note frontmatter SHALL include `kind: sticky`, `status: inbox`, and tags including `sticky` and `capture`
- **AND** the template body SHALL remain a short capture note without creating todo checkboxes, `board_column`, or managed project item metadata.

#### Scenario: Sticky project signal keeps project context without becoming a board item
- **WHEN** a user runs `pinax note add "子项目看板线索" --template sticky.project_signal --project research --folder inbox --vault ./my-notes --json`
- **THEN** Pinax SHALL create a Markdown note in the project inbox path
- **AND** the note SHALL keep `kind: sticky` and `status: inbox`
- **AND** it SHALL NOT write `board_column` or `kind: task`; creating a movable board item SHALL continue to require `pinax project item add`.

#### Scenario: Ideas index template creates a managed index page
- **WHEN** a user runs `pinax index page create ideas --template index.ideas --vault ./my-notes --json`
- **THEN** Pinax SHALL create a managed index page for notes where `kind` is `idea` and `status` is `parked`.

#### Scenario: Explicit note fields override template defaults
- **WHEN** a user runs `pinax note add "Later idea" --template inbox.capture --kind reference --status active --dir custom --vault ./my-notes --json`
- **THEN** Pinax SHALL prefer explicit CLI fields over template defaults and output path pattern
- **AND** stdout SHALL include facts showing the effective path, kind, status, and template name.

### Requirement: Template recommendation helps users choose templates

Pinax SHALL recommend workflow starters from local template metadata by intent, use case, pack, lifecycle, and readiness without requiring memorized template names or external provider calls.

#### Scenario: Recommend workflow starter by intent

- **WHEN** a user runs `pinax template recommend --intent meeting --vault ./my-notes --json`
- **THEN** Pinax SHALL return a primary recommendation such as `meeting.notes` and at most three alternatives
- **AND** the JSON output SHALL preserve existing envelope, facts, actions, and template fields
- **AND** the recommendation MAY include optional workflow fields for `scenario_id`, `maturity`, `pack`, `fit_reason`, `preview_command`, `create_command`, `proof_gate`, and `after_create_actions`
- **AND** it SHALL NOT call external providers, execute templates, execute SQL, write `.pinax` state, write Markdown, mutate Git state, or access the network.

#### Scenario: Recommend conservative fallback with next command

- **WHEN** a user runs `pinax template recommend --intent unknown-intent --vault ./my-notes --json`
- **THEN** Pinax SHALL return a conservative capture workflow such as `note.quick`, `inbox.capture`, or `sticky.capture`
- **AND** the recommendation SHALL include a real preview or create command the user can run next
- **AND** it SHALL mark the fit as fallback or low confidence without inventing an unsupported scenario.

#### Scenario: Agent output remains stable while recommendation grows

- **WHEN** a user runs `pinax template recommend --intent "便签" --vault ./my-notes --agent`
- **THEN** stdout SHALL remain stable key=value output
- **AND** new workflow fields SHALL be added as optional keys such as `recommendation.0.scenario_id` or `recommendation.0.proof_gate`
- **AND** stdout SHALL NOT include localized prose, raw prompts, provider payloads, secrets, Authorization headers, hidden system prompts, or full chain-of-thought.

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

### Requirement: Notes MAY belong to a project subproject

Pinax SHALL allow notes and managed project items to carry an optional `subproject` field inside a project while preserving existing project-only note behavior.

#### Scenario: Add note to subproject directory
- **WHEN** the user runs `pinax note add "Stock Learning Charter" --project research --subproject stock-learning --dir projects/stock-learning/00-charter --body "目标：建立个人股票学习和研究流程。" --vault yeisme-notes --json`
- **THEN** Pinax SHALL create the note through the application service inside the vault content boundary
- **AND** frontmatter SHALL include project and subproject facts
- **AND** `.pinax` structured assets SHALL NOT be hand-written by the caller.

#### Scenario: Project-only notes remain compatible
- **WHEN** the user runs `pinax note add "Research Log" --project research --vault yeisme-notes --json`
- **THEN** Pinax SHALL preserve existing project-only behavior
- **AND** it SHALL NOT require a subproject field.

#### Scenario: Subproject directory cannot escape vault
- **WHEN** a note command combines `--project`, `--subproject`, and `--dir` with a path that escapes the vault or targets a reserved directory
- **THEN** Pinax SHALL fail with a stable error code
- **AND** it SHALL NOT write Markdown, `.pinax` assets, Git state, provider state, or remote state.

### Requirement: Pinax SHALL provide an Obsidian compatibility pack

Pinax SHALL support common Obsidian-style Markdown vault structures as local source material while keeping Pinax-owned metadata, repairs, views, and receipts behind CLI/application service boundaries.

#### Scenario: Obsidian-style vault can be inspected safely

- **GIVEN** a vault contains Markdown notes, wikilinks, aliases, headings, properties/frontmatter, daily notes, attachments, templates, `.obsidian/**`, and plugin metadata
- **WHEN** the user runs `pinax vault doctor --vault ./my-notes --json`
- **THEN** Pinax SHALL inspect supported note, link, property, attachment, template, and index facts
- **AND** it SHALL treat `.obsidian/**` and unknown plugin metadata as non-Pinax-owned inputs unless a future explicit importer is selected
- **AND** it SHALL NOT rewrite Obsidian config or plugin metadata.

#### Scenario: Link repair is plan-first

- **WHEN** Pinax finds broken, ambiguous, or conflicting wikilinks in an Obsidian-style vault
- **THEN** `pinax repair plan --vault ./my-notes --json` SHALL report candidates, risks, and proposed edits without modifying note bodies
- **AND** applying a repair SHALL require explicit approval and snapshot protection according to the normal proof loop.

#### Scenario: Properties remain user-editable Markdown

- **WHEN** a user edits note frontmatter properties in Obsidian or a text editor
- **THEN** Pinax SHALL read and index those properties as source facts
- **AND** Pinax SHALL NOT overwrite unknown properties unless a user-approved metadata or repair plan explicitly owns the change.

### Requirement: Obsidian-style graph and backlink facts SHALL be bounded

Pinax SHALL expose graph, backlinks, orphan notes, unresolved references, aliases, headings, and block references as bounded facts suitable for agents and dashboards.

#### Scenario: Backlink projection includes ambiguity facts

- **WHEN** a user runs `pinax note backlinks <target> --vault ./my-notes --json`
- **THEN** stdout SHALL include backlink count, resolved count, broken count, ambiguous count, candidate paths or note ids when applicable, alias facts when applicable, and index status
- **AND** it SHALL NOT automatically choose between ambiguous candidates.

#### Scenario: Graph projection is body-safe

- **WHEN** a user runs `pinax graph show --vault ./my-notes --json` or an equivalent graph command
- **THEN** stdout SHALL include bounded node and edge facts, graph scope, filters, warnings, and next actions
- **AND** it SHALL NOT include full note bodies, provider payloads, raw prompts, secrets, or hidden system prompts.

### Requirement: Obsidian-compatible workflows SHALL remain local-first

Pinax SHALL let users use Obsidian-like workflows without requiring Obsidian itself, external plugins, cloud services, provider credentials, or network access for core local behavior.

#### Scenario: Daily notes and templates work without external plugins

- **WHEN** the user runs `pinax journal daily open --template journal.daily --vault ./my-notes --json`
- **THEN** Pinax SHALL create or show a local daily note from inspectable templates
- **AND** it SHALL NOT require Obsidian, DataviewJS, Templater, Lark, Notion, Pinax Cloud, provider tokens, cookies, or network access.

#### Scenario: Publish plan treats vault as source and output as artifact

- **WHEN** the user runs `pinax publish plan --profile public --target github-pages --vault ./my-notes --json`
- **THEN** Pinax SHALL plan a generated publish artifact from local vault content and configured profile
- **AND** GitHub Pages, Wiki, or other publish targets SHALL NOT become the note source of truth.

#### Scenario: Plugin failures cannot replace core behavior

- **WHEN** a Pinax plugin or Obsidian-origin plugin metadata is present but invalid, disabled, or unsupported
- **THEN** core local commands for note list/show, search, query, backlinks, vault doctor, database view render, project board show, and publish plan SHALL continue to work or return bounded warnings
- **AND** plugin failure SHALL NOT corrupt Markdown, `.pinax/**`, index, sync state, provider state, or Git state.

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

### Requirement: Templates are workflow starters

Pinax SHALL treat executable templates as workflow starters that declare intent, scenario, variables, output policy, maturity, proof gate, pack, lifecycle, and after-create actions through local metadata.

#### Scenario: Inspect exposes workflow starter metadata

- **WHEN** a user runs `pinax template inspect meeting.notes --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with command `template.inspect`
- **AND** existing facts such as `template`, `template_kind`, `engine`, `path_pattern`, and `source` SHALL remain present
- **AND** Pinax MAY add optional workflow metadata for `scenario_id`, `intents`, `variable_schema`, `output_policy`, `maturity`, `pack`, `lifecycle`, `proof_gate`, and `after_create_actions`.

#### Scenario: Design drafts are not primary executable recommendations

- **GIVEN** a template declares lifecycle `draft_design`
- **WHEN** a user runs `pinax template recommend --intent "meeting" --vault ./my-notes --json`
- **THEN** Pinax SHALL NOT present that draft as the primary executable create path
- **AND** it MAY show the draft as a design-only alternative when output explicitly marks it as non-executable.

#### Scenario: Deprecated templates recommend replacements without removal

- **GIVEN** a template declares lifecycle `deprecated` and replacement `meeting.notes.v2`
- **WHEN** a user runs `pinax template inspect meeting.notes --vault ./my-notes --json`
- **THEN** Pinax SHALL mark the template as deprecated
- **AND** the output SHALL include a replacement preview or inspect command
- **AND** Pinax SHALL NOT delete or rewrite the existing template as part of inspect, recommend, or preview.

### Requirement: Template preview describes write impact and proof gate

Pinax SHALL make template preview a read-only workflow review that explains variables, output path policy, body exposure, proof gate, and next command before any write.

#### Scenario: Preview workflow starter is read-only

- **WHEN** a user runs `pinax template preview meeting.notes --title "Client Meeting" --vault ./my-notes --json`
- **THEN** Pinax SHALL render a preview projection without writing notes, `.pinax` structured assets, render receipts, Git state, provider state, or remote services
- **AND** the output MAY include optional fields for required variables, effective output policy, proof gate, body exposure, and next command.

#### Scenario: Preview reports missing variables with rerun command

- **GIVEN** a workflow template requires variable `client`
- **WHEN** a user runs `pinax template preview meeting.notes --vault ./my-notes --json` without `--var client=...`
- **THEN** Pinax SHALL fail with stable error code `template_variable_missing`
- **AND** the error projection SHALL include a rerun command such as `pinax template preview meeting.notes --var client=... --vault ./my-notes --json`
- **AND** the rerun command SHALL NOT include secret-like original values, raw prompts, provider payloads, Authorization headers, hidden system prompts, or private tool arguments.

### Requirement: Template use produces reviewable evidence

Pinax SHALL expose template use evidence when a workflow starter creates a note, journal page, index page, or project workspace artifact through application services.

#### Scenario: Note created from template reports use evidence

- **WHEN** a user runs `pinax note add "Client Meeting" --template meeting.notes --dir index --vault ./my-notes --json`
- **THEN** Pinax SHALL create the Markdown note through the application service
- **AND** stdout SHALL preserve existing JSON envelope, facts, actions, note id, path, and template fields
- **AND** stdout MAY include optional evidence fields such as `template_use_id`, `template_pack`, `scenario_id`, `maturity`, `effective_path`, `receipt_ref`, `proof_gate`, and `next_actions`.

#### Scenario: Template use evidence is redacted

- **WHEN** a template-backed create command emits JSON, agent output, event evidence, or a receipt
- **THEN** Pinax SHALL NOT include raw provider payloads, hidden system prompts, private tool arguments, Authorization headers, cookies, tokens, secret-like variable values, or full chain-of-thought
- **AND** persisted receipt or event data SHALL be written only by Pinax CLI/application service, not by agent-authored file edits.

#### Scenario: Dry-run and preview do not write evidence receipts

- **WHEN** a user runs a template preview or a supported template-backed command with `--dry-run --json`
- **THEN** Pinax SHALL return planned operations or preview output
- **AND** it SHALL NOT write Markdown notes, `.pinax` structured assets, template use receipts, Git state, provider state, or remote services.

### Requirement: Local template packs are discoverable without marketplace behavior

Pinax SHALL support local template pack discovery for built-in and vault-local packs while excluding remote marketplace, scoring, and cloud sync behavior from the template catalog MVP.

#### Scenario: Built-in pack metadata is discoverable

- **WHEN** a user runs `pinax template list --pack starter --vault ./my-notes --json`
- **THEN** Pinax SHALL list matching built-in templates
- **AND** each listed item MAY include optional pack metadata such as pack id, source, readiness, lifecycle, and scenario ids
- **AND** existing list fields SHALL remain present for compatibility.

#### Scenario: Vault-local pack overrides are explicit

- **GIVEN** a vault-local template overrides a built-in template name
- **WHEN** a user runs `pinax template inspect <name> --vault ./my-notes --json`
- **THEN** Pinax SHALL identify the effective source as vault-local or override
- **AND** it SHALL NOT delete, rewrite, or silently publish the overridden built-in or local template.

#### Scenario: Remote template marketplace is not used

- **WHEN** a user runs `pinax template recommend --intent "stock learning" --vault ./my-notes --json`
- **THEN** Pinax SHALL use local metadata from built-in and vault-local templates only
- **AND** it SHALL NOT fetch remote packages, call a marketplace, send template metadata to a provider, or sync templates to a cloud service.

### Requirement: Template scenarios have readiness and handoff evidence

Pinax SHALL classify broad template workflow scenarios by readiness and expose validation, evidence, and handoff expectations in docs and OpenSpec.

#### Scenario: Scenario matrix distinguishes exploratory workflows

- **WHEN** a template pack or workflow scenario is documented
- **THEN** the scenario matrix SHALL include scenario id, target user, job-to-be-done, required artifacts, gate/review checks, evidence path, export/handoff path, validation command, and readiness label
- **AND** exploratory scenarios SHALL NOT be presented as production-ready.

#### Scenario: Project workspace consumes template output without owning template model

- **WHEN** a template-backed workflow creates or links a project workspace artifact
- **THEN** the template catalog SHALL own starter metadata, variable schema, output policy, and after-create action recommendations
- **AND** the project workspace SHALL own board item state, columns, milestones, and project progress
- **AND** template recommendation SHALL NOT directly mutate project board state; project writes SHALL continue through explicit project commands or application services.

