# pinax-cli-remote-api-mode Delta Spec

## ADDED Requirements

### Requirement: 客户端 CLI 覆盖通过 capability registry 增量扩展

Pinax SHALL evolve client CLI parity by registering safe capabilities in `RemoteCapabilities()` / `RemoteRoutes()` and SHALL NOT expose a generic remote shell or arbitrary command runner.

#### Scenario: 新客户端能力必须可发现

- **WHEN** a CLI capability is made available to Remote API clients
- **THEN** `pinax api routes --vault <vault> --json` SHALL list its capability id, command, route or RPC method, readonly status, body allowance, approval requirement, snapshot requirement, and stable errors
- **AND** `pinax api schema export --format openapi --vault <vault> --json` SHALL derive REST operation metadata from the same registry when a REST route exists.

#### Scenario: unsupported command never falls back locally

- **GIVEN** remote mode is enabled with `--api-url`, `PINAX_API_URL`, or `remote.api_url`
- **WHEN** the user runs a command that is not registered for remote mode and is not explicitly local-only
- **THEN** Pinax SHALL return `remote_command_unsupported`
- **AND** it SHALL NOT execute the command against a local vault as a fallback.

#### Scenario: local control commands remain local

- **GIVEN** remote mode is enabled only by persisted `remote.api_url`
- **WHEN** the user runs `pinax config`, `pinax api`, `pinax token`, `pinax profile`, `pinax vault`, `pinax cloud`, `pinax sync daemon`, completion, foreground server, or editor commands
- **THEN** Pinax SHALL keep those commands local unless a dedicated safe capability explicitly covers the operation.

### Requirement: 客户端写操作复用 CLI 安全门禁

Pinax SHALL keep every remote client write behind the same application service, approval, dry-run, snapshot, receipt, and redaction boundaries as the equivalent CLI command.

#### Scenario: readonly server rejects writes

- **GIVEN** `pinax api serve --vault <vault>` is running without `--allow-write`
- **WHEN** a client calls a write-capable REST route or RPC method with `yes=true`
- **THEN** Pinax SHALL return `write_disabled`
- **AND** no Markdown, `.pinax/**`, SQLite index, Git state, provider state, sync-state, token file, or remote service SHALL be modified.

#### Scenario: allow-write still requires confirmation

- **GIVEN** `pinax api serve --vault <vault> --allow-write` is running
- **WHEN** a write-capable client call omits both `yes=true` and `dry_run=true`
- **THEN** Pinax SHALL return `approval_required`
- **AND** the returned projection SHALL include a safe next action when one is available.

#### Scenario: risky writes require snapshot evidence

- **GIVEN** a remote client write would rename, move, delete, archive, apply repairs, apply organize plans, restore, publish, deploy, or batch-modify managed content
- **WHEN** the request lacks required snapshot evidence
- **THEN** Pinax SHALL return `snapshot_required` or an equivalent plan-only projection
- **AND** it SHALL NOT perform the risky write.

### Requirement: 客户端覆盖矩阵可审计

Pinax SHALL provide or test a coverage matrix that compares the CLI command tree with remote capability support.

#### Scenario: command parity audit classifies every command

- **WHEN** the parity audit runs
- **THEN** every user-visible CLI command SHALL be classified as `remote_supported`, `local_only`, or `unsupported`
- **AND** every `remote_supported` command SHALL point to a capability id or RPC method
- **AND** every `local_only` command SHALL include a reason such as runtime control, credential control, foreground process, editor, completion, local filesystem diagnostic, or daemon lifecycle.
