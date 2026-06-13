## MODIFIED Requirements

### Requirement: Links and backlinks are inspectable
Pinax SHALL let users inspect note links, backlinks, orphan notes, unresolved references, ambiguous references, and local bidirectional graph facts from local Markdown content.

#### Scenario: Show note outgoing links
- **WHEN** a user runs `pinax note links note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL return wiki links and Markdown note links found in the note body
- **AND** each link SHALL include source path, target text, link kind, resolved target path when available, broken status, ambiguous status, alias when available, heading when available, and line number when available.

#### Scenario: Show note backlinks
- **WHEN** a user runs `pinax note backlinks note_123 --vault ./my-notes --json`
- **THEN** Pinax SHALL return notes that link to the target note by note id, vault-relative path, exact title, unique case-insensitive title, or wiki reference
- **AND** it SHALL include stable facts for backlink count, resolved count, broken count, ambiguous count, and unresolved count.

#### Scenario: Show ambiguous backlink candidates
- **WHEN** a target reference could match multiple notes
- **AND** a user runs `pinax note backlinks <target> --vault ./my-notes --json`
- **THEN** Pinax SHALL fail or return partial graph facts with stable error code `note_ref_ambiguous` or `link_target_ambiguous`
- **AND** the projection SHALL include candidate paths or note ids without selecting one automatically.

#### Scenario: List orphan notes
- **WHEN** a user runs `pinax note orphans --vault ./my-notes --json`
- **THEN** Pinax SHALL list notes with no incoming and no outgoing note links by default
- **AND** system index notes SHALL NOT be counted as ordinary orphans.

#### Scenario: Classify partial orphans
- **WHEN** a user runs `pinax note orphans --mode no-incoming --vault ./my-notes --json` or `pinax note orphans --mode no-outgoing --vault ./my-notes --json`
- **THEN** Pinax SHALL list notes matching the selected orphan class
- **AND** stdout facts SHALL include the selected mode and returned count.
