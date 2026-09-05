## MODIFIED Requirements

### Requirement: Current federated catalog
Pinax SHALL consume promptrepo v0.5.0 and document the official public source while preserving the separation between external promptrepo references and local pinax://prompt/ assets.

#### Scenario: Shared official profile
- **WHEN** the official profile exists under the shared promptrepo roots
- **THEN** Pinax can inspect it without copying credentials or source state into the vault
