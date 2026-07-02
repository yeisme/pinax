# graph Command

`pinax graph` builds and queries local knowledge graph projections derived from the vault. Markdown, prompt assets, and receipts remain the source records; `.pinax/graph/prompt_graph.json` is a rebuildable local projection.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `graph summary` | Summarize the local note link graph, including broken, ambiguous, and orphan counts. | Does not write. |
| `graph rebuild` | Rebuild the prompt knowledge graph projection from local prompt assets. | Writes `.pinax/graph/prompt_graph.json`. |
| `graph query` | Query bounded prompt graph context by node kind and label match. | Does not write. |

## Common Workflow

```bash
pinax graph summary --vault ./my-notes --json
pinax graph rebuild --vault ./my-notes --json
pinax graph query --kind technique --match storyboard --vault ./my-notes --json
pinax graph query --kind category --match poster --vault ./my-notes --agent
```

## Prompt Graph v1

The first graph slice derives prompt relationships from prompt asset tags and source refs:

- `prompt -> source`
- `prompt -> category`
- `prompt -> technique`
- `prompt -> style`
- `prompt -> subject`

Query output is intentionally bounded for agents. It returns prompt asset IDs and titles, not full prompt bodies or local filesystem paths.

## Boundaries

- The graph projection is local and rebuildable; it is not a new source of truth.
- `graph query` is read-only. If the projection is missing, Pinax can compute graph context from current prompt assets without writing.
- Do not use graph output as provenance by itself; source refs and collection receipts remain the audit evidence.
- Cloud Sync should not upload `.pinax/graph/` projections by default. After sync pull or import, rebuild graph projections locally with `pinax graph rebuild --vault ./my-notes --json` when graph context is needed.

## Agent Brain Role

`pinax graph summary`, `pinax note links`, `pinax note backlinks`, and `pinax graph query` contribute relationship evidence to staged Agent Brain context. They are evidence refs for future answer synthesis, not permission proofs and not a full company knowledge graph. Planned entity merge or dream cycle behavior must first produce a reviewable plan and must not rewrite note bodies silently.
