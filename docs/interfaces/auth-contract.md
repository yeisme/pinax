# Token Permission Management Contract

## Overview

The Pinax API server supports three authentication modes:

| Mode | Startup Parameter | Description |
|------|----------|------|
| Temp token | Default | Generates a temporary token in process memory, outputs it once to stderr, invalid after exit |
| Long-term token | `--token-file` | Loads persisted tokens from `.pinax/tokens/tokens.json` |
| No authentication | `--no-auth` | Does not verify tokens, enforces loopback access |

## Token Model

```go
type TokenRecord struct {
    ID          string                    // pt_<hex16>
    SecretHash  string                    // SHA256(salt + secret)
    Salt        string                    // Random hex
    Scope       map[TokenScope]ScopeTarget
    Label       string
    CreatedAt   string                    // RFC3339
    ExpiresAt   string                    // RFC3339, optional
    LastUsedAt  string
    RotatedFrom string                    // Source ID for rotation
    CreatedBy   string                    // "auto" | "manual" | "rotate"
}
```

## Scope Definition

| Scope | Description |
|-------|------|
| `read` | All GET routes |
| `write` | All mutation routes (POST/PUT/PATCH/DELETE) |
| `admin` | Token management itself |

### ScopeTarget

```go
type ScopeTarget struct {
    Groups  []string // Empty array = all groups
    Actions []string // Empty array = all actions
}
```

Available route groups: `capabilities`, `folders`, `inbox`, `drafts`, `notes`, `projects`

## Verification Flow

1. Extract the `Authorization: Bearer <secret>` header
2. Compare `SHA256(salt + secret)` with `secret_hash`
3. Check whether `ExpiresAt` has expired
4. Look up the route group and required scope
5. Verify that the token scope covers the current route group
6. Record the audit log
7. Allow the request

## CLI Commands

```bash
pinax token create --label my-agent --scope read,write --groups notes,folders --expires 30d
pinax token list
pinax token revoke pt_a8f3c2d4e5f6g7h8
pinax token rotate pt_a8f3c2d4e5f6g7h8 --label my-agent-v2
```

## Audit Log

Written to `.pinax/events/api-audit.jsonl`, in NDJSON format:

```jsonl
{"ts":"2026-06-10T12:34:56Z","token_id":"pt_a8f3c2","method":"GET","path":"/v1/notes/note-001","scope":"read","group":"notes","status":200}
```

The audit log does not contain the token secret, request body, or response body.

## Security Constraints

- The token secret is output in plaintext only once during `create`; only the hash is stored in the file
- The `.pinax/tokens/tokens.json` file permissions must be 0600
- `--no-auth` mode enforces checking that RemoteAddr is loopback (127.0.0.1 or [::1])
- Non-loopback requests return 403 directly
