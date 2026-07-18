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

MCP tools and resources are read-only, including note links, backlinks, context, search, graph summary, Agent Brain previews, and organize plan preview. MCP should not directly write Markdown, `.pinax/`, Git, provider, or remote state.

## Agent Brain Role

The current MCP surface exposes these read-only Agent Brain tools:

| Tool | Purpose | Boundary |
| --- | --- | --- |
| `pinax.brain.context` | Return `pinax.agent_brain.context_bundle.v1` from bounded projections. | `readonly=true`, `body_exposure=bounded_projection`, `scope=local_vault`. |
| `pinax.brain.answer` | Return extractive `pinax.agent_brain.answer.v1` preview. | `cost_class=none`, `provider_id=extractive`, no provider/network call. |
| `pinax.brain.sources` | List bounded answer sources without synthesis. | Does not return full note bodies. |
| `pinax.brain.maintenance_plan` | Return plan-only maintenance next action. | Suggests `pinax proof loop run`; does not write vault state. |

These tools reject or downgrade full-body requests by returning bounded projections only. Future provider-backed synthesis, write tools, or hosted team behavior must add new contracts and tests first.

HTTP MCP, OAuth, rate limit enforcement, and team/company KB permission backends are future-owner work. They must be owned by `mcp/gateway`, a hosted/backend subproject, or another explicit OpenSpec, not silently added to the local CLI-only MCP server.
