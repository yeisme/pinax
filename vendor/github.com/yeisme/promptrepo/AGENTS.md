# promptrepo agent instructions

## Scope and architecture

`promptrepo` is the canonical public, pure-Go module
`github.com/yeisme/promptrepo`. It owns the public contracts, embedded engine,
source adapters, and conformance fixtures for user-scoped prompt repositories.
It does not own prompt execution, provider calls, domain assets, a server, or
credentials.

Do not add dependencies on Template Registry server/internal packages. Keep
the default build pure Go and compatible with `CGO_ENABLED=0`.

## Compatibility and security

- Preserve exported API, state paths/schema, digests, error codes, exact refs,
  and JSON field semantics unless an OpenSpec change documents migration,
  deprecation, consumer impact, and rollback.
- The private `sdk/go/promptrepo/v0.1.0` source at
  `e59e5a060b98125e55197cb9f5ed9179cdacc46a` is immutable compatibility
  baseline.
- Never commit credentials, signed URLs, private catalogs, prompt bodies, raw
  provider payloads, or hidden prompts.
- Built-in source adapters must keep paths bounded and Git invocations
  argument-based; do not use shell interpolation.

## Validation

```bash
go mod verify
go test ./...
go vet ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build ./...
openspec validate promptrepo-public-sdk-extraction-v1 --strict --no-interactive
```

Commit, tag, publish, and release remain root-maintainer actions. They are
allowed only when the current user explicitly authorizes the exact repository
and release operation; implementation workers must never perform them.

## Skill routing

- Use `yeisme-prompt-repository-router` to distinguish public SDK/contract work here from official content in `data/yeisme-prompt-templates`, Registry service state in `backend-server/template-registry`, and domain execution in consuming products.
- Use `yeisme-evolutionary-change-policy` for exact refs, template address, digests, schema, error codes, interfaces, or compatibility windows.
- Use `backend-system-workflow` for embedded engine, source adapters, locking, atomic writes, and bounded Git/S3 behavior; use `cso` for credential, path, prompt-body, and trust-boundary review.
