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
```

## Boundaries

Template functions are allowlisted. They do not execute scripts, read environment variables, or access the network. Available safe functions include pure functions such as `slug`, `date`, `yaml`, `json`, and `quote`.

`schema_version: pinax.template_design.v1` is a draft template and can only be used for inspect/validate; it cannot be used with `template preview`, `template render`, or `note add/new --template`. To execute a template, first publish it as `schema_version: pinax.template.v2`.

`template preview` is a read-only path. When a query-backed template preview has a missing or stale index, it returns an error/partial projection and the next step `pinax index rebuild --vault <vault>`; it does not implicitly create `.pinax/index.sqlite` or write events. `template render` can still use query-backed rendering within controlled boundaries; `template inspect` only explains the query and does not execute it.

Built-in starter note templates can declare `defaults.kind`, `defaults.status`, and `output.path_pattern`. When creating notes with `pinax note add/new --template <name>`, template defaults participate in path and metadata calculation; explicit CLI parameters such as `--kind`, `--status`, `--dir`, `--folder`, and `--slug` take precedence.

`source.github` is the built-in durable source note template for GitHub repositories. It is local-only, does not call the GitHub API, and creates an ordinary Markdown source card. See [Durable Source Notes](../overview/durable-source-notes.md).

```bash
pinax note add "iptv-org/iptv" --template source.github --var url=https://github.com/iptv-org/iptv --vault ./my-notes --json
```
