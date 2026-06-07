## 1. Command Factory 基础拆分

- [ ] 1.1 新增 `internal/cli` 包，定义 `Deps`，包含 app service、version、stdin/stdout/stderr 注入点和后续 config/render options 插槽。
- [ ] 1.2 将 root command 创建逻辑迁移到 `internal/cli.NewRootCommand(deps Deps)`，保持当前外部行为不变。
- [ ] 1.3 将 help template、output mode selection、output mode conflict validation、flag error rendering 迁移到 `internal/cli` helper。
- [ ] 1.4 将 `cmd/pinax/main.go` 瘦身为版本注入、root command 创建、Execute 和退出码处理。
- [ ] 1.5 增加 root command factory 单元测试，确认多次创建 command 不共享 flag 状态或 writer 状态。
- [ ] 1.6 增加当前命令树 smoke 测试，确认拆分前后 `pinax --help`、`pinax version --json` 行为不回归。

## 2. 按领域拆分现有命令文件

- [ ] 2.1 抽出 `vault_cmd.go`，承载 init、vault-wide stats/validate/doctor/dashboard 相关 builder。
- [ ] 2.2 抽出 `note_cmd.go`，承载 note create/list/show/read/edit/open/relationship/attachment/mutation/tag builder。
- [ ] 2.3 抽出 `journal_cmd.go`，承载 daily/weekly/monthly open/show/append 的共享 period builder。
- [ ] 2.4 抽出 `inbox_cmd.go`、`view_cmd.go`、`template_cmd.go`、`project_cmd.go`。
- [ ] 2.5 抽出 `storage_cmd.go`、`index_cmd.go`、`sync_cmd.go`、`git_cmd.go`、`mcp_cmd.go`。
- [ ] 2.6 抽出 `organize_cmd.go`、`repair_cmd.go`、`metadata_cmd.go`，复用计划/应用类 helper。
- [ ] 2.7 保持 command 层只构造 request、调用 app service、调用 renderer，不直接写 vault 或 `.pinax` structured assets。

## 3. Vault 主路径和兼容 Alias

- [ ] 3.1 新增 `pinax vault` command group。
- [ ] 3.2 新增主路径 `pinax vault stats`，复用现有 stats service 和 projection。
- [ ] 3.3 新增主路径 `pinax vault validate`，复用现有 validate service 和 projection。
- [ ] 3.4 新增主路径 `pinax vault doctor`，复用现有 doctor service、flags 和 projection。
- [ ] 3.5 新增主路径 `pinax vault dashboard`，复用现有 dashboard 启动逻辑。
- [ ] 3.6 保留 root `stats`、`validate`、`doctor`、`dashboard` 作为兼容 alias，可在 help 中标注或隐藏。
- [ ] 3.7 增加 `--json` alias 等价测试：root 旧路径与 `vault` 新路径输出 envelope 等价。

## 4. Journal 主路径和兼容 Alias

- [ ] 4.1 新增 `pinax journal` command group。
- [ ] 4.2 新增 `pinax journal daily open/show/append`，复用现有 daily request 和 journal loader 行为。
- [ ] 4.3 新增 `pinax journal weekly open/show/append`，复用现有 weekly service。
- [ ] 4.4 新增 `pinax journal monthly open/show/append`，复用现有 monthly service。
- [ ] 4.5 保留 root `daily`、`weekly`、`monthly` 作为兼容 alias，可在 help 中标注或隐藏。
- [ ] 4.6 增加 alias 等价测试，覆盖 `show --json`、`append --body ... --json` 和 editor-open request 构造。
- [ ] 4.7 确认 journal completion 只做轻量读取，不触发写入或远程操作。

## 5. Note 和维度命令整理

- [ ] 5.1 保持 `pinax note create/new`、`show/read`、`edit/open` 共享同一 builder 和 handler。
- [ ] 5.2 增加 note alias 等价测试，确认同参数下 aliases 产生等价 projection。
- [ ] 5.3 评估并实现维度主路径：`pinax note tags`、`pinax note folders`、`pinax note kinds`、必要时 `pinax note groups`。
- [ ] 5.4 保留 root `tag list`、`folder list`、`kind list`、`group list` 作为兼容 alias 或迁移说明。
- [ ] 5.5 更新 `pinax note --help`，让高频 note 工作流更易扫描，兼容 alias 不挤占主说明。
- [ ] 5.6 增加维度命令 default human 和 `--json` 输出合同测试。

## 6. Planning 和 Storage 命令整理

- [ ] 6.1 统一 `organize plan/list/apply` 为主路径，明确 `organize suggest` 是否作为 alias 或 agent-oriented 入口保留。
- [ ] 6.2 为 `organize plan` 与 `organize suggest` 的兼容关系添加中文注释和测试。
- [ ] 6.3 确认 `metadata plan/apply` 和 `repair plan/apply` 保持安全 gate，不因 tree refactor 改变写入条件。
- [ ] 6.4 新增 `pinax storage set local`，复用现有 `storage set-local` service。
- [ ] 6.5 新增 `pinax storage set s3`，复用现有 `storage set-s3` service。
- [ ] 6.6 保留 `storage set-local` 和 `storage set-s3` 作为兼容 alias。
- [ ] 6.7 增加 storage alias 等价测试，并确认 S3 secret 不进入 stdout/stderr/fixture。

## 7. Help、Completion 和文档

- [ ] 7.1 更新 root help，使主路径按 vault、note、journal、inbox、search、view、organize、template、config、storage、index、sync、git、mcp 分组或至少更易扫描。
- [ ] 7.2 将新 examples 改为主路径，例如 `pinax vault doctor`、`pinax journal daily show`、`pinax storage set s3`、`pinax organize plan`。
- [ ] 7.3 为兼容 alias 设置 `Hidden` 或明确标注“兼容入口”，避免 root help 膨胀。
- [ ] 7.4 检查 shell completion，确保新增路径 completion 不写 vault、不写 `.pinax`、不调用 provider、不改 Git。
- [ ] 7.5 更新 Pinax docs/README 或 CLI 运行文档中的命令示例，不新增 docs checklist 作为执行状态。
- [ ] 7.6 增加 help smoke/golden 测试，覆盖 root、vault、journal、storage、organize、note help。

## 8. 输出合同和回归验证

- [ ] 8.1 增加 primary path 与 alias path 的 `--json` 等价测试，覆盖 vault、journal、storage、note、organize。
- [ ] 8.2 增加 `--agent` smoke，确认 alias 不输出中文 prose 或 ANSI。
- [ ] 8.3 增加默认 human smoke，确认新主路径输出仍为中文摘要或表格。
- [ ] 8.4 增加参数错误测试，确认新主路径和 alias 都返回 stable error code、中文 message 和 runnable hint。
- [ ] 8.5 运行 `gofmt -w <changed-go-files>`。
- [ ] 8.6 运行 `go test ./...`。
- [ ] 8.7 运行 `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax` 或 `task check`。
- [ ] 8.8 运行 `openspec validate --all`。
- [ ] 8.9 手动 smoke：在临时 vault 中运行旧路径和新路径，确认用户可见行为、机器输出和写入安全 gate 未变化。
