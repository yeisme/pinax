# Tasks

## 1. 领域合同冻结

- [ ] 1.1 冻结信任/生命周期 frontmatter 字段（`generated`/`verified`/`stale_after`、bare-mapping 兼容、ISO8601 fail-closed）、actor 约定与派生规则（`TrustTier`/`Freshness`，绝不落盘）。Validation: `openspec validate pinax-okf-trust-discovery-v1 --strict --no-interactive`。

## 2. 领域与索引实现

- [ ] 2.1 实现 `internal/domain` TrustSignals 解析 helper + 派生函数；单测覆盖：空字段、bare mapping、非法时间戳 fail-closed、未知 actor 容忍、human 前缀分级。Validation: `go test ./internal/domain -count=1`。
- [ ] 2.2 索引 note records 增派生列（trust/stale_after/verified_at_latest），rebuild/refresh 维护并保持可重建；guard 不变。Validation: `go test ./internal/index -count=1`。

## 3. 维护命令

- [ ] 3.1 实现 `pinax note verify <ref>`（幂等、identity 默认 actor、无配置 fail-closed、atomic frontmatter patch）+ testscript e2e。Validation: `go test ./cmd/pinax -run NoteVerify -count=1`。
- [ ] 3.2 `pinax metadata plan/apply` 支持 `trust_fields` 回填操作，复用既有安全模型与 receipt。Validation: `go test ./internal/app -run Metadata -count=1`。

## 4. 搜索面

- [ ] 4.1 实现 `--facets`（消费时合成、全匹配集计数、稳定排序）与 human/json/agent 三模式渲染。Validation: `go test ./cmd/pinax -run Search -count=1`。
- [ ] 4.2 实现 `--trust`/`--stale` 过滤与徽标（ASCII 徽标、notty 退化、agent `trust=`/`fresh=` 字段）；默认行为零变更回归。Validation: `go test ./cmd/pinax -run Search -count=1`。
- [ ] 4.3 实现 `pinax search show <ref>` 聚合详情卡（信任面板/snippet/链计数/邻居；歧义 fail-closed）。Validation: `go test ./cmd/pinax -run SearchShow -count=1`。
- [ ] 4.4 实现 `pinax browse`（合成目录视图、只读、`--lazy-index off` 不写索引）+ e2e 只读断言。Validation: `go test ./cmd/pinax -run Browse -count=1`。

## 5. Agent Brain 消费

- [ ] 5.1 `brain answer` 候选携带 trust/fresh 标注（unverified/stale 靠后不剔除）；`brain maintain` 产出 stale human-review 重新验证候选。Validation: `go test ./internal/app -run Brain -count=1`。

## 6. 收口

- [ ] 6.1 docs（`docs/commands/search.md` 增 facets/show/trust、新 `docs/commands/browse.md` 与 `note verify` 段）+ CLI 输出合同 golden + 递归 body-leak 扫描覆盖新面。Validation: `task check`。
- [ ] 6.2 e2e testscript 全链（verify → facets → trust/stale 过滤 → show → browse 只读）+ 集成证据。Validation: `task test:integration`。
