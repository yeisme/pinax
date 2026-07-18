## ADDED Requirements

### Requirement: 同步配置来源与运行态分离

Cloud Sync SHALL treat repository declaration as the portable source for transport topology and local `.pinax/cloud/config.yaml` as generated device runtime state.

#### Scenario: 配置驱动同步

- **WHEN** a repository contains a valid `pinax-sync.yaml` and the user runs `pinax sync repo apply --vault ./my-notes --json`
- **THEN** subsequent `pinax sync push`, `pinax sync pull` and daemon commands SHALL resolve the generated Capsa runtime config without requiring repeated backend flags
- **AND** sync SHALL retain existing encrypted revision, manifest, blob and CAS semantics

#### Scenario: 设备状态不进入远程内容清单

- **WHEN** Pinax builds a content manifest after repository bootstrap
- **THEN** generated cloud config, device state, daemon runtime, secrets assets and sync receipts SHALL follow explicit protected-path rules
- **AND** SHALL NOT be uploaded as ordinary plaintext note content

### Requirement: 首次设备 bootstrap 安全

Cloud Sync SHALL require explicit first-device or new-device bootstrap semantics before enabling bidirectional writes.

#### Scenario: 新设备首次只拉取

- **WHEN** a new device bootstraps a workspace with no local sync receipt
- **THEN** Pinax SHALL perform pull-only initialization by default
- **AND** SHALL require explicit approval before uploading local deletions or replacing remote state

#### Scenario: 加密 key identity 不匹配

- **WHEN** unlocked secrets resolve to a different encryption key identity than the remote head
- **THEN** sync SHALL fail with `encryption_key_mismatch`
- **AND** SHALL provide a recovery action without generating a replacement key automatically
