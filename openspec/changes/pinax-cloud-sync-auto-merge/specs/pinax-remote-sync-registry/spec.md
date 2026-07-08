## MODIFIED Requirements

### Requirement: Cloud Sync protects local-first moves and deletes

Pinax SHALL use the latest Cloud Sync path as the default sync target and SHALL plan pull/sync operations from base, local, and remote manifests instead of blindly replaying the remote manifest over the local vault.

#### Scenario: Pull does not restore a locally moved note

- **GIVEN** a device has synced `index/home.md` from Cloud Sync
- **AND** the user locally moves it to `notes/home.md` without pushing
- **WHEN** the user runs `pinax sync pull --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL fail with `LOCAL_UNPUSHED_CHANGES`
- **AND** it SHALL NOT recreate `index/home.md`
- **AND** it SHALL recommend `pinax sync --target cloud --vault ./my-notes --yes`.

#### Scenario: Bidirectional sync pushes the local move

- **GIVEN** a device has synced `index/home.md` from Cloud Sync
- **AND** the user locally moves it to `notes/home.md`
- **WHEN** the user runs `pinax sync --vault ./my-notes --yes --json`
- **THEN** Pinax SHALL push the current manifest to Cloud Sync
- **AND** another device pulling that revision SHALL remove `index/home.md` and create `notes/home.md`.

#### Scenario: Sync subcommands default to Cloud Sync

- **WHEN** a user runs `pinax sync diff`, `pinax sync push`, or `pinax sync pull` without `--target`
- **THEN** Pinax SHALL use `cloud` as the target
- **AND** `--target git` and `--target s3` SHALL remain explicit compatibility paths.
