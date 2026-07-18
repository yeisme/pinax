## ADDED Requirements

### Requirement: Vault objects use canonical UUID identities
Pinax SHALL assign every managed vault object a canonical `object_id` generated once as UUIDv7 and SHALL treat path, title, project, group, folder, kind, status and tags as mutable object properties rather than identity inputs. Note objects SHALL expose the same canonical value through the compatibility field `note_id`.

#### Scenario: Create a new note with one stable identity
- **WHEN** a user or Agent creates a note through an approved Pinax command
- **THEN** Pinax SHALL allocate exactly one UUIDv7 object identity, persist it through the Record Ledger, mirror it to note frontmatter, reuse it in the index and output projection, and SHALL NOT call a random generator again for that note lifecycle operation.

#### Scenario: Rename and reclassify a note
- **WHEN** a note is renamed, moved, assigned to another project, moved to another folder, given another kind or has its tags changed
- **THEN** its `object_id` and `note_id` SHALL remain unchanged while the ledger records the changed locator or classification facts.

### Requirement: Identity, locator and revision are separate concepts
Pinax SHALL model object identity, current path and content revision as separate fields. `object_id` SHALL identify the logical object, `current_path` SHALL identify its mutable vault location, and `revision_id` or content hash SHALL identify a specific content state produced by a device.

#### Scenario: Same object receives a new revision
- **WHEN** an existing note body or managed metadata changes without creating a copy
- **THEN** Pinax SHALL retain the same `object_id`, update revision evidence, and attribute the change to the current `device_id`.

#### Scenario: Two objects claim one path
- **WHEN** different object IDs resolve to the same current path
- **THEN** Pinax SHALL report a path collision and SHALL NOT silently merge, overwrite or reassign either identity.

### Requirement: Legacy identities migrate through a reviewable plan
Pinax SHALL support existing path-derived and user-authored note IDs through an explicit audit and migration workflow. Migration SHALL be previewable, snapshot-protected, idempotent, resumable and reversible, and SHALL preserve a compatibility mapping from every legacy ID to the canonical object ID during the supported migration window.

#### Scenario: Audit an existing vault
- **WHEN** a user runs the identity migration audit against a vault containing missing, duplicate, path-derived or conflicting note IDs
- **THEN** Pinax SHALL return bounded issue counts, affected paths, proposed canonical IDs, conflict reasons and runnable next actions without modifying Markdown or `.pinax/**`.

#### Scenario: Apply a reviewed migration
- **WHEN** the user applies a saved migration plan with explicit approval after a fresh snapshot
- **THEN** Pinax SHALL update ledger identity, frontmatter mirrors, index relations, link targets and local sync metadata atomically enough to resume safely, write a redacted receipt, and preserve a restore path.

### Requirement: All durable vault object kinds participate in identity
Pinax SHALL use the same object identity contract for notes, assets, projects, subprojects, folders, database views, templates and future durable structured objects while allowing each kind to define its own mutable metadata.

#### Scenario: Move an asset or project object
- **WHEN** an asset path or project workspace path changes through an approved command
- **THEN** Pinax SHALL record a move for the same object ID instead of materializing a new logical object.
