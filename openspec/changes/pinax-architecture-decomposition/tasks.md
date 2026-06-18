## 1. Architecture Change Baseline

- [x] 1.1 Create the architecture decomposition OpenSpec change.
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: none
  - Scope: Create `openspec/changes/pinax-architecture-decomposition/` with proposal, design, tasks, and spec delta.
  - Validation: `openspec validate pinax-architecture-decomposition --strict`
  - Expected: OpenSpec accepts the change as a valid active change.
  - Failure re-check: If validation fails, fix the OpenSpec artifact shape before editing Go code.

- [x] 1.2 Record affected compatibility surfaces.
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: 1.1
  - Scope: Keep CLI/output/config/vault/schema surfaces contract-preserving and document rollback/deprecation policy in this change.
  - Validation: `rg -n "Compatibility|Rollback|deprecation|Surface" openspec/changes/pinax-architecture-decomposition`
  - Expected: The change names preserved surfaces, internal-only surfaces, rollback, and stop conditions for breaking changes.
  - Failure re-check: If any external breaking surface appears, add migration, deprecation window, consumer list, and rollback before implementation.

## 2. Package Ownership Guardrails

- [x] 2.1 Add capability package docs.
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 1.2
  - Scope: Add `doc.go` files for `noteops`, `searchops`, `vaultops`, `templateops`, `syncops`, `versionops`, `briefingops`, and `planningops` that state responsibilities and prohibited dependencies.
  - Validation: `go test ./internal/architecture -run TestCapabilityPackagesDeclareOwnership -count=1`
  - Expected: The guard can find every required package doc.
  - Failure re-check: If an existing package name conflicts, rename the capability before adding behavior.

- [x] 2.2 Add architecture import guard tests.
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 2.1
  - Scope: Add a focused test package that inspects Go imports and blocks direct CLI/cmd imports of capability packages plus capability imports of `internal/cli` or `internal/output`.
  - Validation: `go test ./internal/architecture -count=1`
  - Expected: Guard tests pass without changing runtime behavior.
  - Failure re-check: If the current code violates a guard, either narrow the first guard to the intended boundary or move the violating dependency behind the facade before proceeding.

## 3. CLI Structure Split

- [ ] 3.1 Split command-family builders from `internal/cli/root.go`.
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 2.2
  - Scope: Move Cobra builder functions into command-family files while preserving `NewRootCommandWithDeps`, command names, flags, help text semantics, and output behavior.
  - Validation: `go test ./internal/cli ./cmd/pinax -run 'Help|Flag|CLITree|Output|Command' -count=1`
  - Expected: Focused CLI tests pass with no output contract regression.
  - Failure re-check: If a command behavior changes, restore the old behavior before continuing the split.

- [ ] 3.2 Split command tests from `cmd/pinax/main_test.go`.
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 3.1
  - Scope: Move tests into command-family test files and shared helpers into `cli_testkit_test.go` without changing expectations.
  - Validation: `go test ./cmd/pinax -count=1`
  - Expected: The same command tests pass after mechanical split.
  - Failure re-check: If helper extraction changes test setup, restore helper behavior and rerun the affected tests.

## 4. App Capability Extraction

- [ ] 4.1 Extract note/search/vault use cases behind the `app.Service` facade.
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 2.2
  - Scope: Move note, search/query/database, and vault maintenance logic into `noteops`, `searchops`, and `vaultops`; keep facade methods callable by CLI.
  - Validation: `go test ./internal/app ./internal/index ./cmd/pinax -run 'Note|Search|Query|Database|Vault|Repair|Organize|Folder|Record' -count=1`
  - Expected: Existing behavior remains compatible and app tests pass.
  - Failure re-check: If a move requires output rendering, keep rendering in `internal/output` and return a projection from the app layer.

- [ ] 4.2 Extract template/sync/version/briefing/planning use cases behind the facade.
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 4.1
  - Scope: Move template, journal, cloud sync, version, briefing, and planning orchestration into the corresponding capability packages.
  - Validation: `go test ./internal/app ./internal/cloudsync ./internal/cloudclient ./cmd/pinax ./tests/e2e -run 'Template|Journal|Render|IndexPage|Cloud|Sync|Conflict|Backend|Version|Briefing|Plan|Redaction' -count=1`
  - Expected: Sync/provider tests keep using fakes and no real credentials or network side effects.
  - Failure re-check: If provider boundaries are unclear, stop and add design notes before moving behavior.

## 5. Documentation and Final Gate

- [ ] 5.1 Update Pinax architecture docs.
  - Owner: `cli/pinax`
  - Lane: D
  - Depends on: 4.1
  - Scope: Update `docs/architecture/architecture-boundaries.md` with capability package responsibilities, facade rule, and import guard policy.
  - Validation: `rg -n "noteops|searchops|vaultops|Service facade|architecture guard" docs/architecture openspec/changes/pinax-architecture-decomposition`
  - Expected: Docs and OpenSpec describe the same package ownership model.
  - Failure re-check: If docs drift from code, update docs in the same slice as the code move.

- [ ] 5.2 Run full quality gate and record closeout evidence.
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: 3.2, 4.2, 5.1
  - Scope: Run full Pinax validation and record final evidence before archiving this change.
  - Validation: `task check`
  - Expected: Formatting, lint, tests, build, and `openspec validate --all` pass.
  - Failure re-check: If failures are unrelated to this change, record the failing command and run focused commands for changed packages before escalation.

## Current Slice Verification

- 2026-06-18: `go test ./internal/architecture -run TestCapabilityPackagesDeclareOwnership -count=1` failed before capability `doc.go` files existed, proving the ownership guard catches missing package declarations.
- 2026-06-18: `go test ./internal/architecture -count=1` passed after adding capability package docs and import guard tests.
- 2026-06-18: `openspec validate --all` passed with 38 items.
- 2026-06-18: `task check` passed after OpenSpec, guard tests, capability docs, and architecture docs were added.
