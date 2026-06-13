# configuration-layer Specification

## Purpose
TBD - created by archiving change pinax-configurable-output-rendering. Update Purpose after archive.
## Requirements
### Requirement: Pinax loads layered typed configuration
Pinax SHALL load configuration into a typed config model from built-in defaults, user config, project config, environment variables, and explicitly changed command-line flags.

#### Scenario: Configuration precedence is deterministic
- **WHEN** the same setting is provided by built-in defaults, user config, project config, environment variable, and explicit command-line flag
- **THEN** Pinax SHALL use the command-line flag value
- **AND** lower-precedence values SHALL NOT override it.

#### Scenario: Project config overrides user config
- **WHEN** user config sets `output.theme=mono` and `<vault>/.pinax/config.yaml` sets `output.theme=pinax`
- **THEN** commands run with that vault SHALL use `pinax` unless an environment variable or explicit flag overrides it.

#### Scenario: Flag defaults do not mask config files
- **WHEN** a Cobra flag has a default value and the user does not explicitly provide that flag
- **THEN** Pinax SHALL NOT treat the flag default as a higher-precedence override
- **AND** user or project config values for that setting SHALL remain effective.

### Requirement: Pinax resolves user and project configuration paths safely
Pinax SHALL read user-level configuration from XDG-compatible paths and project-level configuration from the selected vault's `.pinax/config.yaml`.

#### Scenario: User config path follows XDG
- **WHEN** `$XDG_CONFIG_HOME` is set
- **THEN** Pinax SHALL look for user configuration under `$XDG_CONFIG_HOME/pinax/config.yaml`
- **AND** it SHALL fall back to `~/.config/pinax/config.yaml` when `$XDG_CONFIG_HOME` is unset.

#### Scenario: Project config stays inside the vault
- **WHEN** a command is run with `--vault ./my-notes`
- **THEN** Pinax SHALL read project configuration from `./my-notes/.pinax/config.yaml`
- **AND** it SHALL reject config paths that escape the selected vault boundary.

#### Scenario: Read-only commands do not create config files
- **WHEN** a user runs a read-only command such as `pinax note list` or `pinax stats`
- **THEN** Pinax SHALL NOT implicitly create or modify user-level or project-level config files.

### Requirement: Pinax supports stable environment variable overrides
Pinax SHALL expose stable `PINAX_` environment variables for supported configuration keys and SHALL retain compatibility with standard terminal environment variables.

#### Scenario: Nested config maps to PINAX environment variables
- **WHEN** `PINAX_OUTPUT_COLOR=always` and `PINAX_OUTPUT_MARKDOWN_STYLE=dark` are set
- **THEN** Pinax SHALL apply them as `output.color=always` and `output.markdown.style=dark` unless explicit flags override them.

#### Scenario: NO_COLOR disables color unless explicitly overridden
- **WHEN** `NO_COLOR` is present and the user does not pass `--color always`
- **THEN** Pinax SHALL render default human output without ANSI color
- **AND** machine output modes SHALL remain ANSI-free regardless of color settings.

#### Scenario: Editor fallback remains compatible
- **WHEN** `editor.command` is unset in Pinax config and `$EDITOR` is set
- **THEN** editor-opening commands SHALL use `$EDITOR` as the fallback editor command.

### Requirement: Pinax authors configuration through CLI commands
Pinax SHALL create and update structured configuration files through CLI commands or application services rather than requiring agents or users to hand-edit machine-readable metadata.

#### Scenario: Config get shows effective values
- **WHEN** a user runs `pinax config get output.theme --vault ./my-notes`
- **THEN** Pinax SHALL return the effective value after applying defaults, user config, project config, environment variables, and explicit flags.

#### Scenario: Config set requires explicit scope
- **WHEN** a user runs `pinax config set output.theme high-contrast` without `--scope user` or `--scope project`
- **THEN** Pinax SHALL fail with a stable error code and a runnable hint
- **AND** it SHALL NOT write any configuration file.

#### Scenario: Config doctor reports sources
- **WHEN** a user runs `pinax config doctor --vault ./my-notes --json`
- **THEN** stdout SHALL contain one JSON envelope with effective config facts, source paths, environment keys used, explicit flag keys used, validation issues, and next actions.

### Requirement: Pinax validates configuration before command execution
Pinax SHALL validate the typed configuration before application services consume it.

#### Scenario: Invalid enum fails with stable error
- **WHEN** config sets `output.color=sometimes`
- **THEN** Pinax SHALL fail before executing the command with a stable config validation error code
- **AND** the error projection SHALL include a Chinese message and a runnable correction hint.

#### Scenario: Storage config does not persist secrets
- **WHEN** a user configures S3 storage
- **THEN** Pinax SHALL allow non-secret fields such as bucket, region, prefix, endpoint, and profile
- **AND** it SHALL reject or redact secret-like keys such as token, secret, password, cookie, authorization, and webhook URL.

