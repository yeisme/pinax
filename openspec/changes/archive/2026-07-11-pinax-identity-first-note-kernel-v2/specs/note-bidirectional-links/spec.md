## ADDED Requirements

### Requirement: Resolved links persist object identity edges
Pinax SHALL project every resolved note relationship as a source object ID and target object ID while preserving the source path, current target path, raw target text, alias, heading and link style as evidence.

#### Scenario: Target note is renamed
- **WHEN** a resolved link target is renamed or moved without changing object identity
- **THEN** the graph SHALL continue to connect the same source and target object IDs, expose the target current path and optionally propose a reviewable Markdown rewrite without marking the relationship broken.

#### Scenario: Raw target is ambiguous
- **WHEN** a raw wikilink or Markdown link matches multiple object IDs
- **THEN** Pinax SHALL preserve the raw evidence, mark the edge ambiguous and SHALL NOT assign a target object ID until the user or Agent approves a resolution plan.

### Requirement: Link repair plans are identity-aware
Link repair and organization plans SHALL compare object identity before proposing rewrites, merges or duplicate handling.

#### Scenario: Same object has stale path text
- **WHEN** link text points to an old path but ledger history proves the target object moved
- **THEN** Pinax SHALL propose a bounded rewrite for that object and SHALL NOT classify it as a newly created replacement note.
