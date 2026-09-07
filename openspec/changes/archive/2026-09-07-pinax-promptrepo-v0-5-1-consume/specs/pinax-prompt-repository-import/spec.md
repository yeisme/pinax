## MODIFIED Requirements

### Requirement: Pinax SHALL consume promptrepo v0.5.0 with the official public source documented
Pinax SHALL consume promptrepo v0.5.1 and document the official public source while preserving the separation between external promptrepo references and local `pinax://prompt/` assets.

#### Scenario: Shared official profile
- **WHEN** the official profile exists under the shared promptrepo roots
- **THEN** Pinax can inspect it without copying credentials or source state into the vault.
