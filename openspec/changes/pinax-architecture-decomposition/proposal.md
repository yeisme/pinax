## Why

Pinax has grown around a large `internal/app/service.go`, a broad `internal/cli/root.go`, and a command test file that covers many unrelated command families. The current shape makes feature ownership, focused tests, and compatibility review too expensive: small changes require loading large files, unrelated behaviors share helpers implicitly, and new business logic tends to land in the existing large files because no stronger boundary exists.

This change establishes package-level ownership and guardrails before moving large amounts of business logic. The goal is to make future feature work smaller and easier to review while preserving the existing CLI, output, config, vault layout, and index contracts.

## What Changes

- Introduce a Pinax architecture decomposition plan with a stable `app.Service` facade and capability-owned app packages.
- Add package responsibility requirements for note, search, vault, template, sync, version, briefing, and planning use cases.
- Split CLI command construction by command family while keeping `NewRootCommandWithDeps` as the CLI-facing entrypoint.
- Split broad command tests by command family and keep shared command helpers in focused testkit files.
- Add architecture guard tests that prevent direct CLI imports of capability packages and prevent capability packages from importing output/rendering packages.
- Treat runtime skill copies as generated artifacts and keep their source-of-truth policy outside feature implementation files.

## Capabilities

### New Capabilities

- `pinax-architecture`: Pinax SHALL define and enforce package ownership, facade boundaries, and code growth guardrails for app and CLI layers.

### Modified Capabilities

- `go-dev-toolchain`: Pinax quality gates SHALL include architecture guard tests once the first guard is introduced.
- `cli-tree-ux`: Command builders MAY be split by family, but command names, flags, help semantics, and output contracts SHALL remain compatible.

## Compatibility

This change is intended to be internal and contract-preserving.

- CLI command names, flags, default output, `--json` envelopes, `--agent` keys, `--events` event types, config keys, vault file layout, OpenSpec schema, and SQLite/GORM persisted data SHALL NOT be removed, renamed, or retyped by this change.
- `internal/app` exported request/method/package shape MAY change during implementation only when all in-repo callers and tests are updated in the same slice.
- `app.Service` remains the only CLI-facing facade. `internal/cli`, `cmd/pinax`, MCP/API surfaces, and future dashboard code SHALL NOT call capability packages directly.
- If implementation discovers a stable external surface must change, pause coding and update this OpenSpec with migration, deprecation window, consumer update list, and rollback before continuing.

Rollback: revert the decomposition commits in reverse order. Since external contracts are preserved, rollback does not require data migration. Any moved internal method must keep a facade shim until the corresponding slice is fully verified.

## Impact

- Code: `internal/app`, new app capability packages, `internal/cli`, `cmd/pinax` tests, architecture guard tests.
- Docs: update Pinax architecture docs with package responsibility and dependency rules.
- Tests: focused package tests plus architecture guard tests and existing `task check`.

## Non-Goals

- No database schema migration.
- No CLI output contract migration.
- No new provider behavior, cloud sync behavior, or note command behavior.
- No hosted service, daemon, dashboard, or MCP behavior change.
- No removal of existing compatibility shims before a later OpenSpec change explicitly approves it.
