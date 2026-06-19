## ADDED Requirements

### Requirement: Publish profiles define static publishing policy
Pinax SHALL manage static publishing policy through CLI-authored publish profiles stored as structured vault metadata.

#### Scenario: Initialize a publish profile
- **WHEN** a user runs `pinax publish profile init public --target github-pages --renderer hugo --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update `.pinax/publish/profiles/public.yaml` through the application service
- **AND** the profile SHALL include `schema_version`, `name`, `target`, `renderer`, selection rules, body policy, asset policy and deploy policy
- **AND** stdout SHALL contain exactly one JSON projection without human prose outside the envelope.

#### Scenario: Validate a publish profile
- **WHEN** a user runs `pinax publish profile validate public --vault ./my-notes --json`
- **THEN** Pinax SHALL validate the profile schema, target, renderer, body policy, asset policy and deploy policy
- **AND** it SHALL return stable issue codes for invalid fields
- **AND** it SHALL NOT modify Markdown notes, assets, Git state, provider state or remote services.

#### Scenario: Reject hand-written unsafe profile values
- **WHEN** a profile contains unknown target values, path traversal, absolute output paths inside the vault, disabled safety gates or unsupported secret references
- **THEN** Pinax SHALL reject the profile with a structured error
- **AND** it SHALL include a safe next action to repair or recreate the profile through `pinax publish profile` commands.

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

### Requirement: GitHub Pages builds use Hugo as a bounded renderer
Pinax SHALL build GitHub Pages output by generating bounded Hugo input, invoking the external Hugo CLI, then scanning the final output before reporting success.

#### Scenario: Build GitHub Pages output with Hugo
- **WHEN** a user runs `pinax publish build --profile public --target github-pages --renderer hugo --out ./dist/site --vault ./my-notes --json`
- **THEN** Pinax SHALL generate a Hugo staging tree containing only allowed `content/`, `data/`, `static/` and Hugo config files
- **AND** it SHALL invoke `hugo` with the staging tree as source and `--out` as destination
- **AND** it SHALL scan both staging and final output before returning success.

#### Scenario: Hugo missing is a structured failure
- **WHEN** `hugo` is not available on `PATH`
- **THEN** `publish build --target github-pages --renderer hugo` SHALL fail with error code `hugo_unavailable`
- **AND** it SHALL include a safe install or doctor next action
- **AND** it SHALL NOT create a partial successful publish receipt.

#### Scenario: Hugo output cannot bypass redaction
- **WHEN** a Hugo theme or generated file emits a forbidden field, private note body, token, absolute path or provider payload into the destination directory
- **THEN** Pinax SHALL fail the build with a publish leak violation
- **AND** it SHALL identify the redacted file path and violation class without echoing the sensitive value.

### Requirement: Hugo staging project exposes a stable theme contract
Pinax SHALL generate a complete Hugo staging project with a stable `pinax.publish_theme.v1` contract for themes.

#### Scenario: Generate Hugo staging project structure
- **WHEN** a user builds GitHub Pages output with Hugo
- **THEN** Pinax SHALL generate `hugo.yaml`, `content/`, `data/pinax/`, `static/assets/` and a resolved theme source in the staging directory
- **AND** `data/pinax/` SHALL include publish-safe manifest, graph, search-index, taxonomies, sources and build metadata JSON files
- **AND** the staging tree SHALL NOT include `.pinax/**`, provider raw payloads, private note bodies, local absolute paths or unselected assets.

#### Scenario: Theme contract uses bounded data files
- **WHEN** a Hugo theme renders a Pinax static site
- **THEN** it SHALL rely only on generated frontmatter and `data/pinax/*.json`
- **AND** it SHALL NOT require access to Pinax CLI, SQLite, the source vault, `.pinax` internals, provider credentials or network services.

#### Scenario: Hugo config uses safe defaults
- **WHEN** Pinax writes the Hugo config for Pages output
- **THEN** it SHALL set the theme contract version, normalized base URL, site title and selected theme
- **AND** it SHALL keep raw HTML passthrough disabled by default
- **AND** it SHALL pass only a minimal sanitized environment to the Hugo process.

### Requirement: Built-in encyclopedia theme is local, inspectable and progressively enhanced
Pinax SHALL provide a built-in `pinax-encyclopedia` Hugo theme that works without external network assets and remains useful without JavaScript.

#### Scenario: Built-in theme renders core encyclopedia surfaces
- **WHEN** a user builds GitHub Pages output with the built-in theme
- **THEN** the generated site SHALL include homepage, entry pages, tag/type indexes, source pages, related links, backlinks, graph data, search index and not-found fallback surfaces
- **AND** each page SHALL render from publish-safe theme contract data only.

#### Scenario: Built-in theme has no external runtime dependency
- **WHEN** Pinax materializes the built-in theme into Hugo staging
- **THEN** the theme SHALL use local CSS and JavaScript assets only
- **AND** it SHALL NOT load fonts, analytics, scripts, stylesheets or images from external CDN or remote URLs by default.

#### Scenario: Theme remains usable without JavaScript
- **WHEN** browser JavaScript is disabled or a graph/search script fails
- **THEN** published pages SHALL still provide navigable HTML lists for entries, tags, sources and relationships
- **AND** the failure SHALL NOT hide the primary article content.

### Requirement: Theme customization is explicit and still scanned
Pinax SHALL allow theme customization only through explicit profile configuration and SHALL apply the same safety gates to custom theme output.

#### Scenario: List and eject built-in themes
- **WHEN** a user runs `pinax publish theme list --json` or `pinax publish theme eject pinax-encyclopedia --out ./theme --json`
- **THEN** Pinax SHALL report built-in theme names, contract versions and required layouts
- **AND** eject SHALL copy the inspectable theme files only to the requested safe output directory.

#### Scenario: Use a local custom theme
- **WHEN** a publish profile references a local theme directory
- **THEN** Pinax SHALL validate the path, materialize or reference it inside staging, and run staging plus final output leak scans
- **AND** it SHALL reject theme paths that escape allowed roots, point into private vault metadata, require network fetches or attempt to disable safety gates.

### Requirement: GitHub Wiki builds generate Markdown-only output
Pinax SHALL build GitHub Wiki output as a Markdown-compatible static mirror that does not depend on Hugo or browser runtime code.

#### Scenario: Build GitHub Wiki output
- **WHEN** a user runs `pinax publish build --profile public --target github-wiki --out ./dist/wiki --vault ./my-notes --json`
- **THEN** Pinax SHALL generate `Home.md`, note Markdown pages, index pages, `_Sidebar.md`, optional `_Footer.md`, allowed assets and a publish manifest
- **AND** it SHALL rewrite internal note links to Wiki-compatible page links
- **AND** it SHALL scan the output before returning success.

#### Scenario: Wiki target does not require Hugo
- **WHEN** `hugo` is unavailable and the user builds `--target github-wiki`
- **THEN** Pinax SHALL build the Wiki output without invoking Hugo
- **AND** it SHALL still apply the same profile selection, asset allowlist and leak scanning rules.

### Requirement: Publish artifacts are auditable and reproducible
Pinax SHALL write publish manifests, receipts and evidence that describe what was generated without leaking private source content.

#### Scenario: Build writes manifest and receipt
- **WHEN** `publish build` completes successfully
- **THEN** Pinax SHALL write a publish manifest into the output or requested metadata location
- **AND** it SHALL write a CLI-authored receipt under `.pinax/publish/runs/` unless disabled by an explicit dry-run mode
- **AND** the receipt SHALL include profile name, target, renderer, source vault hash or version evidence, selected counts, skipped counts, violation counts, output hash, started_at, finished_at and redaction summary.

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
- **AND** Hugo, git, scan and diagnostic messages SHALL go to stderr or redacted evidence, not mixed into stdout.

#### Scenario: Agent output contains stable facts
- **WHEN** a user runs `pinax publish plan --profile public --agent`
- **THEN** stdout SHALL include stable machine-readable facts for profile, target, selected_count, skipped_count, blocking_count, manual_review_count and next actions
- **AND** it SHALL NOT include full private note bodies or raw sensitive values.

#### Scenario: Events output is NDJSON lifecycle stream
- **WHEN** a user runs `pinax publish build --profile public --events`
- **THEN** stdout SHALL contain NDJSON lifecycle events for start, plan, staging, render, scan, receipt and complete or error
- **AND** each event SHALL be redacted and SHALL include enough ids to correlate with the final receipt.

### Requirement: Publish tests use isolated fakes and fixtures
Pinax SHALL verify static publishing behavior without depending on real GitHub, real credentials, a user vault or public network access.

#### Scenario: E2E publish tests use fake executables and temporary repositories
- **WHEN** publish e2e tests run
- **THEN** they SHALL use fixture vaults, fake `hugo` executables, fake or temporary git repositories and local output directories
- **AND** they SHALL NOT require real GitHub credentials, real provider tokens, public network access or a user vault.

#### Scenario: Contract tests recursively reject leaks
- **WHEN** publish contract tests inspect stdout, stderr, events, receipts, Hugo staging files, Pages output and Wiki output
- **THEN** they SHALL recursively reject forbidden fields and sentinel values for private bodies, tokens, Authorization headers, cookies, provider payloads, absolute local paths and `.pinax` internals.
