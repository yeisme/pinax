# e2e-contract-testing Specification

## Purpose
TBD - created by archiving change pinax-e2e-test-suite. Update Purpose after archive.
## Requirements
### Requirement: CLI Output Mode Contract Validation
CLI 必须对支持渲染输出的命令统一遵循 Summary（默认中文）、Agent（键值事实）、JSON（信封契约）、Events（流式 NDJSON）等渲染模式。测试套件中的断言机制 SHALL 校验其输出格式的完整性与契约一致性。

#### Scenario: Verify JSON Output Envelope Compliance
- **WHEN** 执行 `pinax` 任意带有 `--json` 标志的命令时
- **THEN** 正常输出（stdout）中 SHALL 返回合法的 JSON，且含有 `spec_version`、`mode`、`command` 和 `status` 标准顶层字段

#### Scenario: Verify Agent Output Fact Formatting
- **WHEN** 执行 `pinax` 带有 `--agent` 标志的命令时
- **THEN** 标准输出中的每一行 SHALL 遵循 `key=value` 的格式规则，并且包含必备字段，值中带有空格时使用双引号包裹

### Requirement: Proof loop readiness SHALL have command-level e2e coverage

Pinax SHALL verify the first-user proof loop through command-level e2e tests using the owning project's existing Go/testscript stack.

#### Scenario: Preview e2e covers read-only proof loop

- **WHEN** the proof-loop preview e2e runs against the deterministic demo vault
- **THEN** it SHALL assert JSON envelope validity, `proof_loop_run_id`, stage facts, next actions, and `local_write=false`
- **AND** it SHALL fail if preview writes Markdown, `.pinax` apply assets, Git state, provider state, or remote state.

#### Scenario: Apply e2e covers plan and snapshot gates

- **WHEN** the apply e2e runs against the deterministic demo vault
- **THEN** it SHALL save a repair plan, create a version snapshot, apply only approved low-risk operations, and assert receipt facts
- **AND** it SHALL reject stale plans and missing snapshot paths with stable machine-readable error codes.

#### Scenario: Restore e2e covers controlled rollback

- **WHEN** the restore e2e runs after a proof-loop apply
- **THEN** it SHALL generate a restore plan, apply that plan through the CLI/application service, and assert `local_write=true` and `remote_write=false`
- **AND** it SHALL verify stale restore plans fail safely.

### Requirement: Proof loop outputs SHALL be recursively redaction-tested

Pinax SHALL recursively scan proof-loop output and evidence surfaces for body leaks and sensitive values.

#### Scenario: Machine outputs contain no bounded-projection body leak

- **WHEN** proof-loop commands run in `--json`, `--agent`, or `--events` modes without an explicit body-display command
- **THEN** stdout SHALL NOT contain non-empty `body`, `note_body`, or `raw_body` fields at any nesting depth
- **AND** stdout SHALL NOT contain body sentinel text from fixture notes.

#### Scenario: Evidence surfaces contain no forbidden sensitive values

- **WHEN** tests inspect stdout, stderr, saved plans, receipts, events, snapshots, restore evidence, integration evidence, and fixtures
- **THEN** those surfaces SHALL NOT contain Authorization headers, Bearer tokens, API keys, raw prompts, provider payloads, hidden system prompts, private tool arguments, or complete chain-of-thought.

