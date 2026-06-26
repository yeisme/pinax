## MODIFIED Requirements

### Requirement: Links and backlinks are inspectable
Pinax SHALL let users inspect note links, backlinks, orphan notes, unresolved references, ambiguous references, and local bidirectional graph facts from local Markdown content.

#### Scenario: Wiki links preserve alias and heading
- **WHEN** a note contains `[[Title|Alias]]`, `[[Title#Heading]]`, or `[[Title#Heading|Alias]]`
- **THEN** `pinax note links <note> --vault ./my-notes --json` SHALL return wiki link edges
- **AND** each edge SHALL preserve raw target, normalized target, alias when present, heading when present, link kind, line number, and resolution status.

#### Scenario: Ambiguous wiki targets are not guessed
- **GIVEN** multiple notes can satisfy the same title, alias, or filename stem
- **WHEN** a note links to that target with `[[Target]]`
- **THEN** Pinax SHALL mark the link edge as `ambiguous`
- **AND** the edge SHALL include candidate paths or note ids without selecting one automatically.

#### Scenario: Non-note wiki embeds do not become broken note links
- **WHEN** a note contains `![[image.png]]` or wiki-style non-Markdown asset references
- **THEN** Pinax SHALL NOT count those references as broken note graph edges
- **AND** asset reference handling MAY report them through asset projections instead.

#### Scenario: Link repair remains reviewable
- **WHEN** `pinax repair plan --vault ./my-notes --json` detects a broken or ambiguous note link
- **THEN** the plan SHALL use `manual_review` operations such as `link_resolution` or `link_rewrite`
- **AND** Pinax SHALL NOT automatically rewrite the Markdown body.
