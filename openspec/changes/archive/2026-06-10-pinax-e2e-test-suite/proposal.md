## Why

现有的 E2E 测试主要验证常规流程，缺少对 CLI 各种输出模式（Summary/Agent/JSON/Events/Explain）的契约校验、底层索引状态投影一致性（SQLite 与 Markdown 的数据自愈）以及异常路径（安全脱敏、断网/超时/冲突）的覆盖。这使得 AI 代理和 CI 在消费 CLI 输出时缺乏稳定契约，后端代码修改后也易发生状态同步和元数据管理回归。

## What Changes

* **输出契约验证框架**：在 E2E 测试套件中增加 JSON 模式 Envelope 契约结构的校验机制，断言 `spec_version`、`mode`、`command` 和 `status` 等稳定顶层字段。
* **物理-索引一致性测试**：新增双向链接、元数据增删改之后底层的 SQLite 缓存索引状态投影一致性黑盒测试脚本。
* **安全脱敏与异常断言**：新增包含敏感 Token、授权凭据的配置/同步命令执行测试，验证输出日志及 Stdout 中敏感字段已被完全脱敏（`[REDACTED]` 或 `***`）。
* **无网络环境 Mock 工具集**：提供 fake 外部 CLI（如 lark-cli 和 ntn）的本地注入机制，使得事件同步的断言完全在本地沙盒中执行。

## Capabilities

### New Capabilities
- `e2e-contract-testing`: 校验 CLI 多种输出渲染模式（Summary、Agent、JSON Envelope、Events）的输出规范与契约一致性。
- `state-projection-testing`: 验证物理 Markdown 文件的元数据、双向链接等属性在同步后正确投影到 SQLite 数据库索引中，并在破坏后具备自愈校验能力。
- `provider-mocking-integration`: 通过在 PATH 中注入 Mock CLI 模拟外部 Provider，在本地无网络环境中闭环测试 Sync 同步和事件流 NDJSON 行为。

### Modified Capabilities

## Impact

* **`tests/e2e/` 目录**：将新增相应的测试脚本文件和测试数据夹（testdata）。
* **`internal/cli` 与 `internal/output`**：如果测试中暴露出某些命令的输出字段不符合统一信封契约，可能会对展现层渲染做小幅调整。
* **`Taskfile.yml`**：新编写 of `testscript` 场景会在运行 `task check` 或 `task test` 时自动执行。
