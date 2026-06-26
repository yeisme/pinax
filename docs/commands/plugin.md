# `pinax plugin`

`pinax plugin` manages dynamic plugin manifests, local registry state, permission grants, runtime diagnostics, and audited dry-run execution.

Plugins extend Pinax through bounded projections and action plans. They do not make the Markdown vault, `.pinax` metadata, Git state, providers, or remote services writable by themselves.

## Trust Model

- `wasm` is the preferred untrusted runtime direction. The current built-in WASM adapter fixes the call/result contract and denies network, env, and host filesystem by default.
- `javascript`, `python`, and `process` use external trusted runners. Pinax does not embed V8 or Python and does not claim these runners are a strong sandbox.
- Plugin writes must be returned as action plans and then applied through existing Pinax approval, snapshot, record, index, and evidence gates.
- `.pinax/plugins/registry.json`, `.pinax/plugins/plugin-lock.json`, and `.pinax/events/plugin-audit.jsonl` are CLI-authored structured assets. Do not hand-edit them.

## Common Commands

```bash
pinax plugin validate ./plugins/project-dashboard --vault ./my-notes --json
pinax plugin install ./plugins/project-dashboard --scope vault --vault ./my-notes --json
pinax plugin list --vault ./my-notes --json
pinax plugin inspect project-dashboard --vault ./my-notes --json
pinax plugin enable project-dashboard --vault ./my-notes --yes --json
pinax plugin permissions grant project-dashboard projection.read --capability render_dashboard --vault ./my-notes --yes --json
pinax plugin run project-dashboard render_dashboard --vault ./my-notes --dry-run --json
pinax plugin doctor --vault ./my-notes --json
pinax plugin disable project-dashboard --vault ./my-notes --yes --json
pinax plugin uninstall project-dashboard --vault ./my-notes --yes --json
```

`plugin validate` is read-only and does not write registry, lock, audit, Git, provider, or remote state. `plugin install` writes registry, lock, and audit files through the plugin service, but installed plugins remain disabled until `plugin enable --yes` is run.

`plugin run` is dry-run/read-only by default. If a plugin is disabled it returns `plugin_disabled`; if a required grant is missing it returns `plugin_permission_denied`; if the runtime is not available it returns `plugin_runner_unavailable`.

## Manifest Shape

```yaml
schema_version: pinax.plugin.v1
id: project-dashboard
name: Project Dashboard
version: 0.1.0
runtime:
  kind: wasm
  entrypoint: dist/plugin.wasm
capabilities:
  - id: render_dashboard
    kind: view.render
permissions:
  vault:
    read: projection
  network: false
budgets:
  timeout_ms: 3000
  max_input_bytes: 262144
  max_output_bytes: 262144
  max_memory_mb: 64
```

Manifests must not contain API tokens, Authorization headers, Cookie values, webhook URLs, provider payloads, raw prompts, or secret-like values. Use user-level local config, a user-level secret store, or explicit environment variables for CI and temporary overrides.

## Permissions

Supported grant names are additive and deny-by-default:

| Permission | Meaning |
| --- | --- |
| `projection.read` | Read bounded Pinax projections. |
| `note.body.read` | High-risk body access, denied unless explicitly granted. |
| `action_plan.write` | Return a reviewable action plan, not direct writes. |
| `temp.write` | Write only to a Pinax-managed temporary directory. |
| `network` | Trusted external runner network access. |
| `env.read` | Read only allowlisted environment names. |

Use scoped grants where possible:

```bash
pinax plugin permissions grant project-dashboard projection.read --capability render_dashboard --vault ./my-notes --yes --json
pinax plugin permissions list project-dashboard --vault ./my-notes --json
pinax plugin permissions revoke project-dashboard projection.read --capability render_dashboard --vault ./my-notes --yes --json
```

## Output Modes

All plugin commands share the standard Pinax projection renderers:

```bash
pinax plugin list --vault ./my-notes --agent
pinax plugin doctor --vault ./my-notes --events
pinax plugin inspect project-dashboard --vault ./my-notes --explain
```

Machine stdout must not include raw note bodies, provider payloads, Authorization headers, cookies, token values, hidden prompts, private tool arguments, local absolute paths, or plugin entrypoint bytes.

See also [`api`](./api.md), [`token`](./token.md), [`profile`](./profile.md), and [`publish`](./publish.md) for adjacent integration surfaces.
