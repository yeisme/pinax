## ADDED Requirements

### Requirement: Dashboard displays repair plan drilldowns without writes
Pinax dashboard SHALL expose repair plan summaries and issue drilldowns as readonly views.

#### Scenario: Dashboard shows repair plan summary
- **WHEN** a user opens the dashboard for a vault with a saved repair plan
- **THEN** the dashboard SHALL display plan id, operation counts, risk distribution, expiry, and apply command
- **AND** it SHALL NOT provide a write-capable endpoint or button.

#### Scenario: Dashboard exposes readonly repair data endpoint
- **WHEN** dashboard serves `/api/repair-plans` or equivalent readonly endpoint
- **THEN** it SHALL return redacted repair plan summaries generated through application services
- **AND** non-GET methods SHALL be rejected.
