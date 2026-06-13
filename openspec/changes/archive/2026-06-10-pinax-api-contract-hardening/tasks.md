## 1. OpenAPI registry correctness

- [x] 1.1 Add RED test in `internal/app` proving `rest.project.item.plan` exports `/v1/project-items/{ref}:{action}` as `post`, not `get`.
- [x] 1.2 Add RED table-driven test that every `surface=rest` route from `RemoteRoutes()` appears exactly once in exported OpenAPI `paths` with a lowercase method matching `RemoteRoute.Method`.
- [x] 1.3 Update `APISchemaExport` to derive operation method keys from `RemoteRoute.Method` instead of hardcoding `get`.
- [x] 1.4 Add OpenAPI operation extensions from route metadata: `x-pinax-command`, `x-pinax-capability`, `x-pinax-readonly`, `x-pinax-body-allowed`, `x-pinax-approval-required`, and `x-pinax-snapshot-required`.
- [x] 1.5 Verify focused app tests with `go test ./internal/app -run 'API|Remote|Schema|OpenAPI' -count=1` and record evidence.
  - Evidence 2026-06-09: `go test ./internal/app -run 'API|Remote|Schema|OpenAPI' -count=1` -> `ok github.com/yeisme/pinax/internal/app 0.013s`.

## 2. REST handler drift contract

- [x] 2.1 Add representative fixture path mapping for each registered REST route id in `internal/api` tests.
- [x] 2.2 Add RED table-driven test that each registered REST route reaches a handler and returns a valid Pinax projection envelope.
- [x] 2.3 Add RED test that unsupported method on a registered path returns HTTP 405 with `error.code=method_not_allowed` in the response body.
- [x] 2.4 Add RED test that unknown REST path returns HTTP 404 with `error.code=route_not_found` in the response body.
- [x] 2.5 Implement projection-based HTTP status mapping without moving vault, Git, provider, repository, or business logic into handlers.
- [x] 2.6 Verify focused REST tests with `go test ./internal/api -run 'REST|HTTP|Route|Projection' -count=1` and record evidence.
  - Evidence 2026-06-09: `go test ./internal/api -run 'REST|HTTP|Route|Projection' -count=1` -> `ok github.com/yeisme/pinax/internal/api 0.173s`.

## 3. RPC dispatcher drift contract

- [x] 3.1 Add representative params for each registered RPC route id in `internal/api` tests.
- [x] 3.2 Add RED table-driven test that every `surface=rpc` route from `RemoteRoutes()` is accepted by `RPCDispatcher` and does not return `rpc_method_not_found`.
- [x] 3.3 Add RED test that an unknown RPC method returns a failed projection with `error.code=rpc_method_not_found` and a hint mentioning `pinax api routes`.
- [x] 3.4 Add test that REST and RPC routes sharing a capability point to the same projection command and response schema version.
- [x] 3.5 Keep RPC dispatcher as a projection adapter only; do not add direct Markdown, `.pinax`, Git, provider, database, or remote-service access.
- [x] 3.6 Verify focused RPC tests with `go test ./internal/api -run 'RPC|Remote|Route' -count=1` and record evidence.
  - Evidence 2026-06-09: `go test ./internal/api -run 'RPC|Remote|Route' -count=1` -> `ok github.com/yeisme/pinax/internal/api 0.395s`; `internal/api/rpc.go` remains a projection adapter that dispatches to `app.Service` only.

## 4. Remote write gate transport semantics

- [x] 4.1 Add RED REST test for archive without approval returning non-2xx transport status plus `error.code=approval_required`.
- [x] 4.2 Add RED REST test for approved archive without snapshot returning non-2xx transport status plus `error.code=snapshot_required` and runnable `pinax version snapshot` hint or action.
- [x] 4.3 Add RED REST test for risky move to `done` covering approval and snapshot gate ordering.
- [x] 4.4 Add assertions that gated remote write requests do not modify Markdown files, `.pinax` assets, Git state, provider state, or remote services.
- [x] 4.5 Add redaction assertions for REST/RPC gate responses and test fixtures: no token, Authorization header, Cookie, webhook URL, raw provider payload, or hidden prompt content.
- [x] 4.6 Implement gate status mapping while preserving failed projection body, command name, stable error code, Chinese message, and runnable hint/action.
- [x] 4.7 Verify gate tests with `go test ./internal/api -run 'Gate|Approval|Snapshot|Redaction' -count=1` and record evidence.
  - Evidence 2026-06-09: `go test ./internal/api -run 'Gate|Approval|Snapshot|Redaction' -count=1` -> `ok github.com/yeisme/pinax/internal/api 0.184s`; tests snapshot the fixture vault before/after gated REST calls and assert REST/RPC responses do not include token/header/cookie/webhook/raw-payload/hidden-prompt markers.

## 5. `api serve` lifecycle output

- [x] 5.1 Add CLI RED test that `pinax api serve` without `--readonly` returns projection error `readonly_required` and does not start a server.
- [x] 5.2 Add lifecycle test proving `api.ListenAndServe` binds `127.0.0.1` when `--port 0` is used.
- [x] 5.3 Add default-mode test proving startup URL is reported on stderr and stdout is free of logs, banners, and non-structured progress.
- [x] 5.4 Add `--events` test for `start`, `ready`, and `shutdown` or `error` NDJSON lifecycle events with diagnostics kept off stdout.
- [x] 5.5 Decide and implement `--json` / `--agent` semantics: either one startup projection with quiet stdout afterward, or stable `unsupported_output_mode` failed projection.
- [x] 5.6 Verify serve CLI tests with `go test ./cmd/pinax ./internal/api -run 'APIServe|Serve|Lifecycle|Output' -count=1` and record evidence.
  - Evidence 2026-06-09: `go test ./cmd/pinax ./internal/api -run 'APIServe|Serve|Lifecycle|Output' -count=1` -> `ok github.com/yeisme/pinax/cmd/pinax 1.196s`; `ok github.com/yeisme/pinax/internal/api 0.007s [no tests to run]`.

## 6. Documentation and spec sync

- [x] 6.1 Update `docs/interfaces/remote-api-contract.md` to document registry-derived OpenAPI methods and operation extensions.
- [x] 6.2 Update `docs/interfaces/remote-api-contract.md` to document REST transport status mapping and failed projection envelope requirements.
- [x] 6.3 Update `docs/interfaces/remote-api-contract.md` to document `api serve` stdout/stderr and machine-mode lifecycle behavior.
- [x] 6.4 Confirm `cli-tree-ux` remains unchanged; hidden root `schema` alias remains covered by the completed API discovery UX change.
- [x] 6.5 Run `openspec validate --all` and record evidence.
  - Evidence 2026-06-09: `openspec validate --all` -> `Totals: 25 passed, 0 failed (25 items)`; `git diff -- openspec/specs/cli-tree-ux/spec.md` produced no diff.

## 7. Final verification and closeout evidence

- [x] 7.1 Run focused contract suite: `go test ./internal/app ./internal/api ./cmd/pinax -run 'API|Remote|OpenAPI|Serve|RPC|Gate' -count=1`.
  - Evidence 2026-06-09: `go test ./internal/app ./internal/api ./cmd/pinax -run 'API|Remote|OpenAPI|Serve|RPC|Gate' -count=1` -> `ok github.com/yeisme/pinax/internal/app 0.358s`; `ok github.com/yeisme/pinax/internal/api 0.738s`; `ok github.com/yeisme/pinax/cmd/pinax 0.223s`.
- [x] 7.2 Run full local gate: `task check`.
  - Evidence 2026-06-09: `task check` -> `golangci-lint run` reported `0 issues`; `openspec validate --all` reported `Totals: 25 passed, 0 failed (25 items)`; `go test ./...` completed through `ok github.com/yeisme/pinax/tests/e2e 33.251s`; build completed with `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`.
- [x] 7.3 Inspect changed files and confirm no generated `dist/`, coverage, local vault, temp reports, provider cache, secrets, or raw payload fixtures are included.
  - Evidence 2026-06-09: `git status --short | rg "\.codegraph|dist/|coverage|temp/integration-test-runs|\.pinax" || true` produced no tracked/untracked status output after removing generated `.codegraph/` and `dist/pinax`; redaction tests use synthetic marker strings only.
- [x] 7.4 Record final command evidence in this `tasks.md` before marking the change ready to archive.
