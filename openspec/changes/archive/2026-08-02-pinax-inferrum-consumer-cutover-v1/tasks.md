## 1. Dependency and Protocol Migration

- [x] 1.1 Add/update focused tests for `github.com/yeisme/inferrum`, `inferrum.sidecar.v1`, `inferrum-lancedb-sidecar`, and `inferrum_sidecar_*`; Verification: semantic tests RED before implementation; Expected: old dependency is explicitly rejected. Evidence: protocol and `rebuild_inferrum_v1` tests failed against the old implementation before passing.
- [x] 1.2 Update `go.mod`, semantic adapter imports/aliases, sidecar config/error mapping, Pinax-owned filenames/symbol names, and fake executable fixtures; Verification: `go test ./internal/semantic -count=1`; Expected: Inferrum dependency and protocol pass. Evidence: full semantic suite passed.
- [x] 1.3 Update KB application/command/testkit tests and Taskfile integration commands without changing CLI/output/receipt contracts; Verification: focused `internal/app`, `cmd/pinax`, and testkit tests; Expected: business behavior is unchanged. Evidence: focused application and KB command suites plus `task kb:sidecar:test` passed.

## 2. Active Documentation Cleanup

- [x] 2.1 Update KB docs and `pinax-inferrum-ollama-local-kb-review-mvp` active proposal/design/specs/tasks to Inferrum current facts while preserving historical evidence and LanceDB terms; Verification: strict OpenSpec + forbidden-name search; Expected: no active old product contract remains.
- [x] 2.2 Reframe retained `legacy_v1_readonly` text/errors as Pinax historical projection compatibility, not an active Lance dependency; Verification: focused legacy tests; Expected: existing N/N+1/N+2 behavior remains. Evidence: legacy doctor/review tests pass with `rebuild_inferrum_v1`.

## 3. Verification and Closeout

- [x] 3.1 Run focused semantic/app/command/testkit tests and `task check`; Expected: formatter, lint, tests, build, sidecar protocol, and OpenSpec gates pass. Evidence: `task check` completed with 0 lint issues, all Go/Web tests, build, sidecar contracts, and 64 OpenSpec items passing.
- [x] 3.2 Archive this change and inspect staged migration-only paths; commit and push are the immediate release handoff. Expected: root can update the Pinax submodule pointer and clean clone resolves Inferrum. Evidence: archived as `2026-08-02-pinax-inferrum-consumer-cutover-v1`; staged diff contains only KB/Inferrum-owned paths and the retired Python sidecar deletion.
