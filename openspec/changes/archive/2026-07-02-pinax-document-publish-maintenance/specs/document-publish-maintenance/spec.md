# document-publish-maintenance Specification

## Purpose

定义 Pinax 维护本地 note 到外部协作文档之间发布关系的合同。Pinax vault 是真源；Notion 页面和飞书 Docs 是发布副本。Pinax 负责 prepare、push、status、link/unlink、mapping、receipt 和 provider adapter 安全边界；评论、批注、权限、协作者等平台原生协作动作由 agent 调用原生 CLI 完成，不进入 Pinax publish mapping。

## ADDED Requirements

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
