# memory Command

`pinax memory` manages a non-vector agent memory ledger. It is for durable, cited, lifecycle-aware records such as facts, decisions, events, and tasks. It does not require embeddings, LanceDB, or the KB sidecar.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax memory capture` | Capture a typed memory record. | Writes `.pinax/memory/ledger.sqlite` unless `--dry-run` is set. |
| `pinax memory list` | List memory records with lifecycle filters. | No record writes; may initialize the local ledger if missing. |
| `pinax memory recall <query>` | Recall records with deterministic non-vector ranking. | No record writes; may initialize the local ledger if missing. |
| `pinax memory context <task>` | Return bounded agent context from memory. | No record writes; may initialize the local ledger if missing. |
| `pinax memory stats` | Show ledger counts. | No record writes; may initialize the local ledger if missing. |
| `pinax memory link` | Reserved for linking existing records to entities. | Not implemented in this slice. |
| `pinax memory prune` | Reserved for pruning expired or obsolete records. | Not implemented in this slice. |

## Common Workflow

```bash
pinax memory capture --type fact --subject pinax --predicate release_workflow --object "tag push triggers GitHub Actions" --source docs/operations/release-packaging.md --vault ./my-notes --json
pinax memory capture --type decision --subject pinax --object "Use structured memory before vector recall" --source openspec/changes/pinax-agent-memory-ledger/design.md --vault ./my-notes --json
pinax memory list --entity pinax --vault ./my-notes --json
pinax memory recall "release workflow" --entity pinax --vault ./my-notes --json
pinax memory context "prepare next release" --entity pinax --limit 12 --vault ./my-notes --agent
pinax memory stats --vault ./my-notes --json
```

## Record Types

| Type | Use for |
| --- | --- |
| `fact` | Stable project facts, preferences, paths, commands, and constraints. |
| `decision` | Product, architecture, release, or implementation decisions with rationale. |
| `event` | Published releases, failed runs, fixes, incidents, and external state changes. |
| `task` | Follow-up commitments, deferred work, and dependency-aware reminders. |

## Lifecycle States

| Status | Default recall behavior |
| --- | --- |
| `confirmed` | Included. |
| `draft` | Excluded unless `--include-draft` is used with `list`. |
| `superseded` | Excluded unless `--include-superseded` is used with `list`. |
| `expired` | Excluded unless `--include-expired` is used with `list`. |
| `rejected` | Excluded unless `--include-rejected` is used with `list`. |

## Memory vs KB

- `pinax memory` is deterministic structured memory: facts, decisions, events, tasks, source citations, lifecycle state, and explainable recall reasons.
- `pinax kb` is semantic search over note chunks through a vector backend such as LanceDB.
- Use `memory` when the agent needs stable project context or prior decisions. Use `kb` when the agent needs fuzzy semantic retrieval over larger note bodies.

## Safety Boundaries

- `.pinax/memory/` is a CLI-authored structured asset. Do not edit the SQLite files directly.
- `capture --dry-run` is read-only and does not create the ledger database.
- `memory context --agent` emits stable key=value facts and does not include full private note bodies, raw prompts, provider payloads, Authorization headers, cookies, or secrets.
- Cloud Sync should treat the memory ledger as a local rebuildable projection, not as authoritative cross-device state.
