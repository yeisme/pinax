## 1. Review and Conflict Resolution

- [x] 1.1 Review publish/cloud/output/theme specs for conflicts with the CLI-tool theme.
  - Evidence: Identified default English output drift in docs/specs, cloud/server wording that needed transport-boundary clarification, and publish target scope limited to Pages/Wiki.
- [x] 1.2 Create this alignment change with proposal, design, tasks, and delta specs.
  - Evidence: Added `pinax-cli-sharing-sync-theme-alignment` OpenSpec change.

## 2. Publish Sharing Surfaces

- [x] 2.1 Add failing command tests for `github-gist`, `http`, and `publish serve`.
  - Evidence: `go test ./cmd/pinax -run 'TestPublish(Gist|HTTP|Serve)' -count=1` first failed with `publish_target_invalid` and missing `publish serve` flags.
- [x] 2.2 Extend domain and profile validation for `github-gist`, `http`, `gist`, `http`, endpoint, secret-ref, Gist ID, and visibility fields.
  - Evidence: Added new publish targets/deploy modes and deploy policy parsing; profile validation rejects unsafe HTTP endpoint and non-`env:` secret refs.
- [x] 2.3 Implement Markdown bundle build for Gist/HTTP targets using the existing publish plan, scan, manifest, and receipt flow.
  - Evidence: `publish build --target github-gist|http` writes `pinax-gist.md` and `pinax-publish-manifest.json` and passes focused tests.
- [x] 2.4 Implement Gist deploy through fakeable `gh` CLI and HTTP deploy through fakeable HTTP server.
  - Evidence: Focused command tests verify fake `gh gist create`, fake HTTP POST fields, `--yes` gate, output scan/receipt validation, and no local root leak.
- [x] 2.5 Implement loopback `publish serve` smoke path.
  - Evidence: `publish serve --host 127.0.0.1 --port 0 --once --json` returns `publish.serve` with `served=true`.

## 3. Docs and Spec Alignment

- [x] 3.1 Update publish command docs with Pages/Wiki/Gist/HTTP/serve examples and safety boundaries.
- [x] 3.2 Update product positioning and documentation design to repeat: local vault source of truth, proof loop, share/sync surfaces are delivery/transport only.
- [x] 3.3 Update architecture/output docs so default human output is Chinese and machine fields remain English.
- [x] 3.4 Add delta specs for publish sharing, CLI output language, and Pinax source-of-truth/cloud-server boundary.

## 4. Verification

- [x] 4.1 Run focused RED/GREEN command tests.
  - Evidence: `go test ./cmd/pinax -run 'TestPublish(Gist|HTTP|Serve)' -count=1` passed after implementation.
- [x] 4.2 Run broader publish package tests.
  - Evidence: `go test ./cmd/pinax ./internal/app ./internal/app/publishops -run 'Publish|Theme|Deploy|Serve|Gist|HTTP' -count=1` passed.
- [x] 4.3 Run `openspec validate --all`.
  - Evidence: `openspec validate --all` passed with 40/40 items.
- [x] 4.4 Run `task check` or documented fallback.
  - Evidence: `task check` passed, covering `golangci-lint run`, `golangci-lint fmt --diff`, `go test ./...`, `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`, and `openspec validate --all` with 40/40 items.
