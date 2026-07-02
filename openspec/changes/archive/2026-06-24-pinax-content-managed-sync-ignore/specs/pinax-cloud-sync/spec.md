# pinax-cloud-sync Delta Spec

## MODIFIED Requirements

### Requirement: Cloud Sync 每台设备拥有本地 vault

Cloud Sync SHALL synchronize Pinax-managed vault content selected by `.pinaxignore`, including Markdown notes, scripts, assets, attachments, and other regular files, while excluding hard-denied runtime paths.

#### Scenario: 未忽略普通文件纳入 manifest

- **GIVEN** a vault contains `notes/a.md`, `scripts/build.sh`, and `assets/logo.png`
- **AND** `.pinaxignore` does not exclude those paths
- **WHEN** the user runs `pinax sync push --target cloud --dry-run --vault <vault> --json`
- **THEN** the sync plan SHALL include those files in the local content manifest
- **AND** protected output SHALL report counts and hashes, not file payload bytes.

#### Scenario: `.pinaxignore` 排除内容文件

- **GIVEN** `.pinaxignore` excludes `.env*` and `dist/`
- **WHEN** Pinax builds a Cloud Sync manifest
- **THEN** matching files SHALL NOT be uploaded, pulled, or recorded as content entries
- **AND** `.gitignore` SHALL NOT be used as an implicit Pinax content rule source.

#### Scenario: hard deny 路径永不同步

- **GIVEN** a vault contains `.git/`, `.pinax/index.sqlite`, `.pinax/cloud/blob-cache/`, or symlinks
- **WHEN** Pinax builds a Cloud Sync manifest
- **THEN** those paths SHALL be skipped even if `.pinaxignore` tries to re-include them.
