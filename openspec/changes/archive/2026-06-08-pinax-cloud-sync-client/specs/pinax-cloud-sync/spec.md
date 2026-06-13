## ADDED Requirements

### Requirement: Local-only 模式完整可用

所有普通笔记、vault 和索引命令 SHALL 在无后端时正常工作；Cloud 命令只在用户配置后端时可用。

#### Scenario: 无后端正常使用

- **WHEN** 用户未配置云端后端
- **THEN** 所有本地笔记、vault 和索引命令 SHALL 正常执行
- **AND** cloud 命令 SHALL 提示用户先配置后端

### Requirement: 端侧加密保护明文

Manifest 和 blob SHALL 使用 client-side encryption；明文 SHALL NOT 离开本地。

#### Scenario: 加密 manifest 和 blob

- **WHEN** 客户端上传 manifest 或 blob
- **THEN** 数据 SHALL 使用端侧加密
- **AND** 后端只看到加密数据

#### Scenario: 脱敏验证

- **WHEN** 测试或 event 写入 cloud 相关数据
- **THEN** 不得暴露明文 note body、路径或 token

### Requirement: Sync plan 支持 dry-run 和冲突

Sync planner SHALL 支持 dry-run 模式和冲突检测。

#### Scenario: dry-run sync

- **WHEN** 用户运行 `pinax sync diff --dry-run`
- **THEN** 输出 SHALL 显示 sync plan 但不执行任何写入
- **AND** 不改写 vault

#### Scenario: 冲突检测

- **WHEN** base revision mismatch
- **THEN** sync SHALL 返回 REVISION_CONFLICT 并进入冲突队列
- **AND** 不得静默覆盖
