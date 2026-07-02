# profile Command

`pinax profile` manages backend/API connection profile aliases. Profiles store connection metadata and secret references, not raw credentials.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax profile add <name>` | Add a backend connection profile alias. | Writes profile metadata. |
| `pinax profile list` | List all backend connection profiles. | Read-only. |
| `pinax profile show <name>` | Show one profile in detail. | Read-only; must not reveal raw secrets. |
| `pinax profile remove <name>` | Remove a profile alias. | Writes profile metadata. |

## Common Workflows

Register a local API profile that references a token through an environment variable:

```bash
pinax profile add local --endpoint http://127.0.0.1:8787 --workspace default --device laptop --secret-ref env://PINAX_API_TOKEN --vault ./my-notes --json
pinax profile list --vault ./my-notes --json
pinax profile show local --vault ./my-notes --json
```

Remove an obsolete profile:

```bash
pinax profile remove local --vault ./my-notes --json
```

## Profile Fields

Profiles may record endpoint, workspace id, device id, default scope, and secret refs such as `env://PINAX_API_TOKEN` or a user keychain reference. They must not persist raw token values, access keys, cookies, Authorization headers, provider payloads, or generated private prompts.

## Safety Boundary

Profiles are for connection aliases. Cloud Sync backend configuration remains under [`cloud`](./cloud.md) and [`sync`](./sync.md); object storage backend profiles remain under [`backend`](./backend.md) and [`storage`](./storage.md). Remote API Mode uses [`api`](./api.md) and [`token`](./token.md).
