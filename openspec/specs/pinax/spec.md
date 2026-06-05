# pinax Specification

## Purpose

Pinax 是本地优先统一笔记 Agent CLI。当前 spec 记录子项目底座和后续实现的稳定边界，具体能力通过 `openspec/changes/pinax-*` 增量落地。

## Requirements

### Requirement: Pinax owns a local-first note CLI subproject

Pinax SHALL be implemented under `cli/pinax` as an independent Go CLI subproject and SHALL keep root repository OpenSpec limited to design handoff and governance.

#### Scenario: validating the development base
- **GIVEN** a developer enters `cli/pinax`
- **WHEN** they run `go test ./...` and `openspec validate --all`
- **THEN** the Go development base and OpenSpec workflow SHALL validate without requiring external provider credentials or a user vault

### Requirement: Machine-readable assets are CLI-authored

Pinax SHALL create and update machine-readable vault assets through commands or application services rather than requiring agents to hand-write JSON, YAML, or JSONL metadata.

#### Scenario: adding structured asset behavior
- **GIVEN** an implementation change adds config, provider profile, mapping, sync-state, event, briefing receipt, delivery receipt, feedback, or MCP evidence
- **WHEN** tasks are written
- **THEN** they SHALL include a command or service path that authors the asset
- **AND** tests SHALL validate schema version, redaction, path boundaries, and stable machine-readable errors

