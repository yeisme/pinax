## Design

### Product boundary

Pinax remains a short-lived Go CLI over a local Markdown vault. Publishing and Cloud Sync are not alternate sources of truth:

- `publish` writes generated delivery artifacts under `--out` and deploys only after `--yes`.
- `cloud` and `sync` converge local files across devices through configured transports.
- Server transport can provide auth/audit/policy for encrypted sync artifacts, but the Pinax CLI keeps local vault ownership and local write gates.

### Sharing surfaces

`github-pages`, `github-wiki`, `github-gist`, `http`, and `publish serve` share the same safety path:

1. Load CLI-authored publish profile.
2. Build a read-only plan from vault notes/assets.
3. Reject blocked private/secret/provider/raw payload content.
4. Generate output into explicit `--out`.
5. Scan output and write a receipt with output hash.
6. Deploy only when `--yes` is present and receipt/hash/scan validation passes.

Gist uses the user-installed `gh` CLI so Pinax does not own GitHub token storage. HTTP deploy posts a manifest and content bundle to a configured endpoint; optional auth is passed only through `secret-ref` indirection such as `env:PINAX_SHARE_TOKEN`.

### Local preview

`pinax publish serve` serves an already-built output directory on loopback. It is a preview surface, not a daemon requirement. Tests use `--once` to prove the server can start and serve one request without hanging.

### Output language and theme

Human CLI text is Chinese by default in this subproject. Machine output remains stable and English-keyed:

- `--json`: projection envelope keys and facts are English.
- `--agent`: ASCII `key=value` lines with English keys.
- `--events`: NDJSON event fields are English.
- Error codes and schema names stay English.

Publish site theme design is separate from CLI chrome. The built-in site theme remains local, inspectable, and work-focused: no remote fonts/CDNs/analytics by default, no marketing hero as the primary surface, and content/data comes from publish-safe manifest files only.

### Cloud server boundary

The Pinax repo may include client protocol, fake/local transport, direct object-store transport, and tests for server transport behavior. A production hosted Cloud Server implementation is outside this CLI repo unless a future change explicitly creates a separate service boundary. No app service, CLI command, MCP tool, or provider adapter may bypass local write gates and write remote state directly.

## Risks

- Gist/HTTP deploy could leak local paths through external tool errors. Mitigation: redact process output and keep projection facts path-free.
- HTTP endpoints could be mistaken for arbitrary webhook storage. Mitigation: profile validation allows HTTPS or loopback HTTP only, and secrets are `env:` references.
- `publish serve` could look like a daemon. Mitigation: document it as local preview only and keep it loopback-bound by default.
