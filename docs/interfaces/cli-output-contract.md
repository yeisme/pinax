# CLI Output Contract

Pinax commands must render all output modes from the same command projection:

- Default output: concise English summary; on success, show only result facts, risks, and next steps, and do not show command execution status.
- `--agent`: low-token `key=value`, stable field names, suitable for agent consumption.
- `--json`: a single JSON envelope; stdout contains only JSON.
- `--events`: NDJSON event stream.
- `--explain`: explain inputs, decisions, risks, and reproducible commands.
- Output modes are mutually exclusive: only one of default mode, `--agent`, `--json`, `--events`, or `--explain` can be selected at a time; on conflict, return `cli.output_mode` / `output_mode_conflict`.

stdout/stderr rules:

- In machine output modes, stdout contains only the selected machine format.
- `--events` must output at least `start` and `end` / `error` events; omit `facts`, `actions`, `evidence`, and `error` fields that have no value to avoid ambiguity around `null` semantics.
- progress, diagnostics, provider stderr, logs, and unstructured errors go to stderr.
- All errors must have stable status and error code.
- Default human output does not show `status=success` or a “success” status bar; success itself is expressed by the command exit code and result summary. Show command execution status only for `partial`, `failed`, dry-run, approval required, when a warning/risk exists, or when the user explicitly requests a machine mode. Status fields in business facts may still be shown, such as note frontmatter `status`, `index_status`, `ledger_status`, or provider health status.
- New notebook core commands must reuse the same projection: default output for `daily`, `inbox`, `view`, `index`, `search`, `note links/backlinks/orphans/attach/attachments`, `import markdown`, `export markdown`, `organize plan/list/apply`, `project board show/plan/configure/export`, `project item add/move/archive`, and `api routes/schema/serve` is an English summary, `--json` is a single envelope, and `--agent` is low-token facts.
- Common stable facts include but are not limited to: `path`, `note_id`, `group`, `folder`, `kind`, `status`, `index_status`, `engine`, `returned`, `links`, `backlinks`, `broken`, `ambiguous`, `orphans`, `link_target`, `unresolved`, `attachments`, `missing`, `view`, `display`, `note_display`, `project`, `columns`, `items`, `next`, `doing`, `blocked`, `review`, `done`, `snapshot_id`, `board_snapshot_id`, `plan_id`, `operations`, `receipt_path`, `writes`.
- Argument, flag, and usage errors must also be actionable: default output explains missing or extra arguments, provides real runnable examples, and offers `--help` or the next command; it must not expose only framework errors such as `accepts N arg(s)`.
- Argument errors under `--json`, `--agent`, `--events`, and `--explain` must be rendered from the same failed projection, containing stable `error.code`, English `error.message`, and executable `error.hint` / `actions`.
- Tokens, webhook URLs, cookies, Authorization headers, external CLI configuration contents, and raw payloads must be redacted.

Project board and NoteDisplay output contract:

- `pinax project board show <project> --note-display card|detail|context --json` must output a bounded board projection and must not output the full note body; use `pinax note read/show <ref> --display body --json` when the body is needed.
- `pinax note read/show --display card|detail|context|body` must reuse the `NoteDisplay` structure in `data.note`; `card/detail/context` return only title, path, project, status, column, tags, excerpt, and redaction warnings; only `body` may include `data.note.body`.
- If `project item archive` is missing `--yes`, return `approval_required`; if it is missing a version snapshot, return `snapshot_required`; both must provide an executable action or hint and must not rewrite Markdown.
- `pinax api routes --json` and `pinax api schema export --format openapi --json` must be rendered from the remote capability registry; handlers, documentation, and tests are not allowed to maintain different fields independently.

Output contract for relationship commands:

- `pinax note links`, `pinax note backlinks`, `pinax note orphans`, and `pinax search --link-target` must render the default English summary, `--agent`, `--json`, `--events`, and `--explain` from the same relationship projection.
- `--json`/`--agent`/`--events` do not output the full note body and do not leak secrets, raw prompts, provider payloads, or hidden system prompts; ambiguous candidates return only bounded summaries.
- When the index is missing/stale, output `engine=scan`, `index_status`, and a runnable action; for low-cost maintenance, prefer recommending `pinax index refresh --vault <vault>`; handle structural abnormalities with `pinax index doctor --vault <vault>` or explicit `rebuild`; do not describe the SQLite index as the source of truth.
- Output related to broken, ambiguous, orphan, and link rewrite cases may only guide users to manual review via `repair plan` or `organize plan --save`; it must not imply automatic body rewrites.


## Human Rendering Configuration

`--color`, `--theme`, `--width`, `--markdown-style`, and `output.*` configuration affect only the default human summary output. `--json`, `--agent`, `--events`, and `--explain` must continue to render the machine contract from the same projection and must not include ANSI, Glamour decorations, table colors, or pager control sequences.

Themes take effect through semantic roles: `accent`, `muted`, `rule`, `success`, `warning`, `danger`, `key`, `value`, `path`, `link`, `code`, `heading`. Built-in themes are `pinax`, `mono`, and `high-contrast`; the `custom` theme only overrides roles declared by the user, and missing roles fall back to `pinax`.

Markdown/Glamour is used only for body-reading scenarios in default human mode, such as `note show/read`, `daily/weekly/monthly show`, and `template show/render`. Machine modes preserve the original `data.body` or `data.note.body` and must not write rendered ANSI text.
