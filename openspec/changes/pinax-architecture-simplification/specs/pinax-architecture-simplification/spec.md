# Pinax 架构简化变更

## ADDED Requirements

### Requirement: Watcher and debounce goroutines SHALL shut down without hanging

File-watching event forwarding and debouncing SHALL never block forever on a
consumer that has stopped draining, and a burst of watcher errors SHALL not
silently stop event forwarding.

#### Scenario: Watcher error burst does not stall forwarding

- **WHEN** the underlying watcher emits two errors before any consumer drains
  the error channel
- **THEN** the forwarding goroutine SHALL remain live and at least one error
  SHALL be observable (delivered or logged to stderr)

#### Scenario: Debounce cancels cleanly

- **WHEN** the debounce context is cancelled while an output consumer has
  already returned
- **THEN** the debounce goroutine SHALL exit instead of blocking on send

### Requirement: Library code SHALL return errors instead of panicking on bad configuration

Cloud sync client and identity helpers SHALL surface invalid or missing IDs as
command errors rather than process panics.

#### Scenario: Missing vault id fails the command

- **WHEN** a cloud sync command runs against a vault whose configuration has an
  empty workspace/vault id
- **THEN** Pinax SHALL return a stable command error envelope and exit non-zero
  without panicking

### Requirement: Evidence persistence failures SHALL be visible

Monitoring and sync evidence writes (run records, appended events, sync state)
SHALL report failures on stderr instead of silently discarding them.

#### Scenario: Disk-full during monitor run

- **WHEN** a monitor run finishes but its run record cannot be written
- **THEN** the command output remains successful AND a single-line warning is
  printed to stderr naming the failed operation

### Requirement: Sync projection wording SHALL be constructed, not string-rewritten

Capsa/cloud alias wording SHALL be decided when projections are built instead
of blindly replacing the word "cloud" inside rendered summaries and error
messages.

#### Scenario: Legitimate cloud text is preserved

- **WHEN** a sync summary or error message legitimately contains the word
  "cloud" (for example a vault named "Cloud Notes" or a provider message)
- **THEN** the rendered text SHALL keep that wording unchanged while command
  aliases still point at the current `pinax capsa` surface
