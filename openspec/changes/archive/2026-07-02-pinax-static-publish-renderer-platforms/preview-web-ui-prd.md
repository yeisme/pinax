# Pinax Static Publish Preview Handoff

This change does not implement an internal Web UI, Electron preview, or standalone workbench page. `pinax-web-renderer` only generates public static publish output from a publish-safe bundle.

## Allowed Output

```text
index.html
notes/<slug>/index.html
tags/<tag>/index.html
assets/**
pinax-data/manifest.json
pinax-data/graph.json
pinax-data/search-index.json
```

These files are user publish artifacts. They are not Yeisme internal operator pages.

## Future UI

Future Pinax internal UI belongs in `client/yeisme-workbench` as a Pinax module. It should consume Pinax CLI/API projections, not import renderer internals or write vault metadata.

## Non-goals

- No React/Vite page scaffold in `web/pinax-web-renderer`.
- No Electron main/preload or desktop shell.
- No browser-side writes to `.pinax/**`, SQLite/GORM, sync state, receipts, or provider config.
- No secrets, raw provider payloads, raw prompts, private paths, or full chain-of-thought in fixtures or generated output.

## Verification

```bash
cd web/pinax-web-renderer && bun run test
cd web/pinax-web-renderer && bun run build
cd web/pinax-web-renderer && bun run render:static
```
