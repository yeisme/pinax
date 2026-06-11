# pinax-cli-remote-api-mode Delta Spec

## ADDED Requirements

### Requirement: Local API HTTP RPC transport

Pinax SHALL expose an HTTP RPC route for the local API server that dispatches registered RPC capabilities through application services and returns Projection envelopes.

#### Scenario: Call registered RPC method over HTTP

- **GIVEN** `pinax api serve` is running for a vault
- **WHEN** a client sends `POST /v1/rpc` with method `Pinax.Folder.List`
- **THEN** the server SHALL dispatch through the existing RPC dispatcher and application service
- **AND** the response SHALL be a valid Pinax Projection envelope with command `folder.list`.

#### Scenario: Reject unknown RPC method

- **GIVEN** `pinax api serve` is running
- **WHEN** a client sends `POST /v1/rpc` with an unregistered method
- **THEN** the server SHALL return a failed Projection with error code `rpc_method_not_found`
- **AND** the server SHALL NOT execute any vault mutation.

#### Scenario: Enforce write gates for RPC

- **GIVEN** the RPC method is write-capable
- **WHEN** the server was not started with `--allow-write`
- **THEN** the server SHALL return `write_disabled`
- **AND** the vault SHALL remain unchanged.

#### Scenario: Require confirmation for remote write RPC

- **GIVEN** the server was started with `--allow-write`
- **WHEN** a write RPC is sent without `yes=true` and without `dry_run=true`
- **THEN** the server SHALL return `approval_required`
- **AND** the vault SHALL remain unchanged.

### Requirement: CLI remote mode selection

Pinax SHALL provide a CLI remote mode that forwards supported ordinary commands to a running local API service when an API URL is configured.

#### Scenario: Enable remote mode with flag

- **GIVEN** `pinax api serve` is running at `http://127.0.0.1:8787`
- **WHEN** the user runs `pinax --api-url http://127.0.0.1:8787 folder list --json`
- **THEN** the CLI SHALL call the remote API instead of reading a local vault
- **AND** stdout SHALL contain only the returned JSON Projection envelope.

#### Scenario: Enable remote mode with environment variable

- **GIVEN** `PINAX_API_URL` is set to `http://127.0.0.1:8787`
- **WHEN** the user runs a supported command such as `pinax inbox list --json`
- **THEN** the CLI SHALL execute the command through the remote API service.

#### Scenario: Reject explicit vault conflict

- **GIVEN** remote mode is enabled with `--api-url`
- **WHEN** the user also supplies an explicit `--vault`
- **THEN** the CLI SHALL return `remote_vault_conflict`
- **AND** the CLI SHALL NOT execute against either local or remote vault.

#### Scenario: Unsupported command does not fallback

- **GIVEN** remote mode is enabled
- **WHEN** the user runs a command not supported by remote mode
- **THEN** the CLI SHALL return `remote_command_unsupported`
- **AND** the CLI SHALL NOT execute the command locally.

### Requirement: Remote API client safety

Pinax SHALL keep remote transport credentials and request bodies out of user output, logs, fixtures, and errors.

#### Scenario: Send bearer token without leaking it

- **GIVEN** a bearer token is configured through `--api-token`, `--api-token-file`, or `PINAX_API_TOKEN`
- **WHEN** the CLI calls the remote API
- **THEN** the client SHALL send the token only as an Authorization header
- **AND** no stdout, stderr, Projection error, audit log, or test fixture SHALL contain the raw token or Authorization header.

#### Scenario: Decode non-2xx projection response

- **GIVEN** the remote API returns a non-2xx HTTP status with a Projection envelope
- **WHEN** the CLI receives the response
- **THEN** the CLI SHALL render that Projection through the selected output mode
- **AND** it SHALL NOT replace it with an unstructured transport error.

#### Scenario: Report invalid remote response

- **GIVEN** the remote API response is not a valid Projection envelope
- **WHEN** the CLI receives the response
- **THEN** the CLI SHALL return `remote_api_invalid_response`
- **AND** the error SHALL NOT include credentials or raw request body.

### Requirement: Remote output contract parity

Remote mode SHALL preserve the existing Pinax CLI output modes.

#### Scenario: JSON output stays machine-only

- **GIVEN** remote mode is enabled
- **WHEN** the user runs a supported command with `--json`
- **THEN** stdout SHALL contain one valid JSON Projection object
- **AND** stdout SHALL NOT contain diagnostics, logs, ANSI decoration, or prose outside the JSON object.

#### Scenario: Agent output remains key-value

- **GIVEN** remote mode is enabled
- **WHEN** the user runs a supported command with `--agent`
- **THEN** stdout SHALL contain stable key=value lines rendered from the returned Projection
- **AND** stdout SHALL NOT contain human prose or credentials.
