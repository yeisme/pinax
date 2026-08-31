# Future hosted Pinax service admission packet

## Status

`candidate / not approved`。`backend-server/pinax-service` 只是便于讨论的候选名；本 change 不批准创建项目、迁移数据真源、部署服务或引入多用户能力。

## Decision To Make

根级产品/架构 OpenSpec 必须先回答：future hosted service 是单用户自托管 Pinax owner、托管的个人 vault service、团队知识服务，还是仅代理本地 owner 的 control plane。不同答案会改变数据真源、身份、同步、备份、定价和项目 owner，不能由 `cli/pinax` 的现有 loopback API 自动推导。

## Admission Classification

在根级决策完成前，当前分类是 `split-owner candidate / reject-now for implementation`：合同候选可以继续研究，但不批准在 `cli/pinax`、`mcp/gateway` 或现有 backend 中直接加入 hosted business state。

| 结论 | 何时成立 | 后续动作 |
| --- | --- | --- |
| `fit` | 有明确 target user/JTBD，hosted owner 的数据真源、身份、备份、运营责任与付费/采用证据闭环。 | 新建独立 backend subproject/OpenSpec；`cli/pinax` 只提供 SDK/domain contract。 |
| `split-owner` | 产品价值成立，但 identity、storage、MCP transport、client UX 属于不同 owner。 | 冻结 provider-neutral API/event contracts，再按 owner 建依赖 DAG。 |
| `reject-now` | 需求可由 local/self-hosted owner 满足，或数据真源/多用户/运营问题未决。 | 保留本 handoff，不创建项目、不部署、不扩公网监听。 |

## Owner-Fit Questions

1. Markdown vault 是否仍是每个 workspace 的 canonical source，还是 hosted relational/object state 成为新真源？
2. 服务是单用户、单组织还是多租户？tenant/workspace/project/principal 如何隔离？
3. 远程写是直接修改 server-owned vault，还是向 device owner 发 operation？离线和冲突如何处理？
4. Cloud Sync 与 Remote API 的关系是什么？是否会形成两个并行写真源？
5. 身份由哪个 owner 提供：personal access key、workload identity、OAuth/OIDC 或 shared identity platform？
6. 谁拥有 backup、restore、retention、audit、rate limit、abuse prevention、SLO 和 incident response？
7. Body exposure、full-text search、Agent memory 和 provider egress 在多人场景下如何授权？
8. 哪个付费/使用场景证明需要 hosted service，而不是 self-hosted loopback/HTTPS owner？

任一问题没有可信答案时，admission 应保持 `reject-now` 或 `split-owner`，不得为了“支持远程”把公网监听加入 `pinax api serve`。

建议根级决策顺序：产品需求与 owner-fit → 数据真源/迁移 rollback → identity/tenant isolation → API/operation recovery → backup/restore/ops → MCP/gateway/client composition → private canary → production promotion。后序 owner 不得先于前序合同自行开工。

## Required Capability Ledger

| Capability | Candidate owner | Admission requirement |
| --- | --- | --- |
| Pinax domain/projection contract | `cli/pinax` | stable API/SDK manifest，不移动现有 local source silently |
| hosted runtime/API | future backend subproject | independent AGENTS/OpenSpec/repo/submodule |
| identity and credential issuance | identity platform or backend | issue/list/revoke/rotate、scope、expiry、audit |
| multi-tenant relational storage | backend | Go application layer使用 GORM；无 handler raw SQL |
| object/Markdown storage and backup | backend/storage owner | encryption、revision、backup/restore evidence |
| Streamable HTTP MCP | `mcp/gateway` + `mcp/pinax` | 不由 hosted domain handler 内嵌 MCP protocol |
| client UX | future cross-platform client | 只消费 API/events，不成为业务真源 |

## Minimum Hosted Contract If Admitted

- 非 loopback 仅 HTTPS；TLS termination、Host/Origin、proxy trust 和 redirect policy 显式配置。
- 可撤销 access key/workload identity；credential 与 provider secret 分离；服务端只保存 secret hash或批准的 secret-store ref。
- principal/tenant/workspace/project/purpose scope；所有 read/write/operation/resource 都先授权。
- typed OpenAPI 3.1、transport manifest、readiness、stable error、request/operation correlation。
- mutation idempotency、expected revision、operation status/reconcile、receipt 和 rollback。
- path-free note/resource/artifact ref；不得返回 host filesystem path、DB key 或长期签名 URL。
- rate limit、quota、audit、backup/restore、monitoring、deployment health 和 production readiness evidence。
- stdout/log/evidence 不保存 raw prompt、note body、Authorization header、provider payload、hidden prompt 或 full chain-of-thought。

## Explicit Non-Goals For First Hosted Slice

- 不做万能远程 shell 或 arbitrary CLI execution。
- 不让 client/MCP 直接读取数据库、对象存储或 server vault path。
- 不把 local Cloud Sync transport 当作组织级 backend API。
- 不在没有 operation recovery 的情况下开放大范围 mutation。
- 不因为 route/fixture 存在就声称 multi-user production ready。

## Suggested First Product Slice

如果 root admission 证明值得建设，最小 slice 应是“一个 authenticated principal 操作一个 server-owned disposable/test workspace”的 private self-hosted canary：

1. manifest/readiness/auth；
2. bounded note list/read/search；
3. 一个 idempotent inbox capture canary；
4. operation status/reconcile；
5. backup/restore smoke；
6. sibling MCP read-only composition。

团队 RBAC、共享编辑、组织搜索、billing 和公网 production promotion 必须在该 canary 之后分别晋级。

## Admission Acceptance Criteria

- 根级 proposal 明确 target user、job-to-be-done、付费/运营假设、owner-fit 和 source-of-truth。
- 根级 design 明确 identity、tenant isolation、data flow、operation recovery、backup/restore、MCP/gateway 边界和 rollback。
- backend subproject 具有独立 repository、AGENTS.md、CLAUDE.md、skills profile、OpenSpec、docs source of truth 和 integration evidence runner。
- localhost fixture、remote auth canary、backup/restore 和 security tests 分层报告 readiness，不合成 production ready。
- 任何 live deployment、credential provisioning、DNS、付费资源或外部写入仍需要用户明确授权。
- readiness 必须逐层证明 `contract`、`transport`、`auth`、`owner`、`mutation_recovery`、`production`；private canary、loopback fixture 或 backup mock 均不得单独把 `production` 标为 ready。

## Rollback Requirement

Hosted canary 的停止或回滚 MUST NOT 破坏 local Pinax vault、local CLI/MCP、Cloud Sync 或 existing remote CLI contract。若 hosted data 成为新真源，迁移设计 MUST 先提供可逆 export/handoff 回 local Markdown vault；没有该路径不得进入 production admission。
