## ADDED Requirements

### Requirement: Pinax SHALL separate direct backup transport, Cloud Sync, and realtime daemon boundaries

Pinax SHALL document and preserve the boundary between CLI-side backup mirror transports, Pinax Cloud server transport, and realtime daemon/conflict behavior.

#### Scenario: S3 direct is not Pinax Cloud storage
- **GIVEN** a user configures S3 direct or rclone direct object-store transport
- **WHEN** Pinax builds a sync or backup mirror plan
- **THEN** Pinax SHALL treat the direct transport as a CLI-side provider-credential backup mirror for encrypted Cloud Sync objects
- **AND** it SHALL NOT claim Pinax Cloud server-side auth, server audit, object lifecycle policy, multi-tenant controls, or rate limiting for direct transport writes
- **AND** Pinax Cloud server transport SHALL remain the server-authenticated path for cloud sync semantics
- **AND** realtime daemon behavior, automatic merge, conflict resolution, and push notification semantics SHALL require separate OpenSpec coverage before implementation

#### Scenario: backup mirror wording does not expand daemon or conflict behavior
- **WHEN** docs, plans, or command help describe backup mirror behavior
- **THEN** Pinax SHALL NOT imply realtime daemon convergence, automatic merge, conflict resolution, or transport-specific push notification behavior
- **AND** `pinax sync daemon` SHALL remain the local realtime automation layer
- **AND** `pinax sync conflicts` SHALL remain the explicit conflict inspection and resolution surface unless a separate OpenSpec change modifies that behavior
