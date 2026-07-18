## ADDED Requirements

### Requirement: SQL and Dataview expose stable object identity
Pinax SQL, Dataview-compatible queries and database saved views SHALL expose `object_id` and the note compatibility alias `note_id` alongside current path, title, project, group, folder, kind, status, Tags and custom properties.

#### Scenario: Query a project by Tag and object ID
- **WHEN** a user executes a bounded query selecting project notes and Tag relations
- **THEN** every result row SHALL include a stable object ID and current path, and internal joins SHALL use object identity rather than assuming path permanence.

### Requirement: Existing query surfaces remain compatible during migration
Existing safe query syntax using path, title, project, classification and Tag fields SHALL continue to work while the index migrates to object-first relations.

#### Scenario: Saved view created before identity migration
- **WHEN** a saved view that does not select object ID runs after migration
- **THEN** Pinax SHALL preserve its result columns and filtering semantics while using canonical object relations internally.
