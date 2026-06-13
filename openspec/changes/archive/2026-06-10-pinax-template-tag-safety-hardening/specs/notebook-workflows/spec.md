## MODIFIED Requirements

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
