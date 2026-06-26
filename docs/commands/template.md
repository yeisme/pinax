# template Command

`pinax template` manages Markdown templates, template previews, final rendering, and render runs. Templates are stored in `.pinax/templates/*.md` and are managed by the CLI/service.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `template init` | Initialize built-in templates. | Writes template assets. |
| `template create <name>` | Create a template; supports `--body`, `--from`, and `--engine`. | Writes template assets. |
| `template list` | List templates. | Does not write. |
| `template recommend` | Recommend templates by intent. | Does not write. |
| `template show <name>` | Read a template. | Does not write. |
| `template inspect <name>` | View template metadata, variables, queries, and runs. | Does not write. |
| `template validate <name>` | Validate a template. | Does not write. |
| `template preview <name>` | Preview the rendered result. | Does not write. |
| `template render <name>` | Render a template; `--save-run` saves a receipt. | May write a render run. |
| `template runs prune|repair` | Clean up or repair the render run index. | Writes `.pinax/renders/` according to parameters. |
| `template delete <name> --yes` | Delete a custom template. | Writes template assets. |

## Common Workflows

```bash
pinax template init --vault ./my-notes
pinax template create weekly --engine go-template --body "# {{ .Title }}" --vault ./my-notes
pinax template inspect weekly --vault ./my-notes --json
pinax template preview weekly --title "Client Meeting" --var client=Acme --vault ./my-notes --agent
pinax template render weekly --title "Client Meeting" --save-run weekly-demo --vault ./my-notes --json
pinax note add "Client Meeting" --template weekly --var client=Acme --vault ./my-notes
pinax template recommend --intent "论文" --vault ./my-notes --json
pinax template recommend --intent "便签" --vault ./my-notes --json
pinax template recommend --intent "K线" --vault ./my-notes --json
pinax note add "某篇小说是怎么写成的" --template idea.research_seed --vault ./my-notes --json
pinax note add "临时线索" --template sticky.capture --vault ./my-notes --json
pinax note add "子项目看板线索" --template sticky.project_signal --project research --folder inbox --vault ./my-notes --json
pinax note add "K线基础" --template learning.stock.indicator --project investing --vault ./stock-learning-notes --json
pinax note add "仓位风险" --template learning.stock.risk_rule --project investing --vault ./stock-learning-notes --json
pinax index page create ideas --template index.ideas --vault ./my-notes --json
```

## Obsidian Compatibility

Daily notes use `journal.daily`, which writes to `daily/{{ .Date }}.md` and keeps Pinax automation inside managed blocks only:

- `pinax plan daily --task-review --vault ./my-notes` previews the `daily-task-review` managed block update and does not write.
- `pinax plan daily --task-review --yes --vault ./my-notes` replaces only `<!-- pinax:managed name=daily-task-review --> ... <!-- /pinax:managed -->`.
- User-written content, Obsidian plugin content, and headings outside Pinax managed blocks remain user-owned.

Template preview and inspect paths are read-only. `template create`, `template init`, `template delete --yes`, and `template runs prune|repair` write only Pinax template assets or render-run assets under `.pinax/`. Note creation from a template writes the new Markdown note through `pinax note add/new`; later manual edits in Obsidian are preserved unless the user runs an explicit Pinax write command such as `note property set`, `note tag add`, or a reviewed repair apply.

Pinax property writes are field-scoped. `pinax note property set/remove` updates the requested property and Pinax-managed timestamps while preserving unknown frontmatter keys such as `cssclasses`, plugin state fields, and custom Obsidian properties.

## Boundaries

Template functions are allowlisted. They do not execute scripts, read environment variables, or access the network. Available safe functions include pure functions such as `slug`, `date`, `yaml`, `json`, and `quote`.

`schema_version: pinax.template_design.v1` is a draft template and can only be used for inspect/validate; it cannot be used with `template preview`, `template render`, or `note add/new --template`. To execute a template, first publish it as `schema_version: pinax.template.v2`.

`template preview` is a read-only path. When a query-backed template preview has a missing or stale index, it returns an error/partial projection and the next step `pinax index rebuild --vault <vault>`; it does not implicitly create `.pinax/index.sqlite` or write events. `template render` can still use query-backed rendering within controlled boundaries; `template inspect` only explains the query and does not execute it.

Built-in starter note templates can declare `defaults.kind`, `defaults.status`, and `output.path_pattern`. When creating notes with `pinax note add/new --template <name>`, template defaults participate in path and metadata calculation; explicit CLI parameters such as `--kind`, `--status`, `--dir`, `--folder`, and `--slug` take precedence.

Chinese idea, sticky, learning, and content templates are built in. `idea.*` templates, such as `idea.research_seed`, `idea.anime_watch`, `idea.game_explore`, `idea.paper_read`, `idea.novel_read`, `idea.novel_write`, and `idea.video_note`, create parked idea notes with `kind: idea` and `status: parked`. `sticky.*` templates, such as `sticky.capture`, `sticky.quote`, `sticky.link`, `sticky.question`, `sticky.term`, `sticky.person_signal`, and `sticky.project_signal`, create short inbox notes with `kind: sticky` and `status: inbox`; they do not create managed project board items or write `board_column`. Detailed note templates such as `media.drama`, `media.anime`, `game.playlog`, `reading.paper`, `reading.novel`, `writing.novel`, `learning.video`, `learning.book`, `learning.term`, `learning.source`, `learning.practice_log`, `learning.weekly_review`, `learning.case_review`, and `research.topic` create active notes for deeper review.

Stock-learning templates use the `learning.stock.*` namespace. They include `learning.stock.term`, `learning.stock.indicator`, `learning.stock.case_review`, `learning.stock.trade_journal`, `learning.stock.risk_rule`, and `learning.stock.weekly_review`. These templates are learning and review notes only; they do not write managed board items and do not provide trading recommendations, automated trading decisions, or return promises.

`source.github` is the built-in durable source note template for GitHub repositories. It is local-only, does not call the GitHub API, and creates an ordinary Markdown source card. See [Durable Source Notes](../overview/durable-source-notes.md).

```bash
pinax note add "iptv-org/iptv" --template source.github --var url=https://github.com/iptv-org/iptv --vault ./my-notes --json
```

See also [`index`](./index.md) for managed index pages, [`inbox`](./inbox.md) and [`draft`](./draft.md) for review indexes, and [`project`](./project.md) for learning/project workspaces.
