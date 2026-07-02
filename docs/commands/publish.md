# publish

`pinax publish` builds reviewable static delivery surfaces from a local Pinax vault. The vault remains the source of truth. The default design renderer is `pinax-web`: a static HTML renderer for deployable publish output, not an internal workbench page.

## Minimal Flow

```bash
pinax publish profile init public --target github-pages --renderer pinax-web --title "Knowledge" --base-url https://example.github.io/kb/ --vault ./my-notes --json
pinax publish plan --profile public --target github-pages --vault ./my-notes --json
pinax publish build --profile public --target github-pages --out ./dist/site --vault ./my-notes --json
pinax publish serve --profile public --out ./dist/site --host 127.0.0.1 --port 4173 --vault ./my-notes
pinax publish preview approve --profile public --out ./dist/site --vault ./my-notes --json
pinax publish deploy --profile public --target github-pages --out ./dist/site --repo ../kb-pages --yes --vault ./my-notes --json
```

For a rebuild-on-change local preview, use watch mode on loopback. In CI, combine `--watch` with `--once` to build, serve, wait for one approved change, rebuild, smoke the preview, and exit:

```bash
pinax publish dev --profile public --out ./dist/site --host 127.0.0.1 --port 4173 --watch --vault ./my-notes
pinax publish dev --profile public --out ./dist/site --host 127.0.0.1 --port 0 --watch --once --vault ./my-notes --json
```

## Preview Logs And Events

Default preview commands keep the final human summary on stdout and write concise live progress to stderr. This makes long-running local preview steps visible without polluting machine output modes:

```bash
pinax publish build --profile public --target local --out ./dist/site --vault ./my-notes
pinax publish dev --profile public --out ./dist/site --host 127.0.0.1 --port 4173 --watch --vault ./my-notes
```

For automation, use `--events` to receive NDJSON on stdout. Events include stage names such as `profile_ready`, `plan_checked`, `renderer_started`, `scan_completed`, `receipt_written`, `serve_ready`, `smoke_completed`, `watch_started`, `rebuild_completed`, and `preview_approved`:

```bash
pinax publish build --profile public --target local --out ./dist/site --vault ./my-notes --events
pinax publish serve --profile public --out ./dist/site --host 127.0.0.1 --port 0 --once --vault ./my-notes --events
pinax publish dev --profile public --out ./dist/site --host 127.0.0.1 --port 0 --once --vault ./my-notes --events
pinax publish preview approve --profile public --out ./dist/site --vault ./my-notes --events
```

`--json`, `--agent`, and `--explain` remain projection-only modes. They do not include live progress logs on stderr, and stdout remains valid for parsers.

For GitHub Wiki output, use Markdown output and deploy to a separate Wiki repository path:

```bash
pinax publish profile init wiki --target github-wiki --renderer none --vault ./my-notes --json
pinax publish plan --profile wiki --target github-wiki --vault ./my-notes --json
pinax publish build --profile wiki --target github-wiki --out ./dist/wiki --vault ./my-notes --json
pinax publish deploy --profile wiki --target github-wiki --out ./dist/wiki --repo ../kb.wiki --yes --vault ./my-notes --json
```

For a single Markdown bundle shared through GitHub Gist:

```bash
pinax publish profile init gist --target github-gist --renderer none --vault ./my-notes --json
pinax publish build --profile gist --target github-gist --out ./dist/gist --vault ./my-notes --json
pinax publish deploy --profile gist --target github-gist --out ./dist/gist --yes --vault ./my-notes --json
```

For a controlled HTTP delivery surface, send the scanned manifest and Markdown bundle to an HTTPS endpoint. Authentication uses secret references, not literal tokens in profiles or command output:

```bash
pinax publish profile init share --target http --renderer none --vault ./my-notes --json
pinax publish build --profile share --target http --out ./dist/share --vault ./my-notes --json
pinax publish deploy --profile share --target http --out ./dist/share --endpoint https://share.example.test/publish --secret-ref env:PINAX_SHARE_TOKEN --yes --vault ./my-notes --json
```

For external static hosts, Pinax calls the provider CLI against the scanned output directory. Provider credentials stay in those tools, not in Pinax project files:

```bash
pinax publish deploy --profile public --target vercel --out ./dist/site --project my-notes --yes --vault ./my-notes --json
pinax publish deploy --profile public --target cloudflare-pages --out ./dist/site --project my-notes --yes --vault ./my-notes --json
```

## Renderer Contract

`pinax-web` is the canonical static HTML renderer. It emits ordinary publish files:

```text
dist/site/
  index.html
  notes/<slug>/index.html
  tags/<tag>/index.html
  assets/
  pinax-data/
    manifest.json
    graph.json
    search-index.json
```

The renderer consumes publish-safe projection data from Pinax. It does not read the private vault, `.pinax/**`, SQLite, provider config, token files or sync state directly.

## Safety Boundaries

- Do not publish the private vault repository directly. Build into `dist/` and deploy only generated output to a separate Pages/Wiki repository, Gist, HTTP endpoint, or local preview.
- `publish plan` is read-only and reports selected, skipped and blocking items without dumping private note bodies.
- `publish build` scans the publish-safe data bundle and final output for secrets, Authorization/Cookie headers, provider payloads, absolute paths, `.pinax` internals and private-body leaks before writing a success receipt.
- `publish deploy` requires `--yes`, validates the latest receipt and output hash, scans output again, and rejects deploy targets at the vault root or inside `.pinax/**`.
- Gist deploy uses the system `gh` CLI and does not store GitHub credentials. HTTP deploy accepts HTTPS or loopback HTTP endpoints and optional `env:` secret references only.
- Vercel deploy uses the system `vercel` CLI. Cloudflare Pages deploy uses `wrangler pages deploy`. Missing CLIs return stable missing-dependency errors.
- `publish serve` and `publish dev` are loopback preview commands, not required daemons or hosted notebook services. `publish dev --watch` watches only vault Markdown, publish profile YAML files, and renderer source files; it does not watch `.pinax/**` secret/config internals.
- Machine output modes stay protocol-only: `--json`, `--agent`, `--events` and `--explain` do not change business behavior.

## LAN Share Boundary

Use [`share`](./share.md) when the generated site or a bounded read-only vault projection must be visible from another device on the LAN. `publish serve` and `publish dev` remain loopback-only.

## CI Recommendation

A conservative CI job should run `publish plan`, `publish build`, inspect the receipt, and only then `publish deploy --yes` against a clean/orphan publishing repository. GitHub private Pages availability and permissions are controlled by GitHub settings, not by Pinax.

See also [`share`](./share.md), [`api`](./api.md), [`token`](./token.md), [`profile`](./profile.md), and [`sync`](./sync.md) for adjacent integration and distribution boundaries.
