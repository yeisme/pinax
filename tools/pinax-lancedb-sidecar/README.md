# Pinax LanceDB Sidecar

`pinax-lancedb-sidecar` provides the real LanceDB runtime for `pinax kb --backend lancedb`.

Install from a Pinax checkout:

```bash
pipx install ./tools/pinax-lancedb-sidecar
pinax kb doctor --vault ./my-notes --json
```

Pinax sends bounded chunk previews, metadata, and vectors over stdin. The sidecar does not receive full note bodies.
