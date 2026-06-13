# Contributing to Pinax

Pinax is a local-first Go CLI. The Markdown vault is the source of truth; `.pinax/` contains CLI-authored projections, receipts, indexes, and configuration. Contributions should preserve that boundary.

## Before You Start

1. Read [docs/README.md](./docs/README.md) and [docs/architecture/architecture-boundaries.md](./docs/architecture/architecture-boundaries.md).
2. For behavior changes, create or update an OpenSpec change under `openspec/changes/pinax-<slug>/`.
3. Keep user-facing examples runnable with the real `pinax` CLI. Do not document local shell aliases or agent-only wrappers.

## Development Setup

Prerequisites:

- Go 1.26.1 or newer.
- Optional: [Task](https://taskfile.dev/) for project shortcuts.

Useful commands:

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```

With Task installed:

```bash
task check
```

`task check` runs formatting checks, lint, tests, build, and OpenSpec validation.

## Code Boundaries

- CLI wiring lives in `cmd/pinax` and `internal/cli`.
- Use-case orchestration lives in `internal/app`.
- Stable domain models live in `internal/domain`.
- Output renderers live in `internal/output` and must preserve default human output, `--agent`, `--json`, `--events`, and `--explain` contracts.
- Redaction lives in `internal/redaction`; do not scatter token or payload filtering through command handlers.
- SQLite/GORM indexes are rebuildable projections, not the Markdown source of truth.

## Safety Rules

- Do not write secrets, provider payloads, raw Authorization/Cookie headers, webhook URLs, or plaintext note bodies into logs, fixtures, receipts, docs, or test output.
- Commands that write Markdown, `.pinax/`, provider state, Git/version state, or remote sync state must have explicit approval gates such as `--yes`, `--dry-run`, or snapshot requirements where appropriate.
- Local REST/RPC and MCP surfaces must reuse application services. They must not bypass CLI/service write gates.
- Cloud Sync `remote_write=true` is only valid after a durable revision commit and local sync-state evidence.

## Pull Request Checklist

- [ ] Added or updated focused tests for changed behavior.
- [ ] Updated OpenSpec specs/tasks when behavior changed.
- [ ] Updated README or docs when user-visible commands, status, or workflows changed.
- [ ] Ran `task check` or the documented fallback commands.
- [ ] Verified new output does not leak secrets or mix diagnostics into machine stdout.

## License

A public open-source license has not been selected yet. Do not assume reuse rights until a `LICENSE` file is added by the project owner.
