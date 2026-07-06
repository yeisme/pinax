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

For a rebuild-on-change local preview, use watch mode on loopback. `publish serve` and `publish dev` bind to `127.0.0.1` by default and reject public bind addresses such as `0.0.0.0`; use `pinax share` for bounded LAN visibility. In CI, combine `--watch` with `--once` to build, serve, wait for one approved change, rebuild, smoke the preview, and exit:

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


## Document Publish Maintenance

`pinax publish doc` maintains the relationship between local notes and external collaborative document copies. The Markdown vault remains the source of truth; Notion pages and Feishu Docs/files are publish copies. Pinax owns prepare, dry-run, push, status, list, link, unlink, mapping and receipt state. It does not own platform-native comments, annotations, permissions, collaborators, approvals or notification workflows.

Initial targets are `notion-page` and `lark-doc`:

```bash
pinax publish doc provider list --vault ./my-notes --json
pinax publish doc profile set notion-page --workspace <workspace-id> --parent-page <page-id> --vault ./my-notes --json
pinax publish doc profile set lark-doc --folder <folder-token-or-url> --as user --renderer native-docx --layout mirror --template vault --index-page --vault ./my-notes --json
pinax publish doc provider doctor --target lark-doc --vault ./my-notes --json
```

The normal agent-safe flow for a vault is prepare, dry-run, then approved push:

```bash
pinax publish doc prepare --all --target lark-doc --vault ./my-notes --json
pinax publish doc push --all --target lark-doc --vault ./my-notes --dry-run --json
pinax publish doc push --all --target lark-doc --vault ./my-notes --yes --json
pinax publish doc list --target lark-doc --vault ./my-notes --json
```

For a narrow local update, the single-note flow is still available:

```bash
pinax publish doc prepare --note <note-id> --target lark-doc --vault ./my-notes --json
pinax publish doc push --package <package-id> --target lark-doc --vault ./my-notes --dry-run --json
pinax publish doc push --package <package-id> --target lark-doc --vault ./my-notes --json
pinax publish doc status --note <note-id> --vault ./my-notes --json
```

`lark-doc --as user|bot|auto` selects an already configured `lark-cli` identity. Pinax stores only the selector, not Feishu tokens, cookies or raw auth payloads. For user-owned Feishu folders, prefer `--as user`; `provider doctor` verifies that the selected identity is available before publish.

For Feishu, the recommended cloud-vault profile is `--renderer native-docx --layout mirror --template vault --index-page`. `native-docx` is the default renderer for new `lark-doc` profiles: it converts the note Markdown into Feishu native Docs/Docx blocks (headings, paragraphs, lists, tables, code blocks, callouts) instead of uploading a `.md` file, so Feishu renders the content as a readable native document with comments and block-level collaboration. Assets are rendered as native blocks: local images are uploaded as image blocks with auto-detected dimensions (`lark-cli docs +media-insert --caption`); Mermaid diagrams and inline/`.svg`-file SVG content are rendered server-side into native Feishu whiteboards (`lark-cli whiteboard +update --input_format mermaid|svg`); remote image/SVG URLs are downloaded over HTTPS only with private, loopback, link-local, multicast and redirect-to-private targets rejected (10 MB cap), then rendered the same way; non-image file links like `[report](assets/report.pdf)` are uploaded as file blocks with preview cards. If a single asset fails to download or render, the publish still succeeds but records a `mermaid_render_unavailable`, `svg_render_unavailable`, `image_upload_failed`, `attachment_upload_failed` or `remote_image_download_failed` warning for that asset; repeated pushes are idempotent (stale whiteboard/image/figure blocks are cleared before re-insertion). `mirror` creates or reuses remote folders that match the local note path, such as `notes/index/`. The `vault` template adds a compact Pinax overview block before the note body and removes duplicate top-level titles. `--index-page` maintains a native Feishu index document listing every published note with folder, title, status, renderer and link.

`--renderer markdown-file` is a legacy fallback: it keeps the original Drive `.md` file upload behavior (`lark-cli markdown +create/+overwrite`) and produces a `file` object, not a native document. Use it only when the configured `lark-cli` lacks native document support or when you explicitly need the Markdown source file. If `renderer=native-docx` is selected but `lark-cli` does not expose the native document capability, Pinax fails with `provider_capability_missing` and does not silently fall back to Markdown file upload; either upgrade `lark-cli` or switch the profile to `--renderer markdown-file`.

An existing mapping that points to a Drive `file` is not retyped in place when the profile switches to `native-docx`. For a vault migration, detach the old local mappings once and publish the vault again:

```bash
pinax publish doc unlink --all --target lark-doc --vault ./my-notes --json
pinax publish doc prepare --all --target lark-doc --vault ./my-notes --json
pinax publish doc push --all --target lark-doc --vault ./my-notes --yes --json
```

Use `unlink --note <note-id>` only when migrating one document.

**Cross-document references**: when a note references another note via `[[wikilink]]` or `[label](relative/note.md)`, Pinax resolves the target through the same note-link resolver used by local backlinks. During native `lark-doc` prepare, resolved targets that already have a same-target Feishu document URL are rewritten as normal Markdown links, such as `[[Beta]]` -> `[Beta](https://.../docx/...)` and `[text](b.md)` -> `[text](https://.../docx/...)`. `prepare --all` reports `cross_doc_links`, `cross_doc_unpublished`, `cross_doc_ambiguous`, `cross_doc_broken` and `cross_doc_self` facts for the whole vault. Before publishing, inspect wikilink conflicts with `pinax note links --all --kind wiki --status ambiguous --vault ./my-notes --json` and broken wikilinks with `pinax note links --all --kind wiki --status broken --vault ./my-notes --json`. `push --all --yes` uses two passes for native Feishu documents: the first pass creates or updates every document mapping, and the second pass prepares and pushes again so links to documents created in the same run become clickable Feishu document links. Ambiguous, broken, unpublished and self links are preserved in the local form instead of being guessed.

The `status`, `list` and `push` outputs include `renderer`, `external_object_type` (`docx`, `doc` or `file`) and `render_warnings` facts so consumers can tell native documents apart from legacy Markdown files.

For existing external documents, use `link` and `unlink` to maintain local mapping state without mutating the remote document:

```bash
pinax publish doc link --note <note-id> --target lark-doc --external-url <url> --vault ./my-notes --json
pinax publish doc unlink --note <note-id> --target lark-doc --vault ./my-notes --json
```

After `status` returns an external URL or id, agents should call the native provider CLI for platform-specific follow-up. These actions are outside Pinax publish mapping state:

```bash
pinax publish doc status --note <note-id> --vault ./my-notes --json
lark-cli doc comment add --doc <doc-token> --text "Ready for review" --json
```

```bash
pinax publish doc status --note <note-id> --vault ./my-notes --json
notion page comment add --page <page-id> --text "Ready for review" --json
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
