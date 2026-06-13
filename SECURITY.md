# Security Policy

Pinax is local-first software. It stores user notes in a local Markdown vault and keeps machine-authored projections under `.pinax/`. Security reports should focus on vulnerabilities that can expose note content, credentials, provider payloads, local files, sync metadata, or command output contracts.

## Reporting a Vulnerability

Until a dedicated public security contact is published, please open a private security advisory on GitHub for this repository if available. If private advisories are not available, contact the project owner through the least-public channel you have access to and include only a minimal reproduction. Do not paste real secrets, real note bodies, raw provider payloads, cookies, or Authorization headers into public issues.

Include:

- Affected command, API route, MCP tool, or sync transport.
- Expected vs. observed behavior.
- Minimal reproduction using a temporary vault.
- Whether stdout, stderr, JSON/agent output, receipts, events, fixtures, or logs exposed sensitive data.

## Sensitive Data Rules

Pinax must not expose these values in stdout, stderr, events, receipts, logs, fixtures, docs, or OpenSpec evidence:

- Provider tokens, webhook URLs, cookies, Authorization headers, API keys, passwords, or secret refs with embedded secret values.
- Raw provider payloads or provider stderr that may contain credentials.
- Plaintext note bodies in bounded agent/MCP/dashboard/remote projections.
- Hidden prompts, private tool arguments, or model-internal reasoning.

## Local API and MCP Scope

`pinax api serve` binds to localhost by default and is intended as a local projection adapter, not a public hosted API. MCP tools are read-only unless a future spec explicitly introduces a write surface with approval gates.

## Cloud Sync Scope

Cloud Sync transports exchange encrypted blobs, manifests, and revision metadata. `remote_write=true` must only be emitted after a durable revision commit and local sync-state evidence. A failed, dry-run, unsupported, or uncertain transport path must report `remote_write=false`.
