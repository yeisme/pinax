# organize Command

`pinax organize` is used to organize note structure. Its core principle is: first generate a reviewable plan, then apply low-risk actions only under explicit confirmation and snapshot protection.

It is not the same as “letting an agent automatically modify the entire vault.” By default, `plan` is read-only, `plan --save` only saves the plan file, and actually moving notes requires running `apply --yes`.

## When to Use It

Cases suitable for `organize`:

- Markdown files are scattered in the vault root or non-standard directories, and you want to move them under `notes/`.
- Note paths, titles, kind, status, tags, or project metadata are clearly inconsistent, and you want to see organization suggestions first.
- You want an agent to produce an organization plan based on local evidence, but do not allow it to directly modify note bodies.
- You want to batch-process low-risk file moves while leaving broken links, ambiguous links, and orphan notes for human review.

Cases not suitable for `organize`:

- Only changing the location of one note: use `pinax note move`.
- Only filling in frontmatter metadata: prefer `pinax metadata plan` and `pinax metadata apply`.
- Performing maintenance such as archive, tag, or index rebuild based on health issues: prefer `pinax repair plan` and `pinax repair apply`.
- Automatically rewriting body links, merging duplicate notes, or deleting content: these high-risk actions are not currently performed automatically.

## Main Workflow

### 1. Preview a Plan

```bash
pinax organize plan --vault ./my-notes
pinax organize plan --vault ./my-notes --json
```

Without `--save`, it is read-only and does not write Markdown, `.pinax/`, Git, or remote state. Human output displays the operation preview, mode, risk, actions, source, target, and reasons; use `--json` to view the full structure.

### 2. Save a Plan

```bash
pinax organize plan --vault ./my-notes --save
pinax organize plan --vault ./my-notes --save --json
pinax organize plan --vault ./my-notes --save --agent
```

`--save` writes through the application service to:

```text
.pinax/organize-plans/<plan_id>.json
```

This file is a CLI-authored structured asset. Do not write or edit it by hand. To regenerate a plan, run `pinax organize plan --save` again.

### 3. List Plans

```bash
pinax organize list --vault ./my-notes
pinax organize list --vault ./my-notes --json
```

Use this to confirm the plan id, status, operation count, creation time, expiration time, and saved path.

### 4. Apply a Plan

It is recommended to let `apply` create a pre-organization snapshot directly:

```bash
pinax organize apply --vault ./my-notes --plan organize-abc123 --yes --snapshot-message "Pre-organization snapshot"
```

You can also manually create a snapshot first, then apply:

```bash
pinax version snapshot --vault ./my-notes --message "Pre-organization snapshot"
pinax organize apply --vault ./my-notes --plan organize-abc123 --yes
```

`apply` must satisfy these conditions:

- There is an existing saved plan, usually from `pinax organize plan --save`.
- `--yes` is explicitly passed.
- A version snapshot exists, or `--snapshot-message` is passed so Pinax creates a snapshot first.
- The saved plan still matches the current vault; if notes have changed, `plan_stale` is returned and the plan must be regenerated.

## What Gets Applied

`organize plan --save` may generate multiple kinds of operations, for example:

| operation | Meaning | apply behavior |
| --- | --- | --- |
| `move` | Move or normalize note paths. | Can be automatically applied when low-risk. |
| `tag_patch` | Suggest adding tags. | Kept as plan suggestions. |
| `kind_patch` | Suggest adding kind. | Kept as plan suggestions. |
| `status_patch` | Suggest adding status. | Kept as plan suggestions. |
| `link_resolution` | Broken or ambiguous links require target confirmation. | Manual review; does not automatically modify body text. |
| `link_rewrite` | Body links may need to be rewritten. | Manual review; does not automatically modify body text. |
| `orphan_review` | Orphan notes require human judgment on whether to archive or link them. | Manual review. |
| `attachment_repair` | Attachment references or paths are abnormal. | Handled conservatively according to the planned risk. |
| `manual_review` | Items that cannot be safely judged automatically. | Not automatically applied. |

The current focus of `organize apply` is low-risk moves protected by snapshots. Body rewriting, merging, deletion, duplicate-title disambiguation, and broad folder moves are not performed automatically.

## Relationship to suggest

`pinax organize suggest` is the old entry point. It remains compatible with existing scripts, but is hidden from the main help. The new main path is:

```bash
pinax organize plan --vault ./my-notes --save
pinax organize list --vault ./my-notes
pinax organize apply --vault ./my-notes --plan organize-abc123 --yes --snapshot-message "Pre-organization snapshot"
```

If scripts are still using `suggest`, they can be migrated gradually to `plan --save`. Both should generate reviewable plans rather than bypassing the service to directly modify the vault.

## Output Modes

| Mode | Command | Purpose |
| --- | --- | --- |
| Default English summary | `pinax organize plan --vault ./my-notes` | For humans to quickly scan risks and next steps. |
| JSON envelope | `pinax organize plan --vault ./my-notes --json` | For scripts, tests, and debugging to read the full plan. |
| agent key=value | `pinax organize plan --vault ./my-notes --save --agent` | For agents to read the plan id, operation count, risk counts, and saved path with low token usage. |
| explain | `pinax organize plan --vault ./my-notes --explain` | View an English reviewable explanation. |

Output modes only change the shape of stdout; they do not change whether anything is written. What actually determines writing is `--save`, `apply`, `--yes`, and snapshot parameters.

## Common Errors

| Error | Cause | Handling |
| --- | --- | --- |
| `approval_required` | `apply` does not have `--yes`. | Review the plan, then append `--yes`. |
| Missing snapshot | `apply` will write to the vault, but there is no version protection. | First run `pinax version snapshot ...`, or add `--snapshot-message` to `apply`. |
| `plan_stale` | Vault content changed after the saved plan was generated. | Run `pinax organize plan --save` again, then apply the new plan id. |
| Plan not found | `--plan` is not a valid plan id or path. | Use `pinax organize list --vault ./my-notes` to check currently saved plans. |

## Shortest Usable Workflow

```bash
pinax organize plan --vault ./my-notes --save
pinax organize list --vault ./my-notes
pinax organize apply --vault ./my-notes --plan organize-abc123 --yes --snapshot-message "Pre-organization snapshot"
```

Replace `organize-abc123` with the real plan id from the `organize list` output.
