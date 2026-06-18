# Pinax Demo Proof Loop

This demo shows the core Pinax promise with a synthetic local vault: diagnose a messy Markdown vault, save a reviewable plan, take a protective snapshot, apply only low-risk changes, and restore a file if the result is not wanted.

The fixture lives at `examples/messy-vault/`. It contains only synthetic notes and intentionally includes six issue classes:

- broken wikilink: `notes/research/auth-design.md`
- orphan note: `notes/research/api-notes.md`
- missing metadata: `notes/research/meeting-2026.md`
- duplicate title: `notes/projects/pinax-plan.md` and `notes/projects/pinax-plan-2.md`
- empty body: `notes/inbox/random-thought.md`
- stale note: `notes/archive/old-spec.md`

## Run the demo on a copy

Run from `cli/pinax/`. The first block copies the fixture so the checked-in example remains clean.

```bash
rm -rf temp/demo-messy-vault
mkdir -p temp
cp -a examples/messy-vault temp/demo-messy-vault
touch -d "120 days ago" temp/demo-messy-vault/notes/archive/old-spec.md
```

`stale_note` uses file modification time. Git does not preserve mtimes, so the `touch` command makes the stale note deterministic after clone.

## 1. Diagnose

```bash
go run ./cmd/pinax vault doctor --vault ./temp/demo-messy-vault --json
```

Expected summary:

- command: `vault.doctor`
- status: `partial`
- issue codes include `broken_link`, `orphan_note`, `missing_tags`, `duplicate_title`, `empty_note`, and `stale_note`
- output is a bounded projection: facts, issue summaries, evidence strings, and next commands; it does not dump full note bodies or provider payloads

## 2. Save a repair plan

```bash
go run ./cmd/pinax repair plan --save --vault ./temp/demo-messy-vault --json
```

Expected summary:

- command: `repair.plan`
- status: `partial`
- `.pinax/repair-plans/<plan-id>.json` is written inside the copied vault
- low-risk operations include metadata/index/archive-status maintenance
- broken links, orphan notes, duplicate titles, and empty notes remain manual-review operations

Capture the plan id for the next step:

```bash
PLAN_ID=$(python3 - <<'PY'
import json, pathlib
plans = sorted(pathlib.Path('temp/demo-messy-vault/.pinax/repair-plans').glob('*.json'))
print(plans[-1].stem)
PY
)
```

## 3. Take a protective snapshot

```bash
go run ./cmd/pinax version snapshot --vault ./temp/demo-messy-vault --message "before demo repair" --json
```

Expected summary:

- command: `version.snapshot`
- status: `success`
- `.pinax/version/snapshots/<snapshot-id>.json` and `.pinax/last_snapshot` are written
- this snapshot is local evidence that a write was protected before apply

## 4. Apply low-risk operations

```bash
go run ./cmd/pinax repair apply --vault ./temp/demo-messy-vault --plan "$PLAN_ID" --yes --json
```

Expected summary:

- command: `repair.apply`
- status: `success`
- `applied` is non-zero and `skipped` covers manual-review operations
- `notes/research/meeting-2026.md` is normalized with a low-risk metadata patch
- `auth-design.md`, `api-notes.md`, both duplicate-title project notes, and `random-thought.md` are unchanged because they require human review

You can confirm manual-review issues remain visible:

```bash
go run ./cmd/pinax vault doctor --vault ./temp/demo-messy-vault --json
```

Expected issue codes still include `broken_link`, `orphan_note`, `duplicate_title`, and `empty_note`.

## 5. Restore one file

`version restore` needs a readable historical revision. For a local demo, seed a local Git baseline before applying the repair plan. The E2E test does this automatically; for a manual demo, run this on a fresh copy before step 2:

```bash
git -C temp/demo-messy-vault init
git -C temp/demo-messy-vault config user.email pinax-demo@example.invalid
git -C temp/demo-messy-vault config user.name "Pinax Demo"
git -C temp/demo-messy-vault add .
git -C temp/demo-messy-vault commit -m "dogfood demo baseline"
```

After step 4, generate and apply a restore plan for the metadata-patched file:

```bash
go run ./cmd/pinax version restore notes/research/meeting-2026.md --revision HEAD --plan --vault ./temp/demo-messy-vault --json

RESTORE_ID=$(python3 - <<'PY'
import json, pathlib
plans = sorted(pathlib.Path('temp/demo-messy-vault/.pinax/restore-plans').glob('*.json'))
print(plans[-1].stem)
PY
)

go run ./cmd/pinax version restore apply --vault ./temp/demo-messy-vault --plan "$RESTORE_ID" --yes --json
```

Expected summary:

- `version.restore` writes a restore plan and reports `git_commit`
- `version.restore.apply` reports `local_write=true` and `remote_write=false`
- `notes/research/meeting-2026.md` returns to its baseline content
- restore writes a receipt under `.pinax/receipts/` in the copied vault

## Safety guarantees to call out

- The fixture is synthetic; it contains no real names, credentials, webhook URLs, cookies, or provider payloads.
- Read stages are bounded projections. They expose issue facts and next actions, not complete note bodies.
- Writes require a saved plan and explicit `--yes`.
- Apply is protected by local snapshot evidence.
- Restore is also plan-based and local-only; it does not call a provider, MCP server, Cloud Sync backend, or network service.

## Demo tips

- Lead with the messy-vault problem: agents can help clean a vault, but they need a control loop before they write.
- Show `doctor` first so the audience sees all six issue classes at once.
- Open the saved repair plan if someone asks what will be changed; it is plain JSON under `.pinax/repair-plans/` in the copied vault.
- Emphasize that manual-review issues remain unchanged after `repair apply`.
- End with restore so the audience sees that Pinax can recover from an unwanted local write through a CLI-authored path.
