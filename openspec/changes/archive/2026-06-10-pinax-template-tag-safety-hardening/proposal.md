## Why

当前模板和标签路径已经进入日常笔记核心工作流，但 review 发现几个信任边界缺口：tag 值可以破坏 YAML frontmatter，设计稿模板会被当生产模板执行，query-backed preview 会隐式写 `.pinax/index.sqlite`。这些问题会让 CLI-authored metadata 不再可信，也会让用户以为是只读的交互实际产生 vault 状态变化。

## What Changes

- 强化 tag 输入和 frontmatter 写入：所有 CLI/service 写入的 tag 必须经过统一校验和规范化，拒绝 YAML 注入、换行、控制字符和会破坏 inline/list 表达的字符。
- 阻断 `pinax.template_design.v1` 设计稿执行：设计稿只能 inspect/validate，不能 preview/render，也不能被 `note new --template` 当作生产模板执行。
- 保证 `template preview` 的只读语义：preview 不得创建或更新 `.pinax/index.sqlite`、render receipt、event 或其它 structured assets。
- 让 v2 note template metadata 真实参与 note 创建：`output.path_pattern`、`defaults.kind`、`defaults.status` 等默认值由 note application service 应用，显式 CLI 参数优先。
- 修复 template example context：`template preview` 在未提供显式参数时使用 `example` 中的 title/project/tags/vars，显式参数覆盖 example。
- 补齐 template inspect/output 合同：starter note template 同时给 preview 和 create action，并统一 machine facts key。
- 让 `note tag` mutation 与 record ledger/index/output facts 对齐，至少明确 record event、index 更新或 stale 状态。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `pinax`: 调整模板执行、template preview 只读、template inspect actions、safe function/output 合同。
- `note-command-ux`: 调整 note tag 输入校验和 tag mutation 输出事实。
- `vault-record-ledger`: 明确 note metadata/tag mutation 的 record event 行为。
- `notebook-workflows`: 调整内置 note template metadata 在 note 创建中的应用行为。

## Impact

- 代码范围：`internal/cli/template_cmd.go`、`internal/cli/note_cmd.go`、`internal/app/service.go`、`internal/app/builtin_templates.go`、`internal/templateengine/*`、`internal/output/*`、`internal/domain/records.go`、`internal/index/*`。
- 测试范围：`internal/app`、`internal/templateengine`、`cmd/pinax` 的 CLI contract 和安全回归测试，必要时补充 testscript/e2e。
- 数据影响：新增或调整 record event kind/facts 时需要保持现有 ledger replay 兼容；不迁移用户 Markdown 正文。
- 输出影响：新增稳定 error code 和 facts/actions，不破坏既有 `--json` envelope、`--agent` 基础字段和默认中文摘要。
