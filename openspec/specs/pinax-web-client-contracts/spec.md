# Pinax Workbench Module Contracts

## Purpose

This spec defines how future Pinax internal UI should consume Pinax bounded projections through `client/yeisme-workbench`. Pinax does not own an Electron/Web standalone client in `cli/pinax`.

## Requirements

### Requirement: Workbench module consumes bounded projections only

Pinax SHALL expose workbench-facing state through Local REST/RPC, CLI JSON, MCP/dashboard shared projections, or copyable real `pinax ...` commands. The Workbench module SHALL NOT read `.pinax/**`, SQLite, LanceDB, token files, provider config, sync state, receipts, or other structured assets directly.

#### Scenario: Workbench module discovers capabilities
- **WHEN** a Workbench module needs to discover Pinax features
- **THEN** it SHALL call Pinax API/capability projections such as `pinax api routes --json`, `pinax api status --json`, or the local API capability registry
- **AND** it SHALL NOT inspect local private file layout.

### Requirement: Workbench module is a thin orchestration surface

Pinax SHALL keep product policy and write authority in the Go CLI/application service. The Workbench module may compose UI, but it SHALL NOT implement a parallel persistence model for notes, boards, graph, search, canvas, provider state, publish state, or proof receipts.

#### Scenario: User triggers a write
- **WHEN** a user requests a write from the Workbench module
- **THEN** Pinax SHALL require the same service-side gates as CLI/API writes, including proof gate, version snapshot, confirmation, receipt, and redaction
- **AND** the frontend SHALL NOT directly mutate vault metadata, `.pinax/**`, indexes, sync state, or provider config.

### Requirement: Static publish renderer remains a delivery artifact generator

The `pinax-web` renderer SHALL generate static publish output from publish-safe bundles. It SHALL NOT become an internal workbench page or standalone app shell.

#### Scenario: Publish output is generated
- **WHEN** `pinax publish build` renders a bundle
- **THEN** it MAY write public delivery files such as `index.html`, `notes/**/index.html`, `tags/**/index.html`, and `pinax-data/**`
- **AND** those files SHALL be treated as user publish artifacts, not Yeisme internal operator UI.

### Requirement: Future UI belongs to client/yeisme-workbench

Internal Pinax UI SHALL be implemented as a `client/yeisme-workbench` Pinax module after module admission declares route namespace, contracts, fixtures, evidence, redaction, and not-owned-here responsibilities.

#### Scenario: Full notes/search/sync UI is requested
- **WHEN** a future request asks for a Pinax notes/search/sync/project UI
- **THEN** the implementation SHALL be routed to `client/yeisme-workbench`
- **AND** `cli/pinax` SHALL provide stable contracts rather than adding a React/Electron app.
