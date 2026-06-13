# API Cache Contract

## Overview

The Pinax API server provides HTTP caching semantics (Cache-Control / ETag / 304) for read-only GET routes.

## Cache Policies

| Route | Max-Age | Scope |
|------|---------|-------|
| `/v1/capabilities` | 300s | public |
| `/v1/folders`, `/v1/folders/` | 60s | private |
| `/v1/notes/` | 30s | private |
| `/v1/inbox`, `/v1/inbox/` | 10s | private |
| `/v1/drafts`, `/v1/drafts/` | 10s | private |
| `/v1/projects/` | 30s | private |

## Behavior Rules

1. **Only cache GET requests**: POST, PUT, PATCH, DELETE, and RPC calls are not cached.
2. **ETag calculation**: `SHA256(response body hex)`, wrapped in double quotes.
3. **Conditional requests**: The client sends the `If-None-Match` header; when it matches, return `304 Not Modified` with an empty body.
4. **Cache-Control**: `<scope>, max-age=<seconds>`, where scope is `public` or `private`.
5. **No external cache introduced**: Caching occurs at the HTTP semantics layer; actual caching is implemented by browsers/clients/reverse proxies.

## Configuration

Cache policies are derived from the default configuration. Redis, memcached, or external cache services are not introduced.

In the future, the default values can be overridden through the `api.cache.policies` configuration section.

## Implementation Locations

- Cache middleware: `internal/api/cache.go`
- Route registration: wrapped with `cacheMiddleware` in Handler() in `internal/api/http.go`
