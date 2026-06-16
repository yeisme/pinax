# Pinax

[中文说明](./README.zh-CN.md)

Pinax is a local-first Markdown notes CLI for people and agents who want a portable knowledge base instead of another hosted note silo. Your Markdown vault stays the source of truth; `.pinax/` stores CLI-authored config, indexes, receipts, events, and audit projections that can be rebuilt or reviewed.

Pinax focuses on safe local workflows: capture notes, index and search them, inspect links and backlinks, plan repairs and organization, snapshot before risky writes, expose bounded JSON/agent output, and sync encrypted revisions through explicit Cloud Sync transports.

## Five core workflows

Pinax is built around one agent-safe proof loop. A user or agent drives a real Markdown vault through five stages, and every stage stays bounded — projections never dump full note bodies, and writes only happen through plan, snapshot, receipt and explicit apply.

| Path | What it does | Entry commands |
| --- | --- | --- |
| **Capture** | Add notes, inbox items and journal entries to the vault. | `pinax init`, `pinax note add`, `pinax inbox capture`, `pinax journal daily append` |
| **Retrieve** | Build the index projection and read bounded context. | `pinax index sync`, `pinax search`, `pinax note links`, `pinax note backlinks`, `pinax note orphans` |
| **Diagnose** | Check vault health and surface low-risk and review items. | `pinax vault doctor`, `pinax vault stats` |
| **Plan** | Turn issues into reviewable, savable repair and organize plans. | `pinax repair plan --save`, `pinax organize plan --save` |
| **Apply safely** | Snapshot first, then apply low-risk changes with explicit confirmation. | `pinax version snapshot`, `pinax repair apply --yes`, `pinax organize apply --yes` |

Agents can run the whole loop in one command. Preview is read-only; add `--apply --yes` to take a fresh snapshot and apply approved operations:

```bash
pinax proof loop run --vault ./my-notes --json            # preview: one projection with proof_loop_run_id
pinax proof loop run --vault ./my-notes --apply --yes     # fresh snapshot + approved repair/organize apply
```

If an apply goes wrong, revert a file from the last snapshot through a CLI-authored restore path (never direct file surgery):

```bash
pinax version restore notes/example.md --revision HEAD --plan --vault ./my-notes
pinax version restore apply --vault ./my-notes --plan restore-<id> --yes   # local_write=true, remote_write=false
```

```bash
pinax init ./my-notes --title "My Knowledge Base"
pinax inbox capture "an idea" --vault ./my-notes
pinax note add "Research Log" --body "First note" --vault ./my-notes
pinax index sync --vault ./my-notes --json
pinax search "First note" --vault ./my-notes --json
pinax vault doctor --vault ./my-notes --json
pinax repair plan --vault ./my-notes --save --json
pinax version snapshot --vault ./my-notes --message "checkpoint"
pinax repair apply --vault ./my-notes --plan repair-abc123 --yes
```

Every command supports `--json`, `--agent`, `--events` and `--explain` output modes that share one projection boundary: bounded facts and next actions, never raw note bodies, tokens, or provider payloads. Cloud Sync, daily briefing, provider expansion and hosted platform capabilities are separate advanced workflows, not part of this local proof loop.

## Status

| Area | Status |
| --- | --- |
| Local Markdown vault, notes, journals, inbox/drafts, templates, search, links/backlinks, assets, project boards, repair/organize plans | Supported |
| CLI output modes: default summary, `--agent`, `--json`, `--events`, `--explain` | Supported |
| Local dashboard, read-only MCP, localhost REST/RPC adapter | Supported |
| Cloud Sync over server, file/S3-compatible object store, and rclone transports | Preview |
| Provider automation and briefing delivery | Experimental |

## Installation

Prerequisites:

- Go 1.26.1 or newer.
- Optional: [Task](https://taskfile.dev/) for `task check` and local development shortcuts.

Install from source:

```bash
go install github.com/yeisme/pinax/cmd/pinax@latest
```

For local development from a checkout:

```bash
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
./dist/pinax version
```

For release rehearsal from a checkout:

```bash
task release:check
task release:local
```

`task release:local` builds linux, macOS and Windows archives for amd64 and arm64, plus `dist/checksums.txt`, without publishing.

## Quick start

```bash
pinax init ./my-notes --title "My Knowledge Base"
pinax vault validate --vault ./my-notes --json
pinax note add "Research Log" --body "First note" --tags research --vault ./my-notes
pinax index refresh --vault ./my-notes --json
pinax search "First note" --vault ./my-notes --json
```

See the [command map](./docs/commands/README.md) for the recommended entry point for each workflow.

## Local vault workflow

Initialize a Markdown vault:

```bash
pinax init
pinax init ./my-notes --title "My Knowledge Base"
pinax vault validate --vault ./my-notes --json
```

When `pinax init` has no arguments, it initializes the current directory; you can also specify the vault path with `--vault <path>` or a positional argument.

Register a vault once to make `--vault <TAB>` and default vault selection useful across note commands:

```bash
pinax vault register ./my-notes --name work --default
pinax vault list --json
pinax note list
pinax note list --vault work
pinax vault remote refresh --profile cloud-work --json
pinax vault remote list --profile cloud-work --json
```

Shell completion reads only local registry/cache files: it completes registered local aliases and cached remote selectors without contacting remotes, resolving secrets, or writing state.
Regular commands support default English summaries, `--agent`, `--json`, `--events`, and `--explain` output modes; only one of these modes can be selected at a time.

The new primary path should prefer `pinax vault stats|validate|doctor|dashboard`, `pinax journal daily|weekly|monthly`, and `pinax storage set local|s3`; old root aliases remain compatible with existing scripts.

View vault statistics, health issues, and the read-only local dashboard:

```bash
pinax vault stats --vault ./my-notes
pinax vault stats --vault ./my-notes --json
pinax vault doctor --vault ./my-notes --stale-after 90d --agent
pinax vault dashboard --vault ./my-notes --port 0
```

`stats` and `doctor` are read-only by default and do not modify Markdown, `.pinax/`, Git, or remote services; `dashboard` binds only to localhost and reuses the same set of application service projections.

Convert health issues into reviewable, savable, snapshot-protected maintenance actions:

```bash
pinax repair plan --vault ./my-notes --json
pinax repair plan --vault ./my-notes --save --json
pinax version snapshot --vault ./my-notes --message "snapshot before repair"
pinax repair apply --vault ./my-notes --plan repair-abc123 --yes
pinax repair apply --vault ./my-notes --plan repair-abc123 --yes --snapshot-message "snapshot before repair"
```

`repair plan` is read-only by default; only `--save` writes `.pinax/repair-plans/<plan_id>.json` through the service. `repair apply` only performs low-risk metadata, tags, index rebuild, and archive status fixes. Duplicate titles, broken links, ambiguous links, empty notes, and orphan notes only generate manual review; it does not automatically delete, merge, or rewrite body text. Dashboard `/api/repair-plans` and `/api/graph-summary` provide read-only display of saved plans, relationship health summaries, and CLI apply/rebuild commands.

Complete note metadata:

```bash
pinax metadata plan --vault ./my-notes --json
pinax metadata apply --vault ./my-notes --yes
```

Manage multiple projects inside one vault:

```bash
pinax project create research --name "Research" --notes-prefix notes/research --vault ./my-notes
pinax project list --vault ./my-notes --json
pinax project switch research --vault ./my-notes
```

View and maintain the local project board workspace. The board comes from local Markdown, project metadata, SQLite/GORM projections, and saved planning snapshots; it is not a remote Todo provider, and it does not treat TaskBridge as the source of truth:

```bash
pinax project board show research --vault ./my-notes --json
pinax project board show research --note-display card --vault ./my-notes
pinax project board configure research --columns inbox,next,doing,blocked,review,done --vault ./my-notes --json
pinax project board plan research --save --vault ./my-notes --json
pinax project board export research --format markdown --vault ./my-notes --json
pinax project item add research "Implement local board" --column next --body "Controlled work item" --vault ./my-notes --json
pinax project item move research/Implement local board.md doing --vault ./my-notes --json
pinax version snapshot --vault ./my-notes --message "snapshot before project item archive"
pinax project item archive research/Implement local board.md --yes --vault ./my-notes --json
```

`project board show` and `export` are read-only by default and do not write `.pinax/`, Markdown, Git, TaskBridge, or remote providers. `project board plan --save` only writes `.pinax/planning/project-boards/<snapshot_id>.json` as review evidence; `plan weekly --taskbridge --dry-run` reads the next/doing/blocked counts from the latest board snapshot, but does not automatically write board items into TaskBridge. `project item archive` requires `--yes` and an explicit version snapshot.

Manage daily Markdown notes:

```bash
pinax note add "Research Log" --body "Today's observations" --tags research --status active --dir work --vault ./my-notes
pinax note add "Meeting Notes" --stdin --vault ./my-notes
pinax note list --tag research --status active --recent --limit 20 --vault ./my-notes
pinax note read "Research Log" --vault ./my-notes --json
pinax note read "Research Log" --display card --vault ./my-notes --json
pinax note read "Research Log" --display body --vault ./my-notes --json
pinax note edit "Research Log" --editor "$EDITOR" --vault ./my-notes
pinax note rename "Research Log" "Pinax Research Log" --vault ./my-notes
pinax note move "Pinax Research Log" archive --vault ./my-notes
pinax note archive "Pinax Research Log" --vault ./my-notes
pinax note tag add "Pinax Research Log" important --vault ./my-notes
pinax note delete "Pinax Research Log" --yes --vault ./my-notes
```

`note add` is the recommended entry point for adding notes, while `note new` and `note create` remain compatible aliases. By default, `note list/search/show`, the relationship graph, and the index only process Pinax notes with `schema_version: pinax.note.v1`; ordinary Markdown files in the vault do not automatically become notes just because their extension is `.md`. Use `pinax import markdown ... --yes` to batch adopt external Markdown.

User-visible note paths use vault-relative canonical paths: by default, ordinary notes are output as root-level `foo.md`; after using `--dir work` or move, they are output as `work/foo.md`. The resolver remains compatible with historical `notes/foo.md`, stems, note IDs, and unique title input, but the primary output of CLI/JSON/agent/search/record/MCP consistently uses canonical paths.

`note show/read/edit/rename/move/archive/delete/tag` all support note IDs, paths inside the vault, or unique titles; when a title has multiple candidates, `note_ref_ambiguous` is returned to avoid accidental edits. `note edit/open/new --open` supports editors with arguments, such as `--editor "code --wait"`; `note list --recent` means sorting by update time, not implicitly filtering old notes; `note delete` moves to `.pinax/trash/YYYYMMDD/` by default and generates a unique target on same-name conflicts. Real deletion requires passing both `--hard --yes`.

`note read/show --display card|detail|context|body` uses the shared `NoteDisplay` projection. `card/detail/context` do not output the full body and are suitable for agents, dashboards, MCP, and project boards; only `--display body` puts the body into the local JSON projection.

Use notebook core workflows to capture, index, browse, and search:

```bash
pinax journal daily open --vault ./my-notes --editor "$EDITOR"
pinax journal daily append --body "Today's review" --vault ./my-notes
pinax inbox capture "Temporary idea" --body "Put it in inbox first" --tags idea --vault ./my-notes
pinax inbox triage "Temporary idea" --group work --folder ideas --kind reference --status active --vault ./my-notes

pinax index --vault ./my-notes
pinax index refresh --vault ./my-notes --json
pinax index doctor --vault ./my-notes --agent
pinax index rebuild --vault ./my-notes --json
pinax search "authentication" --tag auth --group work --folder architecture --kind reference --status active --vault ./my-notes --json

pinax note tags --vault ./my-notes --json
pinax note folders --vault ./my-notes --json
pinax note kinds --vault ./my-notes --json
pinax note groups --vault ./my-notes --json
pinax view save active-work --group work --status active --kind reference --sort updated --vault ./my-notes --json
pinax view show active-work --vault ./my-notes --json
```

Check local Markdown relationships and attachments:

```bash
pinax note links "Authentication Plan" --vault ./my-notes --json
pinax note links "Authentication Plan" --broken-only --vault ./my-notes --json
pinax note backlinks "Authentication Plan" --include-broken --vault ./my-notes --json
pinax note orphans --mode full --vault ./my-notes --json
pinax search "authentication" --link-target notes/design/auth.md --vault ./my-notes --json
pinax note attach "Authentication Plan" ./diagram.png --vault ./my-notes --json
pinax note attachments "Authentication Plan" --vault ./my-notes --json
```

`pinax note links` parses `[[Title]]`, `[[Title|Alias]]`, `[[Title#Heading]]`, and Markdown relative links; external URLs, `mailto:`, plain headings, and non-Markdown attachments are not treated as note edges. Relationship statuses include `resolved`, `broken`, `ambiguous`, `external`, and `ignored`: broken links mean the target does not exist, and ambiguous means multiple title or alias candidates; Pinax does not guess for the user. `pinax note orphans` modes `--mode full|no-incoming|no-outgoing` respectively view completely isolated notes, notes with no incoming links, and notes with no outgoing links; `search --link-target` supports filtering by resolved note ID/path/title or unresolved raw target, and returns `link_target_ambiguous` when ambiguous.

The SQLite/GORM index is a rebuildable projection, not the note source of truth; the Markdown vault remains the source of truth. `pinax index --vault ./my-notes` summarizes index state and recommends next steps; when missing/stale, prefer running the low-cost `pinax index refresh --vault ./my-notes`; for structural exceptions or corrupt projections, first use `pinax index doctor --vault ./my-notes` to view issues, then follow prompts to run `repair --dry-run` or an explicit `rebuild`. `pinax note links`, `pinax note backlinks`, `pinax note orphans`, and `search --link-target` prefer a fresh index; when the index is unavailable, they fall back to scanning Markdown and provide executable actions.

`note attach` copies files into `attachments/` inside the vault and appends a Markdown reference; if the source file is missing, it returns `attachment_source_missing` and does not modify the note body or attachment directory.

Import and export local Markdown bundles:

```bash
pinax import markdown ./source --group research --tags imported --dry-run --vault ./my-notes --json
pinax import markdown ./source --group research --kind reference --status active --conflict rename --yes --vault ./my-notes --json
pinax import markdown ./source/beta.md --group research --conflict overwrite --yes --vault ./my-notes --json
pinax export markdown ./out --tag imported --vault ./my-notes --json
```

`import markdown --dry-run` does not write notes, receipts, Git, or provider state; apply writes `.pinax/receipts/import-*.json` through the service. `export markdown` exports Markdown and referenced attachments according to note filters, and writes `.pinax/receipts/export-*.json`.


Manage Markdown templates and generate notes from templates:

```bash
pinax template init --vault ./my-notes
pinax template list --pack starter --vault ./my-notes --json
pinax template recommend --intent "meeting sync" --vault ./my-notes --json
pinax journal daily show --date 2026-06-08 --template journal.daily --vault ./my-notes --json
pinax index page create home --template index.home --vault ./my-notes --json
pinax note add "Client Meeting" --template meeting.notes --tags meeting,client --vault ./my-notes --json
pinax template create "Video Learning" --vault ./my-notes
pinax template create meeting --body "# {{title}} - {{client}}" --vault ./my-notes
pinax template create weekly --engine go-template --body "# {{ .Title }}
{{ .Vars.client }}" --vault ./my-notes
pinax template inspect weekly --vault ./my-notes --json
pinax template preview weekly --title "Client Meeting" --var client=Acme --vault ./my-notes --agent
pinax template render weekly --title "Client Meeting" --var client=Acme --save-run weekly-demo --vault ./my-notes --json
pinax template render weekly --run weekly-demo --vault ./my-notes --json
pinax template inspect weekly --runs --vault ./my-notes --json
pinax note new "Client Meeting" --template weekly --var client=Acme --tags meeting,client --vault ./my-notes
pinax template runs prune weekly --keep 20 --dry-run --vault ./my-notes --json
pinax template runs repair --vault ./my-notes --json
pinax template delete weekly --vault ./my-notes --yes
```

Templates are stored in `.pinax/templates/*.md` and are ordinary Markdown text. Legacy templates continue to support simple tokens such as `{{title}}` and `{{client}}`; after declaring `schema_version: pinax.template.v2` and `engine: go-template`, they use Go `text/template` syntax. Use `template inspect` to view variable schemas, query facts, and render runs. Template functions are allowlisted: they do not execute scripts, read environment variables, or access the network.

Built-in templates are divided into journal, index, and note starter packs: `journal.daily|weekly|monthly` create root-level journals, `index.home` and topic index pages only refresh `pinax:managed` managed blocks, and starter templates such as `note.quick`, `inbox.capture`, and `learning.video` are suitable for quick capture. `template list --pack starter` and `template recommend --intent <intent>` are the recommended entry points for choosing templates; JSON/agent output from `template inspect <name>` gives next-step actions, and template names, `--template`, `--var`, and render runs support shell Tab completion.

Query-backed templates use Pinax SQL, not raw SQLite. Templates can declare `language: sql` in `queries` in v2 frontmatter, or use `pinax-sql` fenced blocks in the body; `template inspect` only explains, while `template preview/render` executes bounded queries and puts results into `.Queries`, such as `{{ table .Queries.active }}` or `{{ list .Queries.active "title" }}`.

```bash
pinax query run 'SELECT title, status FROM notes WHERE status = "active" LIMIT 5' --lazy-index --vault ./my-notes --json
pinax template preview project-dashboard --vault ./my-notes --json
```

For formal rendering, use `--save-run <name>` to save a redacted receipt, `rendered.md`, and scope `index.json` in `.pinax/renders/templates/<template>/<run-id>/`; `--run <name-or-id>` reuses historical parameters and re-renders against the current vault. `latest` and aliases in the same scope can be used for shell completion.

Notes also support source/rendered views and controlled write-back:

```bash
pinax note show projects/dashboard.md --view source --vault ./my-notes --json
pinax note show projects/dashboard.md --view rendered --vault ./my-notes --json
pinax note refresh projects/dashboard.md --rendered --save-run dashboard-latest --yes --vault ./my-notes --json
pinax note show projects/dashboard.md --view rendered --snapshot latest --vault ./my-notes --json
pinax note show projects/dashboard.md --runs --vault ./my-notes --json
```

`note show --view rendered` read-only executes restricted `pinax-sql` blocks and does not write Markdown, `.pinax/`, Git, or providers. `note refresh --rendered --yes` only updates managed blocks from `<!-- pinax:render <name> start -->` to `<!-- pinax:render <name> end -->`, preserving source `pinax-sql` blocks and user body text; note-scoped render runs are saved in `.pinax/renders/<note-path-without-notes-prefix-and-ext>/<run-id>/`.

Configure the storage backend for vault artifacts and diagnostics. S3-compatible configuration stores non-secret profile metadata; credentials stay in the provider profile or environment:

```bash
pinax storage set local --root ./my-notes --vault ./my-notes
pinax storage set s3 --bucket notes --region us-east-1 --prefix pinax/ --profile work --vault ./my-notes --json
pinax storage doctor --vault ./my-notes --json
```

Preview and create an explicit version snapshot before organizing structure:

```bash
pinax organize plan --vault ./my-notes --json
pinax version snapshot --vault ./my-notes --message "snapshot before organize"
pinax organize apply --vault ./my-notes --yes
```

You can also provide a snapshot message during apply, so Pinax creates a protective snapshot before applying changes:

```bash
pinax organize apply --vault ./my-notes --yes --snapshot-message "snapshot before organize"
```

Agents can generate reviewable organize plans instead of directly modifying notes:

```bash
pinax organize plan --vault ./my-notes --json
pinax organize plan --vault ./my-notes --save --agent
pinax organize list --vault ./my-notes --json
pinax organize apply --vault ./my-notes --plan organize-abc123 --yes --snapshot-message "snapshot before organize" --json
```

`organize plan --save` generates move, tag_patch, kind_patch, status_patch, link_resolution, link_rewrite, orphan_review, attachment_repair, and manual_review operations; `organize apply --plan` only executes low-risk moves protected by snapshots, leaving other operations for humans or future dedicated apply flows. The old `organize suggest` entry point remains compatible with existing scripts, but is no longer shown as the primary path.

Start the read-only MCP surface:

```bash
pinax mcp serve --vault ./my-notes
```

The local REST/RPC projection adapter binds only to localhost and only reuses application service projections; it is not a public hosted API. The root path returns a small discovery projection, while `/v1/capabilities` lists callable REST/RPC capabilities:

```bash
pinax api routes --vault ./my-notes --json
pinax api schema export --format openapi --vault ./my-notes --json
pinax api serve --readonly --no-auth --port 8787 --vault ./my-notes
curl -s http://127.0.0.1:8787/
curl -s http://127.0.0.1:8787/v1/capabilities
```

REST `GET /v1/projects/{slug}/board`, RPC `Pinax.ProjectBoard.Show`, and the MCP board tool return the same class of bounded board projection; write-like remote calls only return dry-run/plan, `approval_required`, or `snapshot_required` by default. Real Markdown changes still go through explicit CLI commands.

## Cloud Sync preview

Pinax Cloud Sync is separate from `pinax api serve`. The Local API exposes one centralized vault through REST/RPC; Cloud Sync is a distributed protocol where each device keeps its own local vault and exchanges encrypted revisions, manifests, and blobs through a selected transport.

Configure a direct object-store transport and sync two local devices:

```bash
pinax init ./device-a --title "Device A"
pinax init ./device-b --title "Device B"
mkdir -p ./device-a/notes
printf '# Alpha\n\nfrom device A\n' > ./device-a/notes/alpha.md
pinax cloud login --endpoint "file://$PWD/.pinax-cloud-store" --workspace personal --device laptop --secret-ref env://PINAX_SYNC_SECRET --vault ./device-a
pinax cloud login --endpoint "file://$PWD/.pinax-cloud-store" --workspace personal --device desktop --secret-ref env://PINAX_SYNC_SECRET --vault ./device-b
pinax sync push --target cloud --vault ./device-a --yes --json
pinax sync pull --target cloud --vault ./device-b --yes --json
```

S3-compatible storage uses the same Cloud Sync protocol without a Pinax Cloud Server:

```bash
pinax cloud backend set s3 --bucket notes --region us-east-1 --prefix pinax-sync/ --profile work --workspace personal --device laptop --vault ./my-notes
pinax cloud doctor --vault ./my-notes --json
```

Server and rclone backends are explicit transports, not aliases for Local API. Server transport uses `internal/cloudclient.Transport` so Pinax Cloud can own auth/audit/policy, while rclone direct transport uses the shared object-store sync path for providers such as OneDrive. Native Microsoft Graph is a separate future transport.

`remote_write=true` is valid only after the selected transport durably commits a revision and Pinax writes local sync-state evidence. Dry-runs, plans, blob uploads, failed or unsupported transport operations, and pull operations keep `remote_write=false`.

See [cloud command docs](./docs/commands/cloud.md), [sync command docs](./docs/commands/sync.md), and [Cloud Sync architecture](./docs/architecture/cloud-sync-design.md).

MCP tools and resources are read-only, including `pinax.note.links`, `pinax.note.backlinks`, `pinax.note.context`, and `pinax.vault.graph_summary`. They reuse the CLI relationship projection and return bounded facts, candidate summaries, and next-step commands. They do not return full note bodies and do not write Markdown, `.pinax/`, Git, providers, or remote state.

## Local validation

```bash
task build
task test
task check
```

When `task` is not installed, use the equivalent commands:

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```

## Documentation entry points

- [Documentation map](./docs/README.md)
- [Product positioning](./docs/overview/product-positioning.md)
- [Command manual](./docs/commands/README.md)
- [Architecture boundaries](./docs/architecture/architecture-boundaries.md)
- [Local development](./docs/operations/local-development.md)
- [中文 README](./README.zh-CN.md)
- [中文文档地图](./docs/README.zh-CN.md)
- [Contributing](./CONTRIBUTING.md) / [贡献指南](./CONTRIBUTING.zh-CN.md)
- [Security policy](./SECURITY.md) / [安全策略](./SECURITY.zh-CN.md)

## License

No open-source license has been selected yet. Before publishing this repository as open source, add a `LICENSE` file and update this section with the chosen license.
