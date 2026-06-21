## 1. Contract and CLI Slice

- [x] 1.1 Add failing command-level coverage for collection import/diff/doctor/export and graph rebuild/query.
  - Validation: `go test ./cmd/pinax -run TestCollectionImportDiffDoctorExportAndGraphCommands -count=1`
  - Evidence: Failed first with `unknown command "collection" for "pinax"`.

- [x] 1.2 Add content bundle parser and app service workflow.
  - Validation: `go test ./cmd/pinax -run TestCollectionImportDiffDoctorExportAndGraphCommands -count=1`
  - Evidence: Passed after adding `internal/contentbundle`, app service methods, and CLI wiring.

- [x] 1.3 Add bounded graph projection and query workflow.
  - Validation: `go test ./cmd/pinax -run TestCollectionImportDiffDoctorExportAndGraphCommands -count=1`
  - Evidence: Passed with `graph.rebuild` writing `.pinax/graph/prompt_graph.json` and `graph.query` omitting prompt bodies.

## 2. Docs and Validation

- [x] 2.1 Document collection and graph command surfaces.
  - Validation: `rg -n "pinax collection|pinax graph|pinax.content_bundle.v1|prompt_graph" docs/commands openspec/changes/pinax-content-collection-pipeline`

- [x] 2.2 Run full quality gate.
  - Validation: `task check`
  - Evidence: Passed on 2026-06-21; covered `golangci-lint fmt --diff`, `golangci-lint run`, `go test ./...`, sidecar tests, build, and `openspec validate --all`.
