## MODIFIED Requirements

### Requirement: Publish profiles define canonical static publishing policy

Pinax SHALL manage static publishing policy through CLI-authored publish profiles stored as structured vault metadata, and new publishing profiles SHALL use the `pinax-web` renderer for GitHub Pages style HTML output.

#### Scenario: Initialize a canonical publish profile

- **WHEN** a user runs `pinax publish profile init public --target github-pages --renderer pinax-web --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update `.pinax/publish/profiles/public.yaml` through the application service
- **AND** the profile SHALL include `schema_version`, `name`, `target`, `renderer`, selection rules, body policy, asset policy, canonical renderer policy and deploy policy
- **AND** stdout SHALL contain exactly one JSON projection without human prose outside the envelope.

### Requirement: Static HTML builds use the Pinax canonical renderer

Pinax SHALL build GitHub Pages style HTML output by generating a publish-safe projection bundle, invoking the `pinax-web` canonical renderer, then scanning the final output before reporting success.

#### Scenario: Build GitHub Pages output with pinax-web

- **WHEN** a user runs `pinax publish build --profile public --target github-pages --out ./dist/site --vault ./my-notes --json`
- **THEN** Pinax SHALL generate a publish-safe projection bundle containing only selected notes, selected assets, link graph facts, search metadata, taxonomy data, source facts and build metadata
- **AND** it SHALL invoke the canonical static renderer to emit HTML, local assets and `pinax-data/**` files under `--out`
- **AND** it SHALL scan both the projection bundle and final output before returning success.

#### Scenario: Renderer output matches client preview semantics

- **WHEN** a Markdown note is rendered in Electron preview and in static publish output
- **THEN** wikilinks, frontmatter, headings, attachments, managed block placeholders, safe dataview/database-view results, code highlighting and redaction markers SHALL follow the same renderer contract
- **AND** divergence SHALL be caught by renderer fixture tests before release.

### Requirement: Canonical renderer consumes bounded data only

Pinax SHALL expose a stable publish data contract for the renderer and SHALL NOT require the renderer to read the source vault, `.pinax/**`, SQLite, LanceDB, provider config, token files or sync state.

#### Scenario: Generate publish data bundle

- **WHEN** a user builds GitHub Pages output
- **THEN** Pinax SHALL generate a bounded data bundle containing `manifest.json`, `graph.json`, `search-index.json`, `taxonomies.json`, `sources.json` and `build.json`
- **AND** the bundle SHALL NOT include private note bodies, provider raw payloads, Authorization headers, cookies, local absolute paths or `.pinax` internal file contents.

#### Scenario: Renderer contract uses controlled Markdown extensions

- **WHEN** the canonical renderer renders a note
- **THEN** it SHALL support GFM, wikilinks, frontmatter, safe attachments, managed block placeholders and Pinax projection-backed dataview/database view results
- **AND** it SHALL NOT execute MDX, arbitrary scripts, arbitrary imports, user environment reads or network fetches from note content.
