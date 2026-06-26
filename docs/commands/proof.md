# proof Command

`pinax proof` runs the local agent-safe proof loop: Capture -> Retrieve -> Diagnose -> Plan -> Snapshot -> Apply safely. The default workflow is read-only; apply mode is explicit and requires approval.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax proof loop run` | Run the full proof loop and emit one projection with a `proof_loop_run_id`. | Read-only by default. With `--apply --yes`, may execute approved repair/organize operations after a fresh snapshot. |

## Common Workflows

Preview proof-loop state without writing:

```bash
pinax proof loop run --vault ./my-notes --json
pinax proof loop run --vault ./my-notes --agent
```

Apply only after reviewing the projection and creating/accepting the required snapshot gate:

```bash
pinax proof loop run --vault ./my-notes --apply --yes --json
```

## Safety Boundary

`pinax proof loop run` is the compact command for agent-safe maintenance review. It must not hide the underlying approval model: apply mode is not the default, and high-risk repairs or organize actions still require an explicit `--apply --yes` request after the user has reviewed the plan and snapshot requirement.

For lower-level maintenance commands, see [`vault`](./vault.md), [`metadata`](./metadata.md), [`repair`](./repair.md), [`organize`](./organize.md), [`version`](./version.md), and [`record`](./record.md).
