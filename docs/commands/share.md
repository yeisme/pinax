# share

`pinax share` exposes an explicit read-only Web/API surface for local or LAN review. It is separate from `pinax publish serve`, which is loopback-only, and from `pinax api serve`, which is the local REST/RPC projection adapter.

## Published Scope

Share an already-built static site and its bounded publish API:

```bash
pinax publish build --profile public --target local --out ./dist/site --vault ./my-notes --json
pinax share start --scope published --profile public --out ./dist/site --host 0.0.0.0 --port 8787 --allow-lan --readonly --vault ./my-notes --json
```

`published` scope serves only the generated output directory. Its `/api/share/notes` route reads the publish-safe search index and returns bounded metadata fields such as `id`, `title`, `path`, `tags`, `kind`, and `status`. It does not read the private vault root, `.pinax/**`, provider config, token files, sync state, draft/private/unpublished notes, or note bodies.

## Vault-Readonly Scope

Share a controlled metadata-only read projection from the vault:

```bash
pinax share start --scope vault-readonly --host 0.0.0.0 --port 8787 --allow-lan --readonly --token-file ~/.config/pinax/share-token --vault ./my-notes --json
```

`vault-readonly` scope requires token auth unless it is explicitly started with `--no-auth` on loopback. The first route group exposes a minimal HTML shell plus `/api/share/status` and `/api/share/notes`. Notes are metadata-only card projections with no full body route. Mutation methods return `405` after authentication.

For CI smoke tests, `--once` starts the server, performs authenticated Web/API smoke requests, records `web_smoke=true` and `api_smoke=true`, and exits:

```bash
pinax share start --scope vault-readonly --host 127.0.0.1 --port 0 --readonly --token-file ~/.config/pinax/share-token --once --vault ./my-notes --json
```

## Security Gates

- Non-loopback hosts require `--allow-lan`; otherwise Pinax returns `share_allow_lan_required` before binding a socket.
- All share modes require `--readonly`; otherwise Pinax returns `share_readonly_required`.
- `vault-readonly` without token auth returns `share_auth_required`, except loopback-only `--no-auth` mode.
- Token file contents, token file paths, local vault roots, private note bodies, provider payloads, and `.pinax/**` internals must not appear in stdout, stderr, events, docs, fixtures, screenshots, or integration evidence.

See also [`publish`](./publish.md), [`api`](./api.md), and [`token`](./token.md).
