# init Command

`pinax init` initializes a local Markdown vault. It creates the directories and base configuration required by Pinax, making the current directory or a specified directory a migratable note repository.

## Usage

```bash
pinax init
pinax init ./my-notes --title "My Knowledge Base"
pinax init --vault ./my-notes --title "My Knowledge Base"
```

## Write Boundaries

- Creates the vault directory structure and structured assets managed by the CLI under `.pinax/`.
- Does not connect to the cloud, write provider tokens, or start a long-running daemon.
- After initialization, use `pinax vault validate --vault ./my-notes` to check the result.

## Next Steps

```bash
pinax note add "First Note" --body "Start using Pinax" --vault ./my-notes
pinax index refresh --vault ./my-notes
pinax vault stats --vault ./my-notes
```
