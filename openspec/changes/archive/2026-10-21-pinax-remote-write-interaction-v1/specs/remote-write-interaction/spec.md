# Remote write interaction

## ADDED Requirements

### Capability metadata

### Requirement: Advertise execution and recovery metadata

Every compiled capability definition MUST advertise `execution_location`, `write_strategy`, `recovery`, and `owner_action` when the corresponding value is applicable. Existing consumers that ignore these optional fields remain compatible.

#### Scenario: Remote mutation is discoverable

- **WHEN** a client reads `pinax api manifest --vault ./my-notes --json`
- **THEN** `inbox.capture` and `folder.rename` identify `remote_service`, their write strategy, and the original operation status/reconcile commands.

### Requirement: Bindings identify their execution owner

Every available REST/RPC binding MUST identify `remote_service`; MCP bindings MUST identify `vault_owner`; CLI bindings MUST identify `client_local`.

#### Scenario: MCP client avoids filesystem guessing

- **WHEN** a client reads `pinax://manifest`
- **THEN** it can distinguish owner-local execution from remote service execution without treating the vault path as a client-local file.

## Typed remote writes

### Requirement: Preserve mutation identity

Typed inbox capture and folder rename methods MUST send the original `operation_id` and `idempotency_key` and MUST NOT silently fall back to local mutation.

#### Scenario: Ambiguous response

- **WHEN** a typed mutation receives a timeout, 5xx, or malformed success
- **THEN** it queries the original operation and only retries with the same identity when the durable operation is terminal, retryable and replay-safe.

### Requirement: Respect dry-run, approval, revision and snapshot gates

Remote writes MUST preserve the existing server gates and return the existing stable error codes.

#### Scenario: Folder revision drift

- **WHEN** the expected revision is stale
- **THEN** the owner returns `revision_conflict` and does not rename the folder.

## MCP boundary

### Requirement: Keep default MCP read-only

The default stdio MCP server MUST expose discovery, bounded reads and plan projections. It MUST NOT expose a self-approval or arbitrary mutation tool.

#### Scenario: Client asks to save without CLI

- **WHEN** the connected client has no write-capable MCP tool
- **THEN** the response gives a bounded owner action and execution location instead of guessing an upload or local file operation.
