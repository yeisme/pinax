# Local Development

Common commands:

```bash
task build
task test
task test:integration
task check
```

If `task` is not installed, use Go and OpenSpec commands directly:

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```

Build artifacts are only for local validation and are not committed:

```bash
rm -rf dist
```

Create an OpenSpec change before adding a new implementation:

```bash
openspec new change pinax-<slug>
openspec validate pinax-<slug>
```

## Local Smoke

After building, you can validate the local end-to-end loop with a temporary vault:

```bash
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
rm -rf /tmp/pinax-notes
./dist/pinax init /tmp/pinax-notes --title "My Knowledge Base"
./dist/pinax vault validate --vault /tmp/pinax-notes --json
./dist/pinax vault validate --vault /tmp/pinax-notes --agent
./dist/pinax vault validate --vault /tmp/pinax-notes --events
./dist/pinax vault validate --vault /tmp/pinax-notes --explain
./dist/pinax project create research --name "Research" --notes-prefix notes/research --vault /tmp/pinax-notes --json
./dist/pinax project list --vault /tmp/pinax-notes --agent

./dist/pinax template init --vault /tmp/pinax-notes --json
./dist/pinax template list --pack starter --vault /tmp/pinax-notes --json
./dist/pinax template recommend --intent "meeting sync" --vault /tmp/pinax-notes --json
./dist/pinax journal daily show --date 2026-06-08 --template journal.daily --vault /tmp/pinax-notes --json
./dist/pinax index page create home --template index.home --vault /tmp/pinax-notes --json
./dist/pinax note add "client sync" --template meeting.notes --vault /tmp/pinax-notes --json
./dist/pinax template create "video learning" --vault /tmp/pinax-notes --json
./dist/pinax template create meeting --body "# {{title}} - {{client}}" --vault /tmp/pinax-notes --json
./dist/pinax template validate meeting --vault /tmp/pinax-notes --json
./dist/pinax template render meeting --title "client meeting" --var client=Acme --save-run meeting-demo --vault /tmp/pinax-notes --json
./dist/pinax template render meeting --run meeting-demo --vault /tmp/pinax-notes --json
./dist/pinax note new "client meeting" --template meeting --var client=Acme --tags meeting,client --vault /tmp/pinax-notes --json
./dist/pinax note show "client meeting" --view source --vault /tmp/pinax-notes --json
./dist/pinax note list --tag meeting --vault /tmp/pinax-notes
./dist/pinax note read "client meeting" --vault /tmp/pinax-notes --json
./dist/pinax note read "client meeting" --display card --vault /tmp/pinax-notes --json
./dist/pinax project board show research --note-display card --vault /tmp/pinax-notes --json
./dist/pinax project board configure research --columns inbox,next,doing,blocked,review,done --vault /tmp/pinax-notes --json
./dist/pinax project board plan research --save --vault /tmp/pinax-notes --json
./dist/pinax plan weekly --taskbridge --dry-run --vault /tmp/pinax-notes --json
./dist/pinax project board export research --format markdown --vault /tmp/pinax-notes --json
./dist/pinax project item add research "local board task" --column next --body "controlled work item" --vault /tmp/pinax-notes --json
./dist/pinax storage set s3 --bucket notes --region us-east-1 --prefix pinax/ --profile work --vault /tmp/pinax-notes --json
./dist/pinax storage doctor --vault /tmp/pinax-notes --json
./dist/pinax metadata plan --vault /tmp/pinax-notes --json
./dist/pinax index --vault /tmp/pinax-notes
./dist/pinax index refresh --vault /tmp/pinax-notes --json
./dist/pinax index doctor --vault /tmp/pinax-notes --agent
./dist/pinax index rebuild --vault /tmp/pinax-notes --json
./dist/pinax note links "client meeting" --vault /tmp/pinax-notes --json
./dist/pinax note backlinks "client meeting" --include-broken --vault /tmp/pinax-notes --json
./dist/pinax note orphans --mode full --vault /tmp/pinax-notes --json
./dist/pinax search "client" --link-target "client meeting" --vault /tmp/pinax-notes --json
./dist/pinax repair plan --vault /tmp/pinax-notes --json
./dist/pinax repair plan --vault /tmp/pinax-notes --save --json
./dist/pinax organize plan --vault /tmp/pinax-notes --json
./dist/pinax api routes --vault /tmp/pinax-notes --json
./dist/pinax api schema export --format openapi --vault /tmp/pinax-notes --json
./dist/pinax api serve --readonly --no-auth --port 8787 --vault /tmp/pinax-notes
curl -s http://127.0.0.1:8787/
curl -s http://127.0.0.1:8787/v1/capabilities
./dist/pinax mcp serve --vault /tmp/pinax-notes
```

`pinax init` can initialize the current directory without arguments; positional arguments and `--vault` are used to specify another vault path. The default human output is English; for machine consumption, prefer `--agent`, `--json`, `--events`, or `--explain`.

For the main CLI tree paths, prefer `pinax vault ...`, `pinax journal ...`, and `pinax storage set ...`; old commands are kept as compatibility aliases, and their output projection is equivalent to the main paths.

S3-compatible configuration stores non-secret profile metadata such as bucket, region, prefix, endpoint, and profile name. Access keys and secrets stay in the provider profile or environment, not in Pinax config, receipts, logs, or fixtures.

The `--recent` flag for note commands only means sorting by update time; it does not implicitly filter out old notes. `note edit/open/new --open` can use an editor with arguments, for example `--editor "code --wait"`; deletion enters `.pinax/trash/YYYYMMDD/` by default. If there is a same-name conflict, a suffixed path is generated and existing trash is not overwritten.

Real organization apply must first create a version snapshot, or provide `--snapshot-message` during apply:

```bash
./dist/pinax version snapshot --vault /tmp/pinax-notes --message "snapshot before organization"
./dist/pinax organize apply --vault /tmp/pinax-notes --yes
./dist/pinax repair apply --vault /tmp/pinax-notes --plan repair-abc123 --yes --snapshot-message "snapshot before repair"
```

`pinax note links`, `pinax note backlinks`, `pinax note orphans`, and `search --link-target` preferentially read a fresh SQLite/GORM index. First use `pinax index --vault /tmp/pinax-notes` to view the summary. When missing/stale, prefer running `pinax index refresh --vault /tmp/pinax-notes`. For structural abnormalities, use `pinax index doctor --vault /tmp/pinax-notes` to view issues, and only execute an explicit `rebuild` as prompted when necessary. The index is a rebuildable projection and must not be maintained as the source of truth. Do not handwrite `.pinax/*.json` or index metadata in development or tests.

`repair plan` does not write Markdown or `.pinax/` assets by default; only `--save` creates `.pinax/repair-plans/<plan_id>.json`. Broken links, ambiguous links, body link rewrites, and orphan note organization only generate manual review. `repair apply` must explicitly use `--yes` and be protected by a version snapshot. If the plan expires or notes change, it returns `plan_stale` and requires regenerating the plan.

Project board is a local project workspace. `project board show/export` is read-only; `project board plan --save` only writes planning snapshot evidence; `project item archive` requires `--yes` and a version snapshot. `note read/show --display card|detail|context`, board, dashboard, MCP, REST, and RPC share bounded `NoteDisplay`, and do not output full bodies by default.

The REST/RPC adapter is only used for local projection validation and is not a public Internet service:

```bash
task test:integration
ls temp/integration-test-runs
```

`task test:integration` runs project board e2e and REST/RPC component tests, and writes command/stdout/stderr/env evidence to `temp/integration-test-runs/<run-id>/`.


## Template Workflow

Templates are stored in `.pinax/templates/*.md`. They can be created with the CLI, and their bodies can also be edited with a regular editor. When `pinax template create <name>` has no source parameter, it creates a template design document with `pinax.template_design.v1` YAML frontmatter; creation, deletion, and event recording are handled by the Pinax service:

```bash
pinax template list --pack starter --vault ./my-notes --json
pinax template recommend --intent "meeting sync" --vault ./my-notes --json
pinax journal daily show --date 2026-06-08 --template journal.daily --vault ./my-notes --json
pinax index page create home --template index.home --vault ./my-notes --json
pinax note add "client sync" --template meeting.notes --vault ./my-notes --json
pinax template create "video learning" --vault ./my-notes
pinax template create meeting --from ./meeting.md --vault ./my-notes
pinax template create weekly --engine go-template --body "# {{ .Title }}" --vault ./my-notes
pinax template inspect weekly --vault ./my-notes --json
pinax template preview weekly --title "client meeting" --var client=Acme --vault ./my-notes --agent
pinax template render weekly --title "client meeting" --var client=Acme --save-run weekly-demo --vault ./my-notes --json
pinax template render weekly --run weekly-demo --vault ./my-notes --json
pinax template inspect weekly --runs --vault ./my-notes --json
pinax note new "client meeting" --template weekly --var client=Acme --tags meeting,client --vault ./my-notes
pinax template runs prune weekly --keep 20 --dry-run --vault ./my-notes --json
pinax template runs repair --vault ./my-notes --json
pinax template delete weekly --vault ./my-notes --yes
```

The main path for built-in templates is to first choose a template with `template list --pack starter` or `template recommend --intent <intent>`, then materialize it with `journal daily|weekly|monthly --template`, `index page create|refresh --template`, or `note add --template`. `template inspect <name>` returns the next action in the projection; `template inspect` arguments, each command's `--template` and `--var`, and render runs all support shell Tab completion. Completion only reads template metadata and does not execute queries or write the vault.

`--var key=value` can be used repeatedly. Legacy simple templates continue to use `{{title}}`; v2 templates use Go `text/template` after declaring `pinax.template.v2` and `engine: go-template`. Rendering fails and returns `template_variable_missing` when variables are missing, preventing half-finished notes from being generated.

Query-backed templates only reuse the Pinax SQL query service; they do not execute raw SQLite or dynamic template query functions. `template inspect` only explains; `template preview/render` executes bounded queries and exposes them to the `table`/`list` helpers through `.Queries`. Example queries can first be validated with real commands:

```bash
pinax query explain 'SELECT title, status FROM notes WHERE status = "active" LIMIT 5' --vault ./my-notes --json
pinax query run 'SELECT title, status FROM notes WHERE status = "active" LIMIT 5' --lazy-index --vault ./my-notes --json
pinax template preview project-dashboard --vault ./my-notes --json
```

A render run writes `.pinax/renders/templates/<template>/<run-id>/receipt.json` and `rendered.md`, and maintains a local `index.json` for `--run`, `--snapshot`, `latest`, and Tab completion. Parameters in receipts are redacted, and provider payloads, Authorization headers, cookies, or tokens are not recorded.

Note rendered view and write-back commands:

```bash
pinax note show projects/dashboard.md --view source --vault ./my-notes --json
pinax note show projects/dashboard.md --view rendered --vault ./my-notes --json
pinax note refresh projects/dashboard.md --rendered --save-run dashboard-latest --yes --vault ./my-notes --json
pinax note show projects/dashboard.md --view rendered --snapshot latest --vault ./my-notes --json
pinax note show projects/dashboard.md --runs --vault ./my-notes --json
```

`note show --view rendered` is read-only and does not write Markdown, `.pinax/`, Git, providers, or remotes. `note refresh --rendered --yes` only updates managed blocks from `<!-- pinax:render <name> start -->` to `<!-- pinax:render <name> end -->`; source `pinax-sql` blocks, regular body text, and unknown markers remain unchanged. Note-scoped render runs mirror note paths and are placed under `.pinax/renders/<note-path-without-notes-prefix-and-ext>/<run-id>/`.


## Configuration Layer Smoke

Pinax reads configuration with this precedence: explicit command-line flags > `PINAX_`/standard environment variables > project-level `<vault>/.pinax/config.yaml` > user-level `$XDG_CONFIG_HOME/pinax/config.yaml` or `~/.config/pinax/config.yaml` > built-in defaults. Cobra flag defaults do not override configuration files; only flags explicitly passed by the user participate in the overlay.

Common configuration commands:

```bash
pinax config path --vault ./my-notes
pinax config get output.theme --vault ./my-notes --agent
pinax config doctor --vault ./my-notes --json
pinax config set output.theme high-contrast --scope user
pinax config set remote.api_url http://127.0.0.1:8787 --scope user
pinax config get remote.api_url --agent
pinax config set output.markdown.enabled false --scope project --vault ./my-notes
pinax config unset output.theme --scope project --vault ./my-notes
```

Environment variable examples:

```bash
PINAX_OUTPUT_COLOR=always PINAX_OUTPUT_MARKDOWN_STYLE=dark pinax note show notes/demo.md --vault ./my-notes
NO_COLOR=1 pinax note show notes/demo.md --vault ./my-notes
pinax note show notes/demo.md --vault ./my-notes --color always
PINAX_API_URL=http://127.0.0.1:8787 pinax folder list --json
```

`NO_COLOR` only affects default human output; `--json`, `--agent`, and `--events` must keep stdout free of ANSI under any color configuration. Configuration files must not save tokens, secrets, passwords, cookies, Authorization headers, webhook URLs, or provider raw payloads; `remote.api_url` may store only the HTTP(S) Pinax API endpoint, and bearer credentials must come from flags or environment. S3 storage only allows non-secret fields such as bucket, region, prefix, endpoint, and profile.
