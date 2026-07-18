# document-publish-maintenance Specification

## Purpose
TBD - created by archiving change pinax-document-publish-maintenance. Update Purpose after archive.
## Requirements
### Requirement: Document publish uses explicit document targets

Pinax SHALL expose document publishing through a dedicated `pinax publish doc` command namespace and SHALL initially support `notion-page` and `lark-doc` targets.

#### Scenario: document publish help lists initial targets

- **WHEN** a user runs `pinax publish doc --help`
- **THEN** the help output SHALL describe document publishing as publishing Pinax notes to external document copies
- **AND** it SHALL list `notion-page` and `lark-doc` as initial targets
- **AND** it SHALL NOT describe comments, annotations, permissions or collaborator management as Pinax-owned features.

### Requirement: Provider profiles are CLI-authored structured assets

Pinax SHALL manage document publish provider profiles through CLI/service-authored structured assets, not agent-written files.

#### Scenario: configure Notion page target

- **WHEN** a user runs `pinax publish doc profile set notion-page --workspace <workspace-id> --parent-page <page-id> --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update the target profile through the application service
- **AND** stdout SHALL contain one JSON envelope with `command="publish.doc.profile.set"`
- **AND** the profile SHALL NOT store Notion token values, Authorization headers or raw provider payloads.

#### Scenario: configure Feishu doc target

- **WHEN** a user runs `pinax publish doc profile set lark-doc --space <space-id> --folder <folder-token> --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update the target profile through the application service
- **AND** stdout SHALL contain one JSON envelope with `command="publish.doc.profile.set"`
- **AND** the profile SHALL NOT store Feishu token values, cookies, Authorization headers or raw provider payloads.

#### Scenario: configure Feishu provider identity

- **WHEN** a user runs `pinax publish doc profile set lark-doc --folder <folder-token-or-url> --as user --vault ./my-notes --json`
- **THEN** Pinax SHALL persist only the requested provider identity selector value `user`, `bot` or `auto`
- **AND** subsequent `lark-doc` provider calls SHALL pass the selector to `lark-cli` when supported
- **AND** the profile SHALL NOT persist user access tokens, refresh tokens, cookies or raw auth payloads.

### Requirement: Provider doctor reports capability without leaking credentials

Pinax SHALL diagnose document publish provider availability through normalized doctor output.

#### Scenario: diagnose missing lark-cli

- **WHEN** a user runs `pinax publish doc provider doctor --target lark-doc --vault ./my-notes --json`
- **AND** `lark-cli` is unavailable
- **THEN** Pinax SHALL return a JSON envelope with `status="failed"` or `status="partial"`
- **AND** the error code SHALL be `provider_cli_not_found`
- **AND** stdout, stderr, events and receipts SHALL NOT include token values or raw shell environment values.

#### Scenario: diagnose provider auth failure

- **WHEN** a provider executable reports an authentication failure
- **THEN** Pinax SHALL return `provider_auth_failed`
- **AND** it SHALL include a safe next action to configure that provider outside the vault
- **AND** it SHALL redact Authorization headers, cookies, token-like strings and raw provider payloads.

### Requirement: Prepare creates a local publish package without remote writes

Pinax SHALL prepare a document publish package from a local note without calling external providers or modifying mappings.

#### Scenario: prepare a Notion package

- **WHEN** a user runs `pinax publish doc prepare --note note_123 --target notion-page --vault ./my-notes --json`
- **THEN** Pinax SHALL create a CLI-authored publish package for the selected note and target
- **AND** it SHALL include note id, title, target, content digest, safe render warnings and next actions
- **AND** it SHALL NOT create, update or delete any external Notion page
- **AND** it SHALL NOT create or update active publish mappings.

### Requirement: Dry-run validates provider readiness without remote writes

Pinax SHALL support dry-run document publish pushes that call provider preflight only and do not write remote or mapping state.

#### Scenario: dry-run Feishu document publish

- **WHEN** a user runs `pinax publish doc push --package pubpkg_456 --target lark-doc --vault ./my-notes --dry-run --json`
- **THEN** Pinax SHALL load the package and target profile
- **AND** it SHALL call provider preflight only
- **AND** it SHALL NOT create or update a Feishu doc
- **AND** it SHALL NOT create or update active publish mappings
- **AND** stdout SHALL contain one JSON envelope with `command="publish.doc.push"`.

### Requirement: Push creates or updates external document copies

Pinax SHALL create a new external document when no active mapping exists and update the mapped external document when one exists.

#### Scenario: push creates a new Notion page

- **WHEN** a user runs `pinax publish doc push --package pubpkg_456 --target notion-page --vault ./my-notes --json`
- **AND** no active mapping exists for the package note and target
- **THEN** Pinax SHALL call the Notion provider create operation
- **AND** it SHALL write a mapping with external object id, external URL, content digest and `publish_status="published"`
- **AND** it SHALL write a redacted publish receipt.

#### Scenario: push updates an existing Feishu doc

- **WHEN** a user runs `pinax publish doc push --package pubpkg_456 --target lark-doc --vault ./my-notes --json`
- **AND** an active mapping exists for the package note and target
- **THEN** Pinax SHALL call the Feishu provider update operation
- **AND** it SHALL update the mapping content digest and timestamp after success
- **AND** it SHALL write a redacted publish receipt.

### Requirement: Status detects linked, stale, failed and detached document mappings

Pinax SHALL report document publish status by combining local note digest, mapping state and safe provider status when available.

#### Scenario: status reports stale local note

- **WHEN** a local note changed after the last successful publish
- **AND** the user runs `pinax publish doc status --note note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL report `publish_status="stale"`
- **AND** it SHALL include a next action to prepare and push the note again
- **AND** it SHALL NOT fetch or persist external document body content.

#### Scenario: status reports detached mapping

- **WHEN** a mapping is explicitly unlinked or the provider reports the external object no longer exists
- **THEN** Pinax SHALL report `publish_status="detached"`
- **AND** it SHALL preserve enough redacted evidence for the user to decide whether to relink or publish a new document.

### Requirement: Link and unlink only maintain Pinax mapping state

Pinax SHALL let users attach or detach an external document URL without mutating the external document itself.

#### Scenario: link an existing Feishu doc

- **WHEN** a user runs `pinax publish doc link --note note_123 --target lark-doc --external-url <url> --vault ./my-notes --json`
- **THEN** Pinax SHALL validate the target and URL shape
- **AND** it SHALL create or update the local mapping through the application service
- **AND** it SHALL NOT update the remote Feishu doc body, comments, permissions or collaborators.

#### Scenario: unlink keeps remote document untouched

- **WHEN** a user runs `pinax publish doc unlink --note note_123 --target notion-page --vault ./my-notes --json`
- **THEN** Pinax SHALL mark the local mapping detached or remove the active mapping according to the documented policy
- **AND** it SHALL NOT delete, archive or modify the remote Notion page.

### Requirement: Feishu document publish behaves like a cloud vault

Pinax SHALL provide a cloud-vault publishing experience for `lark-doc` rather than only uploading flat Markdown files.

#### Scenario: mirror layout creates remote folder structure

- **WHEN** a user configures `pinax publish doc profile set lark-doc --folder <folder-token-or-url> --as user --layout mirror --template vault --index-page --vault ./my-notes --json`
- **AND** a note has vault path `notes/index/example.md`
- **THEN** Pinax SHALL publish that note under a remote `notes/index/` folder path when the provider supports Drive folders
- **AND** it SHALL persist the remote folder token through CLI-authored folder mapping state
- **AND** it SHALL search for and reuse an existing same-name child folder before creating a new folder.

#### Scenario: vault template improves reading experience

- **WHEN** Pinax prepares a `lark-doc` package with `template=vault`
- **THEN** the package body SHALL include a Pinax overview section before the note body
- **AND** it SHALL include safe fields such as note id, vault path, kind, status, tags and updated time when available
- **AND** it SHALL remove a duplicate first-level title when the source body already starts with the same H1.

#### Scenario: index page links published notes

- **WHEN** `index_page=true` and a publish push succeeds
- **THEN** Pinax SHALL create or update `_Pinax Vault Index.md` in the target folder
- **AND** the index SHALL list published notes with folder, title, status and external links
- **AND** Pinax SHALL store the index object id in the document publish profile without storing provider credentials.

### Requirement: Agent follow-up uses native provider CLI for platform-specific collaboration

Pinax SHALL expose safe external refs for agent follow-up but SHALL NOT own platform-specific comments, annotations, permissions or collaborator workflows.

#### Scenario: agent adds a Feishu comment after Pinax publish

- **WHEN** `pinax publish doc status --note note_123 --vault ./my-notes --json` returns a `lark-doc` external URL or id
- **THEN** an agent MAY call `lark-cli` commands to add comments, annotations, permissions, collaborators or other Feishu-native document operations
- **AND** Pinax SHALL NOT write those platform-native actions into publish mapping or receipt state unless a later OpenSpec explicitly adds an audit import capability.

#### Scenario: agent adds a Notion comment after Pinax publish

- **WHEN** `pinax publish doc status --note note_123 --vault ./my-notes --json` returns a `notion-page` external URL or id
- **THEN** an agent MAY call Notion native CLI commands to add comments, annotations, permissions, collaborators or other Notion-native page operations
- **AND** Pinax SHALL NOT treat those comments as local note content or sync them back into the vault.

### Requirement: Document publish output follows Pinax machine-output contracts

Pinax SHALL render document publish results from one command projection and maintain stdout/stderr separation.

#### Scenario: JSON output is a single envelope

- **WHEN** a user runs any `pinax publish doc ... --json` command
- **THEN** stdout SHALL contain exactly one valid JSON object
- **AND** the object SHALL include `spec_version`, `mode="json"`, `command`, `status`, and command-specific `data` when useful
- **AND** provider diagnostics SHALL go to stderr or redacted evidence, not JSON stdout.

#### Scenario: Agent output contains stable facts

- **WHEN** a user runs `pinax publish doc push --package pubpkg_456 --target lark-doc --vault ./my-notes --agent`
- **THEN** stdout SHALL include `spec_version`, `mode=agent`, `command=publish.doc.push`, `status`, `fact.note_id`, `fact.target`, `fact.publish_status` and safe external URL facts when available
- **AND** it SHALL NOT include localized prose, ANSI color, raw provider payloads, token values or private note bodies.

#### Scenario: Events output is redacted NDJSON

- **WHEN** a user runs `pinax publish doc push --package pubpkg_456 --target notion-page --vault ./my-notes --events`
- **THEN** stdout SHALL contain NDJSON lifecycle events for start, package load, provider preflight, provider write, mapping write, receipt write and complete or error
- **AND** each event SHALL be redacted and correlatable with the final receipt.

### Requirement: Document publish tests use isolated fakes and evidence

Pinax SHALL test document publish behavior without real provider accounts, real credentials, public network access or user vault state.

#### Scenario: e2e tests use fake provider executables

- **WHEN** document publish e2e tests run
- **THEN** they SHALL use fixture vaults and fake Notion/lark executables
- **AND** they SHALL cover prepare, dry-run, create, update, status, list, link, unlink, provider missing, auth failure and redaction
- **AND** they SHALL NOT require real Notion tokens, real Feishu tokens, public network access or a user vault.

#### Scenario: integration evidence is written and redacted

- **WHEN** a document publish integration, component, system or e2e entrypoint runs
- **THEN** it SHALL write per-run evidence under `temp/integration-test-runs/<run-id>/`
- **AND** evidence SHALL include at least `summary.json`, `command.txt`, `stdout.log`, `stderr.log`, `env.json` and `artifacts/`
- **AND** evidence SHALL redact provider tokens, Authorization headers, cookies, raw provider payloads, hidden prompts, private tool arguments and full chain-of-thought.

### Requirement: Feishu document publish renders native documents by default

Pinax SHALL publish `lark-doc` notes as Feishu native Docs/Docx documents by default, while retaining the existing Markdown Drive file behavior only as an explicit fallback renderer.

#### Scenario: new Feishu profile defaults to native renderer

- **WHEN** a user runs `pinax publish doc profile set lark-doc --folder <folder-token-or-url> --as user --vault ./my-notes --json`
- **THEN** Pinax SHALL store `renderer="native-docx"` in the profile unless the user explicitly provides another supported renderer
- **AND** stdout facts SHALL include `renderer="native-docx"`
- **AND** the profile SHALL NOT store Feishu token values, cookies, Authorization headers or raw provider payloads.

#### Scenario: user explicitly selects legacy Markdown file renderer

- **WHEN** a user runs `pinax publish doc profile set lark-doc --folder <folder-token-or-url> --as user --renderer markdown-file --vault ./my-notes --json`
- **THEN** Pinax SHALL keep the current Drive Markdown file publishing behavior for that profile
- **AND** stdout facts SHALL include `renderer="markdown-file"`
- **AND** Pinax SHALL describe the renderer as a fallback or legacy source-file mode, not as the preferred Feishu reading mode.

#### Scenario: unsupported renderer is rejected

- **WHEN** a user runs `pinax publish doc profile set lark-doc --folder <folder-token-or-url> --renderer html --vault ./my-notes --json`
- **THEN** Pinax SHALL return a failed JSON envelope with a stable validation error code
- **AND** it SHALL NOT write or partially update the profile.

### Requirement: Feishu native renderer converts Markdown to document blocks

Pinax SHALL convert note Markdown into a provider-neutral publish AST and then into Feishu native document content instead of uploading the Markdown source file when `renderer=native-docx`.

#### Scenario: common Markdown structures become native document content

- **WHEN** Pinax prepares a `lark-doc` package for a note containing headings, paragraphs, links, lists, blockquotes, tables and fenced code blocks
- **AND** the profile renderer is `native-docx`
- **THEN** the package SHALL include a native render plan containing ordered document blocks for those structures
- **AND** the package SHALL include `render_revision="pinax.publish.render.v1"`
- **AND** it SHALL NOT depend on uploading the raw `.md` file as the primary publish artifact.

#### Scenario: unsupported Markdown produces render warnings

- **WHEN** the note contains Markdown or HTML that the native renderer cannot represent safely
- **THEN** Pinax SHALL preserve user-readable content where possible through safe fallback blocks
- **AND** it SHALL include stable render warning codes in the package and push output
- **AND** it SHALL NOT silently drop content without a warning.

### Requirement: Mermaid and SVG render as native-readable assets

Pinax SHALL handle Mermaid diagrams and SVG content for native Feishu documents by rendering them into image assets or another Feishu-readable native representation.

#### Scenario: Mermaid block is rendered for native document publish

- **WHEN** a note contains a fenced code block with language `mermaid`
- **AND** the profile renderer is `native-docx`
- **THEN** Pinax SHALL render the diagram into a publish artifact suitable for insertion into a Feishu native document
- **AND** the native render plan SHALL reference that artifact as an image or supported media block
- **AND** if rendering is unavailable, Pinax SHALL emit a `mermaid_render_unavailable` warning and preserve the Mermaid source in a readable fallback block.

#### Scenario: SVG image is converted before native insertion

- **WHEN** a note contains an inline SVG or Markdown image referencing an SVG file
- **AND** the profile renderer is `native-docx`
- **THEN** Pinax SHALL convert the SVG into a safe raster artifact before insertion
- **AND** it SHALL NOT insert raw inline SVG into the Feishu native document body
- **AND** if conversion is unavailable, Pinax SHALL emit a `svg_render_unavailable` warning.

### Requirement: Feishu native provider creates and updates native document objects

Pinax SHALL call `lark-cli` through the provider adapter to create and update Feishu native document objects for `renderer=native-docx`.

#### Scenario: native push creates a native Feishu document

- **WHEN** a user runs `pinax publish doc push --package pubpkg_456 --target lark-doc --vault ./my-notes --json`
- **AND** the package/profile renderer is `native-docx`
- **AND** no active native mapping exists for the note and target
- **THEN** Pinax SHALL call the Feishu native document create operation through the provider adapter
- **AND** the resulting mapping SHALL record `renderer="native-docx"`
- **AND** `external_object.type` SHALL be `docx` or `doc`, not `file`
- **AND** stdout facts SHALL include the renderer and external object type.

#### Scenario: native push does not silently fall back to Markdown file upload

- **WHEN** the configured `lark-cli` does not expose the required native document capability
- **AND** the profile renderer is `native-docx`
- **THEN** Pinax SHALL fail with `provider_capability_missing`
- **AND** it SHALL include a safe next action to upgrade or configure the provider
- **AND** it SHALL NOT call `lark-cli markdown +create` or `lark-cli markdown +overwrite` unless the profile explicitly uses `renderer=markdown-file`.

#### Scenario: existing Markdown file mapping is not retyped in place

- **WHEN** an active mapping points to a Feishu Drive `file` object
- **AND** the user switches the profile renderer to `native-docx`
- **THEN** Pinax SHALL NOT overwrite or reinterpret that `file` mapping as a native document
- **AND** it SHALL require a new native object or an explicit unlink/re-publish flow
- **AND** it SHALL return a clear action such as `pinax publish doc unlink --note <note-id> --target lark-doc --vault ./my-notes --json`.

### Requirement: Feishu index page uses the selected renderer

Pinax SHALL render the Feishu cloud-vault index page using the same renderer class as the profile.

#### Scenario: native renderer creates native index document

- **WHEN** `index_page=true`
- **AND** the profile renderer is `native-docx`
- **AND** a publish push succeeds
- **THEN** Pinax SHALL create or update the index page as a Feishu native document
- **AND** the profile `index_object` SHALL include object type and renderer metadata
- **AND** the index document SHALL list published notes with folder, title, status, renderer and link.

#### Scenario: legacy Markdown index remains fallback only

- **WHEN** the profile renderer is `markdown-file`
- **THEN** Pinax MAY continue maintaining `_Pinax Vault Index.md` as a Drive Markdown file
- **AND** it SHALL NOT describe that output as native Feishu Docs rendering.

### Requirement: Native rendering tests prove object type and render fallbacks

Pinax SHALL test native Feishu rendering with isolated fakes and a real-provider smoke checklist.

#### Scenario: fake lark-cli rejects silent fallback

- **WHEN** document publish e2e tests run for `renderer=native-docx`
- **THEN** the fake `lark-cli` SHALL expose native document create/update/upload/inspect operations
- **AND** the test SHALL fail if Pinax calls `markdown +create` or `markdown +overwrite`
- **AND** tests SHALL assert mapping `external_object.type` is `docx` or `doc`.

#### Scenario: real Feishu smoke verifies native rendering

- **WHEN** a maintainer runs the real Feishu smoke test against an authorized folder
- **THEN** the test note SHALL include at least one Mermaid diagram and one SVG image case
- **AND** verification SHALL record that the remote object type is `docx` or `doc`
- **AND** verification SHALL record whether Mermaid and SVG are visible as images or native-readable blocks in Feishu
- **AND** if API evidence cannot prove visual rendering, the smoke record SHALL require manual document inspection and SHALL NOT substitute Markdown fetch as proof.

