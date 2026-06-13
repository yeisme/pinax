# MVP Scope

MVP advances in four phases:

| Phase | Goal | Validation |
| --- | --- | --- |
| Local Vault Workbench | `init`, `vault validate`, daily/inbox, `note list/show`, `pinax note links`/`pinax note backlinks`/`pinax note orphans`, `search --link-target`, attachments, saved views, index/search, Markdown import/export, `metadata plan/apply`, `repair plan/apply`, `organize plan/list/apply`, `version snapshot` | `go test ./...` and command-level tests |
| CLI-backed Provider Pull | External CLI capability probes, fake executable fixtures, `sync diff`, `sync pull --dry-run` | provider and sync fixture tests |
| Agent/MCP Read and Plan | project board workspace, shared `NoteDisplay`, read-only resources/tools for `pinax mcp serve`, localhost REST/RPC projection adapter, handoff, triage dry-run | MCP frame, REST/RPC component, and output contract tests |
| Controlled Apply | action file apply, local write approval, event evidence, handoff | dry-run/yes gate and redaction tests |

The daily briefing workflow is a later agent workflow slice. It must be based on the local vault, research evidence ledger, review queue, and delivery receipt, and should not become an independent news bot.

The first external evaluation loop of the current MVP prioritizes serving a real Markdown vault: first let users safely connect, capture daily/inbox, build a SQLite/GORM local index, search and browse by tags/group/folder/kind/status, save common views, check resolved/broken/ambiguous links, orphan notes and attachments, search by `--link-target`, import and export Markdown bundles, supplement metadata, generate repair/organize plans and project board plans, and then execute local changes after protection by an explicit version snapshot. The project board is a local project workbench, not a remote Todo provider; `project board plan --save` writes a review snapshot, and weekly planning can read board counts, but it does not automatically write all items into an external task system.
