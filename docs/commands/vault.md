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
pinax vault doctor --vault work --agent
pinax vault dashboard --vault work --port 0
```

## Completion and Selection

- `--vault <TAB>` completes registered local aliases and cached remote selectors. It still allows shell file completion for ordinary paths.
- Shell completion never contacts remote endpoints, resolves secrets, or writes registry/cache files.
- Remote selectors are refreshed explicitly with `pinax vault remote refresh --profile <profile>` and then read offline from the local cache.
- Vault selector precedence is: explicit `--vault`, `PINAX_VAULT`, user config `vault`, `pinax vault use <alias>`, then `.`.

## Relationship to Other Commands

- After discovering health issues, use `pinax repair plan --vault ./my-notes --save` to generate a maintenance plan.
- For index anomalies, check `pinax index doctor --vault ./my-notes` first.
- Root-level `stats`, `validate`, `doctor`, and `dashboard` are compatible aliases; new documentation and the main help path use the `vault` group.
