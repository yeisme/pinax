## MODIFIED Requirements

### Requirement: Pinax treats TaskBridge as the task execution control plane

Pinax SHALL 通过 Connectors 提供的稳定 TaskBridge CLI 合同消费任务事实，且 SHALL NOT 直接读取 Connectors/TaskBridge 本地存储、Provider token 或 Provider API。Pinax SHALL 优先调用 `connectors task agent ...`；仅当 `connectors` 可执行文件不存在时，MAY 在兼容窗口内回退到独立 `taskbridge agent ...`。

#### Scenario: probing TaskBridge capabilities
- **GIVEN** Pinax vault 已存在且 Connectors 已安装
- **WHEN** 用户运行 `pinax plan snapshot --vault ./my-notes --taskbridge --json`
- **THEN** Pinax SHALL 通过 CLI-backed adapter 调用 `connectors task agent` 能力和任务事实命令
- **AND** stdout SHALL 包含 Pinax JSON envelope、能力事实、警告和下一步操作
- **AND** Pinax SHALL NOT 读取 Connectors/TaskBridge store 文件或 Provider credential 文件

#### Scenario: compatibility fallback
- **GIVEN** `connectors` 不可用但兼容 `taskbridge` 可执行文件存在
- **WHEN** 用户运行 `pinax plan daily --vault ./my-notes --taskbridge --dry-run --json`
- **THEN** Pinax MAY 调用 `taskbridge agent today` 并解析相同的 `taskbridge.*.v1` 合同
- **AND** 新生成的下一步执行命令 SHALL 仍然指向 `connectors task agent execute`

#### Scenario: TaskBridge is unavailable
- **GIVEN** `connectors` 与兼容 `taskbridge` 均不可用，或首选运行时返回不支持的 schema
- **WHEN** 用户运行 `pinax plan daily --vault ./my-notes --taskbridge --dry-run --json`
- **THEN** Pinax SHALL 以 `TASKBRIDGE_UNAVAILABLE` 或 `TASKBRIDGE_CONTRACT_UNSUPPORTED` 失败或降级
- **AND** it SHALL NOT 写入 Markdown、`.pinax` planning assets、Git state、任务运行时状态或远端 Provider 状态

### Requirement: Daily TaskBridge planning SHALL write a timestamped Markdown todolist through Pinax

Pinax SHALL 让 `pinax plan daily --taskbridge` 通过 Connectors TaskBridge 兼容合同消费每日任务事实，并在明确批准时把带时间戳的 Markdown todolist 写入当天日记的 Pinax managed block。

#### Scenario: TaskBridge daily dry-run is read-only
- **GIVEN** Pinax vault 与有效的 `connectors task agent today` 响应
- **WHEN** 用户运行 `pinax plan daily --vault ./my-notes --taskbridge --dry-run --json`
- **THEN** stdout SHALL 包含一个 Pinax JSON envelope，带有 `source=taskbridge`、`captured_at`、选中承诺数量、目标日记路径和建议下一步
- **AND** Pinax SHALL NOT 修改 Markdown、`.pinax` planning assets、Git state、任务运行时状态、Provider 状态或远端服务

#### Scenario: approved daily plan writes managed block
- **GIVEN** 当日日记不存在或具有有效的 `planning-daily` managed block
- **WHEN** 用户运行 `pinax plan daily --vault ./my-notes --taskbridge --yes`
- **THEN** Pinax SHALL 通过 journal 和 planning services 创建或更新 `daily/YYYY-MM-DD.md`
- **AND** it SHALL 只在 `<!-- pinax:managed name=planning-daily -->` 与 `<!-- /pinax:managed -->` 内写入 Markdown todolist
- **AND** managed block SHALL 包含 `Captured at: <RFC3339 UTC>`
- **AND** Pinax SHALL 保留 managed block 外的用户内容

#### Scenario: TaskBridge adapter failure is safe
- **GIVEN** 首选任务运行时不可用、返回无效 JSON 或返回不支持的 schema
- **WHEN** 用户运行 `pinax plan daily --vault ./my-notes --taskbridge --yes --json`
- **THEN** Pinax SHALL 返回 `TASKBRIDGE_UNAVAILABLE` 或 `TASKBRIDGE_CONTRACT_UNSUPPORTED`
- **AND** it SHALL NOT 写入 Markdown、`.pinax` planning assets、Git state、任务运行时状态、Provider 状态或远端服务

#### Scenario: daily planning actions remain drafts
- **GIVEN** TaskBridge 兼容任务事实产生了 deferred candidates
- **WHEN** 用户运行 `pinax plan actions --vault ./my-notes --from daily --taskbridge --save --json`
- **THEN** Pinax SHALL 在 `.pinax/planning/actions/` 下写入 `taskbridge.actions.v1` draft
- **AND** 下一步 SHALL 为 `connectors task agent execute --action-file <path> --dry-run`
- **AND** Pinax SHALL NOT 调用 `connectors task agent execute --confirm`
