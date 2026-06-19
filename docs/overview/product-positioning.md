# Product Positioning

Pinax is the **agent-safe knowledge control plane for your Markdown vault**. It lets AI safely read, diagnose, repair, and sync a real local knowledge base, while keeping every agent write auditable, previewable, and reversible.

In one sentence: **Pinax makes AI safe to operate on your private knowledge base — not another notes app, not another cloud silo.**

## Three repeatable concepts

1. **Local Vault is the source of truth** — Markdown files are always the source of truth; SQLite/`.pinax/` are rebuildable projections.
2. **The Proof Loop protects every agent write** — Capture → Retrieve → Diagnose → Plan → Snapshot → Apply → Restore.
3. **Share and Sync are surfaces, not sources** — publish targets are generated delivery artifacts; Cloud Sync coordinates encrypted revisions and never stores plaintext notes or executes local tools.

## Target users

- **AI-heavy developers** who run agents against a real Markdown knowledge base and need every write to be plan-gated, snapshot-protected, and reversible.
- **Privacy-sensitive technical workers** who will not hand plaintext notes to a hosted platform and want self-hosted encrypted sync.
- **Obsidian engineering power users** who want a programmable, agent-safe maintenance and repair layer over their existing vault instead of a second note editor.
- **Self-hosting small teams** who need a portable, auditable Markdown vault with distributed encrypted sync, without adopting a hosted collaboration workspace.

## Competitor positioning

Pinax does not compete on note-editing UX or feature checklists. It occupies a different layer.

| Competitor | Relationship | What Pinax does not do |
| --- | --- | --- |
| Obsidian | **Complements** — agent-safe maintenance layer over your existing vault | Does not replace the editor UI |
| Logseq | **Differentiates** — does not replicate outliner/graph UI | Does not copy the outliner model |
| Notion | **Avoids** — does not build team collaboration or cloud workspaces | No cloud lock-in, no hosted vault |
| Reflect | **Differentiates** — more programmable and verifiable | Does not compete on individual note-taking feel |

## Initial focus

- Initialize and validate a local Markdown vault.
- Create, capture, organize, and retrieve notes through the proof loop.
- Manage note versions, rollback plans, and changed-path evidence through `pinax version`; Git is one optional backend, not the user-facing workflow name.
- Share local notes through reviewed publish surfaces such as Pages, Wiki, Gist, HTTP endpoints and loopback preview.
- Sync local files across devices through CLI-backed Provider adapters and local-first Pinax Cloud distributed sync.
- Serve agent workflows through stable `--agent` / `--json` output.

Cloud Sync positioning: Pinax Cloud is a synchronization coordinator, not the note source of truth. Each user device keeps a local vault that remains usable offline; server transport stores encrypted sync artifacts and orders revisions so devices can converge safely.

## Non-goals

- Not building a long-running daemon as a required MVP capability.
- Not treating Feishu, Notion, or other external platforms as the source of truth for notes.
- Not treating Pinax Cloud as a centralized plaintext note editor or hosted vault source of truth.
- Not directly maintaining native API SDKs for external platforms by default.
- Not allowing agents to hand-write machine-readable metadata directly.
- Not building a team collaboration workspace, web/mobile editor, or Notion-style cloud product.
