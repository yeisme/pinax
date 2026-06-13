## ADDED Requirements

### Requirement: Index maintains vault object lookup projections
Pinax SHALL extend the local index projection to support note, asset, unmanaged Markdown, and vault file lookup while preserving Markdown and CLI-authored assets as the source of truth.

#### Scenario: Lookup a note or asset by filename
- **WHEN** a user runs `pinax index lookup yeisme --scope all --vault ./my-notes --json`
- **THEN** stdout SHALL include ranked candidates from registered notes, adoptable Markdown files, assets, and vault files
- **AND** each candidate SHALL include object kind, path, managed status, match fields, score, index status, and version evidence when available.

#### Scenario: Ordinary search still excludes unmanaged files by default
- **WHEN** a vault contains unmanaged `yeisme.md` and a user runs `pinax search yeisme --vault ./my-notes --json`
- **THEN** Pinax SHALL exclude unmanaged Markdown from ordinary note search results
- **AND** stdout SHALL include an action recommending `pinax index lookup yeisme --scope all` or `pinax record adopt yeisme --plan` when unmanaged candidates exist.

#### Scenario: Lookup supports asset filters
- **WHEN** a user runs `pinax index lookup diagram --kind asset --media-type image/png --vault ./my-notes --json`
- **THEN** Pinax SHALL return matching asset candidates without reading binary payloads into stdout
- **AND** facts SHALL include asset count, engine, index status, and media filters.

#### Scenario: Rebuild indexes attachment links
- **GIVEN** registered notes reference local attachments through Markdown links or Obsidian wiki embeds
- **WHEN** a user runs `pinax index rebuild --vault ./my-notes --json`
- **THEN** Pinax SHALL rebuild `assets`, `asset_links`, and `vault_files` projections through GORM
- **AND** the projection SHALL preserve source note path, raw reference, resolved asset path, media type, link style, status, and line number when available.

#### Scenario: Attachment relationship commands use fresh index
- **GIVEN** `.pinax/index.sqlite` is fresh
- **WHEN** a user runs `pinax note attachments "认证方案" --vault ./my-notes --json` or `pinax asset backlinks diagram.png --vault ./my-notes --json`
- **THEN** Pinax SHALL answer from indexed attachment projections
- **AND** it SHALL NOT rescan every Markdown file or hash every asset during the query.

#### Scenario: Attachment query falls back safely when index is stale
- **GIVEN** the index is missing or stale
- **WHEN** a user runs `pinax asset orphans --vault ./my-notes --json`
- **THEN** Pinax MAY use a bounded local scan fallback
- **AND** stdout SHALL include `index_status` and a next action for `pinax index refresh --vault ./my-notes --json`.

### Requirement: Version-aware index refresh uses VersionBackend candidates
Pinax SHALL route changed-since and revision-aware index refresh through VersionBackend capabilities instead of shelling out to Git or parsing Git porcelain in command/application layers.

#### Scenario: Refresh changed paths since revision
- **WHEN** a user runs `pinax index refresh --changed-since abc123 --vault ./my-notes --json`
- **THEN** Pinax SHALL ask the active VersionBackend for changed path candidates
- **AND** it SHALL refresh only supported note, asset, and vault file projections for those candidates.

#### Scenario: Changed-since unsupported fails clearly
- **WHEN** a user runs `pinax index refresh --changed-since abc123 --vault ./my-notes --json`
- **AND** the active VersionBackend does not support changed path queries
- **THEN** Pinax SHALL fail with stable error code `version_changed_paths_unavailable`
- **AND** it SHALL not modify index, Markdown, record, asset, provider, or version state.

### Requirement: Shared resolver drives note, record, asset, and version commands
Pinax SHALL provide a shared resolver for vault object references so command behavior is consistent across lookup, note, record, asset, metadata, and version workflows.

#### Scenario: Resolver returns candidates for readonly commands
- **WHEN** a readonly command resolves `yeisme` and multiple candidates match
- **THEN** Pinax SHALL return ranked candidates with object kind, path, match fields, managed status, and next actions
- **AND** it MAY return partial status if the command can still provide useful readonly results.

#### Scenario: Resolver rejects ambiguous write targets
- **WHEN** a writing command resolves `yeisme` and multiple candidates match
- **THEN** Pinax SHALL fail before writing with stable error code `vault_object_ref_ambiguous`
- **AND** no Markdown file, asset file, index row, record event, version snapshot, or provider state SHALL be modified.
