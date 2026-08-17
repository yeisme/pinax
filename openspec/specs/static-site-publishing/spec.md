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

Pinax SHALL expose a stable publish data contract for the renderer and SHALL NOT require the renderer to read the source vault, `.pinax/**`, SQLite, external RAG vector files, provider config, token files or sync state.

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

### Requirement: Publish preview commands expose additive live progress events

Pinax SHALL expose publish preview progress through additive human logs and `--events` NDJSON without changing existing JSON, agent, explain, or summary projection contracts.

#### Scenario: Build emits stage events
- **WHEN** 用户运行 `pinax publish build --profile public --target local --out ./dist/site --vault ./my-notes --events`
- **THEN** stdout SHALL contain NDJSON events with `start`, `plan_checked`, `renderer_started`, `renderer_completed`, `scan_completed`, `receipt_written`, and `end`
- **AND** every event SHALL include `spec_version`, `mode=events`, `command=publish.build`, `type`, `seq`, and `status`
- **AND** events SHALL NOT include vault absolute paths, private note bodies, tokens, Authorization headers, Cookie headers, provider payloads, raw prompts, hidden system prompts, or private tool arguments.

#### Scenario: Dev preview emits serve and watch events
- **WHEN** 用户运行 `pinax publish dev --profile public --out ./dist/site --host 127.0.0.1 --port 0 --once --vault ./my-notes --events`
- **THEN** stdout SHALL contain NDJSON events with `start`, build stage events, `serve_ready`, `smoke_completed`, and `end`
- **AND** the `serve_ready` event SHALL include a loopback `url` fact.

#### Scenario: Human preview logs go to stderr
- **WHEN** 用户运行 `pinax publish build --profile public --target local --out ./dist/site --vault ./my-notes` without a machine-output flag
- **THEN** stdout SHALL remain the normal human summary projection
- **AND** stderr SHALL include concise stage logs such as `plan_checked`, `renderer_started`, `scan_completed`, and `receipt_written`.

#### Scenario: Machine projection modes stay pure
- **WHEN** 用户 runs publish preview commands with `--json`, `--agent`, or `--explain`
- **THEN** stdout SHALL contain only that selected output mode
- **AND** stderr SHALL NOT include live progress logs unless a downstream external command emits redacted diagnostics.

#### Scenario: Preview approval emits approval event
- **WHEN** 用户运行 `pinax publish preview approve --profile public --out ./dist/site --vault ./my-notes --events`
- **THEN** stdout SHALL contain NDJSON events with `start`, `scan_completed`, `preview_approved`, and `end`
- **AND** the approval event SHALL include profile, target, selected count, scan finding count, output hash status, and receipt path without private content.

### Requirement: 本地静态发布构建优先可用

Pinax SHALL support a local-first static publish flow that builds reviewable HTML output before any cloud deploy target is used.

#### Scenario: Build local static output
- **WHEN** 用户运行 `pinax publish build --profile public --target local --out ./dist/site --vault ./my-notes --json`
- **THEN** Pinax SHALL generate `index.html`, note pages, tag pages, copied allowed assets and `pinax-data/**` files under `./dist/site`
- **AND** stdout SHALL contain one JSON projection with profile, target, renderer, selected count, skipped count, output hash and scan facts
- **AND** it SHALL NOT modify Markdown source files, Git state, provider state, sync state or remote services.

#### Scenario: Build refuses blocking publish candidates
- **WHEN** publish planning detects private notes, secret sentinel values, Authorization headers, Cookie headers, provider raw payloads, local absolute paths or `.pinax` internals in selected publish candidates
- **THEN** `publish build` SHALL fail with stable blocking issue facts
- **AND** it SHALL NOT produce a success receipt or deployable output.

### Requirement: Pinax-web renderer consumes publish-safe bundle only

The `pinax-web` renderer SHALL render static HTML from a bounded publish-safe bundle and SHALL NOT read private vault internals directly.

#### Scenario: Renderer input is bounded
- **WHEN** `pinax publish build` invokes the renderer
- **THEN** renderer input SHALL be a publish-safe bundle containing manifest, selected notes, selected assets, graph facts, taxonomies, search metadata, source facts and build metadata
- **AND** the renderer SHALL NOT read source vault paths, `.pinax/**`, SQLite, external RAG vector files, provider config, token files or sync state.

#### Scenario: Renderer supports controlled Markdown semantics
- **WHEN** the renderer processes selected Markdown notes
- **THEN** it SHALL support GFM, frontmatter metadata, wikilinks, safe attachment placeholders, managed block placeholders, projection-backed dataview/database-view results and safe code blocks
- **AND** it SHALL NOT execute MDX components, arbitrary scripts, arbitrary imports, environment reads or network fetches from note content.

#### Scenario: Renderer output is reusable by future Workbench module contracts
- **WHEN** a renderer fixture is rendered for static publish and future Workbench module contract tests
- **THEN** wikilinks, headings, frontmatter-derived metadata, attachments, managed placeholders, dataview/database output and redaction markers SHALL match semantically
- **AND** divergence SHALL fail renderer fixture tests.

### Requirement: Local preview and dev serve are loopback-only

Pinax SHALL provide local preview commands for already-built or just-built publish output without exposing the private vault as a web root.

#### Scenario: Serve built output once
- **WHEN** 用户运行 `pinax publish serve --profile public --out ./dist/site --host 127.0.0.1 --port 0 --once --vault ./my-notes --json`
- **THEN** Pinax SHALL serve the generated output directory on a loopback address, perform one local smoke request and exit
- **AND** stdout SHALL include `served=true`, host, port and URL facts
- **AND** the served filesystem root SHALL be `./dist/site`, not the vault root.

#### Scenario: Dev builds and serves local preview
- **WHEN** 用户运行 `pinax publish dev --profile public --out ./dist/site --host 127.0.0.1 --port 4173 --vault ./my-notes`
- **THEN** Pinax SHALL run the build flow, scan output and serve the generated site on loopback
- **AND** it SHALL NOT deploy to any cloud target.

#### Scenario: User approves preview before deploy
- **WHEN** 用户运行 `pinax publish preview approve --profile public --out ./dist/site --vault ./my-notes --json` after inspecting the local preview
- **THEN** Pinax SHALL verify the output manifest, output hash and scan result
- **AND** it SHALL write a CLI-authored preview receipt that records profile, target, output hash, preview URL or preview source, approved time, selected count, skipped count and blocking count
- **AND** the receipt SHALL NOT include private note bodies, provider raw payloads, tokens, Authorization headers or cookies.

#### Scenario: Preview approval rejects stale output
- **WHEN** the generated output hash differs from the latest build manifest or scan receipt
- **THEN** `publish preview approve` SHALL fail with a stable stale-output error
- **AND** it SHALL include a next action to rebuild and serve locally again.

#### Scenario: Watch mode keeps output bounded
- **WHEN** 用户运行 `pinax publish dev --profile public --watch --out ./dist/site --vault ./my-notes`
- **THEN** Pinax SHALL rebuild on approved vault Markdown, publish profile or renderer source changes
- **AND** it SHALL NOT watch or expose `.pinax/**` secret/config internals, provider credential files or paths outside the configured vault and renderer workspace.

#### Scenario: Watch once is CI-smokable
- **WHEN** 用户运行 `pinax publish dev --profile public --watch --once --out ./dist/site --host 127.0.0.1 --port 0 --vault ./my-notes --json`
- **THEN** Pinax SHALL build and serve on loopback, wait for one approved change, debounce rebuild, smoke the preview, and exit
- **AND** stdout SHALL include `watched=true`, `rebuilds=1`, `served=true`, host, port and URL facts.

### Requirement: LAN share starts Web preview and bounded API explicitly

Pinax SHALL provide an explicit LAN share command for internal read-only viewing, separate from default loopback preview commands.

#### Scenario: Share published site on LAN
- **WHEN** 用户运行 `pinax share start --profile public --out ./dist/site --scope published --host 0.0.0.0 --port 8787 --allow-lan --readonly --vault ./my-notes --json`
- **THEN** Pinax SHALL serve the generated published site and its required bounded API projection on the requested LAN-facing address
- **AND** stdout SHALL contain one JSON projection with web URL, API URL, host, port, scope, readonly mode, auth mode and route exposure facts
- **AND** it SHALL NOT expose the private vault root, `.pinax/**`, SQLite, external RAG vector files, provider config, token files or sync state.

#### Scenario: Share command keeps existing serve defaults compatible
- **WHEN** `pinax share start` is added
- **THEN** existing `pinax api serve`, `pinax publish serve` and `pinax vault dashboard` default loopback behavior SHALL remain compatible
- **AND** scripts that already use those commands SHALL NOT need to pass new flags.

#### Scenario: Non-loopback share requires LAN approval flag
- **WHEN** 用户 runs `pinax share start --host 0.0.0.0 --port 8787 --readonly --vault ./my-notes --json` without `--allow-lan`
- **THEN** Pinax SHALL fail with stable error code `share_allow_lan_required`
- **AND** it SHALL NOT bind a socket or expose Web/API routes.

#### Scenario: LAN share is read-only in the first release
- **WHEN** 用户 runs `pinax share start --host 0.0.0.0 --allow-lan --vault ./my-notes --json` without `--readonly`
- **THEN** Pinax SHALL fail with stable error code `share_readonly_required`
- **AND** it SHALL NOT expose mutation routes, write APIs, provider writes, sync writes or publish deploy actions.

#### Scenario: Vault-readonly scope requires token auth
- **WHEN** 用户运行 `pinax share start --scope vault-readonly --host 0.0.0.0 --port 8787 --allow-lan --readonly --vault ./my-notes --json` without token auth
- **THEN** Pinax SHALL fail with stable error code `share_auth_required`
- **AND** it SHALL recommend a real command such as `pinax token create --label lan-preview --scope read --expires 24h --vault ./my-notes --json`.

#### Scenario: Published scope exposes only publish-selected content
- **WHEN** a LAN viewer opens the Web preview or API under `--scope published`
- **THEN** Pinax SHALL expose only publish-selected notes, allowed assets, public taxonomy, public graph facts and public search metadata
- **AND** it SHALL NOT expose draft, private, secret, unpublished, provider raw payload or unselected note body content.

#### Scenario: Vault-readonly scope exposes bounded read-only projections
- **WHEN** an authenticated LAN viewer uses `--scope vault-readonly`
- **THEN** Pinax SHALL expose a minimal read-only Web shell, `/api/share/status`, and `/api/share/notes`
- **AND** `/api/share/notes` SHALL return metadata-only card projections without full note bodies
- **AND** authenticated mutation methods SHALL return `405`, while unauthenticated requests SHALL return `401`.

### Requirement: GitHub Pages deploy uses generated output only

Pinax SHALL deploy GitHub Pages output only from scanned publish artifacts and only after explicit approval.

#### Scenario: Deploy to GitHub Pages repository
- **WHEN** 用户运行 `pinax publish deploy --profile public --target github-pages --out ./dist/site --repo ../kb-pages --branch gh-pages --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL verify output manifest, output hash, scan receipt and preview approval receipt before committing generated files to the target repository branch
- **AND** it SHALL NOT commit the private vault source or `.pinax/**` internals.

#### Scenario: GitHub Pages deploy requires preview approval
- **WHEN** 用户运行 `pinax publish deploy --profile public --target github-pages --out ./dist/site --repo ../kb-pages --branch gh-pages --yes --vault ./my-notes --json` without a matching preview approval receipt
- **THEN** Pinax SHALL return `preview_required`
- **AND** it SHALL NOT commit, push, delete or overwrite target files.

#### Scenario: GitHub Pages deploy requires approval
- **WHEN** 用户运行 `pinax publish deploy --profile public --target github-pages --out ./dist/site --repo ../kb-pages --branch gh-pages --vault ./my-notes --json` without `--yes`
- **THEN** Pinax SHALL return `approval_required`
- **AND** it SHALL NOT commit, push, delete or overwrite target files.

### Requirement: Vercel deploy uses external CLI credentials boundary

Pinax SHALL support Vercel deployment by calling the system `vercel` CLI against scanned static output, while keeping credential persistence outside Pinax project files.

#### Scenario: Deploy to Vercel
- **WHEN** 用户运行 `pinax publish deploy --profile public --target vercel --out ./dist/site --project my-notes --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL verify output manifest, output hash, scan receipt and preview approval receipt before invoking the system `vercel` CLI
- **AND** stdout, stderr, events and receipts SHALL NOT include raw Vercel tokens, Authorization headers, cookies or credential helper payloads.

#### Scenario: Vercel CLI missing is actionable
- **WHEN** Vercel deployment is requested but the `vercel` executable is unavailable
- **THEN** Pinax SHALL return a stable missing dependency error with a real next action
- **AND** it SHALL NOT mark local build or local serve as unavailable.

### Requirement: Cloudflare Pages deploy uses external Wrangler boundary

Pinax SHALL support Cloudflare Pages deployment by calling the system `wrangler pages deploy` command against scanned static output.

#### Scenario: Deploy to Cloudflare Pages
- **WHEN** 用户运行 `pinax publish deploy --profile public --target cloudflare-pages --out ./dist/site --project my-notes --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL verify output manifest, output hash, scan receipt and preview approval receipt before invoking `wrangler pages deploy`
- **AND** stdout, stderr, events and receipts SHALL NOT include raw Cloudflare tokens, Authorization headers, cookies or provider raw payloads.

#### Scenario: Wrangler missing is actionable
- **WHEN** Cloudflare Pages deployment is requested but the `wrangler` executable is unavailable
- **THEN** Pinax SHALL return a stable missing dependency error with a real next action
- **AND** it SHALL NOT block GitHub Pages, Vercel, local build or local serve diagnostics.

### Requirement: Publish integration tests preserve redacted evidence

Pinax SHALL write redacted per-run evidence for publish integration, component and e2e tests.

#### Scenario: Publish smoke test writes evidence
- **WHEN** a publish integration test runs `profile init -> plan -> build -> serve --once`
- **THEN** it SHALL write evidence under `temp/integration-test-runs/<run-id>/`
- **AND** evidence SHALL include `summary.json`, `command.txt`, `stdout.log`, `stderr.log`, `env.json` and `artifacts/`
- **AND** failed tests SHALL preserve evidence and exit with the original failure code.

#### Scenario: Share smoke test writes evidence
- **WHEN** a share integration test runs published and vault-readonly LAN smoke flows
- **THEN** it SHALL write evidence under `temp/integration-test-runs/<run-id>/`
- **AND** summary checks SHALL include a share LAN read-only marker
- **AND** the test SHALL use loopback or fake LAN binding, not public network dependencies.

#### Scenario: Evidence is redacted
- **WHEN** publish evidence contains stdout, stderr, events, receipts, manifests, HTML output or deploy logs
- **THEN** Pinax SHALL redact tokens, Authorization headers, cookies, provider raw payloads, raw prompts, hidden system prompts, private tool arguments and full chain-of-thought
- **AND** recursive contract tests SHALL reject known sentinel leaks.
