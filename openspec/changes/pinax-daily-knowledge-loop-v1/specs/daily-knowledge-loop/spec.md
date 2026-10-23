## ADDED Requirements

### Requirement: Capture accepts long input without exposing body arguments
The CLI SHALL support inbox capture through stdin, reject conflicting explicit body inputs before mutation, and preserve existing body capture behavior.

#### Scenario: Capture a Chinese Markdown document
- **WHEN** a user submits a Chinese document containing links, code and tables through stdin
- **THEN** the stored note preserves the content and returns an identity with readback evidence and an editor action

#### Scenario: Preview or conflicting input
- **WHEN** capture is a dry run or specifies both stdin and body
- **THEN** no note is written and conflicting sources produce an actionable error

### Requirement: Review presents a bounded read-only intake batch
The application SHALL combine inbox and unclassified notes/index items into a bounded review projection without changing lifecycle state, rebuilding indexes or invoking providers.

#### Scenario: Continue or skip a review batch
- **WHEN** a user reads a default batch and continues after its final stable reference
- **THEN** at most five items are shown per batch and skipped items remain unchanged

#### Scenario: Excluded or duplicate records
- **WHEN** intake contains archived, discarded, system or already classified index notes or repeated object identities
- **THEN** excluded records are not proposed and each object appears at most once

### Requirement: Reading preserves stable identity and Unicode
Search detail and intake review SHALL expose bounded valid UTF-8 snippets and actions that resolve the selected object even when titles collide.

#### Scenario: Same title and long Chinese body
- **WHEN** two notes have the same title and the selected note contains a long Chinese paragraph
- **THEN** the detail card remains valid UTF-8 and its preview and editor actions target the selected identity

### Requirement: Daily validation does not replace historical evidence
Daily workflow tests SHALL use isolated fixtures and retain evidence; real usage observations SHALL remain distinct from continuity dogfood and from implementation completion.

#### Scenario: Local checks pass without real usage
- **WHEN** fixture checks pass but the two-week personal observation is incomplete
- **THEN** implementation is reported separately and real-use acceptance remains pending
