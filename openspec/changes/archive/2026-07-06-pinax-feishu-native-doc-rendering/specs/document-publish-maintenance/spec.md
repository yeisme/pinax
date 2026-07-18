# document-publish-maintenance Specification Delta

## ADDED Requirements

### Requirement: Feishu document publish renders native documents by default

Pinax SHALL publish `lark-doc` notes as Feishu native Docs/Docx documents by default, while retaining the existing Markdown Drive file behavior only as an explicit fallback renderer.

#### Scenario: new Feishu profile defaults to native renderer

- **WHEN** a user runs `pinax publish doc profile set lark-doc --folder <folder-token-or-url> --as user --vault ./my-notes --json`
- **THEN** Pinax SHALL store `renderer="native-docx"` in the profile unless the user explicitly provides another supported renderer
- **AND** stdout facts SHALL include `renderer="native-docx"`
- **AND** the profile SHALL NOT store Feishu token values, cookies, Authorization headers or raw provider payloads.

#### Scenario: user explicitly selects legacy Markdown file renderer

- **WHEN** a user runs `pinax publish doc profile set lark-doc --folder <folder-token-or-url> --as user --renderer markdown-file --vault ./my-notes --json`
- **THEN** Pinax SHALL keep the current Drive Markdown file publishing behavior for that profile
- **AND** stdout facts SHALL include `renderer="markdown-file"`
- **AND** Pinax SHALL describe the renderer as a fallback or legacy source-file mode, not as the preferred Feishu reading mode.

#### Scenario: unsupported renderer is rejected

- **WHEN** a user runs `pinax publish doc profile set lark-doc --folder <folder-token-or-url> --renderer html --vault ./my-notes --json`
- **THEN** Pinax SHALL return a failed JSON envelope with a stable validation error code
- **AND** it SHALL NOT write or partially update the profile.

### Requirement: Feishu native renderer converts Markdown to document blocks

Pinax SHALL convert note Markdown into a provider-neutral publish AST and then into Feishu native document content instead of uploading the Markdown source file when `renderer=native-docx`.

#### Scenario: common Markdown structures become native document content

- **WHEN** Pinax prepares a `lark-doc` package for a note containing headings, paragraphs, links, lists, blockquotes, tables and fenced code blocks
- **AND** the profile renderer is `native-docx`
- **THEN** the package SHALL include a native render plan containing ordered document blocks for those structures
- **AND** the package SHALL include `render_revision="pinax.publish.render.v1"`
- **AND** it SHALL NOT depend on uploading the raw `.md` file as the primary publish artifact.

#### Scenario: unsupported Markdown produces render warnings

- **WHEN** the note contains Markdown or HTML that the native renderer cannot represent safely
- **THEN** Pinax SHALL preserve user-readable content where possible through safe fallback blocks
- **AND** it SHALL include stable render warning codes in the package and push output
- **AND** it SHALL NOT silently drop content without a warning.

### Requirement: Mermaid and SVG render as native-readable assets

Pinax SHALL handle Mermaid diagrams and SVG content for native Feishu documents by rendering them into image assets or another Feishu-readable native representation.

#### Scenario: Mermaid block is rendered for native document publish

- **WHEN** a note contains a fenced code block with language `mermaid`
- **AND** the profile renderer is `native-docx`
- **THEN** Pinax SHALL render the diagram into a publish artifact suitable for insertion into a Feishu native document
- **AND** the native render plan SHALL reference that artifact as an image or supported media block
- **AND** if rendering is unavailable, Pinax SHALL emit a `mermaid_render_unavailable` warning and preserve the Mermaid source in a readable fallback block.

#### Scenario: SVG image is converted before native insertion

- **WHEN** a note contains an inline SVG or Markdown image referencing an SVG file
- **AND** the profile renderer is `native-docx`
- **THEN** Pinax SHALL convert the SVG into a safe raster artifact before insertion
- **AND** it SHALL NOT insert raw inline SVG into the Feishu native document body
- **AND** if conversion is unavailable, Pinax SHALL emit a `svg_render_unavailable` warning.

### Requirement: Feishu native provider creates and updates native document objects

Pinax SHALL call `lark-cli` through the provider adapter to create and update Feishu native document objects for `renderer=native-docx`.

#### Scenario: native push creates a native Feishu document

- **WHEN** a user runs `pinax publish doc push --package pubpkg_456 --target lark-doc --vault ./my-notes --json`
- **AND** the package/profile renderer is `native-docx`
- **AND** no active native mapping exists for the note and target
- **THEN** Pinax SHALL call the Feishu native document create operation through the provider adapter
- **AND** the resulting mapping SHALL record `renderer="native-docx"`
- **AND** `external_object.type` SHALL be `docx` or `doc`, not `file`
- **AND** stdout facts SHALL include the renderer and external object type.

#### Scenario: native push does not silently fall back to Markdown file upload

- **WHEN** the configured `lark-cli` does not expose the required native document capability
- **AND** the profile renderer is `native-docx`
- **THEN** Pinax SHALL fail with `provider_capability_missing`
- **AND** it SHALL include a safe next action to upgrade or configure the provider
- **AND** it SHALL NOT call `lark-cli markdown +create` or `lark-cli markdown +overwrite` unless the profile explicitly uses `renderer=markdown-file`.

#### Scenario: existing Markdown file mapping is not retyped in place

- **WHEN** an active mapping points to a Feishu Drive `file` object
- **AND** the user switches the profile renderer to `native-docx`
- **THEN** Pinax SHALL NOT overwrite or reinterpret that `file` mapping as a native document
- **AND** it SHALL require a new native object or an explicit unlink/re-publish flow
- **AND** it SHALL return a clear action such as `pinax publish doc unlink --note <note-id> --target lark-doc --vault ./my-notes --json`.

### Requirement: Feishu index page uses the selected renderer

Pinax SHALL render the Feishu cloud-vault index page using the same renderer class as the profile.

#### Scenario: native renderer creates native index document

- **WHEN** `index_page=true`
- **AND** the profile renderer is `native-docx`
- **AND** a publish push succeeds
- **THEN** Pinax SHALL create or update the index page as a Feishu native document
- **AND** the profile `index_object` SHALL include object type and renderer metadata
- **AND** the index document SHALL list published notes with folder, title, status, renderer and link.

#### Scenario: legacy Markdown index remains fallback only

- **WHEN** the profile renderer is `markdown-file`
- **THEN** Pinax MAY continue maintaining `_Pinax Vault Index.md` as a Drive Markdown file
- **AND** it SHALL NOT describe that output as native Feishu Docs rendering.

### Requirement: Native rendering tests prove object type and render fallbacks

Pinax SHALL test native Feishu rendering with isolated fakes and a real-provider smoke checklist.

#### Scenario: fake lark-cli rejects silent fallback

- **WHEN** document publish e2e tests run for `renderer=native-docx`
- **THEN** the fake `lark-cli` SHALL expose native document create/update/upload/inspect operations
- **AND** the test SHALL fail if Pinax calls `markdown +create` or `markdown +overwrite`
- **AND** tests SHALL assert mapping `external_object.type` is `docx` or `doc`.

#### Scenario: real Feishu smoke verifies native rendering

- **WHEN** a maintainer runs the real Feishu smoke test against an authorized folder
- **THEN** the test note SHALL include at least one Mermaid diagram and one SVG image case
- **AND** verification SHALL record that the remote object type is `docx` or `doc`
- **AND** verification SHALL record whether Mermaid and SVG are visible as images or native-readable blocks in Feishu
- **AND** if API evidence cannot prove visual rendering, the smoke record SHALL require manual document inspection and SHALL NOT substitute Markdown fetch as proof.
