# note Command

`pinax note` is the main entry point for individual notes and note relationships. It handles creating, reading, editing, moving, archiving, deleting, tags, links, attachments, and rendered view.

## Common Subcommands

| Command | Purpose | Writes |
| --- | --- | --- |
| `note add|new|create <title>` | Create a note. `add` is the recommended entry point. | Writes Markdown note. |
| `note list` | Filter notes by tag, group, folder, kind, status, etc. | Does not write. |
| `note show|read <note>` | Read source or rendered view. | Does not write. |
| `note preview <note>` | Read-only preview of a rendered note. | Does not write. |
| `note refresh <note> --rendered --yes` | Refresh rendered managed blocks. | Writes Markdown managed blocks. |
| `note links|backlinks|orphans` | View outgoing links, backlinks, and orphan notes. | Does not write. |
| `note attach <note> <file>` | Copy, move, or register an attachment and append a reference. | May write attachments and notes. |
| `note attachments <note>` | List attachment references. | Does not write. |
| `note edit|open <note>` | Open a note with an editor. | The user's editor may write Markdown. |
| `note rename|move|archive` | Individual note maintenance. | Writes Markdown path or metadata. |
| `note delete <note> --yes` | Move to trash; `--hard --yes` truly deletes. | Writes vault. |
| `note tag add|remove|set` | Update tags. | Writes Markdown frontmatter. |
| `note tags|folders|kinds|groups` | View organization dimensions. | Does not write. |

## Creating and Reading

```bash
pinax note add "Research Log" --body "Today's observations" --tags research --status active --vault ./my-notes
pinax note add "Meeting Notes" --stdin --vault ./my-notes
pinax note list --tag research --recent --limit 20 --vault ./my-notes
pinax note show "Research Log" --view source --vault ./my-notes
pinax note show "Research Log" --view rendered --vault ./my-notes
pinax note read "Research Log" --display card --vault ./my-notes --json
pinax note read "Research Log" --display body --vault ./my-notes --json
pinax note preview "Research Log" --vault ./my-notes
```

`note read|show --display card|detail|context` returns bounded note metadata, excerpts, and `agent_context` without exposing the full note body. `--display body` is the explicit body exposure mode for source editing and review. Agent output stays compact and should not include full body content unless the caller explicitly selected body mode through JSON/detail workflows.

`note preview` is optimized for direct reading. In default human mode it renders the preview body only; it does not print a separate success table such as `Local note read.`. If the rendered body is empty, a successful preview is silent. Use `--json` or `--agent` when automation needs the success envelope, note path, resolver facts, or render metadata.

## Individual Note Maintenance

```bash
pinax note edit "Research Log" --editor "$EDITOR" --vault ./my-notes
pinax note rename "Research Log" "Pinax Research Log" --vault ./my-notes
pinax note move "Pinax Research Log" archive --vault ./my-notes
pinax note archive "Pinax Research Log" --vault ./my-notes
pinax note tag add "Pinax Research Log" important --vault ./my-notes
pinax note delete "Pinax Research Log" --yes --vault ./my-notes
```

Tag writes only accept safe tags: letters, digits, English words, `_`, `-`, `/`; they may include the prefix `#`, but it will be normalized when saved. Newlines, whitespace, commas, colons, square brackets, and control characters are rejected before writing with `invalid_tag`. `note add/new --tags`, `note tag add|remove|set`, import defaults, and organization metadata patch use the same validation.

After `note tag` succeeds, it updates Markdown frontmatter, appends a record ledger metadata event, and refreshes the local index. `--json` and `--agent` output stable facts such as `record_event`, `ledger_seq`, `record_version`, `index_updated`, or `index_status`, making it convenient for automation to decide whether `pinax index rebuild --vault <vault>` is needed next.

When creating a note with `--template`, the v2 note template's `defaults.kind`, `defaults.status`, and `output.path_pattern` are used as defaults; explicit CLI arguments take precedence.

For long-lived GitHub repository source cards, use the built-in `source.github` template. It writes an ordinary Markdown note under `sources/github/` with `kind: source` and source-oriented tags; see [Durable Source Notes](../overview/durable-source-notes.md) for the storage and review workflow.

```bash
pinax note add "iptv-org/iptv" --template source.github --var url=https://github.com/iptv-org/iptv --vault ./my-notes --json
```

## Links and Attachments

```bash
pinax note links "Authentication Plan" --vault ./my-notes --json
pinax note backlinks "Authentication Plan" --vault ./my-notes --json
pinax note orphans --vault ./my-notes --json
pinax note attach "Authentication Plan" ./diagram.png --placement note-folder --embed --vault ./my-notes --json
pinax note attachments "Authentication Plan" --vault ./my-notes --json
```

## Selection Rules

`<note>` supports note id, in-vault path, stem, historical `notes/foo.md` compatibility input, or a unique title. If a title has multiple candidates, an ambiguity error is returned; it does not guess automatically.

User-visible note paths use vault-relative canonical paths: by default, regular notes are output as root-level `foo.md`; after using `--dir work` or moving, they are output as `work/foo.md`. CLI, JSON, agent, search, record ledger, and MCP output all use canonical paths; compatibility paths belong only to the resolver input layer.
