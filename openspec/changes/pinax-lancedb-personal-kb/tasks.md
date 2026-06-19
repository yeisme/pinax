## 1. Contract and CLI Slice

- [x] 1.1 Add failing CLI contract tests for `pinax kb import/rebuild/search/context`.
  - Validation: `go test ./cmd/pinax -run 'TestKB' -count=1`
  - Evidence: failed before implementation with missing `kb` command.

- [x] 1.2 Implement additive `pinax kb` command group through app services.
  - Validation: `go test ./cmd/pinax -run 'TestKB' -count=1`
  - Evidence: passed after wiring CLI, app service, and semantic projection adapter.

## 2. Semantic Projection

- [x] 2.1 Add semantic chunking, provider, and local LanceDB-shaped store boundary.
  - Validation: `go test ./cmd/pinax -run 'TestKB' -count=1`
  - Evidence: superseded by 2.3; initial vertical slice proved chunking and bounded search contracts.

- [x] 2.2 Preserve agent-safe bounded output.
  - Validation: `go test ./cmd/pinax -run 'TestKB' -count=1`
  - Evidence: context test rejects full body/raw body sentinel leaks.

- [x] 2.3 Replace LanceDB-shaped JSONL with Python LanceDB sidecar.
  - Validation: `go test ./cmd/pinax -run 'TestKB' -count=1`; `PYTHONPATH=tools/pinax-lancedb-sidecar/src temp/kb-sidecar-venv/bin/python -m unittest discover tools/pinax-lancedb-sidecar/tests`.
  - Evidence: `backend=lancedb` requires/calls `pinax-lancedb-sidecar`; the Python sidecar writes and searches a real local LanceDB table.

- [x] 2.4 Add provider and import safety hardening.
  - Validation: `go test ./cmd/pinax -run 'TestKB' -count=1`.
  - Evidence: unknown providers return `provider_invalid`; duplicate import titles produce distinct target Markdown files.

## 3. Quality Gate

- [x] 3.1 Run focused package tests, Python sidecar tests, and OpenSpec validation.
  - Validation: `go test ./internal/semantic ./internal/app ./internal/cli ./cmd/pinax -run 'KB|kb' -count=1`; `python3 -m compileall tools/pinax-lancedb-sidecar`; `PYTHONPATH=tools/pinax-lancedb-sidecar/src temp/kb-sidecar-venv/bin/python -m unittest discover tools/pinax-lancedb-sidecar/tests`; `openspec validate --all --strict`.
  - Evidence: Go focused tests and Python sidecar tests passed on 2026-06-19; final OpenSpec validation is part of full check.

- [x] 3.2 Run full Pinax check.
  - Validation: `task check`.
  - Evidence: passed on 2026-06-19; covered fmt-check, lint, `go test ./...`, build, and `openspec validate --all`.

## CodeGraph Evidence

- 2026-06-19: `codegraph diff-impact` reported the root command wiring as the main changed call path, with 21 transitive CLI callers affected.
- 2026-06-19: `codegraph check` passed with existing complexity warnings and no failed boundary/cycle/import rules.
