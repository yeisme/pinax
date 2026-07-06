# pinax-backend-provider-cli Specification Delta

## ADDED Requirements

### Requirement: Backend note inspection commands expose visible cloud note state

Pinax SHALL provide read-only backend note inspection commands that summarize, list, and stat note-like Markdown objects visible through a configured backend profile.

#### Scenario: summarize visible backend notes

- **GIVEN** a vault has a backend profile named `work-s3`
- **AND** `work-s3` is the default backend
- **WHEN** the user runs `pinax backend notes summary --vault ./my-notes --json`
- **THEN** stdout SHALL be one JSON envelope with `command="backend.notes.summary"`
- **AND** facts SHALL include `backend`, `kind`, `objects`, `notes`, and `bytes`
- **AND** the command SHALL NOT write local notes, `.pinax/**` state, remote objects, sync receipts, or provider credentials.

#### Scenario: list visible backend Markdown notes

- **GIVEN** a backend contains `notes/cloud-sync.md` and non-note objects
- **AND** that backend is the default backend
- **WHEN** the user runs `pinax backend notes list --vault ./my-notes --json`
- **THEN** stdout SHALL be one JSON envelope with `command="backend.notes.list"`
- **AND** `data.notes[]` SHALL include note-like objects whose visible key ends with `.md`
- **AND** each note row SHALL include `path`, `key`, and `size_bytes`, with `revision` and `updated_at` when the backend provides them
- **AND** non-Markdown objects SHALL NOT appear in `data.notes[]`.

#### Scenario: stat one visible backend note object

- **GIVEN** a backend contains `notes/cloud-sync.md`
- **WHEN** the user runs `pinax backend notes stat work-s3 notes/cloud-sync.md --vault ./my-notes --json`
- **THEN** stdout SHALL be one JSON envelope with `command="backend.notes.stat"`
- **AND** facts SHALL include `backend`, `path`, `key`, and `revision` when the backend provides a revision
- **AND** missing objects SHALL return `note_object_not_found` rather than a generic internal error.

#### Scenario: complete backend note inspection arguments

- **GIVEN** a vault has a backend profile named `work-s3`
- **AND** that backend exposes visible note-like object `notes/cloud-sync.md`
- **WHEN** shell completion is requested for `pinax backend notes stat <TAB> --vault ./my-notes`
- **THEN** Pinax SHALL suggest backend profile names with backend kind descriptions
- **WHEN** shell completion is requested for `pinax backend notes stat work-s3 notes/ --vault ./my-notes`
- **THEN** Pinax SHALL suggest visible note-like Markdown paths such as `notes/cloud-sync.md`
- **AND** it SHALL NOT suggest non-Markdown objects as note paths.

#### Scenario: complete backend note prefixes

- **GIVEN** a backend exposes visible note-like object `notes/research/cloud-sync.md`
- **WHEN** shell completion is requested for `pinax backend notes list work-s3 <TAB> --vault ./my-notes`
- **THEN** Pinax SHALL suggest note prefixes such as `notes/` and `notes/research/`
- **AND** completion SHALL remain read-only.

### Requirement: Backend provider failures are normalized for cloud note inspection

Pinax SHALL map backend provider access failures to stable Pinax command errors without leaking raw provider payloads or secrets.

#### Scenario: invalid S3 region is actionable

- **GIVEN** an S3-compatible backend profile has an invalid region value or runtime region resolution fails
- **WHEN** the user runs `pinax backend object list default-s3 pinax-storage/ --vault ./my-notes --json`
- **THEN** stdout SHALL be one failed JSON envelope
- **AND** `error.code` SHALL be `backend_region_invalid` or another stable backend provider code
- **AND** the hint SHALL include a runnable command such as `pinax backend show default-s3 --vault ./my-notes`
- **AND** stdout/stderr SHALL NOT include S3 access key, secret key, session token, Authorization header, cookies, or provider raw payload.

#### Scenario: S3 profile fields are used by runtime client

- **GIVEN** an S3 backend profile stores `bucket`, `prefix`, `region`, `endpoint`, and `profile`
- **WHEN** Pinax creates the runtime backend store for object or note inspection
- **THEN** it SHALL pass `region`, `endpoint`, and `profile` into the S3 backend client options
- **AND** custom endpoint backends SHALL use path-style addressing by default.

## MODIFIED Requirements

### Requirement: Pinax exposes a unified backend command namespace

Pinax SHALL provide `pinax backend` as the primary CLI namespace for backend provider profile management, capability checks, diagnostics, sync plans, controlled backend operations, raw object inspection, and visible note-like object inspection.

#### Scenario: backend command exists

- **GIVEN** a user has a Pinax CLI build
- **WHEN** the user runs `pinax backend notes --help`
- **THEN** the command SHALL exist
- **AND** help SHALL list `summary`, `list`, and `stat`
- **AND** help text SHALL use English CLI wording.
