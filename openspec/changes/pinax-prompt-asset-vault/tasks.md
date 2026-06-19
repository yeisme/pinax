## 1. Contract and Test Baseline

- [x] 1.1 Add prompt asset schema fixtures and validator tests.
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: none
  - Scope: Add valid/invalid `yeisme.prompt_asset.v1` fixtures and tests for required fields, lifecycle enum, permission enum, variables schema, and source refs.
  - Validation: `go test ./internal/... -run 'PromptAsset.*Validate|Validate.*PromptAsset' -count=1`
  - Expected: Tests fail before validator implementation and pass after implementation.
  - Failure re-check: If existing packages do not fit, create a focused `internal/promptasset` package instead of placing validation in CLI code.

- [x] 1.2 Classify compatibility surfaces before implementation.
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 1.1
  - Scope: Record affected CLI commands, JSON fields, `--agent` keys, DB migrations, config keys, and structured assets in this change before coding.
  - Validation: `rg -n "Compatibility|Affected surfaces|Migration|Rollback|Deprecation" openspec/changes/pinax-prompt-asset-vault`
  - Expected: This change lists additive surfaces and rollback notes.
  - Failure re-check: If any breaking surface appears, add migration and deprecation tasks before implementation.

## 2. Storage and Index

- [x] 2.1 Add GORM models and migration for prompt assets.
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 1.1
  - Scope: Add models for prompt assets, versions, source refs, and usage feedback using Pinax's GORM/GORM Gen conventions.
  - Validation: `go test ./internal/index -run 'PromptAsset|Migration' -count=1`
  - Expected: Migration applies in a temp database and preserves existing note/index tests.
  - Failure re-check: If direct SQL is needed for migration metadata only, keep it centralized in migration internals and explain why GORM cannot express it.

- [x] 2.2 Add repository and search projection tests.
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 2.1
  - Scope: Repository can create, update lifecycle, resolve by URI, search by domain/tag/text/lifecycle, and import usage feedback idempotently.
  - Validation: `go test ./internal/... -run 'PromptAssetRepository|PromptAssetSearch|PromptUsageFeedback' -count=1`
  - Expected: Duplicate feedback import does not create duplicate lifecycle evidence.
  - Failure re-check: If repository methods become too broad, split read/search/write responsibilities into focused files.

## 3. CLI and Output Contracts

- [x] 3.1 Add `pinax prompt create/import/search/show/resolve` commands.
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 2.2
  - Scope: Commands call app services and render human, `--json`, and `--agent` output without exposing raw note paths in agent mode.
  - Validation: `go test ./cmd/pinax -run 'Prompt.*(Create|Import|Search|Show|Resolve)' -count=1`
  - Expected: Command tests cover success, invalid schema, unknown asset, and agent output.
  - Failure re-check: If command code starts owning business rules, move rules into `internal/app`.

- [x] 3.2 Add `pinax prompt lifecycle` and `pinax prompt feedback import` commands.
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 2.2
  - Scope: Lifecycle command records explicit reason; feedback import consumes Eikona-style usage feedback and lets Pinax decide lifecycle changes.
  - Validation: `go test ./cmd/pinax -run 'Prompt.*(Lifecycle|Feedback)' -count=1`
  - Expected: Eikona feedback imports as metadata-only evidence and does not read Eikona internals.
  - Failure re-check: If feedback import needs artifact inspection, store only resource refs and leave artifact details to Eikona.

## 4. Fixture Integration and Docs

- [x] 4.1 Add fixture integration evidence for prompt asset vault.
  - Owner: `cli/pinax`
  - Lane: D
  - Depends on: 3.1, 3.2
  - Scope: Add an integration entry point that creates a temp vault, imports a fixture prompt asset, searches, resolves, imports feedback, and writes evidence.
  - Validation: `task test:integration`
  - Expected: Evidence under `temp/integration-test-runs/<run-id>/` includes `summary.json`, `command.txt`, `stdout.log`, `stderr.log`, `env.json`, and `artifacts/`.
  - Failure re-check: If existing integration runner cannot scope this scenario, add a prompt-asset fixture scenario to the existing runner rather than creating a parallel framework.

- [x] 4.2 Update Pinax docs for prompt assets.
  - Owner: `cli/pinax`
  - Lane: D
  - Depends on: 3.1, 3.2
  - Scope: Document prompt asset lifecycle, URI resolution, cross-project boundary, and example commands under `docs/`.
  - Validation: `rg -n "prompt asset|pinax://prompt|feedback import|yeisme.prompt_asset.v1" docs openspec/changes/pinax-prompt-asset-vault`
  - Expected: Docs and OpenSpec describe the same command names and lifecycle states.
  - Failure re-check: If command names drift during implementation, update docs and tests in the same task.

## 5. Quality Gate

- [x] 5.1 Run focused and full validation.
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: 4.1, 4.2
  - Scope: Run focused prompt asset tests, full Pinax check, and OpenSpec validation.
  - Validation: `task check`
  - Expected: Formatting, lint, tests, build, and `openspec validate --all` pass.
  - Failure re-check: If `task check` fails outside prompt asset changes, record the unrelated failure and run the narrowest passing command for this change before escalation.

## Current Slice Verification

- 2026-06-18: `go test ./internal/promptasset -run 'PromptAsset.*Validate|Validate.*PromptAsset' -count=1` failed before validator implementation because `Validate`, `Load`, `Asset`, and `SchemaVersion` did not exist.
- 2026-06-18: `go test ./internal/promptasset -run 'PromptAsset.*Validate|Validate.*PromptAsset' -count=1` passed after adding YAML fixtures and the internal prompt asset validator.
- 2026-06-18: `rg -n "Compatibility|Affected surfaces|Migration|Rollback|Deprecation" openspec/changes/pinax-prompt-asset-vault` confirms additive compatibility surfaces, migration, rollback, and deprecation notes are recorded before CLI/database implementation.
- 2026-06-18: `go test ./internal/index -run 'PromptAsset|Migration' -count=1` passed after adding additive prompt asset GORM models to `internal/index/model.AllModels()`.
- 2026-06-18: `go run ./internal/index/gormgen` regenerated typed DAO files for the new prompt asset models.
- 2026-06-18: `go test ./internal/promptasset ./internal/index ./internal/index/query -run 'PromptAsset|PromptUsageFeedback|Migration' -count=1` passed after adding repository create, lifecycle, resolve, search, and idempotent feedback import coverage.
- 2026-06-18: `go test ./cmd/pinax -run 'Prompt.*(Create|Import|Search|Show|Resolve|Lifecycle|Feedback)' -count=1` passed after adding the additive `pinax prompt` CLI group and agent-safe output tests.
- 2026-06-18: `go test ./internal/app ./internal/cli -run 'Prompt' -count=1` passed after wiring prompt commands through `app.Service` projections.
- 2026-06-18: `rg -n "prompt asset|pinax://prompt|feedback import|yeisme.prompt_asset.v1" docs openspec/changes/pinax-prompt-asset-vault` passed after adding `docs/commands/prompt.md` and command-map links.
- 2026-06-18: `task test:integration` passed and wrote prompt asset vault evidence under `temp/integration-test-runs/20260618T041444Z-1637585/` with `summary.json`, `command.txt`, `stdout.log`, `stderr.log`, `env.json`, and `artifacts/`.
- 2026-06-18: `task check` passed, covering formatting, lint, `go test ./...`, build, and `openspec validate --all`.
