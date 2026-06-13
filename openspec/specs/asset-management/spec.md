# asset-management Specification

## Purpose
TBD - created by archiving change pinax-versioned-vault-assets. Update Purpose after archive.
## Requirements
### Requirement: Pinax manages multimedia assets as vault objects
Pinax SHALL manage images, audio, video, documents, and binary files as vault assets with CLI-authored metadata while preserving file portability inside the vault.

Asset manifest SHALL be CLI-authored metadata for vault-relative file paths, content evidence, media facts, link facts, verification state, and repair evidence. The manifest SHALL NOT store asset payload bytes and SHALL NOT replace vault-local files as the portable asset truth source.

#### Scenario: Add an asset to the vault
- **WHEN** a user runs `pinax asset add ./diagram.png --vault ./my-notes --json`
- **THEN** Pinax SHALL copy or register the file inside the vault boundary according to the selected mode
- **AND** stdout SHALL include asset id, vault-relative path, media type, size, sha256, managed status, and version evidence.
- **AND** stdout, stderr, event logs, record logs, and fixtures SHALL NOT include binary payload bytes.

#### Scenario: List and show assets
- **WHEN** a user runs `pinax asset list --vault ./my-notes --json` or `pinax asset show diagram --vault ./my-notes --json`
- **THEN** Pinax SHALL return asset projections from the asset manifest or index fallback
- **AND** each asset SHALL include path, kind, media type, size, sha256, linked note count, and verification status.
- **AND** stdout and stderr SHALL NOT include raw asset payload bytes or optional media provider raw payloads.

#### Scenario: Verify asset integrity
- **WHEN** a user runs `pinax asset verify --vault ./my-notes --json`
- **THEN** Pinax SHALL stream hash asset files without loading whole large files into memory
- **AND** stdout SHALL report verified, missing, changed, unmanaged, and failed counts with evidence paths.

### Requirement: Asset metadata is collected through pure Go core paths
Pinax SHALL collect core asset metadata using Go standard library or maintained Go packages and SHALL NOT require external binaries for core asset management.

#### Scenario: Detect media facts without external tools
- **WHEN** a user adds PNG, JPEG, GIF, WebP, PDF, audio, video, or unknown binary assets
- **THEN** Pinax SHALL always record size, sha256, file extension, and MIME guess
- **AND** it SHALL record image dimensions when a pure Go decoder supports the format
- **AND** it SHALL leave advanced duration, waveform, OCR, transcript, or thumbnail extraction unset unless an explicit optional provider is configured.

#### Scenario: Optional provider failure does not block core asset registration
- **WHEN** an optional media metadata provider is configured but fails
- **THEN** Pinax SHALL still register the asset using core metadata
- **AND** stdout SHALL include a redacted warning fact without leaking provider payloads or command arguments.

### Requirement: Asset links are tracked without rewriting note bodies by default
Pinax SHALL detect and manage relationships between notes and assets while avoiding implicit Markdown rewrites.

#### Scenario: Link asset to a note
- **WHEN** a user runs `pinax asset link diagram --note yeisme --vault ./my-notes --json`
- **THEN** Pinax SHALL resolve both asset and note through the shared resolver
- **AND** it SHALL record the relationship in CLI-authored asset metadata or return a write plan if Markdown body changes are required.

#### Scenario: Remove asset requires a plan
- **WHEN** a user runs `pinax asset remove diagram --vault ./my-notes --json` without `--plan` or `--yes`
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** stdout SHALL include a next action for `pinax asset remove diagram --plan`.

#### Scenario: Asset remove plan reports note references
- **WHEN** a user runs `pinax asset remove diagram --plan --vault ./my-notes --json`
- **THEN** Pinax SHALL report linked notes, raw references, delete risk, version snapshot requirement, and safe next actions without deleting files.

### Requirement: Attachments follow an Obsidian-like vault file model
Pinax SHALL manage note attachments as portable vault files referenced from Markdown, while using index projections for fast lookup, backlinks, orphan detection, and repair planning.

#### Scenario: Attach file with default per-note placement
- **WHEN** a user runs `pinax note attach "认证方案" ./diagram.png --vault ./my-notes --json`
- **THEN** Pinax SHALL copy the file into a vault-relative path shaped like `attachments/<note-id>/diagram.png` unless the vault config chooses another placement policy
- **AND** it SHALL append a Markdown-readable attachment reference to the note body through the application service
- **AND** stdout SHALL include attachment path, reference text, media type, placement policy, index status, and a concrete next action.

#### Scenario: Attach file with note-folder placement
- **WHEN** a user runs `pinax note attach "认证方案" ./diagram.png --placement note-folder --embed --vault ./my-notes --json`
- **THEN** Pinax SHALL place the file under the note directory's `assets/` folder inside the vault boundary
- **AND** it SHALL write an embed-style Markdown reference that remains readable outside Pinax
- **AND** it SHALL NOT move existing note-folder attachments when the note is later moved unless a separate attachment move plan is approved.

#### Scenario: Register existing vault file as attachment
- **WHEN** a user runs `pinax asset add notes/design/assets/diagram.png --as-attachment-for "认证方案" --mode register --vault ./my-notes --json`
- **THEN** Pinax SHALL register the existing vault file as an asset and note attachment without copying bytes
- **AND** it SHALL reject files outside the vault boundary with stable error code `asset_outside_vault`.

#### Scenario: Move source file requires approval
- **WHEN** a user runs `pinax note attach "认证方案" ./diagram.png --mode move --vault ./my-notes --json` without approval
- **THEN** Pinax SHALL fail with stable error code `approval_required`
- **AND** it SHALL include a next action such as `pinax note attach "认证方案" ./diagram.png --mode move --yes --vault ./my-notes --json`
- **AND** it SHALL NOT copy, move, delete, or rewrite any file.

### Requirement: Attachment references are parsed into index projections
Pinax SHALL parse Markdown and Obsidian-style attachment references into the existing local index projection and SHALL keep Markdown note links separate from attachment links.

#### Scenario: Parse Markdown attachment references
- **GIVEN** a note contains `![Diagram](../assets/diagram.png)` and `[Spec](attachments/spec.pdf)`
- **WHEN** the user runs `pinax index rebuild --vault ./my-notes --json`
- **THEN** Pinax SHALL write asset link projection rows for both attachment references
- **AND** each row SHALL preserve source note path, raw reference, resolved asset path when available, link style, media type, line number when available, and status.

#### Scenario: Parse Obsidian wiki embeds
- **GIVEN** a note contains `![[diagram.png]]`, `![[media/demo.mp4|demo]]`, or `[[spec.pdf]]`
- **WHEN** the user runs `pinax index refresh --vault ./my-notes --json`
- **THEN** Pinax SHALL treat non-Markdown wiki targets as asset links
- **AND** it SHALL preserve raw target, alias or display hint when present, embed/link kind, and resolution status.

#### Scenario: Markdown note links remain note links
- **GIVEN** a note contains `[[Project Plan]]` and `[Plan](project-plan.md)`
- **WHEN** Pinax parses links for the index
- **THEN** those references SHALL remain note link graph edges
- **AND** they SHALL NOT be duplicated as asset links.

#### Scenario: External and unsafe references are ignored or marked safely
- **GIVEN** a note contains `https://example.com/a.png`, `mailto:user@example.com`, `data:image/png;base64,...`, or `../../secret.png`
- **WHEN** Pinax parses attachment references
- **THEN** Pinax SHALL NOT treat external or unsafe references as vault assets
- **AND** it SHALL NOT expose data URI payloads, external payloads, or files outside the vault.

### Requirement: Attachment queries reuse the local index
Pinax SHALL answer attachment list, backlink, orphan, missing, and search filters from the local index when fresh, with scan fallback only when needed.

#### Scenario: List note attachments from index
- **WHEN** a user runs `pinax note attachments "认证方案" --vault ./my-notes --json`
- **THEN** Pinax SHALL return attachments linked from the resolved note
- **AND** stdout facts SHALL include attachment count, missing count, index status, and engine
- **AND** a fresh index path SHALL NOT rescan every Markdown file in the vault.

#### Scenario: Show asset backlinks
- **WHEN** a user runs `pinax asset backlinks diagram.png --vault ./my-notes --json`
- **THEN** Pinax SHALL resolve the asset through the shared resolver and return notes that reference it
- **AND** stdout SHALL include linked note count, raw references, source paths, line numbers when available, and index status.

#### Scenario: List orphan attachments
- **WHEN** a user runs `pinax asset orphans --vault ./my-notes --json`
- **THEN** Pinax SHALL list vault assets with no note references according to the current index projection
- **AND** it SHALL include safe next actions for review or removal plans without deleting files.

#### Scenario: List missing attachments
- **WHEN** a user runs `pinax asset missing --vault ./my-notes --json`
- **THEN** Pinax SHALL list attachment references whose vault target does not exist
- **AND** it SHALL include next actions for `pinax asset repair --plan --vault ./my-notes --json` or `pinax index refresh --vault ./my-notes --json` when the index is stale.

#### Scenario: Search notes with attachments
- **WHEN** a user runs `pinax search "认证" --has-attachment --vault ./my-notes --json`
- **THEN** Pinax SHALL filter notes using indexed attachment facts when the index is fresh
- **AND** stdout facts SHALL identify whether the attachment filter used index or scan fallback.

### Requirement: Attachment paths support multiple presentation styles
Pinax SHALL keep a stable vault-relative canonical path for assets while allowing users and agents to request different path display styles for reading, scripting, and Markdown insertion.

#### Scenario: Default asset output uses vault-relative path
- **WHEN** a user runs `pinax asset show diagram.png --vault ./my-notes --json`
- **THEN** `data.asset.path` SHALL be the vault-relative canonical path such as `attachments/note_abc/diagram.png`
- **AND** stdout SHALL NOT include an absolute path unless the user explicitly requests it.

#### Scenario: Show note-relative attachment paths
- **WHEN** a user runs `pinax note attachments "认证方案" --path-style note-relative --vault ./my-notes --json`
- **THEN** each attachment SHALL include canonical `path` and requested `display_path`
- **AND** `display_path` SHALL be relative to the resolved note path.

#### Scenario: Absolute path requires explicit request
- **WHEN** a user runs `pinax asset show diagram.png --path-style absolute --vault ./my-notes --json`
- **THEN** Pinax MAY include an absolute display path for the vault-local asset
- **AND** it SHALL verify the target is inside the vault boundary
- **AND** it SHALL NOT include external source absolute paths or unrelated local paths.

#### Scenario: Markdown path style renders a link snippet
- **WHEN** a user runs `pinax asset show diagram.png --path-style markdown --context-note "认证方案" --vault ./my-notes --json`
- **THEN** `display_path` SHALL be a Markdown link or image snippet suitable for the context note
- **AND** image assets SHOULD use `![label](relative/path)` while non-image assets SHOULD use `[label](relative/path)`.

#### Scenario: Note context is required for note-relative styles
- **WHEN** a user runs `pinax asset show diagram.png --path-style note-relative --vault ./my-notes --json` without `--context-note`
- **THEN** Pinax SHALL fail with stable error code `path_context_required`
- **AND** stdout SHALL include a next action such as `pinax asset show diagram.png --path-style note-relative --context-note <note> --vault ./my-notes --json`.

#### Scenario: Wiki path style avoids basename ambiguity
- **GIVEN** two assets have the basename `diagram.png`
- **WHEN** a user runs `pinax asset show diagram.png --path-style wiki --vault ./my-notes --json`
- **THEN** Pinax SHALL use a vault-relative wiki target such as `![[attachments/note_abc/diagram.png]]`
- **AND** it SHALL NOT emit a short ambiguous target such as `![[diagram.png]]`.

### Requirement: Rendered note preview can inline readable attachments
Pinax SHALL let rendered note preview compose a single Markdown reading view from the note body plus selected readable attachments, while keeping binary and visual media as safe placeholders in terminal output.

#### Scenario: Render note with Markdown attachment embedded
- **GIVEN** `认证方案` contains an embed reference such as `![[spec.md]]`
- **WHEN** a user runs `pinax note show "认证方案" --view rendered --embed-attachments markdown --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with `data.body` as a unified rendered Markdown preview
- **AND** the preview SHALL include the source note content and an inline section for `spec.md`
- **AND** `data.embedded_assets` SHALL include the embedded asset path, media type, render mode, byte count, and status.

#### Scenario: Source view does not inline attachments
- **GIVEN** a note contains Markdown or Obsidian attachment embeds
- **WHEN** a user runs `pinax note show "认证方案" --view source --embed-attachments markdown --vault ./my-notes --json`
- **THEN** Pinax SHALL return the source Markdown body unchanged
- **AND** it SHALL NOT inline attachments, execute SQL, refresh index, or write render evidence.

#### Scenario: Preview alias is readonly
- **WHEN** a user runs `pinax note preview "认证方案" --embed-attachments markdown --vault ./my-notes`
- **THEN** Pinax SHALL behave as a readonly rendered preview of the note
- **AND** it SHALL NOT write Markdown, `.pinax` assets, render run receipts, Git state, provider state, or remote services.

#### Scenario: Text attachment can be inlined with bounds
- **GIVEN** a note embeds `![[transcript.txt]]`
- **WHEN** a user runs `pinax note show "认证方案" --view rendered --embed-attachments text --max-embed-bytes 4096 --vault ./my-notes --json`
- **THEN** Pinax SHALL inline at most the configured byte limit from the text attachment
- **AND** it SHALL mark the embedded asset as truncated when the file exceeds the limit.

#### Scenario: Image attachment renders as placeholder in terminal
- **GIVEN** a note embeds `![[diagram.png]]`
- **WHEN** a user runs `pinax note show "认证方案" --view rendered --embed-attachments markdown --vault ./my-notes`
- **THEN** Pinax SHALL NOT emit image bytes, base64, ANSI image protocols, Sixel, or iTerm inline image payloads
- **AND** it SHALL render a Markdown placeholder containing asset path, media type, and a next action such as `pinax asset show diagram.png --vault ./my-notes --json`.

#### Scenario: Embed depth prevents recursive expansion
- **GIVEN** embedded Markdown files contain further embeds
- **WHEN** a user runs `pinax note show "认证方案" --view rendered --embed-attachments markdown --max-embed-depth 1 --vault ./my-notes --json`
- **THEN** Pinax SHALL inline only the first level of readable attachments
- **AND** deeper embeds SHALL be represented as placeholders with a depth warning.

#### Scenario: Embed cycles are stopped safely
- **GIVEN** note A embeds note B and note B embeds note A
- **WHEN** a user runs `pinax note show A --view rendered --embed-attachments markdown --vault ./my-notes --json`
- **THEN** Pinax SHALL stop recursion and include warning code `attachment_embed_cycle`
- **AND** it SHALL NOT fail the whole preview unless no usable body can be rendered.

#### Scenario: Asset preview renders single readable asset
- **WHEN** a user runs `pinax asset preview spec.md --as markdown --context-note "认证方案" --vault ./my-notes --json`
- **THEN** Pinax SHALL return a preview projection for that one asset
- **AND** Markdown or text assets SHALL expose bounded Markdown content
- **AND** binary visual assets SHALL expose metadata placeholder only.

### Requirement: Attachment move and remove are plan-first
Pinax SHALL avoid destructive attachment changes unless the user has reviewed a plan and provided explicit approval.

#### Scenario: Move attachment creates rewrite plan
- **WHEN** a user runs `pinax asset move diagram.png attachments/archive/diagram.png --plan --vault ./my-notes --json`
- **THEN** Pinax SHALL produce a plan that includes file move operation, affected note references, exact raw references to patch, version snapshot requirement, and apply command
- **AND** it SHALL NOT move the file or rewrite Markdown during plan generation.

#### Scenario: Remove shared attachment is blocked by default
- **GIVEN** `diagram.png` is referenced by more than one note
- **WHEN** a user runs `pinax asset remove diagram.png --plan --vault ./my-notes --json`
- **THEN** Pinax SHALL report that the asset is shared
- **AND** the default plan SHALL refuse file deletion unless the user explicitly chooses unlink/review behavior.

#### Scenario: Repair plan does not rewrite Markdown automatically
- **WHEN** a user runs `pinax asset repair --plan --vault ./my-notes --json`
- **THEN** Pinax SHALL report missing targets, orphan assets, ambiguous references, and stale index facts
- **AND** all Markdown rewrite, file move, and file delete operations SHALL remain planned operations until approved with `--yes` and snapshot protection where required.

### Requirement: Asset completion uses indexed attachment candidates
Pinax SHALL provide contextual shell completion for asset commands without falling back to unrelated filesystem candidates.

#### Scenario: Complete asset references
- **WHEN** a user requests shell completion for `pinax asset show --vault ./my-notes <TAB>`
- **THEN** completion SHALL list indexed asset filename/path/stem candidates with descriptions containing media type, linked note count, and missing/orphan status when known
- **AND** completion SHALL return `ShellCompDirectiveNoFileComp` and SHALL NOT hash files, rebuild the index, write assets, or access network.

#### Scenario: Note attach source keeps file completion
- **WHEN** a user requests shell completion for the source file argument of `pinax note attach "认证方案" <TAB>`
- **THEN** shell file completion MAY remain enabled because the source file may live outside the vault
- **AND** Pinax SHALL still validate the source path at execution time before copying or moving it.

