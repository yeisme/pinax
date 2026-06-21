# Pinax Command Manual

This directory manages Pinax CLI command documentation. The root README keeps only common paths and examples; as the number of commands grows, command responsibilities, recommended workflows, safety boundaries, and migration notes are placed here.

## How to Read

- If you do not know where to start: first read the five core workflows below, then the command map.
- To organize your note structure: see [organize](./organize.md).
- If you only want to look up parameters for a specific command: run `pinax <command> --help`; help is the source of truth for the current binary.

## Five Core Workflows

Pinax is built around one agent-safe proof loop. Each path maps to a small set of real commands, and every command shares one bounded projection boundary (`--json`, `--agent`, `--events`).

| Path | Commands | Description |
| --- | --- | --- |
| **Capture** | [`pinax init`](./init.md), [`pinax note add`](./note.md), [`pinax inbox capture`](./inbox.md), [`pinax journal daily append`](./journal.md) | Add notes, inbox items and journal entries. |
| **Retrieve** | [`pinax index refresh`](./index.md), [`pinax search`](./search.md), [`pinax memory`](./memory.md), [`pinax note links`](./note.md), [`pinax note backlinks`](./note.md), [`pinax note orphans`](./note.md) | Build the index and read bounded context. |
| **Diagnose** | [`pinax vault doctor`](./vault.md), [`pinax vault stats`](./vault.md) | Check vault health and surface issues. |
| **Plan** | [`pinax repair plan --save`](./repair.md), [`pinax organize plan --save`](./organize.md) | Turn issues into reviewable saved plans. |
| **Apply safely** | [`pinax version snapshot`](./version.md), [`pinax repair apply --yes`](./repair.md), [`pinax organize apply --yes`](./organize.md) | Snapshot first, then apply with explicit confirmation. |

Cloud Sync (`pinax cloud`/`pinax sync`), daily briefing (`pinax briefing`), and provider expansion (`pinax backend`) are separate advanced workflows, not part of the local proof loop.

## Command Map

| Group | Command | When to Use |
| --- | --- | --- |
| Local vault | [`pinax init`](./init.md) | Initialize a local Markdown vault. |
| Local vault | [`pinax vault stats`](./vault.md) | View vault size, note count, and index summary. |
| Local vault | [`pinax vault validate`](./vault.md) | Validate whether the vault structure follows Pinax conventions. |
| Local vault | [`pinax vault doctor`](./vault.md) | Find health issues such as old notes, missing metadata, broken links, and duplicate titles. |
| Local vault | [`pinax vault dashboard`](./vault.md) | Start a localhost read-only dashboard. |
| Local vault | [`pinax record`](./record.md) | Manage the vault record ledger for registering and viewing record history. |
| Local vault | [`pinax project`](./project.md) | Manage project partitions inside a vault. |
| Note workflow | [`pinax journal`](./journal.md) | Manage daily, weekly, and monthly journals. |
| Note workflow | [`pinax inbox`](./inbox.md) | Quickly capture temporary content, then triage it into the formal note structure. |
| Note workflow | [`pinax draft`](./draft.md) | Manage draft-box notes, with support for creating, advancing, archiving, and discarding. |
| Note workflow | [`pinax note`](./note.md) | Create, read, edit, move, archive, delete, tag, and view links for notes. |
| Note workflow | [`pinax import markdown`](./import.md) | Bring external Markdown files or directories into the vault. |
| Note workflow | [`pinax export markdown`](./export.md) | Export a Markdown bundle by criteria. |
| Note workflow | [`pinax template`](./template.md) | Manage Markdown templates and template rendering records. |
| Organization and retrieval | [`pinax view`](./view.md) | Save and reuse a set of note filtering criteria. |
| Organization and retrieval | [`pinax folder`](./folder.md) | Uniformly create, move, delete, take over, and repair vault directories. |
| Organization and retrieval | [`pinax search`](./search.md) | Search local notes, with support for filters such as tag, folder, kind, status, and link target. |
| Organization and retrieval | [`pinax kb`](./kb.md) | Import text/Markdown, rebuild the local LanceDB semantic projection, and return bounded agent context. |
| Organization and retrieval | [`pinax memory`](./memory.md) | Capture cited facts, decisions, events, and tasks for deterministic agent memory. |
| Organization and retrieval | [`pinax graph`](./graph.md) | Rebuild and query local knowledge graph projections for prompt/content assets. |
| Organization and retrieval | [`pinax query`](./query.md) | Run controlled Pinax SQL queries against the local note database. |
| Organization and retrieval | [`pinax dataview`](./dataview.md) | Run safe Dataview-compatible table, list, and task queries. |
| Organization and retrieval | [`pinax database`](./database.md) | Manage database views and property schemas. |
| Organization and retrieval | [`pinax metadata`](./metadata.md) | Plan and apply frontmatter metadata completion. |
| Organization and retrieval | [`pinax repair`](./repair.md) | Generate maintenance plans from doctor issues and apply only low-risk fixes. |
| Organization and retrieval | [`pinax organize`](./organize.md) | Plan, save, list, and apply note-structure organization plans. |
| Automation and integration | [`pinax briefing`](./briefing.md) | Manage daily trending-note briefing recipes, runs, and delivery. |
| Automation and integration | [`pinax cloud`](./cloud.md) | Manage local state for cloud sync. |
| Automation and integration | [`pinax sync`](./sync.md) | Generate and execute sync plans. |
| Automation and integration | [`pinax plan`](./plan.md) | Manage personal daily, weekly, and monthly planning workflows. |
| Automation and integration | [`pinax prompt`](./prompt.md) | Manage reusable prompt assets, lifecycle decisions, URI resolution, and prompt usage feedback. |
| Automation and integration | [`pinax collection`](./collection.md) | Import, inspect, and export content bundle production pipelines. |
| Automation and integration | [`pinax publish`](./publish.md) | Build safe GitHub Pages or GitHub Wiki publishing surfaces from the local vault. |
| Automation and integration | [`pinax plugin`](./plugin.md) | Validate, install, grant, run, diagnose, disable, and uninstall dynamic plugins through audited services. |
| Automation and integration | [`pinax backend`](./backend.md) | Manage vault backend providers. |
| Automation and integration | [`pinax mcp`](./mcp.md) | Start a read-only MCP surface. |
| Configuration and maintenance | [`pinax config`](./config.md) | View, set, and diagnose Pinax configuration. |
| Configuration and maintenance | [`pinax version`](./version.md) | View the version backend and create snapshot evidence. |
| Configuration and maintenance | [`pinax asset`](./asset.md) | Manage vault multimedia and binary assets. |
| Configuration and maintenance | [`pinax storage`](./storage.md) | Configure the vault storage backend. |
| Configuration and maintenance | [`pinax index`](./index.md) | Manage local SQLite/GORM index projections. |

## Main Paths for Version and Asset

- `pinax version` is the user-visible entry point for vault version evidence. It is used to view backend capabilities, create snapshots, read history/diff/show/changed, and generate restore plans. Git is only an optional backend type; regular help, error hints, and next actions should all recommend `pinax version ...`.
- `pinax git snapshot` is retained only as a hidden compatibility alias, with behavior routed to `pinax version snapshot`. During migration, change scripts from `pinax git snapshot --vault <vault> --message <msg>` to `pinax version snapshot --vault <vault> --message <msg>`.
- `pinax asset` manages manifests, hashes, references, backlinks, orphans, missing files, and repair plans for images, audio, video, PDFs, and other binary files in the vault. The asset manifest is CLI-authored metadata; asset payloads remain regular files inside the vault and do not enter stdout, stderr, events, or record logs.

## Common Choices

| Goal | Recommended Entry Point |
| --- | --- |
| Create a new knowledge base | `pinax init ./my-notes --title "My Knowledge Base"` |
| Register and select a default vault | `pinax vault register ./my-notes --name work --default` |
| Quickly write a note | `pinax note add "Title" --body "Content" --vault work` |
| Collect unorganized content first | `pinax inbox capture "Temporary idea" --vault work` |
| View today's note | `pinax journal daily show --vault work` |
| Search content | `pinax search "keyword" --vault work` |
| Search semantic context | `pinax kb search "project context" --vault work` |
| Recall agent memory | `pinax memory recall "release workflow" --entity pinax --vault work` |
| Manage directories | `pinax folder create spaces/research --purpose notes --vault ./my-notes` |
| View vault health | `pinax vault doctor --vault ./my-notes` |
| Fix health issues | `pinax repair plan --vault ./my-notes --save` |
| Organize file structure | `pinax organize plan --vault ./my-notes --save` |
| Rebuild the index | `pinax index rebuild --vault ./my-notes` |

## Write Rules

By default, Pinax commands can be divided into three categories:

| Type | Example | Writes to Vault |
| --- | --- | --- |
| Read-only viewing | `search`, `vault stats`, `vault doctor`, `organize plan` without `--save` | Does not write Markdown, `.pinax/`, Git, or remote. |
| Save plan | `repair plan --save`, `organize plan --save` | Writes only `.pinax/*-plans/<plan_id>.json`. |
| Apply changes | `metadata apply --yes`, `repair apply --yes`, `organize apply --yes` | Writes to the local vault; high-risk commands also require a snapshot. |

`--json`, `--agent`, `--events`, and `--explain` are output modes and do not change business semantics. Machine protocol fields, CLI output, automation output, logs, errors, and examples remain in English or existing stable names.

## Remote API Mode

`pinax --api-url http://127.0.0.1:8787 <supported-command>` forwards supported read/write-plan commands to `pinax api serve` through `POST /v1/rpc`. `PINAX_API_URL` enables the same mode for agents, and `pinax config set remote.api_url http://127.0.0.1:8787 --scope user` persists the default endpoint for ordinary supported commands. Use `--api-token`, `--api-token-file`, or `PINAX_API_TOKEN` when the server requires Bearer auth; do not store raw tokens in config. Remote mode rejects explicit `--vault` with `remote_vault_conflict` and rejects unsupported commands with `remote_command_unsupported` instead of falling back to local execution. Local control commands (`config`, `api`, `token`, `profile`, `vault`) stay local when the endpoint comes from `remote.api_url` so configuration remains editable.

Cloud Sync is a different distributed workflow from Remote API Mode. Remote API Mode forwards commands to one running local vault. Cloud Sync (`pinax cloud` + `pinax sync --target cloud`) is intended to keep independent local vaults on each device converged through encrypted backend revisions and conflict handling.
