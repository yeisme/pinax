# pinax-cli-remote-api-mode Specification

## Purpose
Define Pinax local API remote mode: HTTP RPC dispatch, CLI forwarding to a running `pinax api serve` process, safe bearer-token handling, and output-contract parity for JSON and agent consumers.

## Requirements
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

#### Scenario: Forward note list query flags

- **GIVEN** `pinax api serve` is running at `http://127.0.0.1:8787`
- **WHEN** the user runs `pinax --api-url http://127.0.0.1:8787 note list --status active --tag research --limit 20 --json`
- **THEN** the CLI SHALL call RPC method `Pinax.Note.List`
- **AND** the server SHALL return the `note.list` Projection filtered by the supplied list flags.

#### Scenario: Enable remote mode with environment variable

- **GIVEN** `PINAX_API_URL` is set to `http://127.0.0.1:8787`
- **WHEN** the user runs a supported command such as `pinax inbox list --json`
- **THEN** the CLI SHALL execute the command through the remote API service.


#### Scenario: Enable remote mode with user configuration

- **GIVEN** user config contains `remote.api_url: http://127.0.0.1:8787`
- **WHEN** the user runs a supported command such as `pinax folder list --json`
- **THEN** the CLI SHALL execute the command through the remote API service without requiring `--api-url` or `PINAX_API_URL`.

#### Scenario: Keep local control commands editable with configured remote URL

- **GIVEN** user config contains `remote.api_url: http://127.0.0.1:8787`
- **WHEN** the user runs a local control command such as `pinax config get remote.api_url --agent`
- **THEN** the CLI SHALL execute the control command locally so the configured endpoint remains inspectable and editable.

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

#### Scenario: Configured remote command still rejects unsupported business commands

- **GIVEN** remote mode is enabled only by `remote.api_url`
- **WHEN** the user runs a non-control command that is not supported by remote mode
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

### Requirement: RPC operation logging

Pinax SHALL log enough RPC operation metadata to diagnose remote calls without exposing request details or note contents.

#### Scenario: Log RPC method and outcome

- **GIVEN** `pinax api serve` receives `POST /v1/rpc` for a registered method
- **WHEN** the RPC completes or fails at an API gate
- **THEN** stderr diagnostics SHALL include an `api.rpc` log with RPC method, optional RPC id, mapped command, group, readonly flag, HTTP status, duration, and projection error code when present
- **AND** the log SHALL NOT include RPC params, request body, response body, note content, Authorization header, cookies, tokens, or provider payloads.

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

