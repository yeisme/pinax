## Why

Pinax 的产品主语需要回到本地 Markdown 笔记 CLI：用户首先需要管理自己的 vault，而不是接入一个泛化 agent 平台。现有能力已经覆盖 init、metadata、organize、search 和 index 的基础方向，但还缺少能让用户持续理解和治理笔记库的统计、健康检查和本地可视化入口。

## What Changes

- 新增 vault 统计能力，提供笔记数量、目录分布、标签分布、创建/更新趋势、frontmatter 覆盖率和索引状态摘要。
- 新增 vault 健康检查能力，识别缺标题、缺标签、缺 Pinax metadata、重复标题、孤立笔记、长期未更新笔记、空笔记、路径异常和索引过期等问题。
- 新增本地 dashboard 能力，通过 `pinax dashboard` 启动只绑定本机的只读 Web UI，用于查看统计、健康问题和最近活动。
- 新增 dashboard 数据 API 或静态数据投影，复用 CLI application service，不绕过 vault 边界直接读写 `.pinax/` 机器资产。
- 为 `pinax stats`、`pinax doctor`、`pinax dashboard` 定义 human、`--json` 和 `--agent` 输出边界。
- 不把 Pinax 改造成 agent 平台；本 change 不实现 provider 接入、云同步、长期 daemon、LLM 自动整理或真实 token 成本追踪。

## Capabilities

### New Capabilities

- `vault-dashboard-health`: 本地 Markdown vault 的统计、健康检查和只读 dashboard 能力。

### Modified Capabilities

- `pinax`: 明确 note CLI 主线增加 stats、doctor、dashboard 作为本地 vault 管理能力，并要求相关输出遵守 Pinax AI-native CLI 输出合同。

## Impact

- CLI：新增 `pinax stats`、`pinax doctor`、`pinax dashboard` 命令及相关 flags。
- 应用层：新增 vault analytics、health audit 和 dashboard read model service。
- 输出层：新增 stats/doctor/dashboard projection，覆盖 human、`--json`、`--agent` 输出。
- 持久化/索引：读取 Markdown vault 和现有 `.pinax/index.sqlite` 投影；允许在 `.pinax/` 下通过 CLI/service 写入 dashboard cache 或 health receipt，但 MVP 优先计算型只读输出。
- 测试：增加命令级和 testscript 覆盖，验证 stdout/stderr 分离、路径边界、无网络依赖、无 provider 凭据依赖和 dashboard 只读行为。
