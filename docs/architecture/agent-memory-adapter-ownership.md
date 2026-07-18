# Agent Memory Runtime — Adapter Ownership Boundary

## Pinax owns

Pinax (`cli/pinax`) owns the canonical Agent memory runtime:

- `internal/agentprotocol` — versioned DTO, validation, stable errors, adapter descriptor
- `internal/agentmemory` — memory lifecycle, scope, source, proposal, feedback, store repository
- `internal/agentcontext` — permission-first context compilation, ranking, budget
- `internal/agentadapter` — reference adapter **harness** (descriptor negotiation, conversion, degraded status)
- `pkg/agentmemory` — experimental Go SDK facade for embedded consumers
- `pinax agent` CLI command tree
- `pinax.agent.*` MCP tools (read-only by default)
- `Pinax.Agent.*` RPC routes

## Pinax does NOT own

Runtime-specific adapter implementation is **not owned by Pinax**. Pinax ships reference adapter fixtures (`internal/agentadapter/codex.go`, `internal/agentadapter/cohors.go`) that prove the common schema is portable, but does not implement:

- **Codex runtime integration**: Codex plugin install, hooks, config parsing, subprocess management, trace path handling, and plugin packaging belong to the Codex owner.
- **Cohors runtime integration**: Cohors team run, role assignment, trace format parsing, and team state management belong to the Cohors owner (`cli/cohors`).

## Cohors consumer handoff

When Cohors implements its Pinax memory adapter, it should:

1. Import `pkg/agentmemory` (the experimental Go SDK) or call `pinax agent` CLI / `pinax.agent.*` MCP tools.
2. Use the common schema (`yeisme.agent_memory.v1`, `yeisme.agent_context_pack.v1`, `yeisme.agent_handoff.v1`) — do **not** invent new core fields. Team-specific data goes in adapter metadata.
3. Respect the proposal-first contract: Cohors workers default to `propose` capability; confirmed memory mutation requires owner approval or explicit policy.
4. Contribute feedback via `pinax agent feedback add` — feedback does not rewrite memory content.

**Trigger condition**: Cohors should implement its adapter when it needs cross-agent handoff (e.g., Cohors worker → Codex reviewer) or shared memory across its team runtime. Until then, the reference fixture in `internal/agentadapter/cohors.go` proves compatibility without requiring Cohors runtime code in Pinax.

## Stable-from criteria

The agent memory runtime remains `experimental` until:

- Two reference adapters (Codex + Cohors) are implemented and tested against real runtime code.
- Four weeks of product dogfooding evidence is collected (task 8.5).
- Transport parity is verified across CLI/MCP/REST-RPC/SDK (task 6.5).

Until then, old `pinax memory`, `pinax brain`, and `pinax.brain.*` surfaces are NOT deprecated.
