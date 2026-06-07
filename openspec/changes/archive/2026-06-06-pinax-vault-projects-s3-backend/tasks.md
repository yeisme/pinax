# Tasks: Vault Projects and S3 Backend Foundation

## Implementation

- [x] Add domain structs for project registry and storage backend profile.
- [x] Add app service methods for project create/list/switch with path and duplicate validation.
- [x] Add app service methods for storage set-local, set-s3, status, and doctor.
- [x] Wire Cobra commands under `project` and `storage` with output contract rendering.
- [x] Add tests for JSON/agent/default output, CLI-authored assets, duplicate/conflict handling, and S3 validation.
- [x] Update README and docs with project/storage workflow examples.

## Verification Evidence

- 2026-06-06: `go test ./internal/app ./cmd/pinax -run 'TestProject|TestStorage' -count=1` passed.
- 2026-06-06: `go test ./...` passed.
- 2026-06-06: `task check` passed; validated `spec/pinax` and `change/pinax-vault-projects-s3-backend`, then rebuilt `dist/pinax`.
- 2026-06-06: CLI smoke passed for `pinax project create`, `pinax project list --agent`, `pinax storage set-s3 --json`, and `pinax storage doctor --json`; storage output did not include secret material or stale local backend data.
