# Design: Vault Projects and S3 Backend Foundation

## Data Ownership

All structured assets are CLI-authored through `internal/app` services:

- `.pinax/projects.json`
  - `schema_version: pinax.projects.v1`
  - `current_project`
  - `projects[]`: `slug`, `name`, `description`, `notes_prefix`, `created_at`
- `.pinax/storage.json`
  - `schema_version: pinax.storage.v1`
  - `backend`: `local` or `s3`
  - `local.root`, when local backend is selected
  - `s3.bucket`, `s3.region`, `s3.prefix`, `s3.endpoint`, `s3.profile`, when S3 backend is selected

Secrets are not stored. S3 credentials are represented only by a profile name or environment expectation. The CLI must not print tokens, access keys, cookies, Authorization headers, or raw provider payloads.

## Command Surface

Project management:

- `pinax project create <slug> --name <name> --description <text> --notes-prefix <prefix> --vault <vault>`
- `pinax project list --vault <vault>`
- `pinax project switch <slug> --vault <vault>`

Storage backend:

- `pinax storage set-local --root <path> --vault <vault>`
- `pinax storage set-s3 --bucket <bucket> --region <region> [--prefix <prefix>] [--endpoint <url>] [--profile <profile>] --vault <vault>`
- `pinax storage status --vault <vault>`
- `pinax storage doctor --vault <vault>`

## Backend Boundary

This change introduces a storage configuration boundary, not a remote sync implementation. S3 support is represented as a typed profile and diagnostics projection. Future upload/download code must use a storage adapter package with fake server/MinIO tests and context-aware timeouts.

## Safety Rules

- `project create` validates slug and notes prefix stay inside the vault.
- Duplicate project slug is idempotent only when the existing project has the same core fields; conflicting definitions return a stable error.
- `project switch` only changes current project in `.pinax/projects.json`.
- `storage set-s3` refuses empty bucket or region.
- `storage doctor` does not connect to the network; it reports configuration completeness and expected credential source.
- All machine output remains stdout-only for the selected mode.
