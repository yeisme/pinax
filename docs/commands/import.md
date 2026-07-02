# import Command

`pinax import` imports external Markdown files or directories into a Pinax vault. The current main subcommand is `import markdown`.

## Usage

```bash
pinax import markdown ./source --dry-run --vault ./my-notes --json
pinax import markdown ./source --group research --kind reference --status active --conflict rename --yes --vault ./my-notes --json
pinax import markdown ./source/beta.md --conflict overwrite --yes --vault ./my-notes --json
```

## Key Parameters

| Parameter | Purpose |
| --- | --- |
| `--group`, `--folder`, `--kind`, `--status`, `--tags` | Add organizational metadata to imported notes. |
| `--conflict skip|rename|overwrite` | Controls target conflicts. Default is `skip`. |
| `--dry-run` | Only outputs the import plan; does not write to the vault. |
| `--yes` | Confirms execution of import writes. |

## Write Boundaries

`--dry-run` does not write notes, receipts, Git state, or provider state. A real import must explicitly use `--yes`, and writes `.pinax/receipts/import-*.json` through the service.

## Agent Brain Ingest Contract

All Agent Brain ingest paths must enter through service-owned commands before they become searchable context. The current contract is:

| Source | Current entry point | First step | Confirmed write evidence | Body exposure |
| --- | --- | --- | --- | --- |
| Markdown file or directory | `pinax import markdown ./source --dry-run --vault ./my-notes --json` | Dry-run import plan. | `.pinax/receipts/import-*.json` after `--yes`. | Bounded projection; source body is not echoed in machine output. |
| Inbox capture | `pinax inbox capture <title> --vault ./my-notes --json` | Explicit capture command. | Service-authored Markdown note metadata. | Caller-provided body is written to the note, not repeated as raw evidence. |
| Journal/briefing | `pinax journal ...` and `pinax briefing run --dry-run --vault ./my-notes --json` | Preview or dry-run where available. | Service-authored note, receipt, or delivery evidence after explicit confirmation. | Provider/webhook payloads and tokens are redacted. |
| Future email/calendar/webhook/shortcut/Zapier intake | No current production command. | Must add dry-run or preview first. | Must write normalized Markdown/assets and redacted receipts through the application service. | Raw external payloads, email headers with secrets, webhook tokens, provider stderr, and API payloads must not enter stdout/stderr/evidence. |

Source identity should include source kind, source path or provider-neutral external id, import time, conflict/dedupe outcome, and receipt id when available. Dedupe decisions must appear as plan facts before writes; unresolved future connectors stay planned until their owner implements fake-fixture tests and redacted evidence.
