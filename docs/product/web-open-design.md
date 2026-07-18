# Pinax Workbench Module Handoff

Pinax does not own an independent internal Web/Electron workbench in `cli/pinax`. Future internal UI belongs in `client/yeisme-workbench` as a Pinax module. Pinax continues to own vault state, indexing, proof gates, sync, publish plans, redaction, receipts, and CLI/API contracts.

This document replaces the older Electron/Web Studio route. It keeps the useful contract handoff and removes the instruction to build a standalone Pinax client inside this subproject.

## Ownership

| Surface | Owner |
| --- | --- |
| Markdown vault, `.pinax/**`, SQLite/GORM projection, sync state, receipts | `cli/pinax` |
| CLI/API projections and mutation gates | `cli/pinax` |
| Static publish HTML renderer | `cli/pinax/web/pinax-web-renderer` |
| Internal notes/search/sync/project UI | `client/yeisme-workbench` Pinax module |
| Cross-device/team backend | future backend owner, not `cli/pinax` UI code |

## Workbench Module Contracts

Future Pinax UI should consume machine-readable contracts such as:

```bash
pinax api routes --vault ./my-notes --json
pinax api status --vault ./my-notes --json
pinax api schema export --format openapi --vault ./my-notes --json
pinax api serve --vault ./my-notes --readonly --port 8787
```

Workbench must not directly read or write `.pinax/**`, SQLite/GORM indexes, LanceDB files, provider config, sync state, publish receipts, event logs, or arbitrary vault paths. Mutations must go through Pinax services with proof gates, version snapshots, receipts, and redaction.

## Static Publish Renderer

`pinax-web-renderer` generates public static HTML output for `pinax publish build`. It is not an internal workbench page. It may write generated publish files such as:

```text
index.html
notes/<slug>/index.html
tags/<tag>/index.html
pinax-data/manifest.json
pinax-data/graph.json
pinax-data/search-index.json
```

Static publish output is allowed because it is the user's exported site artifact, not a Yeisme internal operator page.

## Non-goals

- No Electron or React workbench source inside `cli/pinax`.
- No Web Studio implementation in this subproject.
- No frontend writes to vault metadata or indexes.
- No provider tokens, raw payloads, private filesystem paths, raw prompts, or full chain-of-thought in renderer fixtures, screenshots, logs, or receipts.

## Module Admission

Before implementing a Pinax module in `client/yeisme-workbench`, create a module plan that declares route namespace, source contracts, read/mutation surfaces, redaction, fixtures, evidence paths, and not-owned-here responsibilities.
