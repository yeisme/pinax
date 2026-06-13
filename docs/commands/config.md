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

## Boundaries

Configuration examples must not include secrets, tokens, cookies, Authorization headers, or raw external CLI configuration content. Store only the remote endpoint in `remote.api_url`; pass bearer credentials with `--api-token`, `--api-token-file`, `PINAX_API_TOKEN`, or `PINAX_API_TOKEN_FILE`.
