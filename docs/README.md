# Pinax Documentation Map

[中文文档地图](./README.zh-CN.md)

This directory is the source of truth for the Pinax subproject's product, design, operations, protocol, implementation, QA, and release documentation. The root repository only keeps cross-project handoff and governance indexes, and does not maintain a mirror of Pinax documentation.

Pinax is the **agent-safe knowledge control plane for your Markdown vault**. Three concepts: the Markdown vault is the user's source of truth, the Proof Loop protects every agent write, and Cloud Sync only coordinates ciphertext. SQLite/GORM is a rebuildable index projection, the version backend only provides version evidence and the snapshot basis for protected workflows, and external platforms are integrated through CLI-backed Provider adapters.

The central guarantee is an [agent-safe boundary](./overview/agent-safe-boundary.md): read commands default to bounded projections (not full note bodies), MCP tools are read-only, and the cloud never stores plaintext or executes local tools.

## Agent-safe Proof Loop

The primary user and agent value of Pinax is a reproducible local proof loop built from five workflows. Every stage stays bounded: projections return facts and next actions, never full note bodies, tokens, or provider payloads; writes only happen through the plan → snapshot → apply → receipt → restore control chain.

- [Demo Proof Loop](./demo-proof-loop.md): copy the synthetic messy vault fixture and run diagnose → plan → snapshot → apply → restore end to end.
- [Documentation Design](./overview/documentation-design.md): explains reader paths, section ownership, command documentation shape, and maintenance rules for Pinax docs.

## Current Status

- Current phase: local-first notebook workflows are usable from the CLI and ready for external developer evaluation.
- Current implementation boundary: supports local init, vault validate, daily/inbox/draft, note add/create/list/read/edit/rename/move/archive/delete/tag, shared `NoteDisplay`, project board workspace, organization-dimension browsing, saved views, SQLite/GORM index, search, `pinax note links`/`pinax note backlinks`/`pinax note orphans`, `search --link-target`, attachments, Markdown import/export, template create/render/validate/delete, metadata plan/apply, repair plan/apply, agent organize plan/list/apply, version snapshot, asset manifest registration/validation/planning, read-only dashboard repair view, read-only MCP, localhost REST/RPC projection adapter, and Cloud Sync over server/file/S3/rclone transports. Provider automation and briefing delivery remain experimental.
- User-visible note paths use vault-relative canonical paths. The default regular note is root-level `foo.md`, and a subdirectory note is `work/foo.md`; the historical `notes/foo.md` is only resolver-compatible input and is not the primary output for CLI, JSON, agent, records, search, or MCP.
- Planning and implementation tracking lives under `openspec/`; external contributors should start with [CONTRIBUTING.md](../CONTRIBUTING.md).

## Bidirectional Relationship Entry Points

- `pinax note links <ref>` shows outgoing links, supporting `--broken-only`, `--kind`, `--include-ignored`, and `--limit`.
- `pinax note backlinks <ref>` shows backlinks, supporting `--include-broken` and `--limit`.
- `pinax note orphans --mode full|no-incoming|no-outgoing` shows fully orphaned notes, notes with no incoming links, or notes with no outgoing links.
- `pinax search <query> --link-target <note-id|path|title|raw-target>` filters search results by relationship target; when the target is ambiguous, it returns `link_target_ambiguous` and does not automatically select a candidate.
- The SQLite/GORM index is only a rebuildable projection; first use `pinax index --vault <vault>` to view the summary. When `index_status=missing|stale`, prioritize running `pinax index refresh --vault <vault>`. For structural anomalies, use `pinax index doctor --vault <vault>` to view issues, and, if necessary, then execute an explicit `rebuild` as prompted. Do not manually write `.pinax/*.json` or index metadata.
- `repair plan`, `organize plan --save`, and the dashboard only generate manual review recommendations for broken/ambiguous/orphan items; MCP relationship tools are read-only and do not write to the vault, `.pinax/`, Git, providers, or remote state.

## Version and Asset Boundaries

- The version backend is the source of version evidence, not the source of truth for user content. The current main path is `pinax version status/snapshot/history/diff/show/changed/restore/backends`; Git is only an optional backend type and hidden compatibility alias, not the user-visible command terminology.
- The asset manifest is CLI-authored metadata used to register vault-relative paths, media types, hashes, linked notes, and validation status; asset files themselves are still ordinary migratable files inside the vault. Both the manifest and the SQLite index can be repaired or rebuilt by the CLI and should not be manually written.
- Sensitive content such as asset payloads, raw diffs, provider payloads, webhook tokens, secret refs, and Authorization/Cookie must not enter stdout, stderr, events, record logs, or fixtures.

## Project Board and Remote Adapter

- `pinax project board show|plan|configure|export` provides a local project workspace. The board is generated from Markdown notes, project metadata, index projections, and saved planning snapshots; the vault remains the source of truth, while TaskBridge and providers are not sources of truth.
- `pinax project item add|move|archive` writes controlled Markdown through the application service. archive must first have `--yes` and a version snapshot; when missing, it returns a stable `approval_required` or `snapshot_required` projection.
- `pinax note read/show --display card|detail|context|body`, project board, dashboard, MCP, REST, and RPC reuse the same `NoteDisplay` projection; the default bounded display does not output the full body.
- `pinax api routes`, `pinax api schema export`, and `pinax api serve --readonly --port 0` are local REST/RPC projection adapters. The server binds to `127.0.0.1` by default and does not provide a public hosted API, CORS, TLS, multi-user permissions, or token auth.
- `pinax prompt` stores reusable `yeisme.prompt_asset.v1` prompt assets, resolves `pinax://prompt/<id>` references, records Pinax-owned lifecycle decisions, and imports metadata-only usage feedback from tools such as Eikona.
- Cloud Sync is a separate distributed sync design: every device keeps a local vault, while the Cloud backend coordinates encrypted revisions, blobs, and conflicts. See [Cloud Sync Architecture](./architecture/cloud-sync-design.md).

## Documentation Sections

- [Agent-Safe Boundary](./overview/agent-safe-boundary.md)
- [Documentation Design](./overview/documentation-design.md)
- [Product Positioning](./overview/product-positioning.md)
- [Architecture Boundaries](./architecture/architecture-boundaries.md)
- [Cloud Sync Architecture](./architecture/cloud-sync-design.md)
- [Go Development Ecosystem Design](./architecture/go-development-ecosystem.md)
- [CLI Output Contract](./interfaces/cli-output-contract.md)
- [Local REST/RPC Contract](./interfaces/remote-api-contract.md)
- [Demo Proof Loop](./demo-proof-loop.md)
- [Command Manual](./commands/README.md)
- [Operations Manual](./operations/local-development.md)
- [Release Packaging](./operations/release-packaging.md)
- [Chinese Documentation Map](./README.zh-CN.md)
- [Contributing](../CONTRIBUTING.md) / [贡献指南](../CONTRIBUTING.zh-CN.md)
- [Security Policy](../SECURITY.md) / [安全策略](../SECURITY.zh-CN.md)

## Command Manual

- [Command Map](./commands/README.md): explains what each root command manages by workflow.
- [prompt](./commands/prompt.md): describes prompt asset lifecycle, `pinax://prompt/<id>` resolution, cross-project boundaries, and feedback import.
- [publish](./commands/publish.md): describes safe GitHub Pages and Wiki publishing surfaces, Hugo/theme use, deploy gates, and why the vault remains the source of truth.
- [organize](./commands/organize.md): describes the organization flow, write boundaries, and snapshot protection for `pinax organize plan/list/apply`.
- [version](./commands/version.md), [asset](./commands/asset.md), [index](./commands/index.md), and other root commands each maintain independent dedicated pages in the [Command Manual](./commands/README.md).

## Validation Entry Points

When only documentation is changed, Go tests are not run by default. After modifying Go code, run:

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
```

If Taskfile is installed, you can also run:

```bash
task check
task release:check
```

Before publishing or handing off release artifacts, run:

```bash
task release:package:validate
```

The package validation target runs GoReleaser in snapshot/no-publish mode, verifies checksums, smokes an extracted archive, checks for SBOM artifacts, and skips unavailable Linux package inspection tools with explicit messages.
