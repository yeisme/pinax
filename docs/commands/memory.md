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

The same stable commands can be forwarded through Local API Remote Mode when the server exposes the corresponding capability:

```bash
pinax --api-url http://127.0.0.1:8787 memory list --entity pinax --json
pinax --api-url http://127.0.0.1:8787 memory recall "memory capture" --entity pinax --json
pinax --api-url http://127.0.0.1:8787 memory context "pinax memory usage" --entity pinax --limit 12 --agent
pinax --api-url http://127.0.0.1:8787 memory stats --json
pinax --api-url http://127.0.0.1:8787 memory capture --type fact --subject pinax --predicate memory_capture_usage --object "Use --body or --subject and --object" --source cli-help --dry-run --json
```

Confirmed remote capture requires `pinax api serve --allow-write` on the server and `yes=true` at the API layer. CLI Remote API Mode currently forwards `--dry-run` for safe previews; direct REST/RPC clients pass `yes=true` when confirming a write.

## Recall Ranking

`memory recall` and `memory context` use deterministic non-vector ranking. The scorer combines keyword matches, source authority, confidence, freshness, and task fitness, then orders ties by score, source authority, creation time, and record id. Default recall includes only confirmed records, hides records superseded by newer entries, and collapses duplicate confirmed records with the same `subject` + `predicate` to the highest-ranked hit.

JSON output includes bounded ranking evidence for each hit:

| Field | Meaning |
| --- | --- |
| `score` | Final deterministic ranking score. |
| `recall_reason` | Compact textual explanation such as `status:confirmed + keyword:fts + source:docs`. |
| `signals.keyword_fts` | Whether SQLite FTS matched the query. |
| `signals.keyword_field` | Best matching structured field: `predicate`, `object`, `subject`, or `body`. |
| `signals.source_kind` | Source class such as `openspec`, `docs`, `github_actions`, or `file`. |
| `signals.source_authority` | Numeric source-authority contribution. |
| `signals.confidence` | Numeric confidence contribution. |
| `signals.freshness` | Recent `event` or `task` contribution. |
| `signals.task_fitness` | Topic-fit contribution for tasks such as release, test, provider, cloud, KB, or memory work. |

Agent output stays compact and low-token. `memory context --agent` emits facts such as `fact.memory.top_score` and `fact.memory.reason.1`, `fact.memory.reason.2`, `fact.memory.reason.3` instead of full memory bodies.

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

## Agent Brain Role

`pinax memory context` is one current building block for the staged Agent Brain context bundle. It contributes structured `memory_refs`, lifecycle state, confidence/freshness signals, supersession information, and source citations. It is not answer synthesis by itself; future `pinax brain answer ...` remains planned and must cite memory records as evidence rather than copying full private memory bodies.

## Safety Boundaries

- `.pinax/memory/` is a CLI-authored structured asset. Do not edit the SQLite files directly.
- `capture --dry-run` is read-only and does not create the ledger database.
- Local REST/RPC exposes `GET /v1/memory`, `POST /v1/memory:capture`, `GET /v1/memory:recall`, `GET /v1/memory:context`, `GET /v1/memory:stats`, and `Pinax.Memory.*` RPC methods. Real capture writes require API write mode and explicit confirmation.
- `memory recall --json` and `memory context --agent` omit full private memory bodies. They may include bounded `object`, ranking `signals`, `score`, and `recall_reason`, but must not include raw prompts, provider payloads, Authorization headers, cookies, or secrets.
- Cloud Sync should treat the memory ledger as local service-owned memory evidence, not as plaintext cross-device content. Source notes and receipts remain the portable authority; any future cross-device memory sync needs an explicit encrypted contract instead of uploading raw ledger state.
