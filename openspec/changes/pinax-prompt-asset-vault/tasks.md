## 1. Contract and Test Baseline

- [ ] 1.1 Add prompt asset schema fixtures and validator tests.
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: none
  - Scope: Add valid/invalid `yeisme.prompt_asset.v1` fixtures and tests for required fields, lifecycle enum, permission enum, variables schema, and source refs.
  - Validation: `go test ./internal/... -run 'PromptAsset.*Validate|Validate.*PromptAsset' -count=1`
  - Expected: Tests fail before validator implementation and pass after implementation.
  - Failure re-check: If existing packages do not fit, create a focused `internal/promptasset` package instead of placing validation in CLI code.

- [ ] 1.2 Classify compatibility surfaces before implementation.
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 1.1
  - Scope: Record affected CLI commands, JSON fields, `--agent` keys, DB migrations, config keys, and structured assets in this change before coding.
  - Validation: `rg -n "Compatibility|Affected surfaces|Migration|Rollback|Deprecation" openspec/changes/pinax-prompt-asset-vault`
  - Expected: This change lists additive surfaces and rollback notes.
  - Failure re-check: If any breaking surface appears, add migration and deprecation tasks before implementation.

## 2. Storage and Index

- [ ] 2.1 Add GORM models and migration for prompt assets.
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 1.1
  - Scope: Add models for prompt assets, versions, source refs, and usage feedback using Pinax's GORM/GORM Gen conventions.
  - Validation: `go test ./internal/index ./internal/store -run 'PromptAsset|Migration' -count=1`
  - Expected: Migration applies in a temp database and preserves existing note/index tests.
  - Failure re-check: If direct SQL is needed for migration metadata only, keep it centralized in migration internals and explain why GORM cannot express it.

- [ ] 2.2 Add repository and search projection tests.
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 2.1
  - Scope: Repository can create, update lifecycle, resolve by URI, search by domain/tag/text/lifecycle, and import usage feedback idempotently.
  - Validation: `go test ./internal/... -run 'PromptAssetRepository|PromptAssetSearch|PromptUsageFeedback' -count=1`
  - Expected: Duplicate feedback import does not create duplicate lifecycle evidence.
  - Failure re-check: If repository methods become too broad, split read/search/write responsibilities into focused files.

## 3. CLI and Output Contracts

- [ ] 3.1 Add `pinax prompt create/import/search/show/resolve` commands.
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 2.2
  - Scope: Commands call app services and render human, `--json`, and `--agent` output without exposing raw note paths in agent mode.
  - Validation: `go test ./cmd/pinax -run 'Prompt.*(Create|Import|Search|Show|Resolve)' -count=1`
  - Expected: Command tests cover success, invalid schema, unknown asset, and agent output.
  - Failure re-check: If command code starts owning business rules, move rules into `internal/app`.

- [ ] 3.2 Add `pinax prompt lifecycle` and `pinax prompt feedback import` commands.
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 2.2
  - Scope: Lifecycle command records explicit reason; feedback import consumes Eikona-style usage feedback and lets Pinax decide lifecycle changes.
  - Validation: `go test ./cmd/pinax -run 'Prompt.*(Lifecycle|Feedback)' -count=1`
  - Expected: Eikona feedback imports as metadata-only evidence and does not read Eikona internals.
  - Failure re-check: If feedback import needs artifact inspection, store only resource refs and leave artifact details to Eikona.

## 4. Fixture Integration and Docs

- [ ] 4.1 Add fixture integration evidence for prompt asset vault.
  - Owner: `cli/pinax`
  - Lane: D
  - Depends on: 3.1, 3.2
  - Scope: Add an integration entry point that creates a temp vault, imports a fixture prompt asset, searches, resolves, imports feedback, and writes evidence.
  - Validation: `task test:integration`
  - Expected: Evidence under `temp/integration-test-runs/<run-id>/` includes `summary.json`, `command.txt`, `stdout.log`, `stderr.log`, `env.json`, and `artifacts/`.
  - Failure re-check: If existing integration runner cannot scope this scenario, add a prompt-asset fixture scenario to the existing runner rather than creating a parallel framework.

- [ ] 4.2 Update Pinax docs for prompt assets.
  - Owner: `cli/pinax`
  - Lane: D
  - Depends on: 3.1, 3.2
  - Scope: Document prompt asset lifecycle, URI resolution, cross-project boundary, and example commands under `docs/`.
  - Validation: `rg -n "prompt asset|pinax://prompt|feedback import|yeisme.prompt_asset.v1" docs openspec/changes/pinax-prompt-asset-vault`
  - Expected: Docs and OpenSpec describe the same command names and lifecycle states.
  - Failure re-check: If command names drift during implementation, update docs and tests in the same task.

## 5. Quality Gate

- [ ] 5.1 Run focused and full validation.
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: 4.1, 4.2
  - Scope: Run focused prompt asset tests, full Pinax check, and OpenSpec validation.
  - Validation: `task check`
  - Expected: Formatting, lint, tests, build, and `openspec validate --all` pass.
  - Failure re-check: If `task check` fails outside prompt asset changes, record the unrelated failure and run the narrowest passing command for this change before escalation.
