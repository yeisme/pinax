# pinax agent

> **Experimental.** The `pinax agent` command tree is a vendor-neutral Agent memory runtime surface. It is additive — it does not replace `pinax memory` or `pinax brain`. Old commands remain fully functional.

## Summary

The `pinax agent` command tree provides a permission-first, bounded Agent memory runtime: principals, scopes, memory proposals, cross-agent handoff, and recall feedback. Agents default to `propose` capability; confirmed memory mutation requires owner approval.

## Current Commands

```text
pinax agent context              Compile a bounded, permission-first context pack
pinax agent memory recall        Recall bounded agent memories
pinax agent memory propose       Submit a memory proposal for review (agents can only propose)
pinax agent memory proposals     List agent memory proposals
pinax agent memory approve       Approve a memory proposal (owner only, requires --yes)
pinax agent memory reject        Reject a memory proposal
pinax agent handoff create       Create a cross-agent bounded working state handoff
pinax agent handoff list         List handoffs
pinax agent feedback add         Add recall quality feedback (does not rewrite memory content)
pinax agent feedback list        List feedback
pinax agent status               Show agent memory runtime status
```

## Common Usage

```bash
# Compile a bounded context pack for a project scope
pinax agent context --vault ./my-notes --scope project:pinax --entities "GORM,SQLite" --json

# Recall confirmed memories
pinax agent memory recall --vault ./my-notes --json

# An agent proposes a memory (returns approval_required)
pinax agent memory propose --vault ./my-notes --kind decision \
  --subject "Use GORM Gen for typed DAO" --sources "note:note_gorm_decision" --json

# Owner approves the proposal (requires --yes)
pinax agent memory approve <proposal-id> --vault ./my-notes --yes --json

# Create a cross-agent handoff
pinax agent handoff create --vault ./my-notes \
  --objective "Review GORM Gen migration slice" --json

# Add recall feedback
pinax agent feedback add --vault ./my-notes --kind useful --memory-id <id> --json
```

## Scope Format

Scopes use `kind:id` format:

- `owner:<owner_id>`
- `workspace:<workspace_id>`
- `project:<project_id>`
- `repository:<repo_id>`
- `session:<session_id>`
- `task:<task_id>`

Default scope is `workspace:default`.

## Memory Kinds

`fact`, `decision`, `preference`, `procedure`, `event`, `task`, `failure`

## Lifecycle States

`proposed` → `confirmed` (approved) or `rejected` (review rejects)

`confirmed` → `superseded`, `expired`, or `conflicted`

Conflicts remain visible (not silently hidden). Conflicts resolve by choosing a winner.

## Proposal Review Status

- `approval_required` — valid proposal, owner approval needed
- `conflict_required` — conflicts with existing confirmed memory
- `rejected` — duplicate of existing confirmed memory

## Security

All agent memory commands produce bounded projections. Context packs do not output full note bodies. Sources carry only references (object_id, note_id, URL), not content. No secrets, Authorization headers, raw prompts, or chain-of-thought sentinels appear in any output surface.

## Compatibility

This command tree is **experimental** (schema `v1`). Old surfaces (`pinax memory`, `pinax brain`, `pinax.brain.*`) are NOT deprecated and remain fully functional. The agent runtime will graduate to stable after two reference adapter implementations and four weeks of product dogfooding.

## Related

- [Architecture: Adapter Ownership](../architecture/agent-memory-adapter-ownership.md)
- [memory](./memory.md) — existing deterministic agent memory
- [brain](./brain.md) — existing Agent Brain preview
