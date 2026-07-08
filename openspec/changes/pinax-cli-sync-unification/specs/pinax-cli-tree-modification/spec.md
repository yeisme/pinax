## MODIFIED Requirements

### Requirement: 移除 pinax cloud 命令组

Pinax SHALL 移除 `pinax cloud` 命令组（capsa 的旧版别名）。

#### Scenario: cloud 命令不存在

- **GIVEN** pinax CLI 已构建
- **WHEN** 用户运行 `pinax cloud --help`
- **THEN** CLI SHALL 返回 unknown command 错误
- **AND** 错误信息 SHALL 提示使用 `pinax capsa`

#### Scenario: cloud login 迁移到 capsa login

- **GIVEN** 用户之前使用 `pinax cloud login`
- **WHEN** 用户运行 `pinax capsa login --endpoint <url> --workspace <id> --device <device> --secret-ref <ref> --vault <vault>`
- **THEN** 命令 SHALL 执行相同的 capsa login 逻辑
- **AND** 配置 SHALL 写入 vault 的 `.pinax/config.yaml`

#### Scenario: cloud status 迁移到 capsa status

- **GIVEN** 用户之前使用 `pinax cloud status`
- **WHEN** 用户运行 `pinax capsa status --vault <vault>`
- **THEN** 命令 SHALL 返回相同的 capsa 状态信息

### Requirement: 移除 pinax git snapshot 命令组

Pinax SHALL 移除 `pinax git snapshot` 隐藏遗留命令。

#### Scenario: git snapshot 命令不存在

- **GIVEN** pinax CLI 已构建
- **WHEN** 用户运行 `pinax git --help`
- **THEN** CLI SHALL 返回 unknown command 或 empty help
- **AND** `git snapshot` 子命令 SHALL 不存在

#### Scenario: git snapshot 已被组织工作流替代

- **GIVEN** 用户需要组织笔记前创建快照
- **WHEN** 用户运行 `pinax organize plan` + `pinax organize apply`
- **THEN** 命令 SHALL 要求先有 Git snapshot（通过 `--yes` 确认）
- **AND** 用户应使用 Git 原生命令或其他版本控制工具

### Requirement: 移除 pinax storage Hidden 命令

Pinax SHALL 移除 `pinax storage set-local` 和 `set-s3` Hidden 命令。

#### Scenario: storage set-local 不存在

- **GIVEN** pinax CLI 已构建
- **WHEN** 用户运行 `pinax storage set-local --root <root>`
- **THEN** CLI SHALL 返回 unknown command 错误
- **AND** 用户应使用 `pinax storage set local --root <root>`

#### Scenario: storage set-s3 不存在

- **GIVEN** pinax CLI 已构建
- **WHEN** 用户运行 `pinax storage set-s3 --bucket <bucket>`
- **THEN** CLI SHALL 返回 unknown command 错误
- **AND** 用户应使用 `pinax storage set s3 --bucket <bucket>`

### Requirement: 保留 pinax storage 主存储配置

Pinax SHALL 保留 `pinax storage set local|s3`/`status`/`doctor`（非 Hidden 命令）。

#### Scenario: storage set local 配置主存储

- **GIVEN** 用户想配置本地主存储
- **WHEN** 用户运行 `pinax storage set local --root ./my-notes --vault ./my-notes`
- **THEN** 命令 SHALL 配置 vault 主存储为本地目录
- **AND** 配置 SHALL 写入 `.pinax/config.yaml`

#### Scenario: storage set s3 配置主存储

- **GIVEN** 用户想配置 S3 主存储
- **WHEN** 用户运行 `pinax storage set s3 --bucket notes --region us-east-1 --vault ./my-notes`
- **THEN** 命令 SHALL 配置 vault 主存储为 S3 后端
- **AND** 配置 SHALL 写入 `.pinax/config.yaml`

#### Scenario: storage status 显示主存储状态

- **GIVEN** vault 已配置主存储
- **WHEN** 用户运行 `pinax storage status --vault ./my-notes`
- **THEN** 命令 SHALL 显示主存储类型和配置摘要

#### Scenario: storage doctor 诊断主存储配置

- **GIVEN** vault 已配置主存储
- **WHEN** 用户运行 `pinax storage doctor --vault ./my-notes`
- **THEN** 命令 SHALL 诊断主存储连接和配置

### Requirement: 移除 pinax backend sync 操作

Pinax SHALL 移除 `pinax backend push`/`pull`/`diff` 命令（与 sync 重叠）。

#### Scenario: backend push 不存在

- **GIVEN** pinax CLI 已构建
- **WHEN** 用户运行 `pinax backend push <name> --vault <vault>`
- **THEN** CLI SHALL 返回 unknown command 错误
- **AND** 用户应使用 `pinax sync push`

#### Scenario: backend pull 不存在

- **GIVEN** pinax CLI 已构建
- **WHEN** 用户运行 `pinax backend pull <name> --vault <vault>`
- **THEN** CLI SHALL 返回 unknown command 错误
- **AND** 用户应使用 `pinax sync pull`

#### Scenario: backend diff 不存在

- **GIVEN** pinax CLI 已构建
- **WHEN** 用户运行 `pinax backend diff <name> --vault <vault>`
- **THEN** CLI SHALL 返回 unknown command 错误
- **AND** 用户应使用 `pinax sync diff`（如存在）

### Requirement: 保留 pinax backend blob 浏览功能

Pinax SHALL 保留 `pinax backend list|add|show|doctor|capabilities|remove` + `object list|stat` + `notes summary|list|stat`。

#### Scenario: backend list 列出所有后端

- **GIVEN** vault 已配置后端
- **WHEN** 用户运行 `pinax backend list --vault ./my-notes`
- **THEN** 命令 SHALL 列出所有后端配置

#### Scenario: backend object list 浏览后端对象

- **GIVEN** vault 已配置 S3 后端
- **WHEN** 用户运行 `pinax backend object list <name> [prefix] --vault ./my-notes`
- **THEN** 命令 SHALL 列出后端对象

#### Scenario: backend notes list 浏览笔记对象

- **GIVEN** vault 已配置后端
- **WHEN** 用户运行 `pinax backend notes list [name] [prefix] --vault ./my-notes`
- **THEN** 命令 SHALL 列出后端笔记对象

### Requirement: 保留 pinax capsa 命令组

Pinax SHALL 保留 `pinax capsa` 命令组（设备/会话配置）。

#### Scenario: capsa login 配置后端状态

- **GIVEN** 用户想配置 capsa 后端
- **WHEN** 用户运行 `pinax capsa login --endpoint <url> --workspace <id> --device <device> --secret-ref <ref> --vault ./my-notes`
- **THEN** 命令 SHALL 配置 capsa 后端状态
- **AND** 配置 SHALL 写入 `.pinax/config.yaml`

#### Scenario: capsa logout 登出设备会话

- **GIVEN** 用户已登录 capsa
- **WHEN** 用户运行 `pinax capsa logout --vault ./my-notes`
- **THEN** 命令 SHALL 登出本地 capsa 设备会话

#### Scenario: capsa status 显示状态

- **GIVEN** 用户已配置 capsa
- **WHEN** 用户运行 `pinax capsa status --vault ./my-notes`
- **THEN** 命令 SHALL 显示 capsa 状态

#### Scenario: capsa doctor 诊断状态

- **GIVEN** 用户已配置 capsa
- **WHEN** 用户运行 `pinax capsa doctor --vault ./my-notes`
- **THEN** 命令 SHALL 诊断 capsa 状态

#### Scenario: capsa backend set 配置同步传输后端

- **GIVEN** 用户想配置 capsa 同步传输后端
- **WHEN** 用户运行 `pinax capsa backend set s3 --bucket notes --region us-east-1 --workspace personal --device laptop --vault ./my-notes`
- **THEN** 命令 SHALL 配置 S3 同步传输后端
- **AND** 配置 SHALL 写入 `.pinax/config.yaml`

### Requirement: pinax sync 保持不变

Pinax SHALL 保持 `pinax sync` 命令组不变（init/status/diff/push/pull/logs/daemon/conflicts）。

#### Scenario: sync push 仍工作

- **GIVEN** 用户已配置 capsa
- **WHEN** 用户运行 `pinax sync push --vault ./my-notes --yes`
- **THEN** 命令 SHALL 执行 sync push 逻辑
- **AND** 输出 SHALL 符合输出契约

#### Scenario: sync pull 仍工作

- **GIVEN** 用户已配置 capsa
- **WHEN** 用户运行 `pinax sync pull --vault ./my-notes --yes`
- **THEN** 命令 SHALL 执行 sync pull 逻辑
- **AND** 输出 SHALL 符合输出契约

### Requirement: 命令树边界清晰

Pinax SHALL 明确各命令组的边界：
- `sync` = 同步操作
- `capsa` = 设备/会话配置
- `storage` = 主存储配置
- `backend` = blob 浏览（list/add/show/doctor/capabilities/remove + object/notes）

#### Scenario: sync vs capsa 边界

- **GIVEN** 用户需要配置同步
- **WHEN** 用户运行 `pinax capsa login` + `pinax sync push`
- **THEN** `capsa login` 配置设备/会话，`sync push` 执行同步操作

#### Scenario: storage vs backend 边界

- **GIVEN** 用户需要配置存储
- **WHEN** 用户运行 `pinax storage set s3` + `pinax backend add s3 work-s3`
- **THEN** `storage set s3` 配置主存储，`backend add` 配置附加后端用于 blob 浏览
