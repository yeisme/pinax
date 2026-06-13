## MODIFIED Requirements

### Requirement: Saved views store reusable local filters
Pinax SHALL let users save and reuse common note list filters and database-style local views through CLI-authored structured assets.

#### Scenario: Save a note view
- **WHEN** a user runs `pinax view save active-work --group work --status active --kind reference --sort updated --vault ./my-notes --json`
- **THEN** Pinax SHALL create or update `.pinax/views.json` through the application service
- **AND** the saved view SHALL store filters rather than note result snapshots.

#### Scenario: Show a saved view
- **WHEN** a user runs `pinax view show active-work --vault ./my-notes --json`
- **THEN** Pinax SHALL resolve the saved filters and return current matching notes
- **AND** it SHALL report the saved view name and filter facts.

#### Scenario: Delete a saved view with approval
- **WHEN** a user runs `pinax view delete active-work --yes --vault ./my-notes --json`
- **THEN** Pinax SHALL remove only that view from `.pinax/views.json`
- **AND** it SHALL NOT delete notes or attachments.

#### Scenario: Save a database table view
- **WHEN** a user runs `pinax view save active-projects --query 'SELECT title, status, due FROM notes WHERE tags CONTAINS "project" AND status = "active" ORDER BY due ASC LIMIT 50' --kind table --vault ./my-notes --json`
- **THEN** Pinax SHALL store a database-style view definition through the application service
- **AND** the saved view SHALL store query text, display kind, columns, limit, and display options rather than result rows.

#### Scenario: Show database table view through legacy view command
- **WHEN** a user runs `pinax view show active-projects --vault ./my-notes --json`
- **AND** the saved view is a database-style table view
- **THEN** Pinax SHALL execute the saved query against the current local index projection
- **AND** it SHALL return table columns, rows, filters, sorts, engine, index status, and pagination facts.

#### Scenario: Old filter-only views remain compatible
- **WHEN** `.pinax/views.json` contains an older filter-only saved view
- **AND** a user runs `pinax view show <name> --vault ./my-notes --json`
- **THEN** Pinax SHALL treat it as a database view with equivalent filters and default columns
- **AND** it SHALL NOT require the user to hand-edit the view registry.
