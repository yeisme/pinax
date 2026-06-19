# prompt Command

`pinax prompt` manages reusable `yeisme.prompt_asset.v1` records in the local Pinax vault. Pinax owns durable prompt assets, lifecycle decisions, URI resolution, source references, and imported usage feedback; external projects resolve prompt assets through Pinax commands instead of reading SQLite or vault metadata directly.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `prompt create --from <file>` | Create a prompt asset from a `yeisme.prompt_asset.v1` YAML file. | Writes prompt asset rows to the local SQLite/GORM projection. |
| `prompt import --from <file>` | Import a prompt asset schema file. | Writes prompt asset rows to the local SQLite/GORM projection. |
| `prompt search [query]` | Search prompt assets by text, domain, tag, and lifecycle. | Does not write. |
| `prompt show <id>` | Show prompt asset metadata and current version details. | Does not write. |
| `prompt resolve <uri-or-id>` | Resolve `pinax://prompt/<id>` for agent or script consumption. | Does not write. |
| `prompt lifecycle <id> --to <state> --reason <reason>` | Update lifecycle through a Pinax-owned decision with an explicit reason. | Writes lifecycle state and local evidence metadata. |
| `prompt feedback import --from <file>` | Import Eikona-style prompt usage feedback as metadata-only evidence. | Writes feedback metadata and artifact refs. |

## Common Workflows

```bash
pinax prompt import --from ./novel-character.yaml --vault ./my-notes --json
pinax prompt search "character portrait" --domain visual_generation --tag character --vault ./my-notes --json
pinax prompt show novel_character_portrait_v1 --vault ./my-notes --json
pinax prompt resolve pinax://prompt/novel_character_portrait_v1 --vault ./my-notes --agent
pinax prompt lifecycle novel_character_portrait_v1 --to tested --reason "fixture render passed" --vault ./my-notes --json
pinax prompt feedback import --from ./eikona-feedback.json --vault ./my-notes --json
```

## Prompt Asset Schema

Prompt asset imports use YAML with `schema_version: yeisme.prompt_asset.v1`. Required fields are `schema_version`, `id`, `domain`, `permission`, `variables`, and `prompt_template`. Supported lifecycle values are `draft`, `tested`, `accepted`, `promoted`, and `retired`. Supported permission values are `unknown`, `internal`, and `public`.

## URI Boundary

`pinax://prompt/<id>` is the stable cross-project reference. Auctra, Eikona, Cohors, scripts, and agents should call `pinax prompt resolve pinax://prompt/<id> --agent` or `--json` rather than reading Pinax SQLite tables, generated DAO files, or `.pinax` metadata directly.

Agent output is intentionally bounded: it includes decision-essential facts such as prompt asset ID, lifecycle, permission, domain, and next action. It does not include prompt body, local filesystem paths, provider payloads, hidden system prompts, private tool arguments, or full chain-of-thought.

## Feedback Import

`pinax prompt feedback import` accepts metadata-only feedback records, for example an Eikona usage feedback JSON file with `feedback_id`, `prompt_asset_id`, `external_run_ref`, `decision`, `reason`, and `artifact_refs`. Imported feedback can suggest a lifecycle decision, but only Pinax changes lifecycle state through `pinax prompt lifecycle`.

## Boundaries

- Pinax stores prompt asset metadata and prompt versions in local SQLite/GORM projection tables.
- Prompt asset commands do not execute providers, render images, call Eikona internals, or inspect artifact payloads.
- External projects must not mutate Pinax lifecycle state directly.
- Agents and scripts must not hand-write prompt asset rows or feedback metadata.
