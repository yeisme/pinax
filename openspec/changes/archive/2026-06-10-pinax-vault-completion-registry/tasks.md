# Tasks

## 1. Tests first

- [x] 1.1 Add CLI regression tests for `vault register/list/use` and default vault resolution in `pinax note list`.
  - Evidence: `TestVaultRegistryDefaultAndCompletionCLI` fails before implementation, passes after implementation.
- [x] 1.2 Add CLI completion tests for persistent `--vault`, local aliases, cached remote selectors, and note ref completion through a vault alias.
  - Evidence: `TestVaultRegistryDefaultAndCompletionCLI` covers local alias completion and note-ref completion via `--vault work`; `TestVaultRemoteRefreshCacheCompletionCLI` covers cached remote selector completion.
- [x] 1.3 Add package tests for registry/cache read-write, selector resolution, and remote discovery response parsing.
  - Evidence: `internal/vaultregistry/registry_test.go` covers registry round-trip, selector resolution, redacted remote refresh, cache persistence, and completion items.

## 2. Registry and cache implementation

- [x] 2.1 Add `internal/vaultregistry` with user config/cache path resolution, schema read-write, alias validation, and selector resolution.
  - Evidence: `internal/vaultregistry/registry.go` and package tests.
- [x] 2.2 Store local aliases with absolute paths and default alias in `vaults.yaml`.
  - Evidence: CLI test checks registered `work`/`personal` aliases and default selection.
- [x] 2.3 Store remote discovery results in cache JSON without tokens, Authorization headers, cookies, or raw provider payloads.
  - Evidence: package and CLI tests reject secret leakage in cache/output.

## 3. CLI wiring

- [x] 3.1 Register persistent `--vault` completion using registry/cache candidates and file-completion fallback.
  - Evidence: `vaultFlagCompletion` is registered on the persistent `--vault` flag and returns default directive rather than `NoFileComp`.
- [x] 3.2 Add `pinax vault register`, `pinax vault use`, and extend `pinax vault list` with registry/cache facts.
  - Evidence: `TestVaultRegistryDefaultAndCompletionCLI` passes.
- [x] 3.3 Add `pinax vault remote list` and `pinax vault remote refresh --profile <name>`.
  - Evidence: `TestVaultRemoteRefreshCacheCompletionCLI` passes against an httptest remote endpoint.
- [x] 3.4 Resolve registered aliases and registry default in `loadCommandConfig` before services receive `VaultPath`.
  - Evidence: `pinax note list` without `--vault` uses the selected registered default in the CLI regression test.
- [x] 3.5 Ensure note ref completions use resolved vault aliases.
  - Evidence: `pinax __complete note show --vault work ''` returns notes from the `work` vault only.

## 4. Docs and verification

- [x] 4.1 Update command docs with vault registry, default vault, and remote-cache completion workflow.
  - Evidence: `README.md`, `docs/commands/README.md`, and `docs/commands/vault.md` updated.
- [x] 4.2 Run focused tests for vault selection/completion.
  - Evidence: `go test ./cmd/pinax ./internal/vaultregistry -run 'TestVaultRegistry|TestVaultRemote|TestRegistryRoundTrip|TestRemoteCacheRefresh' -count=1` passed.
- [x] 4.3 Run `go test ./...`.
  - Evidence: `go test ./...` passed.
- [x] 4.4 Run `task check`.
  - Evidence: `task check` passed, including fmt-check, lint, test, build, and OpenSpec validation.
- [x] 4.5 Run `openspec validate pinax-vault-completion-registry --strict` and `openspec validate --all`.
  - Evidence: strict change validation passed; `task check` ran `openspec validate --all` with 34 passed / 0 failed.
