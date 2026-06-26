# pinax folder

`pinax folder` is the unified operation entry point for vault directories. Agents, remote APIs, and scripts must use it to create, move, delete, or adopt directories, instead of directly running `mkdir` or manually writing `.pinax/folders.json`.

## Common Commands

```bash
pinax folder list --purpose all --include-empty --vault ./my-notes
pinax folder list --under spaces/research --vault ./my-notes
pinax folder show spaces/research --vault ./my-notes
pinax folder create spaces/research --purpose notes --vault ./my-notes
pinax folder rename spaces/research spaces/archive --dry-run --vault ./my-notes --json
pinax folder rename spaces/research spaces/archive --yes --vault ./my-notes
pinax folder move spaces/archive containers --yes --vault ./my-notes
pinax folder delete containers/archive --empty-only --yes --vault ./my-notes
pinax folder adopt manual/assets --purpose assets --yes --vault ./my-notes
pinax folder repair --plan --vault ./my-notes --json
```

## Read Views

`folder list` now renders a detailed human summary by default. It shows each
folder path, purpose, note count, asset count, child count, depth, and modified
time, so agents and humans can inspect a subtree without switching to JSON.

Use `--under <path>` to focus on a subdirectory tree:

```bash
pinax folder list --under notes/projects/research --vault ./my-notes
pinax folder list --under notes/projects/research --purpose notes --json --vault ./my-notes
```

`folder show <path>` reports direct child folders, descendant folder count, note
count, asset count, and managed metadata. Machine outputs keep the same
projection envelope and add facts such as `filter.under`, `child_folders`, and
`descendant_folders` when available.

## Write Rules

- `create` creates the directory, updates `.pinax/folders.json`, appends an event, and refreshes the index.
- `rename`, `move`, `delete`, and `adopt` support `--dry-run`; actual writes require `--yes`.
- `delete` currently only supports `--empty-only` and will not recursively delete non-empty directories.
- `repair --plan` only generates a plan and does not write to the vault; it identifies missing managed directories and directories that can be adopted.
- Paths must be relative directories within the vault; `.pinax`, `.git`, absolute paths, and `..` are rejected.

## Remote API

By default, `pinax api serve` is readonly. REST/RPC directory writes require the server to be started with `--allow-write` and `yes=true` provided in the request parameters; otherwise, it returns projection error `write_disabled` or `approval_required`.

When using CLI remote mode, `pinax --api-url http://127.0.0.1:8787 folder create spaces/research --purpose notes --yes --json` sends the confirmation as RPC `yes=true`. Local `folder create` still works without `--yes`; the flag exists so remote writes can satisfy the API confirmation gate.
