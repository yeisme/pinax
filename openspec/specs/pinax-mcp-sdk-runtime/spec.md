# pinax-mcp-sdk-runtime Specification

## Purpose
TBD - created by archiving change pinax-mcp-official-sdk-v1. Update Purpose after archive.
## Requirements
### Requirement: Official protocol runtime candidate
The future Pinax SDK migration SHALL use the official Go MCP SDK for protocol handling while keeping domain behavior in the existing application service.

#### Scenario: Candidate verification
- **WHEN** an SDK runtime candidate is implemented
- **THEN** strict clients must discover and invoke the existing ordinary MCP tools before any default runtime switch

### Requirement: Preserve text interaction scope
The migration SHALL preserve Markdown, structured results, permission checks and operation recovery, and SHALL NOT reintroduce the canceled HTML or MCP Apps capabilities.

#### Scenario: Client without UI extensions
- **WHEN** a client uses the future SDK runtime without UI support
- **THEN** note search, explicit body read, preview, apply and status remain available according to owner permissions

