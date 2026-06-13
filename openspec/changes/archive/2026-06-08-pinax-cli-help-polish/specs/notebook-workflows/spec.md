## MODIFIED Requirements

### Requirement: Notebook organization views are discoverable
Pinax SHALL expose local organization dimensions as first-class readable views under `pinax note` while preserving old root dimension commands as compatibility aliases.

#### Scenario: List tags with counts
- **WHEN** a user runs `pinax note tags --vault ./my-notes --json`
- **THEN** Pinax SHALL return tags and note counts from the current vault index or scan fallback
- **AND** stdout SHALL contain no human prose outside the JSON envelope.

#### Scenario: List folders with counts
- **WHEN** a user runs `pinax note folders --vault ./my-notes --json`
- **THEN** Pinax SHALL return vault-relative note folders and counts
- **AND** it SHALL NOT include `.pinax`, `.git`, `dist`, or paths outside the vault.

#### Scenario: List kinds and groups
- **WHEN** a user runs `pinax note kinds --vault ./my-notes --json` or `pinax note groups --vault ./my-notes --json`
- **THEN** Pinax SHALL return kind or group values with counts
- **AND** missing values SHALL be represented with a stable empty bucket fact rather than crashing.

#### Scenario: Root dimension aliases remain compatible
- **WHEN** a user runs `pinax tag list --vault ./my-notes --json`, `pinax folder list --vault ./my-notes --json`, `pinax kind list --vault ./my-notes --json`, or `pinax group list --vault ./my-notes --json`
- **THEN** Pinax SHALL preserve backwards-compatible behavior and machine output fields
- **AND** those root aliases MAY be hidden from primary root help.
