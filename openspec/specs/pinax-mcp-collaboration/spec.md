# pinax-mcp-collaboration Specification

## Purpose
TBD - created by archiving change pinax-mcp-collaboration-v1. Update Purpose after archive.
## Requirements
### Requirement: Additive collaboration access
Pinax SHALL expose opt-in collaboration tools and preserve every existing readonly tool and JSON resource contract.

#### Scenario: Default connection
- **WHEN** the owner does not enable collaboration
- **THEN** interaction tools are not enabled and existing readonly discovery remains unchanged

#### Scenario: Explicit body access
- **WHEN** a caller requests the body of a specific note
- **THEN** the server requires owner body permission and an explicit read, summarize, or edit intent, treating that intent as a caller assertion

### Requirement: Review-bound recoverable daily writes
Pinax SHALL provide preview and apply for create, append, replace, tags, and archive through its application service with revision checks, idempotency, and recovery references.

#### Scenario: Preview cancellation
- **WHEN** a user discards a preview
- **THEN** no formal note or operation ledger is changed by the preview

#### Scenario: Changed revision or destination
- **WHEN** a note changes after preview or the create destination becomes occupied
- **THEN** apply refuses to overwrite or silently select another destination and requires a new preview

#### Scenario: Repeated or uncertain write
- **WHEN** a caller repeats the same operation after a timeout or reconnect
- **THEN** the original durable outcome is returned without another write; uncertain outcomes remain inspectable and cannot be automatically replayed with a new identity

### Requirement: Markdown and structured interaction
Pinax SHALL provide readable Markdown and structured data over the same domain results without advertising or serving MCP Apps UI resources.

#### Scenario: Ordinary MCP client
- **WHEN** a client connects without any UI capability
- **THEN** search, read, preview, apply and status are usable through tools and Markdown

#### Scenario: Retired HTML resource
- **WHEN** a client requests the former ui://pinax/collaboration-v1.html resource
- **THEN** the request returns resource_not_found and discovery contains no UI extension or UI resource metadata

#### Scenario: Untrusted note content
- **WHEN** a note contains scripts, raw HTML, images or code fences
- **THEN** the Markdown output quotes them as untrusted note content

#### Scenario: Protocol evidence
- **WHEN** automated protocol checks complete
- **THEN** evidence does not claim real client conversation acceptance

