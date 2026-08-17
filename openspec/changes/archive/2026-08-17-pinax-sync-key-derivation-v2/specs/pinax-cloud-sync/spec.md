# Capsa sync key derivation v2 deltas

## ADDED Requirements

### Requirement: Sync key derivation SHALL use per-secret salt and current iteration guidance

The sync encryption key SHALL be derived with PBKDF2-SHA256 at no fewer than 600,000 iterations over a salt derived from the shared secret itself, so devices sharing a secret derive the same key while distinct secrets get distinct salts.

#### Scenario: New envelopes use the v2 derivation

- **WHEN** a push encrypts a blob or manifest
- **THEN** the envelope SHALL carry the KeyID of the v2 derivation

#### Scenario: Legacy envelopes remain readable

- **WHEN** a pull reads an envelope whose KeyID belongs to the pre-v2 derivation
- **AND** the same secret is configured
- **THEN** decryption SHALL succeed through the legacy fallback key without any migration step

#### Scenario: Foreign keys fail closed

- **WHEN** an envelope's KeyID matches neither the active nor a legacy key for the configured secret
- **THEN** decryption SHALL fail with a key ID mismatch error
