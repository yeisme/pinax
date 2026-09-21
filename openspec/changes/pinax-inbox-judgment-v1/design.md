# Design

## Context

本变更准备 笔记 inbox 分类、关联与重复线索建议。当前产品已有独立状态与审阅流程，接入不能把模型判断升级为 canonical truth。公共 SDK 与 TypeSafe adapter 尚待实现；此文档是实施方案，不表示能力已上线。

## Goals / Non-Goals

split-owner：Pinax 是笔记与记忆事实 owner，模型仅辅助审阅；无独立知识状态库。领域持有问题集、规则、权限和采纳；SDK 只负责基础合同与 transport。不得自动合并或删除笔记、确认长期记忆、修改 vault canon、启动同步或发布。不新增独立服务平台、通用 Agent 或第二套领域数据库。

## Decisions

### 消费位置与数据流

实施入口为 internal/memoryinbox、internal/search、internal/memory 与既有 vault application service。如需新增内部模块，应复用这些应用服务边界，而非复制业务状态机。

```mermaid
flowchart LR
  I[领域已授权输入] --> P[规则检查与最小文本投影]
  P --> S[公共 SDK / 显式 transport]
  S --> E[领域脱敏 evidence 与待审建议]
  E --> R[原有人工审阅]
  R --> A[原有显式采纳服务]
```

输入：明确选中的 inbox 文本、当前 vault 内获授权的候选笔记摘要及 revision；跨 vault 默认不取材。模型只接收有限 inline_text，不自行抓取 source URL、读取任意文件或解释权限。问题示例：“这条 inbox 内容适合链接哪条已有笔记？两条笔记是重复、补充还是矛盾？” 每个问题保持原子化和明确答案域，question_set/version/digest 由 owner 维护，输出 pair 绑定原 candidate/question，缺项不得靠顺序猜测。

输出：建议分类/链接/重复候选，交给原 inbox 审阅入口显式接受或拒绝。确定性检查先于模型；涉及权限、必需字段、类型和可计算约束的规则不能用概率替代。使用既有 CLI/application service 产生结构化投影和审阅状态，不手写元数据文件。

### 领域专属约束

将新建议保存在现有 review/evidence 服务允许的投影中；笔记正文和 schema-bearing metadata 仍通过 CLI/application service 修改。读笔记、搜索和 sync 不得隐式触发付费判断。用户反馈只改该建议的采纳状态；若用于问题集校准，需单独选择，不能静默上传整库。

### 交互与失败处理

默认 `off`：保持原命令、默认配置与数据，零发现/远程调用。显式 `shadow`：只比较建议与基线，也必须有数据与付费授权，不能由普通 read/status 命令暗中触发；显式 `assist`：展示建议、引用、缺失项和下一步现有审阅入口。首版不添加全屏 TUI 或自建客户端；已有 CLI/API 输出即可消费。

人工可查看理由摘要、接受/拒绝建议或回到原流程；接受建议仍要经过 owner 原有权限/审阅门。需要补充输入时给出具体缺失项；模型离线/不可用时显示 unavailable，不伪装成“没有问题”。低信心是 abstained，网络失败是执行错误，提交后超时是 outcome_unknown，不自动重发或切到另一个付费模型。旧 evidence 可零网络 replay；明确重新评估才建立新 attempt。

问题集/策略/模型采用精确版本，阈值按任务和语言校准，无全局 0.8 默认值。probability、provider confidence、source reliability 和事实正确性分开；未来 adapter 不提供的字段为 null，不补造。必需问题任何缺项/拒答阻止完整建议采纳。缓存由 owner 管，绑定 principal/project、授权版本、source/question/policy/model/adapter digest；发出请求和读缓存前都要核验权限。source 或权限变化后结果过期，禁止用于新版本采纳。

### 证据与兼容

证据保存 source refs/revisions/digests、规范化条目、question/policy version、exact model/adapter、attempt、已知 usage 或 unknown、拒答/错误类别及已有 review refs。不记录 raw prompt、原始 provider payload、secret、hidden prompt 或思维链。日志/输出使用脱敏英文摘要；用户设计文档使用中文。既有 JSON envelope、agent keys 和历史 reader 保持兼容，新信息只通过 owner 版本化可选投影增加。

SDK 不读取密钥；显式 transport 指向经过授权的 adapter，由 adapter 取用 credentialctl grant。领域配置只保存合法的引用与 transport 标识，不复制真实 key。未启用能力时不要求安装 provider CLI、启动 Aigora 或配置 TypeSafe。

## Scenario Matrix

| scenario_id | 目标用户 | job-to-be-done | 必需产物 | gate/review | export/handoff | readiness |
| --- | --- | --- | --- | --- | --- | --- |
| inbox-link | 笔记作者整理收件箱 | 给出可审阅的关联建议 | inbox/note revision refs | 用户接受/拒绝 | 原 inbox review | exploratory |
| duplicate-warning | 作者发现近似笔记 | 解释重复或补充关系但不合并 | 两条 note refs | 无自动删除 | 原 merge/edit 入口 | exploratory |
| vault-isolation | 多 vault 用户 | 阻止跨 vault cache/候选泄露 | vault scope refs | 权限隔离 | 原访问边界 | exploratory |

各行 evidence 路径统一为本项目 `temp/integration-test-runs/<run-id>/artifacts/`，引用原 owner source/review 状态，不创建另一套状态。各行 validation command 为 `go test ./...`，实施时使用既有测试组织给场景建立非零用例；integration/component/e2e 通过本项目 evidence runner 包装，不能仅凭未匹配任何测试的退出码通过。

## Migration Plan

1. 先以假 transport 固定领域投影、问题集与版本绑定。
2. 公共 SDK 已可独立消费后添加可选依赖；TypeSafe adapter/grant 未就绪时保持 off 或 fixture，不阻塞原功能。
3. 增量增加审阅建议与 evidence reader；不回填或重写原业务文件与历史 hash。
4. 完成离线合同测试后才允许显式 live canary/校准；不在升级时自动启用。
5. 回滚将该可选能力关闭，恢复原流程，保留历史只读证据；不删除用户数据或凭据。

## Risks / Trade-offs

- [看似客观的高 confidence] → 清楚标识辅助建议，保留原审阅门与拒答；校准前不声称可靠。
- [上下文不足和中文效果差异] → 原子问题、限定输入、双语/领域留出集；不以连通测试冒充效果评估。
- [隐私、延迟和重复计费] → 最小授权投影、输入/调用上限、deadline、unknown outcome 与显式重试。

## Validation

当前方案验证：`openspec validate pinax-inbox-judgment-v1 --strict --no-interactive`。

实施时复用现有测试库与 runner：`go test ./...`；先运行本变更的 focused tests，最后按本项目 AGENTS 完成必要质量门。离线测试覆盖上表业务情景、off 零调用、旧输出兼容、权限撤销、过期版本、缺失 required answer、null confidence、提交后断连、零网络 replay、恶意输入不能变成动作。不得读取用户实际项目或密钥，不依赖付费服务。

integration/component/e2e 通过现有 evidence runner 记录 `summary.json`、`command.txt`、`stdout.log`、`stderr.log`、`env.json`、`artifacts/`，失败保留原退出码和脱敏证据。真实模型效果需另行 opt-in：以小型已标注集探索，校准集与留出集分离，与原流程比较误报、漏报、有用建议、拒答率、延迟及已知用量；20–30 条只能作为探索起点，不能标注 mature。阈值和推广条件由本领域评估记录固定，合同测试通过不代表效果通过。

## Dependencies

- [公共 SDK 合同](../../../../../apigateway/aigora/openspec/changes/aigora-structured-judgment-sdk-v1/design.md)
- [可选 TypeSafe 适配器](../../../../../apigateway/aigora/openspec/changes/aigora-typesafe-judgment-adapter-v1/design.md)
- [跨项目 handoff](../../../../../openspec/changes/structured-judgment-sdk-adoption-v1/design.md)
