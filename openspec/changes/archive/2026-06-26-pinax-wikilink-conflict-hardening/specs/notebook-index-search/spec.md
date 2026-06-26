## ADDED Requirements

### Requirement: Local index projects bidirectional links consistently
Pinax SHALL keep link projection behavior consistent with the note graph query behavior.

#### Scenario: Index rebuild preserves enhanced wiki link fields
- **WHEN** `pinax index rebuild --vault ./my-notes --json` indexes notes with wiki aliases, headings, broken links, and ambiguous targets
- **THEN** the `links` and `backlinks` query sources SHALL expose the same resolved path, raw target, alias, heading, status, evidence, and line facts as `pinax note links`.

#### Scenario: Incremental refresh reclassifies affected links
- **WHEN** a note title, alias, path, or note id changes
- **THEN** Pinax SHALL reclassify affected outgoing and incoming link projection rows
- **AND** it SHALL turn previously resolved links into `broken` or `ambiguous` when appropriate without rewriting note bodies.

#### Scenario: Fresh index engine is truthful
- **WHEN** the local index is fresh and link graph commands report `facts.engine=index`
- **THEN** their output SHALL come from projection data compatible with the shared link graph rules
- **AND** scan fallback SHALL only be used when the index is missing, stale, or unavailable.
