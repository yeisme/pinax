# Go Development Ecosystem Design

Pinax is centered on a Go CLI, with the goals of being local-first, distributable, testable, and reliably drivable by agents. The development ecosystem should let humans and agents use the same set of entry points, avoiding ad hoc command construction for each task.

## Entry Commands

Pinax uses Taskfile as the development task aggregation layer while task implementation remains grounded in the Go toolchain.

```bash
task build
task test
task check
task openspec
task clean
```

If `task` is not installed, you can run the equivalent commands directly:

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```

## Go Module Boundaries

Default directory structure:

```text
cmd/pinax/              Cobra entry point and command wiring
internal/cli/           Future-migratable Cobra command factory and dependency wiring
internal/app/           Application service / use case orchestration
internal/domain/        Stable domain models, state machines, and command projection
internal/config/        Viper defaults, env, project config, validate
internal/output/        summary, --agent, --json, --events, --explain renderer
internal/redaction/     token, webhook, raw payload, trace redaction
internal/runtime/       clock, filesystem, process runner, context/cancellation
internal/vault/         Markdown vault repository
internal/index/         SQLite/GORM index projection repository
internal/git/           Git adapter and snapshot plan
internal/provider/      CLI-backed Provider interface
internal/sync/          diff/pull/push/conflict state machine
internal/briefing/      daily-hot-notes workflow, evidence, scoring, review queue
internal/mcpserver/     pinax mcp serve stdio surface
tests/e2e/              testscript command e2e
testdata/script/        fixture file tree and golden stdout/stderr
```

## Dependency Defaults

- CLI: Cobra / pflag.
- Configuration: Viper, introduced only in `internal/config`; the command layer does not read configuration files directly.
- Persistence: GORM, with SQLite as the default local storage for indexes and projections.
- Markdown/frontmatter: Prefer stable libraries; when entering an implementation change, record the selection rationale and fixtures.
- Command e2e: `github.com/rogpeppe/go-internal/testscript`.
- External systems: Prefer fake executables and process adapters; tests must not depend on real tokens, the real public internet, or the user's vault.

## Output Contract

Every user entry point must render from the same command projection:

- Default human output is Chinese for this subproject.
- `--agent` low-token `key=value`.
- `--json` single JSON envelope.
- `--events` NDJSON.
- `--explain` decision explanation.

Machine output stdout may contain only machine formats; diagnostics, progress, provider stderr, and logs are written to stderr.

## Layered Testing

| Layer | Scope | Default Tool |
| --- | --- | --- |
| unit | domain rule, projection, redaction, slug, score | Go `testing` table-driven tests |
| integration | app service + repository + temp vault / SQLite | Go `testing`, `testing/fstest`, temporary directories |
| component | CLI command + fake provider + temp Git repo | `testscript` |
| e2e | complete user workflow for the `pinax` binary | `testscript` |
| performance | index, search, briefing scoring, provider process overhead | Go benchmark, `task perf-*` to be added later |

## Task Slice Order

1. Go dev ecosystem: Taskfile, CI baseline, testscript harness, output projection skeleton.
2. Local Vault Workbench: `init`, `doctor`, `note new/list/show`, frontmatter, and validate.
3. Index and Search: GORM repository, tag/link/backlink/search.
4. Version Safety: status, snapshot plan, changed paths, restore plan, and optional Git backend.
5. CLI-backed Provider: external CLI capability probes and fake executable fixtures.
6. Sync Engine: diff/pull/push/conflict queue, dry-run/yes gate.
7. Agent Surface: `--agent`, `--json`, `--events`, `--explain` contract tests.
8. MCP Read/Plan: stdio MCP resources/tools, read-only and dry-run by default.
9. Daily Briefing: research evidence ledger, scoring, review queue, webhook delivery.

## Quality Gates

Before committing, run at least:

```bash
task check
```

If `task` is not installed, run:

```bash
gofmt -w cmd internal
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```
