# notebook-index-search Delta

## ADDED Requirements

### Requirement: Search engine selection is explicit and internal by default
Pinax SHALL support explicit search engine selection without requiring external search binaries.

#### Scenario: Native search does not require ripgrep
- **WHEN** a user runs `pinax search "design" --engine native --vault ./my-notes --json`
- **THEN** Pinax SHALL search registered Markdown notes using its built-in native engine
- **AND** stdout facts SHALL include `engine_requested=native` and `engine=native`
- **AND** Pinax SHALL NOT require `rg`, `fzf`, or `bat` to be installed.

#### Scenario: Index search uses SQLite token candidates
- **WHEN** a user runs `pinax search "design" --engine index --vault ./my-notes --json`
- **AND** the SQLite index is fresh
- **THEN** Pinax SHALL use the indexed `search_token_records` projection to select candidate notes
- **AND** it SHALL load note text only for candidate result projection and snippets
- **AND** it SHALL NOT perform a full Markdown body scan or require external search binaries.

#### Scenario: Index-only search fails without fallback writes
- **WHEN** a user runs `pinax search "design" --engine index --vault ./my-notes --json`
- **AND** the index is missing or stale without `--allow-stale`
- **THEN** Pinax SHALL fail or return partial output with a stable index maintenance action
- **AND** it SHALL NOT perform a native fallback silently.

### Requirement: Search lazy-index policy is bounded
Pinax SHALL make search-time index loading explicit and bounded.

#### Scenario: Lazy index off never writes the index
- **WHEN** a user runs `pinax search "design" --lazy-index off --vault ./my-notes --json`
- **AND** the index is missing or stale
- **THEN** Pinax SHALL return native search results or an index-only error according to `--engine`
- **AND** it SHALL NOT create or modify `.pinax/index.sqlite`.

#### Scenario: Auto lazy index defers over-budget refresh
- **WHEN** search detects more changed notes than the lazy refresh budget
- **THEN** Pinax SHALL defer index maintenance, return bounded search output, and include an action for `pinax index refresh --vault <vault> --json`
- **AND** stdout facts SHALL include `lazy_index.deferred=true`.
