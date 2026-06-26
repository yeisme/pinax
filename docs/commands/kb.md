# kb Command

`pinax kb` manages the local semantic knowledge-base projection. Markdown files remain the source of truth; the real LanceDB projection is a rebuildable local artifact under `.pinax/kb/lancedb/` and is accessed through `pinax-lancedb-sidecar`.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax kb import <source> --dry-run` | Preview Markdown/text import into the vault. | Does not write. |
| `pinax kb import <source> --yes` | Import Markdown/text as normalized Pinax notes. | Writes vault notes, index, and receipt evidence. |
| `pinax kb rebuild` | Rebuild the local semantic projection. | Writes `.pinax/kb/lancedb/`. |
| `pinax kb refresh` | Refresh the local semantic projection after vault changes or sync pull. | Writes `.pinax/kb/lancedb/`. |
| `pinax kb doctor` | Check whether the LanceDB sidecar is available. | Read-only, except creating the local store directory during sidecar startup. |
| `pinax kb provider list` | List embedding providers and local configuration status. | Read-only. |
| `pinax kb provider doctor <provider>` | Check one embedding provider. | Read-only. |
| `pinax kb search <query>` | Search semantic chunks. | Read-only. |
| `pinax kb context <task>` | Return bounded agent context. | Read-only. |

## Common Workflow

```bash
pipx install git+https://github.com/yeisme/pinax.git#subdirectory=tools/pinax-lancedb-sidecar
pinax kb doctor --vault ./my-notes --json
pinax kb provider list --vault ./my-notes --json
pinax kb provider doctor openai --vault ./my-notes --json
pinax kb import ./source --include "*.md" --include "*.txt" --vault ./my-notes --dry-run --json
pinax kb import ./source --include "*.md" --include "*.txt" --vault ./my-notes --yes --json
pinax kb rebuild --backend lancedb --provider gemini --vault ./my-notes --json
pinax kb rebuild --backend lancedb --provider openai --model text-embedding-3-small --vault ./my-notes --json
pinax kb rebuild --backend lancedb --provider ollama --model nomic-embed-text --vault ./my-notes --json
pinax kb search "Cloud Sync semantic projection" --vault ./my-notes --agent
pinax kb context "prepare an implementation plan" --limit 8 --vault ./my-notes --json
```

## Provider and Backend Split

Providers create embeddings. Backends store and search vectors. They are configured independently:

| Provider | Default model | Credential source | Notes |
| --- | --- | --- | --- |
| `gemini` | `text-embedding-004` | `GEMINI_API_KEY` | Default provider. |
| `openai` | `text-embedding-3-small` | `OPENAI_API_KEY` | Uses the OpenAI embeddings API; `OPENAI_BASE_URL` can point to a compatible endpoint. |
| `ollama` | `nomic-embed-text` | Local service | Uses `http://127.0.0.1:11434` by default; set `OLLAMA_HOST` for another local endpoint. |
| `fake` | `fake-hash-v1` | none | Deterministic local provider for tests and offline validation. |

| Backend | Use for | Notes |
| --- | --- | --- |
| `lancedb` | Normal local semantic projection. | Requires `pinax-lancedb-sidecar`; stores rebuildable data under `.pinax/kb/lancedb/`. |
| `fake` | Local deterministic tests. | Writes a JSONL projection under `.pinax/kb/fake/`; not a LanceDB store. |

`pinax kb provider list --json` reports provider names, default models, `configured` status, local-only status, and credential source type. It reports source names such as `env:OPENAI_API_KEY`, not credential values.

`pinax kb provider doctor <provider> --json` checks one provider. Missing OpenAI or Gemini credentials return a stable `provider_not_configured` error. Ollama reports local service reachability without requiring a token. `fake` is always available.

## Multi-Device Rule

Cloud Sync synchronizes encrypted vault revisions only. Do not sync `.pinax/kb/lancedb/`; each device should run `pinax kb refresh --vault <vault>` after pulling changes.

## Provider Notes

- `--provider gemini` is the default embedding provider and uses `GEMINI_API_KEY`.
- `--provider openai --model text-embedding-3-small` uses `OPENAI_API_KEY`. Use user-level config or environment variables for automation; do not persist tokens in repository files or shell credential scripts.
- `--provider ollama --model nomic-embed-text` uses the local Ollama service. It does not require a network token.
- `--provider fake` is for local validation and tests; it does not call the network.
- `--backend lancedb` requires `pinax-lancedb-sidecar` on `PATH`, or set `kb.sidecar.executable` / `PINAX_KB_SIDECAR`.
- `--backend fake` is the deterministic built-in test backend; it is not LanceDB.
- Machine output includes provider/model facts but never raw provider payloads or credentials.

## Sidecar Configuration

```bash
pinax config set kb.sidecar.executable pinax-lancedb-sidecar --scope user
pinax config set kb.sidecar.timeout_seconds 30 --scope user
```

The sidecar protocol is `pinax.kb.sidecar.v1`. Pinax sends vectors, provider/model metadata, source metadata, collection metadata, and bounded previews. It does not send full note bodies or raw provider payloads to the sidecar. Provider payloads, Authorization headers, tokens, and raw prompts must not appear in stdout, stderr, events, or integration evidence.
