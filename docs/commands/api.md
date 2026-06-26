# api Command

`pinax api` manages the local REST/RPC projection adapter. It exposes controlled Pinax capabilities from one local vault; it is Remote API Mode, not Cloud Sync.

## Subcommands

| Command | Purpose | Writes/External effects |
| --- | --- | --- |
| `pinax api routes` | List local API route groups and capabilities. | Read-only. |
| `pinax api status` | Show the workbench status projection for future clients. | Read-only. |
| `pinax api schema export` | Export the local API schema, such as OpenAPI. | Writes only the requested output when an output path is provided. |
| `pinax api serve` | Start the local API server. | Starts a local process; read-only by default. |

## Common Workflows

Inspect routes and schema:

```bash
pinax api routes --vault ./my-notes --json
pinax api status --vault ./my-notes --json
pinax api schema export --format openapi --vault ./my-notes --json
```

`api routes --json` includes optional Web-facing metadata on each capability and route: `ui_group`, `body_exposure_default`, `write_gate`, `copy_command`, and `local_only_reason`. These fields help future clients group workbench screens, show body exposure defaults, preview approval/snapshot gates, and display copyable Pinax command templates without changing the existing projection envelope.

Planned client-only areas may appear in discovery without a matching CLI editing command. For example, `canvas.layout.metadata` uses `ui_group=canvas.view` and `local_only_reason=future-client-only` to document the future canvas layout boundary. It is not a `pinax canvas` command; clients must keep using service-owned projections and must not write `.pinax/canvases/*.json` directly.

`api status --json` returns the `workbench.status` projection with bounded facts such as `vault_root`, `index_status`, `write_mode`, `body_exposure_default`, `profile_status`, `token_status`, and a safe index refresh next action when the local index is missing or stale.

Start a read-only loopback server:

```bash
pinax api serve --vault ./my-notes --readonly --port 8787
```

Use a token file when auth is required:

```bash
pinax api serve --vault ./my-notes --readonly --port 8787 --token-file ~/.config/pinax/local-agent.token
```

## Remote API Mode

Clients can forward supported commands with `--api-url`, `--api-token`, `--api-token-file`, or `PINAX_API_URL`/`PINAX_API_TOKEN`. Local control commands such as `config`, `api`, `token`, `profile`, and `vault` remain local when the endpoint comes from persisted `remote.api_url` so users can still edit connection state.

The client coverage goal is full CLI parity through registered capabilities, not a generic remote shell. New client-visible operations must appear in `pinax api routes --json`, share the CLI projection envelope, and preserve the same approval, snapshot, dry-run, write-mode, and redaction gates as the CLI command they mirror. Commands that control the local runtime, foreground server, foreground daemon, editor, completion, credentials, or Cloud Sync device state remain local-only unless a dedicated capability is added.

See [Client CLI Parity and Realtime Sync](../interfaces/client-cli-parity-and-sync.md) for the coverage matrix and phased route expansion plan.

Cloud Sync is separate. Use [`cloud`](./cloud.md) and [`sync`](./sync.md) for distributed encrypted vault convergence across devices.

## Safety Boundary

`pinax api serve` should default to loopback and read-only mode. Use `--allow-write` only when the operator understands the remote mutation boundary and has approval. Do not put raw bearer tokens in repository files, docs, logs, events, screenshots, fixtures, or run evidence.

See also [`token`](./token.md), [`profile`](./profile.md), [`config`](./config.md), and [`mcp`](./mcp.md).
