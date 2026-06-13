# Product Positioning

Pinax helps users consolidate knowledge scattered across Markdown, Notion, Feishu, web research, and agent outputs into a local, portable, searchable, auditable, and rollback-capable note workflow.

In one sentence: **Pinax is a local-first unified note Agent CLI, not a cloud note platform.**

Initial focus:

- Initialize and validate a local Markdown vault.
- Create, capture, organize, and retrieve notes.
- Manage note versions, rollback plans, and changed-path evidence through `pinax version`; Git is one optional backend, not the user-facing workflow name.
- Sync with external systems through CLI-backed Provider adapters and local-first Pinax Cloud distributed sync.
- Serve agent workflows through stable `--agent` / `--json` output.

Cloud Sync positioning: Pinax Cloud is a synchronization coordinator, not the note source of truth. Each user device keeps a local vault that remains usable offline; the Cloud backend stores encrypted sync artifacts and orders revisions so devices can converge safely.

Non-goals:

- Not building a long-running daemon as a required MVP capability.
- Not treating Feishu, Notion, or other external platforms as the source of truth for notes.
- Not treating Pinax Cloud as a centralized plaintext note editor or hosted vault source of truth.
- Not directly maintaining native API SDKs for external platforms by default.
- Not allowing agents to hand-write machine-readable metadata directly.
