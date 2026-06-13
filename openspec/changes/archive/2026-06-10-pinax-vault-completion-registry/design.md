## Context

Pinax has many vault-scoped commands wired through `ctx.vaultPath`; the global `--vault` flag currently defaults to `.` and has no custom completion. Existing completions for note refs, templates, assets, saved views, and journal dates already read only local metadata and use `ShellCompDirectiveNoFileComp` when Pinax owns the candidate set. The new vault selector must follow the same rule: completion must be fast, local, read-only, and secret-free.

## Design

### Registry and cache assets

Pinax owns two user-level structured assets:

- Registry: `$XDG_CONFIG_HOME/pinax/vaults.yaml` or `~/.config/pinax/vaults.yaml`.
- Remote cache: `$XDG_CACHE_HOME/pinax/vaults/cache.json` or `~/.cache/pinax/vaults/cache.json`.

The registry stores named local aliases and a default alias. The cache stores remote discovery results keyed by profile name and fetched timestamp. Both assets use stable schema versions and are only changed by Pinax commands.

### Selector resolution

Vault selector resolution order:

1. Explicit `--vault` flag.
2. `PINAX_VAULT` / user config `vault`.
3. Registry default alias from `vaults.yaml`.
4. `.` fallback.

A selector matching a local alias resolves to its absolute path. A selector containing a path separator, `.` prefix, `~`, or an absolute path remains path-like and is cleaned by existing vault logic. A cached remote selector such as `cloud:team` is returned by completion but is not silently converted into a local path.

### Completion behavior

`--vault <TAB>` returns:

- local registry aliases with descriptions, e.g. `work\tlocal vault /home/me/work`;
- cached remote selectors with descriptions, e.g. `cloud:team\tremote vault profile=cloud workspace=team`;
- file completion remains enabled so users can still complete ordinary paths.

Completion never calls a remote endpoint, never resolves profile secrets, and never writes registry/cache files.

### Commands

- `pinax vault register <path> --name <alias>` validates that the path is a Pinax vault, stores an absolute path, and optionally `--default` selects it.
- `pinax vault list` shows local aliases, default marker, and cached remote selectors.
- `pinax vault use <alias>` selects a registered local alias as the default vault.
- `pinax vault remote list [--profile <name>]` reads the local cache.
- `pinax vault remote refresh --profile <name>` uses a configured profile endpoint to fetch a remote vault list and writes the cache. It may use the profile secret for the network call, but it does not print or persist the raw secret.

### Remote discovery contract

The refresh command accepts either of these response shapes from a profile endpoint:

```json
{"vaults":[{"id":"team","label":"Team Knowledge","workspace":"ws"}]}
```

or a projection envelope with `data.vaults`. The selector is built as `cloud:<id>` unless the response provides an explicit `selector`.

## Risks and mitigations

- Completion could become slow or unreliable if it hits the network. Mitigation: completion is cache-only.
- Remote secrets could leak through completions or logs. Mitigation: completion never resolves secrets; refresh uses Authorization only for the request and never writes secrets to cache/output.
- Default vault selection could break path-based workflows. Mitigation: explicit `--vault` and `PINAX_VAULT` still win; path-like selectors are preserved.
- Remote selectors could imply remote writes. Mitigation: no write semantics are added for remote selectors in this change.

## Verification

- Add red/green CLI tests for registry commands, default vault resolution, `--vault` completion, note-ref completion after alias selection, and remote cache completion.
- Add package tests for registry path resolution, schema read/write, selector resolution, and remote response parsing.
- Run focused command tests, `go test ./...`, and `task check`.
