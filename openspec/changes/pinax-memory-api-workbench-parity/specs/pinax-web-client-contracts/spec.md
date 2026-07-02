## ADDED Requirements

### Requirement: API Workbench surfaces memory and capability workflows

The Local API workbench SHALL provide a browser UI for memory records and capability discovery without moving write behavior into the read-only vault dashboard.

#### Scenario: Workbench shows memory operations
- **WHEN** a user opens `/workbench` on `pinax api serve`
- **THEN** the page provides controls for capture dry-run, confirmed capture, records, recall, context, and stats using Local API routes

#### Scenario: Workbench shows route metadata from capabilities
- **WHEN** the workbench loads capability explorer data
- **THEN** it displays each capability's id, command, REST/RPC route, read-only status, write gate, and copy command from `/v1/capabilities`

#### Scenario: Vault dashboard remains read-only
- **WHEN** a user opens `pinax vault dashboard`
- **THEN** the dashboard does not expose direct memory write controls
