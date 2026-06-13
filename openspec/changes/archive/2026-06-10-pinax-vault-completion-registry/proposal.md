## Why

Pinax currently accepts `--vault <path>` but does not help users choose among multiple local vaults or remote workspaces. Shell completion only covers command-local objects such as templates and assets after a vault has already been selected. This makes `pinax note ...` workflows fragile when users operate multiple vaults, and it forces agents to remember absolute paths instead of using stable user-facing aliases.

## What Changes

- Add a CLI-authored local vault registry at the user config boundary for named local vault aliases and the selected default vault.
- Add a CLI-authored remote vault discovery cache at the user cache boundary. Shell completion reads this cache only; it never performs network calls or resolves secrets during completion.
- Add `pinax vault register/list/use/remote list/remote refresh` commands to manage local aliases, default vault selection, and remote discovery cache refresh.
- Register completion for persistent `--vault`, returning local aliases, cached remote selectors, and local file-path completion fallback.
- Make note and other vault-scoped commands resolve the selected default vault when `--vault` is omitted.
- Keep write commands local-only when a remote selector is provided unless a later sync/materialization workflow explicitly supports remote writes.

## Impact

- Affected packages: `internal/cli`, new `internal/vaultregistry`, `internal/config` config resolution boundary if needed, command tests under `cmd/pinax`.
- Structured assets: `~/.config/pinax/vaults.yaml` and `~/.cache/pinax/vaults/cache.json` are created and updated only by Pinax CLI/service code.
- Remote refresh can contact configured profile endpoints, but completion is local and side-effect free.
- Machine output contracts remain stable: commands use projection envelopes for `--json` and agent-safe facts for automation.
