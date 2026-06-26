## ADDED Requirements

### Requirement: Memory recall SHALL use deterministic multi-signal ranking

Pinax SHALL rank memory recall with deterministic non-vector signals while keeping Memory separate from KB semantic search.

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

### Requirement: Memory recall SHALL collapse obsolete and duplicate records by default

Pinax SHALL avoid filling agent context with duplicate or obsolete records while preserving auditability through list commands.

#### Scenario: Superseded records remain auditable but are not default context

- **WHEN** a confirmed memory record supersedes an older record
- **AND** the user runs `pinax memory context "prepare next release" --entity pinax --vault ./my-notes --json`
- **THEN** Pinax SHALL return the current confirmed record by default
- **AND** it SHALL omit the superseded old record from default context
- **AND** `pinax memory list --include-superseded --vault ./my-notes --json` SHALL still be able to show the old record and supersession link.

#### Scenario: Duplicate subject and predicate records are collapsed

- **WHEN** multiple confirmed records share the same normalized subject and predicate
- **THEN** default recall SHALL return the highest-scoring record for that subject and predicate
- **AND** lower-scoring duplicates SHALL remain stored and auditable, not deleted.

### Requirement: Memory recall explanations SHALL expose bounded signal breakdowns

Pinax SHALL expose recall explanations that help agents and users understand why records were selected without leaking private note bodies or provider payloads.

#### Scenario: JSON output includes optional signal breakdown

- **WHEN** the user runs `pinax memory recall "release workflow" --entity pinax --vault ./my-notes --json`
- **THEN** each returned match SHALL keep `score` and `recall_reason`
- **AND** each match MAY include a `signals` object with bounded numeric contributions such as keyword, entity, source, confidence, freshness, lifecycle, and task_fitness
- **AND** adding `signals` SHALL NOT remove or rename existing JSON envelope fields or existing match fields.

#### Scenario: Agent output remains low-token and body-safe

- **WHEN** the user runs `pinax memory context "prepare next release" --entity pinax --limit 12 --vault ./my-notes --agent`
- **THEN** stdout SHALL include stable key=value facts for command, status, scope, match count, memory types, optional top score, and bounded recall reasons
- **AND** stdout SHALL NOT include localized prose, full memory bodies, raw prompts, provider payloads, Authorization headers, cookies, tokens, hidden system prompts, private tool arguments, or complete chain-of-thought.
