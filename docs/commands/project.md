# pinax project

`pinax project` manages local project workspaces inside one Markdown vault. It is a local organization surface, not a remote issue tracker, and it does not synchronize work items to GitHub, Gitea, TaskBridge, or any provider by itself.

## Commands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax project create <slug>` | Create a vault project registry entry. | `.pinax/projects/*.json` |
| `pinax project delete <slug>` | Move an obsolete project registry entry and recoverable content to trash. | Project registry, trash tombstone |
| `pinax project list` | List vault projects. | No |
| `pinax project show <slug>` | Show one vault project and recommended next actions. | No |
| `pinax project switch <slug>` | Set the active vault project. | Project state |
| `pinax project learning init <project> <slug>` | Initialize a long-term learning project pack. | Project, workspace, board config, starter notes/items |
| `pinax project subproject create <project> <slug>` | Create a vault-local subproject workspace under `notes/projects/<project>/<slug>/`. | Directories and workspace registry |
| `pinax project subproject delete <project> <slug>` | Move a subproject workspace, workspace registry, and board config to trash. | Workspace registry, workspace directory, trash tombstone |
| `pinax project subproject list [project]` | List subproject workspaces for a project, defaulting to the current project when omitted. | No |
| `pinax project subproject show <project> <slug>` | Show one workspace projection. | No |
| `pinax project board configure <project>` | Save project or subproject board columns. | Board config asset |
| `pinax project board show <project>` | Render a local board projection. | No |
| `pinax project board plan <project>` | Generate a project board plan snapshot. | No by default; snapshot evidence with `--save`. |
| `pinax project board export <project>` | Export a project board for handoff. | No |
| `pinax project board view save <project> <view>` | Save board columns/group/sort/display config. | Board view asset |
| `pinax project item add <project> <title>` | Create a managed Markdown work item. | Markdown note |
| `pinax project item show <item>` | Show a managed Markdown work item. | No |
| `pinax project item plan <item>` | Preview a move or archive operation. | No |
| `pinax project item move <item> <column>` | Move a managed work item. | Markdown note |
| `pinax project item archive <item>` | Archive a managed work item after approval and snapshot checks. | Markdown note |
| `pinax task adopt <item> --plan` | Preview adoption for an inferred checklist task. | No |
| `pinax task adopt <item> --yes` | Write a task adoption ledger for an inferred checklist task. | Task adoption ledger |

## Project Workspace

Create a project and a subproject workspace:

```bash
pinax project create research --name "Research" --notes-prefix notes/research --vault ./my-notes --json
pinax project subproject create research stock-learning --title "Stock Learning" --template scenario --vault ./my-notes --json
pinax project subproject show research stock-learning --vault ./my-notes --json
```

The default workspace path is `notes/projects/<project>/<subproject>/`, relative to the active vault. For example, if the vault is `~/data/yeisme-notes`, `stock-learning` under `research` lives at `~/data/yeisme-notes/notes/projects/research/stock-learning/`.

Pinax creates semantic default directories: `charter`, `inbox`, `sources`, `runs`, `outputs`, `retros`, and `tool-candidates`. New workspaces should not use numeric prefixes such as `00-` or `10-` by default; those prefixes are only tolerated as legacy vault content or explicit user-defined template choices. The registry remains `.pinax/project-workspaces/<project>/<subproject>.json` and is written through the application service.

Project Manager surfaces should show `Vault root`, `Workspace path`, and `Full path preview` before creating a subproject. A Pinax subproject is a Markdown workspace inside the vault, not a Yeisme monorepo subproject, Git submodule, independent repository, or development toolchain bootstrap.

Subproject slugs must be safe relative slugs. Empty slugs, `..`, absolute paths, `.pinax`, `.git`, `temp`, `dist`, `node_modules`, and `vendor` are rejected.

## Trash lifecycle

Delete obsolete project shells through the CLI-authored trash lifecycle. Do not hand-edit `.pinax/projects.json` or workspace registry files.

```bash
pinax project delete history --vault ./my-notes --yes --json
pinax trash list --vault ./my-notes --json
pinax trash restore project/history --vault ./my-notes --json
pinax trash purge project/history --dry-run --vault ./my-notes --json
pinax trash purge project/history --hard --yes --vault ./my-notes --json
```

Subproject deletion uses the same tombstone path. Non-empty workspaces require a recent Pinax version snapshot before deletion:

```bash
pinax version snapshot --vault ./my-notes --message "before subproject delete"
pinax project subproject delete research stock-learning --vault ./my-notes --yes --json
pinax trash restore subproject/research/stock-learning --vault ./my-notes --json
```

`project list`, `project subproject list`, board projections, search, and index refresh exclude `.pinax/trash/**` from active projections by default. Trash-aware commands expose tombstones explicitly.

## Long-Term Learning Pack

Initialize a reusable learning workspace, board, starter notes, and starter work items in one command:

```bash
pinax init ./stock-learning-notes --title "学习炒股的全部笔记" --json
pinax project learning init investing stock-learning --title "学习炒股的全部笔记" --project-name "学习炒股" --notes-prefix notes/investing --preset stock-learning --vault ./stock-learning-notes --json
pinax project board show investing --subproject stock-learning --vault ./stock-learning-notes
```

`--preset learning` creates a generic long-term learning pack. `--preset stock-learning` keeps the same local-first structure but adds stock-learning starter content: a charter, source index, weekly review, learning board columns (`inbox,planned,learning,practice,review,retrospective,done`), and starter items for terminology, K-line/volume basics, risk rules, and weekly review.

The pack is idempotent. Re-running the same command reuses the project, workspace, board config, and existing starter files; it reports `notes.created=0` and `items.created=0` when nothing new was needed.

Stock-learning content is for education, source tracking, historical review, simulated practice, and risk rules. It is not an automated recommendation, trading decision, or return-promise system.

## Board

Show a project board or a subproject-scoped board:

```bash
pinax project board configure research --columns inbox,next,doing,blocked,review,done --vault ./my-notes --json
pinax project board configure research --subproject stock-learning --columns inbox,next,doing,blocked,review,done --vault ./my-notes --json
pinax project board show research --subproject stock-learning --note-display card --vault ./my-notes
pinax project board show research --subproject stock-learning --compact --vault ./my-notes
pinax project board show research --subproject stock-learning --vault ./my-notes --json
pinax project board show research --subproject stock-learning --vault ./my-notes --agent
pinax project board plan research --subproject stock-learning --save --vault ./my-notes --json
pinax project board export research --subproject stock-learning --format markdown --vault ./my-notes --json
```

Human output summarizes `Project`, `Path`, `Structure`, `Board`, risks, and the recommended next step. Machine modes use the shared projection: JSON is a single envelope, `--agent` emits stable key=value facts, and `--events` emits NDJSON lifecycle events.

Save reusable board views without saving result items:

```bash
pinax project board view save research active --subproject stock-learning --columns next,doing,blocked,review --display card --vault ./my-notes --json
pinax project board show research --subproject stock-learning --view active --vault ./my-notes --json
```

Saved board views are CLI-authored configuration. They do not snapshot board rows and can be rendered by CLI/API clients from the current vault state.

## Work Items

Create and maintain managed work items:

```bash
pinax project item add research "Read annual report" --subproject stock-learning --column next --labels research,learning --milestone q3 --priority high --due-at 2026-07-01 --blocked-by item_market_data --vault ./my-notes --json
pinax project item show item_abc123 --vault ./my-notes --json
pinax project item plan item_abc123 --action move --column doing --vault ./my-notes --json
pinax project item move item_abc123 doing --vault ./my-notes --json
pinax version snapshot --vault ./my-notes --message "snapshot before archive"
pinax project item archive item_abc123 --yes --vault ./my-notes --json
```

Only managed project work items are writable. Inferred or unmanaged Markdown checklist lines return `project_item_unmanaged`. Archive and high-risk moves require explicit approval and snapshot gates.

Inferred checklist tasks can be adopted in two steps:

```bash
pinax task adopt task_abc123 --plan --vault ./my-notes --json
pinax task adopt task_abc123 --yes --vault ./my-notes --json
```

The plan command is read-only. The `--yes` command writes the task adoption ledger through the application service; agents should not hand-write `.pinax/task-adoptions/**`.

## API Boundary

Local REST/RPC exposes read projections and controlled write plans for dashboards and agents. Workspace show/list, task adopt plan, project item plan, database view render, and graph summary routes return the same projection schema as CLI JSON; write-like remote calls return `write_disabled`, `approval_required`, `snapshot_required`, or a plan unless the server explicitly starts with write mode enabled and the request includes confirmation.

Project Workspace remains local-first. The API does not become a remote issue tracker, does not write provider issues, and does not parse Markdown directly in handlers.

See also [`plan`](./plan.md) for personal daily/weekly/monthly plans and TaskBridge action drafts, and [`api`](./api.md) for local projection access.
