## Design

Pinax consumes a `pinax.content_bundle.v1` file produced by an upstream acquisition/extraction tool. The bundle is local input; Pinax does not fetch remote pages.

`collection import --dry-run` analyzes the bundle without writes. `collection import --yes` writes one Markdown note per item and one `yeisme.prompt_asset.v1` projection per item with non-empty prompt text. Items with missing prompt text stay as notes and are surfaced by `doctor` and `diff`.

Prompt graph v1 is intentionally narrow. It derives nodes and edges from prompt asset tags and source refs, writes `.pinax/graph/prompt_graph.json` on `graph rebuild`, and keeps `graph query` bounded: prompt IDs and titles only, no full prompt bodies.

## Boundaries

- Markdown notes and prompt assets remain source records; graph JSON is rebuildable.
- `collection export` may write full prompt text to the user-requested output file, but command projections do not include prompt bodies.
- `--dry-run` writes nothing; mutation requires `--yes` where applicable.
- Eikona feedback and lifecycle changes continue through existing `pinax prompt feedback import` and `pinax prompt lifecycle` commands.
