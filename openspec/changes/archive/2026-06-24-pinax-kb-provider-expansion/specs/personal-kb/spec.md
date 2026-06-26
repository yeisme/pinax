## ADDED Requirements

### Requirement: KB SHALL expose additive embedding provider discovery

Pinax SHALL expose KB embedding provider discovery without changing the existing `gemini`, `fake`, `lancedb`, or fake backend defaults.

#### Scenario: Provider list reports configured status without secrets

- **WHEN** the user runs `pinax kb provider list --vault ./my-notes --json`
- **THEN** stdout SHALL be a valid Pinax JSON envelope
- **AND** the response SHALL include provider ids for `gemini`, `openai`, `ollama`, and `fake`
- **AND** provider facts SHALL include default model, local-only status, and configured status where available
- **AND** stdout, stderr, events, docs, fixtures, and integration evidence SHALL NOT include raw tokens, Authorization headers, provider payloads, cookies, hidden prompts, private tool arguments, or full note bodies.

#### Scenario: Provider doctor diagnoses missing credentials safely

- **WHEN** the user runs `pinax kb provider doctor --provider openai --model text-embedding-3-small --vault ./my-notes --json` without configured credentials
- **THEN** Pinax SHALL return a valid failure envelope with a stable error code and a concrete next action
- **AND** the diagnostic SHALL mention only credential source type or env var name, not the credential value.

### Requirement: KB SHALL support OpenAI and Ollama embedding providers

Pinax SHALL support cloud and local embedding providers through the semantic provider registry while keeping Markdown vaults and KB projections local-first.

#### Scenario: Rebuild with OpenAI provider writes only local projection

- **WHEN** the user runs `pinax kb rebuild --backend lancedb --provider openai --model text-embedding-3-small --vault ./my-notes --json`
- **THEN** Pinax SHALL scan local notes, request embeddings through the OpenAI provider, and rebuild `.pinax/kb/lancedb/` through the LanceDB sidecar
- **AND** the command SHALL NOT write Markdown note bodies, provider state, Git state, Cloud Sync state, or remote sync objects
- **AND** machine output SHALL include provider/model/backend facts without raw provider payloads or credentials.

#### Scenario: Rebuild with Ollama provider is local-only

- **WHEN** the user runs `pinax kb rebuild --backend lancedb --provider ollama --model nomic-embed-text --vault ./my-notes --json`
- **THEN** Pinax SHALL use the configured local Ollama endpoint for embeddings
- **AND** failure to reach the endpoint SHALL return a stable diagnostic instead of requiring cloud credentials
- **AND** tests SHALL use a fake local HTTP server rather than a real user Ollama instance.

### Requirement: LanceDB sidecar protocol SHALL remain backward-compatible

Pinax SHALL keep the LanceDB sidecar protocol compatible while allowing optional provider metadata for diagnostics and future index tuning.

#### Scenario: Sidecar accepts old and new rebuild requests

- **WHEN** `pinax-lancedb-sidecar rebuild` receives a `pinax.kb.sidecar.v1` request without provider metadata
- **THEN** it SHALL continue to rebuild the local store successfully
- **AND** when the request includes optional provider, model, embedding dimension, distance metric, or collection metadata, the sidecar SHALL accept those fields without requiring existing clients to send them.

#### Scenario: Search remains bounded and provider-aware

- **WHEN** the user runs `pinax kb search "release workflow" --vault ./my-notes --agent`
- **THEN** stdout SHALL include stable key=value facts for command, status, backend, provider, model, matches, and total
- **AND** stdout SHALL NOT include full note bodies, raw provider payloads, credentials, Authorization headers, cookies, or hidden prompts.
