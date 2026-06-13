# mcp Command

`pinax mcp` starts the Pinax MCP surface. The current MCP surface is a read-only entry point for external agents to query vault facts, not to write directly to the vault.

## Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `pinax mcp serve` | Starts a read-only MCP server over stdio. | Does not write to the vault. |

## Usage

```bash
pinax mcp serve --vault ./my-notes
```

## Boundaries

MCP tools and resources are read-only, including note links, backlinks, context, search, graph summary, and organize plan preview. MCP should not directly write Markdown, `.pinax/`, Git, provider, or remote state.
