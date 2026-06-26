## ADDED Requirements

### Requirement: First-user proof loop readiness

Pinax SHALL provide a first-user proof loop path that can be completed from an installed binary against a local Markdown vault without provider credentials, cloud services, a daemon, or a source checkout.

#### Scenario: Installed binary runs the proof loop preview

- **GIVEN** a user has an installed `pinax` binary and an empty temporary directory
- **WHEN** the user runs `pinax version`, `pinax init ./my-notes --title "My Knowledge Base"`, `pinax note add "First Note" --body "My first Pinax note." --vault ./my-notes`, and `pinax proof loop run --vault ./my-notes --json`
- **THEN** stdout SHALL contain valid machine-readable output for each machine mode command
- **AND** the proof loop preview SHALL include a `proof_loop_run_id`, stage facts, and a safe next action
- **AND** it SHALL NOT require Cloud Sync, TaskBridge, provider credentials, MCP, dashboard, or a background daemon.

#### Scenario: Proof loop write path is explicitly protected

- **GIVEN** a demo vault contains low-risk repair candidates and manual-review candidates
- **WHEN** the user runs `pinax repair plan --vault <vault> --save --json`, `pinax version snapshot --vault <vault> --message "before repair"`, and `pinax repair apply --vault <vault> --plan <plan_id> --yes --json`
- **THEN** Pinax SHALL apply only approved low-risk operations through application services
- **AND** it SHALL write receipt evidence linked to the plan and snapshot
- **AND** manual-review items SHALL NOT be silently applied, deleted, merged, or rewritten.

#### Scenario: Proof loop restore is demonstrable

- **GIVEN** a proof loop apply changed a local Markdown file
- **WHEN** the user runs `pinax version restore <path> --revision HEAD --plan --vault <vault> --json` and `pinax version restore apply --vault <vault> --plan <restore_id> --yes --json`
- **THEN** Pinax SHALL restore the selected file through the CLI/application service path
- **AND** output SHALL report `local_write=true` and `remote_write=false`
- **AND** stale restore plans SHALL be rejected with a stable error code and next action.

### Requirement: Demo proof vault SHALL be deterministic and safe

Pinax SHALL maintain a deterministic demo or fixture vault that proves the first-user proof loop against realistic but non-secret content.

#### Scenario: Demo vault contains concrete proof-loop issues

- **WHEN** maintainers run the proof-loop fixture tests
- **THEN** the fixture SHALL contain concrete examples of broken links, missing tags, orphan notes, low-risk repair candidates, manual-review candidates, and a file that can be restored
- **AND** tests SHALL assert the specific issue codes and paths discovered from the fixture.

#### Scenario: Demo vault contains no sensitive local data

- **WHEN** fixture files, receipts, stdout, stderr, events, or integration evidence are scanned
- **THEN** they SHALL NOT include real user paths, raw tokens, Authorization headers, cookies, provider payloads, hidden system prompts, private tool arguments, or full chain-of-thought.

### Requirement: README and quickstart SHALL lead with the proof loop

Pinax SHALL present the first-user proof loop before advanced workflows in README and quickstart documentation.

#### Scenario: Quickstart is a copyable five-minute path

- **WHEN** a user reads `docs/quickstart.md`
- **THEN** the guide SHALL show real commands for install verification, vault initialization, note creation, proof loop preview, repair plan, snapshot, apply, and restore
- **AND** each placeholder such as `<plan_id>` or `<restore_id>` SHALL explain which prior command output provides it.

#### Scenario: Advanced workflows are secondary

- **WHEN** a user reads README or command documentation
- **THEN** Cloud Sync, plugins, publish, KB, Memory, TaskBridge planning, and provider automation SHALL be presented as advanced or separate workflows
- **AND** they SHALL NOT be required to understand or run the first-user proof loop.

