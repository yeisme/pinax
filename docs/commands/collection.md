# collection Command

`pinax collection` turns a `pinax.content_bundle.v1` file into a local-first content collection: Markdown notes, prompt assets, receipts, and graph-ready metadata. Pinax does not crawl websites; upstream tools such as Indagator prepare the bundle, and Pinax owns the durable vault assets.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `collection import --from <bundle> --dry-run` | Preview note and prompt asset imports. | Does not write. |
| `collection import --from <bundle> --yes` | Import bundle items as notes and complete prompts as prompt assets. | Writes notes, prompt projection rows, index updates, receipt, and event evidence. |
| `collection diff --from <bundle>` | Compare bundle items with existing collection notes and prompt assets. | Does not write. |
| `collection doctor --from <bundle>` | Report bundle quality issues such as missing prompt text. | Does not write. |
| `collection export --to <file>` | Export local prompt assets as `eikona.prompt_bundle.v1`. | Writes the requested output file. |

## Common Workflow

```bash
pinax collection import --from ./upma-content-bundle.json --vault ./my-notes --dry-run --json
pinax collection doctor --from ./upma-content-bundle.json --vault ./my-notes --json
pinax collection import --from ./upma-content-bundle.json --vault ./my-notes --yes --json
pinax graph rebuild --vault ./my-notes --json
pinax collection export --to ./eikona-bundle.json --format eikona.prompt_bundle.v1 --vault ./my-notes --json
```

## Bundle Contract

The import format is `schema_version: pinax.content_bundle.v1`. Each item needs a stable `id`; complete prompt assets also need non-empty `prompt`. Items without prompt text are still imported as Markdown notes and marked in `doctor`/`diff` facts as missing prompt items.

## Boundaries

- `--dry-run` never writes notes, `.pinax/`, Git, providers, or remote state.
- A real import requires `--yes` and writes structured evidence through Pinax services.
- `collection export` may include prompt bodies in the output file requested by the user, but command projections stay bounded.
- Provider crawling, extraction, and image rendering remain outside Pinax; use Indagator for acquisition and Eikona for rendering/feedback.
