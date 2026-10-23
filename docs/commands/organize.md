# organize Command

`pinax organize` is used to organize note structure. Its core principle is: first generate a reviewable plan, then apply low-risk actions only under explicit confirmation and snapshot protection.

It is not the same as “letting an agent automatically modify the entire vault.” By default, `plan` is read-only, `plan --save` only saves the plan file, and actually moving notes requires running `apply --yes`.

## Selected-note workflow (experimental)

```bash
pinax inbox review --limit 5 --vault ./my-notes --json
pinax organize plan --note <note-id-a> --note <note-id-b> --vault ./my-notes --json
pinax organize plan --note <note-id-a> --note <note-id-b> --save --vault ./my-notes --json
pinax pipeline describe <plan-id> --vault ./my-notes --json
pinax organize apply --plan <plan-id> --yes --snapshot-message "Before selected organization" --vault ./my-notes --json
```

Repeat `--note` for up to 20 unique canonical objects (up to 100 selector arguments). Exact IDs and vault-relative paths are accepted; ambiguous titles, missing identities, system/archived/discarded notes and unsafe paths are rejected for the whole request. Repeated selectors for the same unique object are deduplicated. Missing identity requires an explicit `pinax record adopt --plan` workflow, never implicit adoption.

The rule engine keeps full-vault relationship context but only selected objects may receive operations. This is **not** automatic folder classification or body rewriting. `--save` writes `pinax.organize_plan.scoped.v1`; without it, preview changes nothing. Older CLIs reject this schema instead of widening its scope. Commands without `--note`, including the hidden `suggest` alias, retain their existing behavior.

Scoped apply always creates a fresh snapshot after whole-selection validation. `--allow-stale` is rejected with `scoped_plan_override_forbidden`. Unrelated edits do not invalidate the plan; selected content drift, path reuse, duplicate targets or destination occupation block it. Moving uses non-overwriting hard-link/unlink on one filesystem; unsupported filesystems leave the source intact and fail rather than falling back to an overwriting rename.

Multi-file writes are not a transaction. `planned → applying → applied|partial|failed` and per-operation outcomes are persisted; interrupted or consumed plans must not be retried. Inspect the original plan/receipt with `pipeline show`, inspect its snapshot with `version history/show`, and explicitly plan recovery without discarding later edits. An evidence failure returns `organize_evidence_incomplete`. Index refresh is explicit (`index_status=not_refreshed`), not silently claimed successful.

The readonly MCP tool `pinax.organize.plan` accepts `note_refs`; it cannot save or apply. The server owns the vault, and owner-local follow-up is not a client filesystem action. No REST/RPC capability is added.

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

### Plan Freshness 与 --allow-stale（plan 新鲜度）

freshness 守卫比较 plan 记录的 facts 摘要与当前 vault 扫描指纹，两者不一致（或 plan 已过期）时 `apply` 返回稳定错误 `plan_stale` 并提示重新 `pinax organize plan --save`，不会产生部分写入；拒绝时 `--events` 输出 `stage.failed`（`reason=plan_stale`）。

- `--allow-stale` 是显式逃生门：跳过 freshness 守卫照常 apply，投影附 `plan_stale_overridden` warning；`--yes` 与 snapshot 要求不变。
- 跨管道统一视图与 freshness 徽标见 [pipeline](./pipeline.md)。

## What Gets Applied

`organize plan --save` may generate multiple kinds of operations, for example:

| operation | Meaning | apply behavior |
| --- | --- | --- |
| `move` | Move or normalize note paths. | Can be automatically applied when low-risk. |
| `tag_patch` | Add frontmatter tags inferred from inline tags. | Applied when planned and approved. |
| `kind_patch` | Suggest adding kind. | Kept as plan suggestions. |
| `status_patch` | Fill missing status with active. | Applied when planned and approved. |
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
