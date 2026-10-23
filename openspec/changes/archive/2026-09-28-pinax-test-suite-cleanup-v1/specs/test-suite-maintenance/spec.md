## ADDED Requirements

### Requirement: Test consolidation preserves behavioral protection
Test-only cleanup SHALL preserve observable security, compatibility and recovery assertions while placing direct library tests with their owning package.

#### Scenario: Evidence helper tests are relocated
- **WHEN** direct evidence.Run and Redact tests leave the Pinax e2e package
- **THEN** the tool package still verifies complete artifacts, original failure exit code, schema, success labels and independent leakage checks

#### Scenario: Implementation detail assertions are removed
- **WHEN** the full HTML digest and source substring scans are removed
- **THEN** page contracts, external-reference/body safety and real sync conflict approval/receipt tests remain and pass
