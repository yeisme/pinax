# KB decouple deltas for the agent memory ledger

## MODIFIED Requirements

### Requirement: Memory recall SHALL use deterministic multi-signal ranking

Pinax SHALL rank memory recall with deterministic non-vector signals while keeping the memory ledger separate from external RAG retrieval.

#### Scenario: Ranking combines query, entity, source, confidence, and freshness

- **WHEN** the memory ledger contains multiple confirmed records matching `release workflow`
- **AND** the user runs `pinax memory recall "release workflow" --entity pinax --vault ./my-notes --json`
- **THEN** Pinax SHALL rank candidates using keyword match, entity match, type affinity, source authority, confidence, freshness, lifecycle, and task-fitness signals
- **AND** the result SHALL be stable across repeated runs with the same ledger and query
- **AND** it SHALL NOT use embeddings, LanceDB, provider calls, remote services, or raw note body search outside the local memory projection.

#### Scenario: Source authority and confidence affect tie-breaks

- **WHEN** two confirmed records have equivalent query and entity matches
- **AND** one record cites an OpenSpec source while the other cites a generic file source
- **THEN** the OpenSpec-sourced record SHOULD rank higher
- **AND** `recall_reason` or `signals` SHALL explain the source and confidence contribution.
