# brain Command

`pinax brain` previews bounded Agent Brain answers and context. Current implementations are read-only and evidence-first; future synthesis and maintenance commands stay planned until their owning implementation tasks land.

## Current Building Blocks

| Capability | Current command | Status | Boundary |
| --- | --- | --- | --- |
| Ingest dry-run | `pinax import markdown ./source --dry-run --vault ./my-notes --json` | implemented | Previews import without writing. |
| Structured memory context | `pinax memory context "prepare for Alice meeting" --entity alice --limit 12 --vault ./my-notes --agent` | implemented | Returns bounded memory facts and ranking reasons, not full private bodies. |
| Semantic KB context | `pinax kb context "prepare for Alice meeting" --limit 8 --vault ./my-notes --json` | implemented | Returns bounded semantic refs and provider metadata. |
| Search context | `pinax search "Alice" --vault ./my-notes --json` | implemented | Returns bounded candidates/snippets through Pinax search. |
| Link evidence | `pinax note backlinks "Alice" --vault ./my-notes --json` | implemented | Returns relationship evidence without writing. |
| Graph context | `pinax graph query --kind technique --match storyboard --vault ./my-notes --json` | implemented | Returns bounded graph projection results. |
| Query/database rows | `pinax query run 'SELECT title, status FROM notes WHERE status = "active" LIMIT 20' --lazy-index --vault ./my-notes --json` | implemented | Runs controlled local query projections; does not bypass repositories. |
| MCP access | `pinax mcp serve --vault ./my-notes` | implemented | Starts read-only stdio MCP. |
| Local API discovery | `pinax api routes --vault ./my-notes --json` | implemented | Discovers implemented local REST/RPC capabilities. |
| Proof loop | `pinax proof loop run --vault ./my-notes --json` | implemented | Produces reviewable maintenance state; apply requires explicit `--apply --yes`. |
| Extractive answer preview | `pinax brain answer "Alice roadmap budget" --vault ./my-notes --json` | implemented | Builds a bounded answer preview from existing search projection evidence; does not call an LLM provider or write the vault. |

## Answer Preview

```bash
pinax brain answer "Alice roadmap budget" --vault ./my-notes --json
```

The output uses schema `pinax.agent_brain.answer.v1` and includes `answer`, `claims[]`, `sources[]`, `open_questions[]`, `next_actions[]`, `cost`, `body_exposure`, and `context_bundle`. The first implementation is extractive: `cost.cost_class=none`, `provider_id=extractive`, `model=none`, `network_required=false`, and `body_exposure=bounded_projection`. It returns bounded source refs and real follow-up commands; it does not include full note bodies, raw provider payloads, hidden prompts, credentials, or private tool arguments.

## Planned Commands

The following commands are planned by OpenSpec change `pinax-agent-brain-layer`. They must not be documented as current commands until the owning implementation task lands and tests pass.

| Planned command | Planned capability | Status | Notes |
| --- | --- | --- | --- |
| `pinax brain context <task> --vault ./my-notes --json` | `brain.context.bundle` | planned | Would combine memory, KB, search, graph, query rows, project state, and receipts into one bounded context bundle. |
| `pinax brain answer <question> --vault ./my-notes --json` | `brain.answer.preview` | implemented extractive preview | Returns citation-first bounded preview with `claims[]`, `sources[]`, `open_questions[]`, cost metadata, and body exposure. |
| `pinax brain sources <question> --vault ./my-notes --json` | `brain.sources.list` | planned | Would list evidence refs without synthesis. |
| `pinax brain maintain --vault ./my-notes --dry-run --json` | `brain.maintenance.plan` | implemented plan-only preview | Produces reviewable maintenance candidates for stale facts, duplicates, and citation repair without writing vault content. |
| `pinax brain provider status --vault ./my-notes --json` | `brain.provider.cost_status` | planned | Would summarize provider/model/local/network/cost status without secrets. |

## Agent Brain Output Boundary

Agent Brain projections must remain body-safe and evidence-first:

- Current commands return bounded facts, snippets, relationship refs, query rows, receipts, and next actions.
- Planned answer synthesis must cite evidence and route unsupported claims to `open_questions[]`.
- Maintenance is plan-only by default; `--dry-run` writes nothing, and `--save-plan` writes only CLI-authored plan evidence under `.pinax/brain-maintenance-plans/`. Content-changing apply still goes through service-owned proof-loop commands.
- Hosted/team/OAuth/rate-limit behavior is future-owner work, not a current `cli/pinax` backend.

## Related

- [memory](./memory.md)
- [kb](./kb.md)
- [search](./search.md)
- [graph](./graph.md)
- [mcp](./mcp.md)
- [api](./api.md)
- [proof](./proof.md)
- [Agent-Safe Boundary](../overview/agent-safe-boundary.md)
