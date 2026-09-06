# prompt Command

`pinax prompt` manages reusable `yeisme.prompt_asset.v1` records in the local Pinax vault. Pinax owns durable prompt assets, lifecycle decisions, URI resolution, source references, and imported usage feedback; external projects resolve prompt assets through Pinax commands instead of reading SQLite or vault metadata directly.

The command group has two surfaces:

1. **Local Prompt Vault** (default): `prompt create|import|search|show|resolve|lifecycle|feedback` operate on Pinax-owned local assets only.
2. **Federated Catalog** (experimental, additive): `prompt repository` and `prompt catalog` discover, verify, and — when rights permit — install external prompts through the public `github.com/yeisme/promptrepo v0.5.0` SDK.

Federated catalog 的 Agent 模板默认使用 `locale=en`。中文笔记和资料可以继续作为输入值，最终笔记语言由用户内容和模板字段决定；中文模板译文只供人类审阅，不参与 preview 或安装。

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `prompt create --from <file>` | Create a prompt asset from a `yeisme.prompt_asset.v1` YAML file. | Writes prompt asset rows to the local SQLite/GORM projection. |
| `prompt import --from <file>` | Import a prompt asset schema file. | Writes prompt asset rows to the local SQLite/GORM projection. |
| `prompt search [query]` | Search **local vault** prompt assets by text, domain, tag, and lifecycle. Never searches external repositories. | Does not write. |
| `prompt show <id>` | Show prompt asset metadata and current version details. | Does not write. |
| `prompt resolve <uri-or-id>` | Resolve `pinax://prompt/<id>` for agent or script consumption. | Does not write. |
| `prompt lifecycle <id> --to <state> --reason <reason>` | Update lifecycle through a Pinax-owned decision with an explicit reason. | Writes lifecycle state and local evidence metadata. |
| `prompt feedback import --from <file>` | Import Eikona-style prompt usage feedback as metadata-only evidence. | Writes feedback metadata and artifact refs. |
| `prompt repository add <id> --source <uri>` | Register a repository profile in the shared user-level promptrepo store (cross-CLI shared). | Writes shared promptrepo state (never the Pinax vault). |
| `prompt repository list` | List shared repository profiles with health and snapshot digests. | Does not write. |
| `prompt repository show <id>` / `doctor <id>` | Show a profile / report repository health. | Does not write. |
| `prompt repository remove <id>` / `enable <id>` / `disable <id>` | Remove or toggle a shared profile. | Writes shared promptrepo state only. |
| `prompt repository sync [<id>...] --all` | Refresh repository snapshots; digest-mismatched catalogs are quarantined and the last good snapshot keeps serving. | Writes shared promptrepo snapshots. |
| `prompt catalog search [query]` | Search the federated catalog across enabled repositories (default locale `en`). | Does not write; `provider_calls=0`. |
| `prompt catalog show <ref>` / `resolve <ref>` | Show or exactly resolve a `promptrepo://<repo>/<package>/<solution>@<version>` ref or template address. | Does not write; `provider_calls=0`. |
| `prompt catalog inspect <ref>` | Inspect verified contract inputs and provenance (Field/Required/Default/Allowed style facts). | Does not write; `provider_calls=0`, `durable_writes=0`. |
| `prompt catalog validate <ref> --values <file>` | Validate input values against the verified template contract; values never appear in output. | Does not write; `provider_calls=0`. |
| `prompt catalog preview <ref> --values <file>` | Render in memory and report readiness, rendered digest, and byte counts. | Does not write; `provider_calls=0`; the rendered body is never printed. |
| `prompt catalog install <ref> [--yes] [--fork]` | Install a rights-permitted template as a Pinax-owned `draft` asset with exact provenance refs and a stage receipt. | With `--yes`, one local durable write (`durable_writes=1`, `remote_writes=0`); without it, a plan only. |

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

`pinax://prompt/<id>` is the stable cross-project reference. Auctra, Eikona, Ordo, scripts, and agents should call `pinax prompt resolve pinax://prompt/<id> --agent` or `--json` rather than reading Pinax SQLite tables, generated DAO files, or `.pinax` metadata directly.

Agent output is intentionally bounded: it includes decision-essential facts such as prompt asset ID, lifecycle, permission, domain, and next action. It does not include prompt body, local filesystem paths, provider payloads, hidden system prompts, private tool arguments, or full chain-of-thought.

## Feedback Import

`pinax prompt feedback import` accepts metadata-only feedback records, for example an Eikona usage feedback JSON file with `feedback_id`, `prompt_asset_id`, `external_run_ref`, `decision`, `reason`, and `artifact_refs`. Imported feedback can suggest a lifecycle decision, but only Pinax changes lifecycle state through `pinax prompt lifecycle`.

## Federated Catalog Workflows

```bash
# Register the official repository and synchronize the shared cross-CLI profile.
pinax prompt repository add official --source https://github.com/yeisme/prompt-templates --trust official --json
pinax prompt repository sync official --json
pinax prompt catalog inspect 'promptrepo://official/general/structured-summary-beta@2.0.0-beta.1?locale=en' --json

# Team and personal repositories remain additive.
pinax prompt repository add team --source file:///path/to/catalog --trust verified --json
pinax prompt repository sync --all --json

# Discover and verify through the federated catalog.
pinax prompt catalog search "中文播客" --json
pinax prompt catalog inspect promptrepo://team/audio/podcast@1.0.0?locale=en --json
pinax prompt catalog preview promptrepo://team/audio/podcast@1.0.0 --values ./inputs.json --json

# Install as a Pinax-owned draft (plan first, then confirm).
pinax prompt catalog install promptrepo://team/audio/podcast@1.0.0 --vault ./my-notes --json
pinax prompt catalog install promptrepo://team/audio/podcast@1.0.0 --vault ./my-notes --yes --json
```

### Rights-gated install

`catalog install` fails closed unless all of the following hold:

- the solution rights are not `blocked`/`prohibited`;
- the Registry-authored companion contract resolves and verifies against the exact snapshot and template digest;
- the verified contract grants an import/copy permission (`local_import`, `local_copy`, `copy`, or `install`).

When the same local asset ID already exists with different content, the command returns a conflict plan (`keep-existing`, `side-by-side`, `fork-local`, `reject`) without writing; the default plan is `reject`, and `--fork` installs side-by-side under a suffixed ID. A template whose contract declares no inputs cannot be installed because the local schema requires at least one variable; use `catalog preview` for static templates.

### Identity and provenance

- External exact identity uses `promptrepo://...` (including `kind=template&role=&path=&digest=&snapshot=` template addresses); local installed identity uses `pinax://prompt/<id>`. The two URIs never impersonate each other.
- Installs store the exact ref, snapshot/template digests, the contract digest, and the `pinax` stage receipt as source refs. Installs never auto-promote: the asset is created as `draft`, and lifecycle changes still go through `pinax prompt lifecycle`.
- `pinax://prompt/catalog_<package>_<solution>` is the deterministic local ID derived from the resolved package and solution.

### Scope policy (experimental)

Catalog commands compose an effective repository set from user (shared store), organization (Template Registry projection, not yet wired), project (workspace binding, not yet wired), and session scopes. More specific pin lists override broader ones, and a deny at any scope always wins. Session-only `--repository`/`--deny` flags affect the current command only.

Pinax 与 Auctra、Eikona、Scaena、Sonora 统一使用 `PROMPTREPO_HOME` / `PROMPTREPO_CACHE` 定位共享用户级 profile 与 snapshot。v0.5 的方案 DAG 与提示包合同不会自动导入 Pinax vault；只有现有 rights-gated `catalog install --yes` 可以创建 Pinax-owned draft。

## Boundaries

- Pinax stores prompt asset metadata and prompt versions in local SQLite/GORM projection tables.
- Prompt asset commands do not execute providers, render images, call Eikona internals, or inspect artifact payloads.
- Repository profiles and credentials live in the shared promptrepo store owned by the SDK; Pinax never copies profiles, credential refs, or source credentials into the vault or Pinax SQLite.
- Catalog inspect/validate/preview are provider-free (`provider_calls=0`, `durable_writes=0`) and never print template bodies, rendered bodies, input values, credentials, or private paths.
- `catalog install` performs one local durable write after the rights gate; it never calls a provider, pushes Git, mutates remotes, or promotes lifecycle.
- Pinax does not implement its own source adapters or state locking; those stay owned by the public promptrepo SDK.
- External projects must not mutate Pinax lifecycle state directly.
- Agents and scripts must not hand-write prompt asset rows or feedback metadata.
