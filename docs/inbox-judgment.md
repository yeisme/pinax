# Inbox Judgment 校准与渐进接入说明

状态：**exploratory**。对应 OpenSpec 变更 [pinax-inbox-judgment-v1](../openspec/changes/pinax-inbox-judgment-v1/)；实现位于 `internal/inboxjudgment`。

本能力为笔记 inbox 分类、关联与重复线索提供结构化判断**建议**。模型只出建议；原确定性规则、原 inbox 审阅流程与人工确认保持权威。默认 `off`，零发现、零远程调用；不会随版本升级自动启用，仅由用户通过实验性开关（见第 3 节）显式开启。

## 1. 固定基线

- **对照组是原流程本身**。任何判断建议都与"用户按现有 `pinax inbox show` / `pinax inbox promote` / `pinax inbox discard` 流程整理收件箱"的结果对比，不引入第二套真值。
- **确定性前置规则永远先于模型**：权限与 vault 隔离（跨 vault 默认不取材）、授权集合绑定（inbox 笔记与全部候选都必须位于授权集合内，空集合 = deny-all）、必需字段与 revision 绑定、字节数与候选数上限、敏感形态 fail closed、内容 digest 全等（`exact_content_digest`，确定性重复线索，无需概率）。
- **离线合同基线**：`go test ./internal/inboxjudgment` 覆盖 wire 合同（schema "1.0"、8 错误码 + submission_state/retry_class、choice/ordinal_score/binary 原语、pair 对齐）、off 零调用、缓存权限撤销、stale/权限撤销不可采纳、零网络 replay。
- **场景矩阵基线**：`TestInboxJudgmentScenarioMatrix`（inbox-link、duplicate-warning、vault-isolation、failure-injection）经本项目 evidence runner 落证据到 `temp/integration-test-runs/inbox-judgment-*/`，六类证据齐全；`TestInboxJudgmentScenarioFailureEvidence` 证明失败运行同样保留原始退出码与完整证据。

## 2. 校准集与留出集（真实模型，另行 opt-in）

合同测试通过不代表效果通过。真实模型效果评估必须显式开启并满足：

1. **数据**：从用户自愿标注的 inbox 整理记录构造小型已标注集；20–30 条只能作为探索起点，不得标注 mature。校准集与留出集严格分离，留出集只评估一次策略变更后统一复核。
2. **双语**：zh 与 en 分别采样；`language` 问题的答案分布进入 evidence（`languages` 字段），用于按语言分层度量。
3. **度量**（与原流程对比记录误报/漏报）：
   - 链接建议 precision / recall（建议的关联是否被用户接受）；
   - 重复线索误报率（`exact_content_digest` 基线应为零误报；`model_relation` 需单独统计）；
   - 有用建议率（用户对 assist 建议的显式接受率）；
   - 拒答率（abstained / error 占比）与缺答阻止采纳的比例；
   - 延迟与已知用量（usage unknown 时如实记录 unknown，不补零）。
4. **阈值**：按任务与语言分别校准，记录在本文件附表中；刻意不设全局 0.8 之类的默认阈值（见 policy digest 绑定 `exploratory-v1`）。

## 3. 实验性开关配置面（experimental）

用户通过 `judgment` 配置节显式开启本能力（experimental）。默认完全休眠：`enabled: false` + `mode: off`。

| 配置 key | 默认 | 取值 | 说明 |
| --- | --- | --- | --- |
| `judgment.enabled` | `false` | `true` / `false` | 实验性总开关 |
| `judgment.mode` | `off` | `off` / `shadow` / `assist` | `off` 即完全休眠 |

```bash
pinax config set judgment.enabled true --scope user
pinax config set judgment.mode shadow --scope user
pinax config get judgment.mode --agent
pinax config doctor --json   # judgment_status: experimental_off / experimental_shadow / experimental_assist
```

环境变量等价物：`PINAX_JUDGMENT_ENABLED=true`、`PINAX_JUDGMENT_MODE=shadow`；也可用 `pinax config unset judgment.mode --scope user` 移除配置。优先级与既有配置一致（flag > env > project > user > default）。

- **默认与 off 完全休眠**：inbox 流程零 judgment 装配、零 transport 调用；`AcceptInboxJudgmentSuggestion` 采纳门一律拒绝并提示未启用（`judgment_not_enabled`），原 inbox 审阅流程保持权威。
- **非法组合 fail-fast**：`enabled: false` 却配置 `shadow`/`assist`（含环境变量关闭后 mode 残留）、或未知 mode 值，配置加载即报 `config_invalid`，不做静默降级。
- **enabled 只解除开关**：启用模式所需的 transport、精确模型 pin 与授权仍须显式注入；配置文件不保存、也不自动发现 adapter 或凭据。
- 开关装配门实现于 `internal/inboxjudgment/gate.go`（`ExperimentalJudgmentGate`：休眠零装配、启用沿用既有 consumer/采纳门语义）。

## 4. 渐进接入阶梯

| 阶段 | 模式 | 行为 | 进入条件 |
| --- | --- | --- | --- |
| 0 | `off`（默认） | 原流程不变，零调用 | 出厂状态 |
| 1 | `shadow` | 只比较建议与基线，不展示给采纳路径 | `judgment.enabled=true` + `judgment.mode=shadow` + 注入显式 transport + 精确模型 pin + 校准集建立 |
| 2 | `assist` | 展示建议、引用、缺失项与原审阅入口 | shadow 误报/漏报达标且用户显式开启 |
| 3 | live canary | 小样本真实评估 | 留出集指标达标，另行 owner 记录 |

每一步都要求：数据授权 + 付费授权显式给出；普通 read/status 命令不隐式触发；`shadow` 建议永不 adoptable。

## 5. 关闭与恢复

- 关闭即把 `judgment.mode` 设回 `off`（或移除 `judgment` 配置节 / 注入的 transport 配置）：原命令、默认配置与 canonical state 全部保持，历史 evidence 只读保留，不删除用户数据或凭据。
- 已缓存的判断 evidence 在权限撤销、vault scope 变化或候选/inbox 笔记离开授权集合时自动失效删除；缓存 key 绑定模式（shadow evidence 不会 replay 给 assist consumer）；重放旧 evidence 零网络、不新建 attempt。
- 回滚后如需再次启用，从阶段 1 重新进入，不复用旧阈值结论。

## 6. 边界（不变的领域限制）

- 不自动合并或删除笔记、不确认长期记忆、不修改 vault canon、不启动同步或发布。
- 采纳建议仍要经过 owner 原有权限/审阅门（`pinax inbox ...` + `--yes`）；`AcceptInboxJudgmentSuggestion` 只做门检并枚举原入口命令，自身不执行业务写入。
- 提交后超时或结果不明记为 `outcome_unknown`，不自动重发、不切换付费模型；不可用显示 unavailable，不伪装成"没有问题"。
- SDK 不读取密钥；显式 transport 指向经授权的 adapter，credentialctl grant 由 adapter 取用。未启用时不要求安装 provider CLI 或启动外部服务。

## 7. 接入模式记录

- 公共 SDK（`github.com/yeisme/judgment-sdk`，合同 "1.0"）未发布前，本仓在 `internal/inboxjudgment/wire.go` 以零外部依赖钉住冻结合同形状（snake_case wire、DescribeCapabilities/Evaluate、8 错误码、三原语），standalone 构建不受 SDK 树漂移影响。
- 真实 SDK 引用通过 build-tag 隔离的 bridge（`judgment_sdk_bridge.go`，tag `judgment_sdk`）验证兼容性；默认构建不编译该文件。详见文件头注释与 `openspec/changes/pinax-inbox-judgment-v1/tasks.md` 任务 1.2 的 evidence。
