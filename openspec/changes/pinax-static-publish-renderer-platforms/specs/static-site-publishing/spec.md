## ADDED Requirements

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
- **AND** the renderer SHALL NOT read source vault paths, `.pinax/**`, SQLite, LanceDB, provider config, token files or sync state.

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
- **AND** it SHALL NOT expose the private vault root, `.pinax/**`, SQLite, LanceDB, provider config, token files or sync state.

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
