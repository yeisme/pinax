# Tasks

## 1. Server RPC contract

- [ ] 1.1 Add `FindRemoteRPCMethod(method string)` near the remote route registry.
- [ ] 1.2 Add HTTP `POST /v1/rpc` request type, handler, route registration, method lookup, and projection response writing.
- [ ] 1.3 Apply auth, route exposure, readonly/write gate, and HTTP status mapping for RPC methods.
- [ ] 1.4 Add server tests for happy path, unknown method, invalid JSON, readonly write, missing `yes=true`, allow-write success, token scope, hidden group, and registry/dispatcher parity.

## 2. Remote API client

- [ ] 2.1 Add `internal/remoteapi.Client` with `Ping`, `Capabilities`, and `Call`.
- [ ] 2.2 Implement base URL validation, default timeout, bearer header injection, non-2xx projection decoding, and redacted transport errors.
- [ ] 2.3 Add client tests for invalid URL, unreachable service, invalid response, non-2xx projection decode, timeout, Authorization header behavior, and token redaction.

## 3. CLI remote mode

- [ ] 3.1 Add global `--api-url`, `--api-token`, `--api-token-file` flags and `PINAX_API_URL`, `PINAX_API_TOKEN`, `PINAX_API_TOKEN_FILE` env resolution.
- [ ] 3.2 Add remote mode conflict handling for explicit `--vault`, token flag conflicts, and unsupported commands.
- [ ] 3.3 Add command-layer remote dispatch helper that calls `internal/remoteapi.Client` and renders returned projections through existing renderers.
- [ ] 3.4 Wire first supported command set: project board, note read, project item plan, folder, inbox, and draft capabilities.
- [ ] 3.5 Add CLI tests proving remote reads/writes hit the server vault, unsupported commands do not run locally, and `--json`/`--agent` stdout stays contract-safe.

## 4. Docs and validation

- [ ] 4.1 Update `docs/interfaces/remote-api-contract.md` and command docs to describe local API remote mode separately from Cloud/vault remote discovery.
- [ ] 4.2 Run `openspec validate pinax-cli-remote-api-mode --strict`.
- [ ] 4.3 Run focused server/client/CLI tests added for this change.
- [ ] 4.4 Run `task check` after implementation is complete.
