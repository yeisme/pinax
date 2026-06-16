## ADDED Requirements

### Requirement: Pinax SHALL use GORM Gen for local index database access

Pinax SHALL route ordinary `.pinax/index.sqlite` projection reads and writes through GORM Gen generated DAO code. The local index remains a rebuildable projection of Markdown vault content, but field references, predicates, ordering and writes SHALL be type-backed rather than hardcoded SQL or direct GORM business chains.

#### Scenario: Index rebuild writes through generated DAO
- **GIVEN** a vault contains notes, tags, links, attachments, properties and assets
- **WHEN** `pinax index rebuild --vault <vault>` updates `.pinax/index.sqlite`
- **THEN** projection rows SHALL be created through generated DAO methods
- **AND** ordinary rebuild code SHALL NOT call `database/sql`, `Raw`, `Exec`, or hardcoded SQL verb strings.

#### Scenario: Search and lookup read through generated DAO
- **GIVEN** the local index exists
- **WHEN** Pinax lists notes, searches, resolves backlinks, checks assets, or serves readonly MCP resources
- **THEN** queries SHALL use generated DAO fields and predicates
- **AND** output ordering, machine fields, and stable error codes SHALL remain compatible with existing behavior.

#### Scenario: Schema exceptions stay centralized
- **GIVEN** Pinax needs connection, migration, transaction, or schema metadata behavior
- **WHEN** GORM Gen cannot express the operation safely
- **THEN** the exception SHALL live in a documented helper or migration boundary
- **AND** guard tests SHALL prevent ordinary index business files from reintroducing raw SQL or direct GORM query chains.
