# token Command

`pinax token` manages API bearer tokens for local Remote API Mode. Token metadata is local structured state; raw token values must stay out of repository files and logs.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax token create` | Create an API token with label, scope, route groups, and expiry. | Writes token metadata and prints the one-time secret only through the command output. |
| `pinax token list` | List token ids, labels, scopes, expiry, and status. | Read-only. |
| `pinax token revoke <token-id>` | Revoke a token. | Writes token state. |
| `pinax token rotate <token-id>` | Rotate a token and optionally set a new label. | Writes token state and prints the replacement secret only once. |

## Common Workflows

Create and inspect a read-only local-agent token:

```bash
pinax token create --label local-agent --scope read --expires 30d --vault ./my-notes --json
pinax token list --vault ./my-notes --json
```

Revoke or rotate by token id:

```bash
pinax token revoke tok_123 --vault ./my-notes --json
pinax token rotate tok_123 --label local-agent-rotated --vault ./my-notes --json
```

Use the token without storing it in project files. The server store and client bearer file are intentionally different files:

```bash
pinax api serve --vault ./my-notes --readonly --port 8787 \
  --token-store ./my-notes/.pinax/tokens/tokens.json
pinax search "release workflow" --api-url http://127.0.0.1:8787 \
  --api-token-file ~/.config/pinax/owner-api.bearer --json
```

`--token-store` reads the hashed server registry. `--api-token-file` reads one plaintext bearer secret from an absolute owner-only `0600` regular file. The deprecated server `--token-file` is only an alias for `--token-store`; never point either server flag at the client bearer file.

## Safety Boundary

Token commands may show a one-time secret. Do not paste that secret into docs, fixtures, screenshots, notes, run receipts, shell credential scripts, or repository-tracked config. Persist long-lived local credentials in user-level local config, a user-level secret store, or a user-controlled token file outside the repository.

See also [`api`](./api.md), [`profile`](./profile.md), and [`config`](./config.md).
