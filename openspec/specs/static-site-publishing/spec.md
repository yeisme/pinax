# static-site-publishing Specification

## Purpose

定义 Pinax 从本地 Markdown vault 生成可审查静态发布产物的合同。发布面是 delivery artifact，不是真源；默认渲染器为 `pinax-web` static renderer，必须提供稳定 Markdown/AST/HTML 语义供发布输出和未来 Workbench module contracts 复用。
## Requirements
### Requirement: Publish profiles define canonical static publishing policy

Pinax SHALL manage static publishing policy through CLI-authored publish profiles stored as structured vault metadata, and new publishing profiles SHALL use the `pinax-web` renderer for GitHub Pages style HTML output.

#### Scenario: Initialize a canonical publish profile

- **WHEN** a user runs `pinax publish profile init public --target github-pages --renderer pinax-web --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update `.pinax/publish/profiles/public.yaml` through the application service
- **AND** the profile SHALL include `schema_version`, `name`, `target`, `renderer`, selection rules, body policy, asset policy, canonical renderer policy and deploy policy
- **AND** stdout SHALL contain exactly one JSON projection without human prose outside the envelope.

### Requirement: Publish planning is read-only and reviewable

Pinax SHALL provide a read-only publish plan that shows what a static publish operation would include, skip, block or require for manual review.

#### Scenario: Plan a GitHub Pages publish

- **WHEN** a user runs `pinax publish plan --profile public --target github-pages --vault ./my-notes --json`
- **THEN** Pinax SHALL return selected notes, selected assets, skipped items, blocking violations, manual review items, estimated output paths and runnable next actions
- **AND** it SHALL NOT write Markdown files, `.pinax/` receipts, Git state, provider state, output directories or remote services.

#### Scenario: Exclude private and draft notes by default

- **WHEN** a vault contains active public notes, draft notes, private notes, secret notes and notes with `publish: false`
- **THEN** `publish plan` SHALL select only notes allowed by the profile
- **AND** it SHALL skip draft, private, secret and explicitly unpublished notes with stable skip reasons.

#### Scenario: Block unsafe publish candidates

- **WHEN** a selected note or asset contains a forbidden secret pattern, provider raw payload, Authorization header, Cookie, webhook URL, absolute local path or `.pinax/` reference
- **THEN** `publish plan` SHALL mark the item as blocking
- **AND** it SHALL not produce a successful build next action until the blocking issue is resolved.

### Requirement: Static HTML builds use the Pinax canonical renderer

Pinax SHALL build GitHub Pages style HTML output by generating a publish-safe projection bundle, invoking the `pinax-web` canonical renderer, then scanning the final output before reporting success.

#### Scenario: Build GitHub Pages output with pinax-web

- **WHEN** a user runs `pinax publish build --profile public --target github-pages --out ./dist/site --vault ./my-notes --json`
- **THEN** Pinax SHALL generate a publish-safe projection bundle containing only selected notes, selected assets, link graph facts, search metadata, taxonomy data, source facts and build metadata
- **AND** it SHALL invoke the canonical static renderer to emit HTML, local assets and `pinax-data/**` files under `--out`
- **AND** it SHALL scan both the projection bundle and final output before returning success.

#### Scenario: Renderer output matches client preview semantics

- **WHEN** a Markdown note is rendered for future Workbench module contract fixtures and in static publish output
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

### Requirement: GitHub Wiki and bundle targets remain generated artifacts

Pinax SHALL support non-HTML publish targets only as generated delivery artifacts that share the same selection, redaction and receipt rules.

#### Scenario: Build GitHub Wiki output

- **WHEN** a user runs `pinax publish build --profile public --target github-wiki --out ./dist/wiki --vault ./my-notes --json`
- **THEN** Pinax SHALL generate `Home.md`, note Markdown pages, index pages, `_Sidebar.md`, allowed assets and a publish manifest
- **AND** it SHALL rewrite internal note links to Wiki-compatible page links
- **AND** it SHALL scan the output before returning success.

#### Scenario: Markdown sharing bundle is auditable

- **WHEN** `publish build` completes successfully for `github-gist` or `http`
- **THEN** Pinax SHALL write a Markdown bundle, `pinax-publish-manifest.json` and a CLI-authored publish receipt
- **AND** the manifest and receipt SHALL use the same scan, hash, selected-count and redaction summary rules as HTML and Wiki builds.

### Requirement: Publish artifacts are auditable and reproducible

Pinax SHALL write publish manifests, receipts and evidence that describe what was generated without leaking private source content.

#### Scenario: Build writes manifest and receipt

- **WHEN** `publish build` completes successfully
- **THEN** Pinax SHALL write a publish manifest into the output or requested metadata location
- **AND** it SHALL write a CLI-authored receipt under `.pinax/publish/runs/` unless disabled by an explicit dry-run mode
- **AND** the receipt SHALL include profile name, target, renderer, selected counts, skipped counts, violation counts, output hash, started_at, finished_at and redaction summary.

#### Scenario: Manifest excludes private source data

- **WHEN** a manifest is generated
- **THEN** it SHALL include publish-safe ids, slugs, titles, tags, output paths, asset hashes and graph metadata
- **AND** it SHALL NOT include private note bodies, provider raw payloads, Authorization headers, cookies, local absolute paths or `.pinax` internal file contents.

### Requirement: Publish deploy only writes to explicit publish targets

Pinax SHALL deploy static publish output only after explicit confirmation and only to the configured publishing target, never to the private vault source.

#### Scenario: Deploy to GitHub Pages branch

- **WHEN** a user runs `pinax publish deploy --profile public --target github-pages --out ./dist/site --repo <repo> --branch gh-pages --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL verify the output manifest and scan result before deploying
- **AND** it SHALL commit and push only the generated publish output to the target repository branch
- **AND** it SHALL not modify Markdown note source files or private vault metadata except for CLI-authored publish receipts.

#### Scenario: Deploy requires confirmation

- **WHEN** a user runs `pinax publish deploy --profile public --target github-pages --out ./dist/site --repo <repo> --branch gh-pages --vault ./my-notes --json` without `--yes`
- **THEN** Pinax SHALL return `approval_required`
- **AND** it SHALL NOT commit, push, delete or overwrite publish target files.

#### Scenario: Deploy redacts git credential material

- **WHEN** git deploy fails with a remote URL or credential helper message containing token-like material
- **THEN** Pinax SHALL redact credentials from stdout, stderr, events, receipts and evidence
- **AND** it SHALL return a stable external dependency error code.

### Requirement: Publish output follows Pinax machine-output contracts

Pinax SHALL render publish command results through the shared output contract and maintain stdout/stderr separation.

#### Scenario: JSON output is a single envelope

- **WHEN** a user runs any `pinax publish` command with `--json`
- **THEN** stdout SHALL contain exactly one JSON projection envelope
- **AND** renderer, git, scan and diagnostic messages SHALL go to stderr or redacted evidence, not mixed into stdout.

#### Scenario: Agent output contains stable facts

- **WHEN** a user runs `pinax publish plan --profile public --agent`
- **THEN** stdout SHALL include stable machine-readable facts for profile, target, selected_count, skipped_count, blocking_count, manual_review_count and next actions
- **AND** it SHALL NOT include full private note bodies or raw sensitive values.

#### Scenario: Events output is NDJSON lifecycle stream

- **WHEN** a user runs `pinax publish build --profile public --events`
- **THEN** stdout SHALL contain NDJSON lifecycle events for start, plan, projection-bundle, render, scan, receipt and complete or error
- **AND** each event SHALL be redacted and SHALL include enough ids to correlate with the final receipt.

### Requirement: Publish tests use isolated fakes and fixtures

Pinax SHALL verify static publishing behavior without depending on real GitHub, real credentials, a user vault or public network access.

#### Scenario: E2E publish tests use fake executables and temporary repositories

- **WHEN** publish e2e tests run
- **THEN** they SHALL use fixture vaults, fake renderer adapters, fake or temporary git repositories and local output directories
- **AND** they SHALL NOT require real GitHub credentials, real provider tokens, public network access or a user vault.

#### Scenario: Contract tests recursively reject leaks

- **WHEN** publish contract tests inspect stdout, stderr, events, receipts, projection bundles, HTML output, Wiki output and bundle output
- **THEN** they SHALL recursively reject forbidden fields and sentinel values for private bodies, tokens, Authorization headers, cookies, provider payloads, absolute local paths and `.pinax` internals.

### Requirement: Local publish preview is loopback-only

Pinax SHALL provide a local preview command for already-built publish output without turning Pinax into a required daemon.

#### Scenario: Serve one preview smoke request

- **WHEN** a user runs `pinax publish serve --profile public --out ./dist/site --host 127.0.0.1 --port 0 --once --vault ./my-notes --json`
- **THEN** Pinax SHALL serve the output directory on a loopback address, perform one local request, and exit
- **AND** stdout SHALL include a `publish.serve` projection with `served=true`, host, port and URL facts
- **AND** it SHALL NOT expose the private vault root, `.pinax/**`, provider credentials or private note bodies.
