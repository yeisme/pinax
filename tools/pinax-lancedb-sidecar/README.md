# Pinax LanceDB Sidecar

`pinax-lancedb-sidecar` provides the real LanceDB runtime for `pinax kb --backend lancedb`.

Install from a Pinax checkout:

```bash
pipx install ./tools/pinax-lancedb-sidecar
pinax kb doctor --vault ./my-notes --json
```

Pinax sends bounded chunk previews, metadata, and vectors over stdin. The sidecar does not receive full note bodies.

Local development uses two test layers:

```bash
task kb:sidecar:protocol
task kb:sidecar:test
```

`task kb:sidecar:protocol` runs offline protocol tests without installing Python packages. `task kb:sidecar:test` installs this package in a temporary venv and runs the real LanceDB rebuild/search tests.
