# version-control Delta Spec

## MODIFIED Requirements

### Requirement: Local version snapshots provide restoreable content evidence

Pinax local version snapshots SHALL store content objects for Pinax-managed regular files so Git can remain metadata-only while `version show` and `version restore apply` can read historical file content by snapshot id.

#### Scenario: Restore from local snapshot without Git-tracked note content

- **GIVEN** a vault has metadata-only Git ignore rules and a note file not tracked by Git
- **AND** the user runs `pinax version snapshot --vault <vault> --message <msg> --json`
- **WHEN** the note is corrupted and the user generates a restore plan using the returned `snapshot_id`
- **THEN** Pinax SHALL restore the historical content from `.pinax/version/objects/`
- **AND** the restore apply projection SHALL report `local_write=true`, `remote_write=false`, `version_backend=local`, and a content hash.
