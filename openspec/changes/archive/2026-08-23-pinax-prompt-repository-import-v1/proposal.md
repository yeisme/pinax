# Why

Pinax 已拥有本地 `yeisme.prompt_asset.v1` Prompt Vault：用户可以 create/import/search/show/resolve，并由 Pinax 管理 draft、tested、accepted、promoted、retired 生命周期。但它只能处理用户已经准备好的本地 schema 文件，不能复用 promptrepo 中的用户目录、Git/GitHub、对象存储和官方中文方案，也不能直接使用统一 TemplateAddress 的 inspect/validate/preview。

Pinax 不应复制 source adapters、共享 user profile、taxonomy 或 Registry 状态。它需要成为一个明确的 domain consumer：外部 catalog 用于发现与验证，显式 install 在 rights 允许时把正文和 schema 导入 Pinax-owned PromptAsset draft，后续 lifecycle 仍完全由 Pinax 决定。

# Admission Decision

`split-owner`：promptrepo 拥有 repository source/state/address/contract/policy/preview；Template Registry/内容仓库拥有 organization distribution 与官方内容；Pinax 拥有本地 Prompt Vault、正文版本、source refs、lifecycle 与 feedback。用户通过 `pinax prompt` 进入，但共享 RepositoryProfile 不复制进 vault 或 Pinax SQLite。

# What Changes

- 在现有 `pinax prompt` 下 additive 规划 `repository` 与 `catalog` 子组；现有 `prompt search/show/resolve` 保持本地 Vault 语义。
- 通过公共 promptrepo 管理 user-level repositories，并读取 organization/project/session effective RepositorySet。
- 提供 `catalog search|show|resolve|inspect|validate|preview|install`；inspect/validate/preview provider-free、默认零写入、正文不进入任何输出。
- `catalog install` 只有在 rights/permission 允许 import/copy 时才读取 verified body，并通过 Pinax application service 创建 lifecycle=`draft` 的本地 PromptAsset/version。
- 安装保存 exact TemplateAddress、snapshot/catalog/template/contract digest、locale、rights/trust 和 stage receipt refs；不得自动 tested/accepted/promoted。
- 增加 stable `operation_id`，同时保持 Pinax 既有 envelope、agent/JSON/events 与 command identity 兼容。
- 中文内容默认 `zh-CN`；machine keys、tags、capabilities 与 reason codes 保持英文稳定。

# Required Capability Ledger

| Capability | Status | Canonical owner | Visible host | Acceptance evidence |
| --- | --- | --- | --- | --- |
| shared repository profiles/sync | required | promptrepo | Pinax CLI | cross-CLI shared profile fixture |
| effective scope/policy | required | promptrepo + Registry + Pinax project binding | Pinax CLI | deny-wins/pin precedence tests |
| catalog discovery/address | committed | promptrepo | `pinax prompt catalog` | exact ref and Chinese search tests |
| inspect/validate/preview | committed | promptrepo + Pinax output | Pinax CLI | provider panic/body sentinel tests |
| body import permission | committed | promptrepo contract + Pinax service | `catalog install` | preview-only refusal/import-allowed tests |
| local PromptAsset lifecycle | retained | Pinax | existing `pinax prompt` | draft-only install and lifecycle regression |
| credential value | retained | credentialctl/secret store | resolver boundary | secret sentinel tests |

# Compatibility and Rollback

- 不删除、重命名或复用现有 `prompt create|import|search|show|resolve|lifecycle|feedback`。
- `prompt search` 永远搜索本地 Vault；远程/官方搜索必须使用 `prompt catalog search`。
- 新 provenance、operation ID 和 scope refs additive；旧 PromptAsset fixtures 与 SQLite rows 继续可读。
- 回滚移除新 command registration、bridge 与 dependency；已显式安装的本地 PromptAsset 仍是普通 Pinax draft，不删除正文或 source refs。
