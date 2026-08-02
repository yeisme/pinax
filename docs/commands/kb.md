# kb Command

`pinax kb` manages the local semantic knowledge-base projection. Markdown files remain the source of truth; new LanceDB projections are immutable candidates under `.pinax/kb/generations/<generation-id>/lancedb/` and are selected only through the single `activation.json` descriptor.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax kb import <source> --dry-run` | Preview Markdown/text import into the vault. | Does not write. |
| `pinax kb import <source> --yes` | Import Markdown/text as normalized Pinax notes. | Writes vault notes, index, and receipt evidence. |
| `pinax kb rebuild` | Stage a new local semantic candidate. | Writes one `.pinax/kb/generations/<id>/` candidate; it does not activate it. |
| `pinax kb refresh` | Stage a new candidate after vault changes or sync pull. | Same staged-generation boundary as rebuild. |
| `pinax kb evaluate` | Replay a versioned retrieval/citation suite against active or candidate generation. | Writes an immutable sanitized evaluation receipt. |
| `pinax kb activate` | Activate a candidate with a matching passed receipt. | CAS-updates one active/previous descriptor. |
| `pinax kb rollback` | Exchange active and previous generation. | CAS-updates the descriptor under the vault lock. |
| `pinax kb doctor` | Check whether the LanceDB sidecar is available. | Read-only, except creating the local store directory during sidecar startup. |
| `pinax kb provider list` | List embedding providers and local configuration status. | Read-only. |
| `pinax kb provider doctor <provider>` | Check one embedding provider. | Read-only. |
| `pinax kb search <query>` | Search semantic chunks. | Read-only. |
| `pinax kb context <task>` | Return bounded agent context. | Read-only. |

If a vault contains only the old `pinax.kb.sidecar.v1` projection, `pinax kb doctor` reports the explicit `legacy_v1_readonly` compatibility profile and the N/N+1/N+2 migration window. Search/context require `--legacy-v1-readonly`; they never start the historical sidecar and only read a valid local legacy projection. Normal rebuild creates a new Inferrum v1 generation. Import, rebuild, refresh, activate, rollback and prune never write the historical projection.

## Common Workflow

```bash
pipx install git+https://github.com/yeisme/inferrum.git#subdirectory=tools/inferrum-lancedb-sidecar
pinax kb doctor --vault ./my-notes --json
pinax kb provider list --vault ./my-notes --json
pinax kb provider doctor openai --vault ./my-notes --json
pinax kb import ./source --include "*.md" --include "*.txt" --vault ./my-notes --dry-run --json
pinax kb import ./source --include "*.md" --include "*.txt" --vault ./my-notes --yes --json
pinax kb rebuild --backend lancedb --provider gemini --vault ./my-notes --json
pinax kb rebuild --backend lancedb --provider openai --model text-embedding-3-small --vault ./my-notes --json
pinax kb rebuild --backend lancedb --provider ollama --model pinax-qwen3-embedding:lowmem --vault ./my-notes --json
pinax kb evaluate --suite .pinax/kb/evaluation-suites/local-canary.json --generation <candidate-id> --vault ./my-notes --json
pinax kb activate --generation <candidate-id> --suite .pinax/kb/evaluation-suites/local-canary.json --run-id <passed-run-id> --vault ./my-notes --json
pinax kb rollback --vault ./my-notes --json
pinax kb search "Capsa Sync semantic projection" --vault ./my-notes --agent
pinax kb context "prepare an implementation plan" --limit 8 --vault ./my-notes --json
pinax kb search "legacy migration" --legacy-v1-readonly --vault ./my-notes --json
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
| `lancedb` | Normal local semantic projection. | Requires the Inferrum-owned `inferrum-lancedb-sidecar`; new writes use `inferrum.sidecar.v1` inside a generation directory. |
| `fake` | Local deterministic tests. | Writes a JSONL projection under `.pinax/kb/fake/`; not a LanceDB store. |

`pinax kb provider list --json` reports provider names, default models, `configured` status, local-only status, and credential source type. It reports source names such as `env:OPENAI_API_KEY`, not credential values.

`pinax kb provider doctor <provider> --json` checks one provider. Missing OpenAI or Gemini credentials return a stable `provider_not_configured` error. Ollama reports local service reachability without requiring a token. `fake` is always available.

## Multi-Device Rule

Capsa Sync synchronizes encrypted vault revisions only. Do not sync `.pinax/kb/generations/`, `.pinax/kb/activation.json`, `.pinax/kb/fake/`, raw vectors, raw provider payloads, or provider credentials; each device should run `pinax kb refresh --vault <vault>` after pulling changes. KB/LanceDB is a local rebuildable projection, not a source of truth.

## Agent Brain Role

`pinax kb context` is one current building block for staged Agent Brain context. It contributes bounded semantic refs, provider/model/source type, local-only or network-backed status, and safe next actions such as `pinax kb provider doctor openai --vault ./my-notes --json`. It does not produce citation-first answer synthesis by itself. `pinax brain answer ...` currently provides extractive preview with `cost_class=none`; future provider-backed synthesis must keep provider/cost metadata visible and must not hide metered or network-backed work.

## Provider Notes

- `--provider gemini` is the default embedding provider and uses `GEMINI_API_KEY`.
- `--provider openai --model text-embedding-3-small` uses `OPENAI_API_KEY`. Use user-level config or environment variables for automation; do not persist tokens in repository files or shell credential scripts.
- `--provider ollama --model nomic-embed-text` uses the local Ollama service. It does not require a network token.
- `--provider fake` is for local validation and tests; it does not call the network.
- `--backend lancedb` requires `inferrum-lancedb-sidecar` on `PATH`, or set `kb.sidecar.executable` / `PINAX_KB_SIDECAR`.
- `--backend fake` is the deterministic built-in test backend; it is not LanceDB.
- Machine output includes provider/model facts but never raw provider payloads or credentials.

## Sidecar Configuration

```bash
pinax config set kb.sidecar.executable inferrum-lancedb-sidecar --scope user
pinax config set kb.sidecar.timeout_seconds 30 --scope user
```

New writes use `inferrum.sidecar.v1` with canonical opaque `records`; Pinax sends vectors, provider/model identity, safe source metadata and bounded previews. The old `pinax.kb.sidecar.v1` projection is not a new-write target and remains only as an explicit compatibility/read-only profile during its release window. Neither protocol receives full note bodies, raw provider payloads, credentials, Authorization headers, tokens or raw prompts.
