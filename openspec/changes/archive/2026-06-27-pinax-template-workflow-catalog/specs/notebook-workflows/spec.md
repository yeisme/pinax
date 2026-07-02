# notebook-workflows Delta Spec

## MODIFIED Requirements

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

## ADDED Requirements

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
