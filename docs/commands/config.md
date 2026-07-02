# config Command

`pinax config` views and modifies Pinax configuration. Configuration writes must be performed through CLI commands; do not handwrite structured configuration assets.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax config path` | Displays user-level and project-level configuration paths. | Does not write. |
| `pinax config get <key>` | Reads the merged effective configuration value. | Does not write. |
| `pinax config doctor` | Checks configuration sources and override relationships. | Does not write. |
| `pinax config set <key> <value>` | Writes user-level or project-level configuration. | Writes configuration. |
| `pinax config unset <key>` | Deletes a user-level or project-level configuration item. | Writes configuration. |

## Common Workflows

```
pinax config path --vault ./my-notes
pinax config get output.theme --vault ./my-notes --json
pinax config set output.theme mono --scope user
pinax config set remote.api_url http://127.0.0.1:8787 --scope user
pinax config unset remote.api_url --scope user
pinax config doctor --vault ./my-notes
```

## Remote API Configuration

`remote.api_url` persists the default API endpoint for supported remote-mode commands. When set at user scope, commands such as `pinax folder list --json` call the configured API without repeating `--api-url`. Local control commands such as `pinax config`, `pinax api`, `pinax token`, `pinax profile`, and `pinax vault` remain local so users can inspect or change configuration even when a remote endpoint is configured.

## Settings Projection

`pinax config get <key> --json` keeps the stable `key` and `value` facts and also includes Settings-facing facts: `source`, `writable`, `write_scope`, `write_scopes`, and a `set` action when the current value can be saved through `pinax config set`.

`pinax config doctor --json` includes `data.settings`, a bounded list of supported settings with effective value, source, writable status, allowed save scopes, safe next action, and `secret_ref_boundary`. Sources are one of `user`, `project`, `env`, `flag`, or `default`. Values from env remain read-only in the projection; values supplied by flags can be saved explicitly to user or project scope.

`pinax config doctor --json` also reports bounded diagnostics such as `local_api_status`, `remote_api_source`, `write_mode`, `redaction_status`, `profile_status`, `token_status`, and `body_exposure_default`. These diagnostics are status-only and do not include credential values.

## Appearance And Keymap Boundaries

Appearance settings for CLI output use existing Pinax config keys:

```
pinax config set output.theme high-contrast --scope user
pinax config set output.color auto --scope user
pinax config set output.markdown.style dark --scope user
pinax config set themes.custom.accent cyan --scope user
```

Keymap settings are a future Web-client preference surface. Pinax CLI currently exposes only `editor.command` for external editor integration:

```
pinax config get editor.command --vault ./my-notes --json
pinax config set editor.command "code --wait" --scope user
```

Do not document or call a `pinax keymap` command unless a later additive contract introduces typed keymap config support.

## Boundaries

Configuration examples must not include secrets, tokens, cookies, Authorization headers, or raw external CLI configuration content. Store only the remote endpoint in `remote.api_url`; pass bearer credentials with `--api-token`, `--api-token-file`, `PINAX_API_TOKEN`, or `PINAX_API_TOKEN_FILE`.

See also [`api`](./api.md), [`token`](./token.md), and [`profile`](./profile.md) for Remote API Mode.
