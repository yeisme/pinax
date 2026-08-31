## ADDED Requirements

### Requirement: Pinax MUST 通过 CLI-authored user-level registry 管理 continuity binding
Pinax MUST 提供 experimental additive command，将一个 canonical Git repository/worktree 精确绑定到一个已注册 vault 和 bounded scope。Binding registry MUST 由 Pinax application service 创建和更新，使用 versioned schema 与 owner-only 权限，Agent 和用户不得手写其 JSON、YAML 或其他机器元数据。

#### Scenario: 为当前 Pinax worktree 建立绑定
- **WHEN** 用户通过 `pinax continue bind` 提交当前 Git worktree、已注册 `yeisme-notes` vault 和 `project:pinax` scope
- **THEN** Pinax 验证 repository、vault 和 scope 后创建唯一 enabled binding，并返回 bounded binding ID、状态和下一步命令
- **AND** 默认输出 MUST NOT 泄漏用户 home 下的完整绝对路径。

#### Scenario: 绑定目标不存在
- **WHEN** vault 未注册、project scope 不存在或 repository 不是可解析的 Git worktree
- **THEN** Pinax MUST 拒绝创建 binding，不写部分 registry 状态
- **AND** 返回一个 canonical recovery command 或缺失字段说明，不隐式创建 project 或猜测其他 vault。

### Requirement: Binding resolution MUST deterministic 且禁止默认跨 vault 搜索
Continuity binding resolution MUST 遵循“显式参数优先、exact worktree binding 次之、legacy default 最后”的固定顺序。Pinax MUST NOT 因目录名、Git remote、语义相似度或最近使用记录而扫描或选择其他 vault。

#### Scenario: 显式参数覆盖已有 binding
- **WHEN** 当前 worktree 已绑定，但调用同时显式提供 `--vault` 或 `--scope`
- **THEN** Pinax MUST 使用显式值并保留现有 `continue` 参数语义
- **AND** binding MUST NOT 静默改写调用者指定的 vault、scope、handoff、task 或 intent。

#### Scenario: 唯一 exact binding 自动解析
- **WHEN** 当前目录属于一个具有唯一 enabled exact binding 的 Git worktree，且调用未显式提供 vault/scope
- **THEN** Pinax MUST 将该 binding 的 vault 与 scope 用于 continuity compilation
- **AND** output MUST 以 additive facts 标识 `binding_status=ready` 和 bounded binding digest。

#### Scenario: 当前 worktree 未绑定
- **WHEN** 调用未显式提供 vault/scope，当前 worktree 没有 enabled binding
- **THEN** Pinax MUST 保留现有 `workspace:default` fallback 行为，并返回 `binding_status=missing`
- **AND** 它 MUST 只给出一个 bind next action，不得搜索所有 vault 或声称已识别当前 project。

#### Scenario: Registry 存在多个 exact active records
- **WHEN** 同一个 canonical worktree 因损坏或旧版本迁移出现多个 enabled exact bindings
- **THEN** Pinax MUST fail closed，返回稳定错误 `continuity_binding_ambiguous`
- **AND** 它 MUST NOT 任意选择 binding，并给出 inspect/rebind recovery action。

### Requirement: Pinax MUST 提供 binding status 与可恢复诊断
Pinax MUST 提供 read-only status projection，报告 repository detection、binding status、vault availability、scope validity、registry schema 和 readiness；诊断输出 MUST 区分 missing、disabled、invalid、ambiguous 和 ready。

#### Scenario: Binding ready
- **WHEN** repository、vault、scope 和 registry 均可解析
- **THEN** status MUST 返回 `ready=true`、bounded owner facts 和可直接执行的 continue action
- **AND** status 查询 MUST NOT 创建 run receipt、handoff、proposal 或 vault mutation。

#### Scenario: Vault 或 scope 在绑定后被删除
- **WHEN** binding record 存在但其 vault 或 scope 已不可解析
- **THEN** status MUST 返回 `ready=false` 和 `binding_status=invalid`
- **AND** 后续 auto-resume MUST NOT 回退到另一个 vault 来掩盖错误。

### Requirement: Binding contract MUST 保持 additive、local-only 和可回滚
Binding schema MUST 是新版本化合同，不得重命名或 repurpose 现有 config/profile keys。禁用 capability 时，既有 explicit `pinax continue` 调用 MUST 保持可用；registry MAY 被安全忽略，但不得在 rollback 时破坏性删除。

#### Scenario: Binding capability 被关闭
- **WHEN** 新 command registration 或 resolver feature 被回滚
- **THEN** 现有显式 `pinax continue --vault ... --scope ...` MUST 继续按原合同工作
- **AND** user-level binding data MUST 保留为可忽略的 additive state，不需要 destructive migration。
