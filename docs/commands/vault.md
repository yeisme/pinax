# vault Command

`pinax vault` manages vault-level state, health checks, and vault selection. It does not process individual note bodies; it answers whether a vault is usable, which vault is selected by default, and where maintenance or remote discovery is needed.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax vault register <path> --name <alias>` | Registers a local vault alias for `--vault` completion and default selection. | Writes the user vault registry. |
| `pinax vault use <alias>` | Selects the default registered vault used when `--vault` is omitted. | Writes the user vault registry. |
| `pinax vault list` | Lists registered local aliases and cached remote selectors. | Does not write. |
| `pinax vault remote refresh --profile <profile>` | Refreshes cached remote vault selectors from a profile endpoint. | Writes the user vault cache; may contact the profile endpoint. |
| `pinax vault remote list` | Lists cached remote selectors without network access. | Does not write. |
| `pinax vault stats` | Summarizes note counts, tags, directories, indexes, and other metadata. | Does not write. |
| `pinax vault validate` | Validates the vault structure and Pinax conventions. | Does not write. |
| `pinax vault ignore status` | Shows `.pinaxignore`, metadata-only `.gitignore`, and managed content counts. | Does not write. |
| `pinax vault ignore plan` | Plans missing Pinax/Git ignore configuration. | Does not write. |
| `pinax vault ignore apply --yes` | Writes missing `.pinaxignore` and patches the Pinax metadata-only `.gitignore` block. | Writes ignore files and event evidence. |
| `pinax vault doctor` | Checks for health issues and provides maintenance suggestions. | Does not write. |
| `pinax vault dashboard` | Starts a localhost read-only dashboard. | Does not write to the vault; only provides a local read-only HTTP surface. |

## Common Workflows

```bash
pinax init ./my-notes --title "My Knowledge Base"
pinax vault register ./my-notes --name work --default
pinax vault list --json
pinax vault use work
pinax note list
pinax note list --vault work
pinax vault remote refresh --profile cloud-work --json
pinax vault remote list --profile cloud-work --json
pinax vault stats --vault work
pinax vault validate --vault work --json
pinax vault ignore status --vault work --json
pinax vault ignore plan --vault work --json
pinax vault doctor --vault work --agent
pinax vault dashboard --vault work --port 0
```

## Ignore Policy

`.pinaxignore` controls Pinax content manifest and Cloud Sync selection. It is separate from `.gitignore`: Git ignore rules do not implicitly exclude Pinax content.

New vaults receive a default `.pinaxignore` plus a metadata-only `.gitignore` block. The Git block keeps Git focused on safe `.pinax` project metadata while Pinax sync/provider transports manage Markdown, scripts, assets, attachments, and other unignored regular files.

Obsidian-style vault compatibility is preview. New vault ignore defaults include `.obsidian/`, and existing vaults can review the gap with `pinax vault ignore status --vault ./my-notes --json` and `pinax vault ignore plan --vault ./my-notes --json`. Pinax supports wikilinks/backlinks, properties, daily managed blocks, template preview, asset missing/orphan doctor, dataview database blocks, canvas/plugin ignore, and publish planning without writing Obsidian plugin config into `.pinax/**`.

## Dashboard Boundary

`pinax vault dashboard` is a local read-only client over shared application service projections. It may display overview, graph health, repair plans, project boards, bounded note displays, and database tabs. Active tab selection remains client-local and is not persisted as a layout registry.

## Completion and Selection

- `--vault <TAB>` completes registered local aliases and cached remote selectors. It still allows shell file completion for ordinary paths.
- Shell completion never contacts remote endpoints, resolves secrets, or writes registry/cache files.
- Remote selectors are refreshed explicitly with `pinax vault remote refresh --profile <profile>` and then read offline from the local cache.
- Vault selector precedence is: explicit `--vault`, `PINAX_VAULT`, user config `vault`, `pinax vault use <alias>`, then `.`.

## Relationship to Other Commands

- After discovering health issues, use `pinax repair plan --vault ./my-notes --save` to generate a maintenance plan.
- For index anomalies, check `pinax index doctor --vault ./my-notes` first.
- Root-level `stats`, `validate`, `doctor`, and `dashboard` are compatible aliases; new documentation and the main help path use the `vault` group.
