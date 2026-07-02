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

## Workflow Catalog

模板 catalog 现在把可执行模板视为本地 workflow starter。`template recommend` 只读取本地内置模板和 vault-local 模板 metadata，不调用 provider、不访问网络、不执行 SQL、不写 Markdown、`.pinax` 或 Git 状态。

```bash
pinax template recommend --intent "meeting" --vault ./my-notes --json
pinax template recommend --intent "便签" --vault ./my-notes --agent
pinax template list --pack starter --vault ./my-notes --json
```

`--json` 输出继续保留既有 `data.primary` 和 `data.templates`，并追加 `data.recommendations[]`。每条 recommendation 可以包含 `scenario_id`、`maturity`、`pack`、`fit_reason`、`preview_command`、`create_command`、`evidence_path`、`proof_gate`、`after_create_actions`、`lifecycle` 和 `executable`。`--agent` 追加稳定 key，例如 `recommendation.0.template`、`recommendation.0.scenario_id` 和 `recommendation.0.proof_gate`。

`template inspect` 和 `template preview` 暴露相同的 workflow metadata，便于在写入前审查变量、路径策略和 proof gate：

```bash
pinax template inspect meeting.notes --vault ./my-notes --json
pinax template preview meeting.notes --title "Client Meeting" --vault ./my-notes --json
```

`template preview` 是只读投影，输出 `read_only=true`、`writes=false`、`output_policy`、`proof_gate`、`write_impact`、`body_exposure` 和下一条真实 `pinax note add ... --template ...` 命令；它不会创建 note、receipt、`.pinax` state、Git snapshot 或 provider side effect。

当用户执行 `pinax note add "Client Meeting" --template meeting.notes --dir index --vault ./my-notes --json` 时，note 创建 projection 会追加模板使用证据字段，而不改变既有 envelope 顶层：`template_use_id`、`template`、`template_pack`、`scenario_id`、`effective_path`、`proof_gate.status` 和 `data.template_use.next_actions[]`。这些字段用于后续 proof loop、搜索和 handoff；事件摘要由 app service 写入，preview/dry-run 路径不写 receipt、Markdown、Git 或 provider state。

模板 lifecycle 影响推荐：`draft_design` 不会成为 primary executable recommendation，`deprecated` 保留可 inspect/preview 并可声明 replacement，vault-local 同名模板会在 inspect 中标记为 `source=vault-local` 和 `lifecycle=overridden`。

## Scenario Matrix

| scenario_id | Target user | Job-to-be-done | Required artifacts | Gate/review checks | Evidence path | Export/handoff path | Validation command | Readiness |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `capture-sticky` | 快速记录用户 | 把临时线索放入 inbox，后续分拣。 | `sticky.capture` note、template use projection。 | `template preview` read-only；写入后进入 inbox/index。 | `template_use_id`、note path、command JSON。 | `pinax search`、`pinax proof loop run`。 | `go test ./cmd/pinax -run TestTemplateRecommend -count=1` | mature |
| `idea-research-seed` | 内容研究者 | 把以后调查的想法停放为 parked idea。 | `idea.research_seed` note、`index.ideas`。 | 不自动创建 task/board item。 | note path + recommendation evidence。 | ideas index/search。 | `go test ./cmd/pinax -run TestTemplateRecommend -count=1` | first-support |
| `meeting-decision` | 项目协作者 | 创建会议/决策记录并生成后续 action。 | `meeting.notes`、`decision.record`。 | proof gate manual review；after-create action 可见。 | `template_use_id` + note id。 | project board/proof loop。 | `go test ./cmd/pinax -run TestTemplatePreview -count=1` | mature |
| `learning-pack` | 长期学习用户 | 初始化长期学习资料、术语、复盘模板。 | learning templates、project workspace refs。 | project workspace preview/dry-run。 | template use projection + board projection。 | project board/export。 | `go test ./cmd/pinax -run 'TestTemplate|TestProject' -count=1` | first-support |
| `stock-learning` | 金融学习用户 | 记录学习、模拟、风险规则，避免投资建议。 | `learning.stock.*` templates。 | safety copy and no-advice assertions。 | template use projection + risk disclaimer evidence。 | learning project workspace。 | `go test ./cmd/pinax -run 'Stock|Template' -count=1` | exploratory |
| `index-page` | vault 维护用户 | 用 index template 创建/刷新托管索引页。 | `index.*` template、managed block。 | preview before create/refresh。 | managed index receipt。 | local index/search。 | `go test ./cmd/pinax -run TestIndexPage -count=1` | mature |

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
