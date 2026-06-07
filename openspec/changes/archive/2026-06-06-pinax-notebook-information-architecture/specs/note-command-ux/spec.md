## ADDED Requirements

### Requirement: Note creation builds notebook information architecture
Pinax SHALL make newly created notes immediately discoverable through group, folder, kind, tags, daily index, and local index projections.

#### Scenario: Create note with group folder and kind
- **WHEN** a user runs `pinax note new "工具笔记" --group work --folder inbox --kind reference --tags pinax,cli --vault ./my-notes --json`
- **THEN** Pinax SHALL create the note under the selected group/project prefix and folder
- **AND** the note frontmatter SHALL include `project`, `folder`, `kind`, and `tags`
- **AND** the JSON envelope facts SHALL include group, folder, kind, daily index path, and index update status.

#### Scenario: Created note is added to daily index
- **WHEN** a note is created without `--dry-run`
- **THEN** Pinax SHALL update `notes/daily/YYYY-MM-DD.md` through the application service
- **AND** the daily index SHALL include the note title, path, tags, group, folder, and kind.

#### Scenario: Created note refreshes local index
- **WHEN** a note is created without `--dry-run`
- **THEN** Pinax SHALL refresh `.pinax/index.sqlite` through the GORM index service
- **AND** a following `pinax stats --vault ./my-notes --json` SHALL report `index_status=fresh`.
