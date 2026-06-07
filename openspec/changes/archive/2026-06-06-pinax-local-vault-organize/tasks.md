# Tasks: Pinax Local Vault Organize

## 1. OpenSpec 和输出边界

- [x] 1.1 完成本 change 的 proposal/design/tasks/spec。
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: none
  - Acceptance: `openspec validate pinax-local-vault-organize`
  - Failure re-check: 如果缺少 Mermaid 图、spec scenario 或任务验收，补齐后重跑。
  - Evidence: 2026-06-06 运行 `openspec validate pinax-local-vault-organize`，退出码 0，显示 change valid。

## 2. 本地 vault 基础

- [x] 2.1 用 TDD 增加 `init`、`validate`、`note list/show` 和 `search` 行为。
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 1.1
  - Acceptance: `go test ./internal/app ./internal/vault ./cmd/pinax -run 'Vault|Note|Search|Init|Validate' -count=1`
  - Failure re-check: 如果测试依赖真实用户目录或网络，改用临时目录 fixture。
  - Evidence: 2026-06-06 先运行聚焦测试，因缺少 `app.NewService`、MCP server 和 CLI 命令失败；实现后运行 `go test ./internal/app ./internal/mcpserver ./cmd/pinax -run 'Vault|Note|Search|Init|Validate|Metadata|Organize|MCP|Agent|LocalVault' -count=1`，退出码 0。

- [x] 2.2 实现 vault repository、frontmatter parser 和 command projection。
  - Owner: `cli/pinax`
  - Lane: A
  - Depends on: 2.1 red
  - Acceptance: `go test ./internal/app ./internal/vault ./internal/output -count=1`
  - Failure re-check: 如果 CLI 命令层拼业务逻辑，抽回 app/vault service。
  - Evidence: 2026-06-06 运行 `go test ./...`，退出码 0；`internal/app` 负责 vault scan/frontmatter/use case，`internal/output` 负责 projection 渲染，`cmd/pinax` 只做命令接线。

## 3. Metadata 和整理落地

- [x] 3.1 用 TDD 增加 metadata plan/apply 和 organize plan/apply。
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 2.2
  - Acceptance: `go test ./internal/app ./internal/vault ./cmd/pinax -run 'Metadata|Organize' -count=1`
  - Failure re-check: 如果 apply 没有 `--yes` 或路径逃逸保护，阻塞实现。
  - Evidence: 2026-06-06 运行 `go test ./internal/app ./internal/mcpserver ./cmd/pinax -run 'Vault|Note|Search|Init|Validate|Metadata|Organize|MCP|Agent|LocalVault' -count=1`，退出码 0，覆盖 metadata apply 需要 `--yes`、organize apply 需要 snapshot。

- [x] 3.2 实现 Git snapshot adapter 和 organize apply 保护门禁。
  - Owner: `cli/pinax`
  - Lane: B
  - Depends on: 3.1 red
  - Acceptance: `go test ./internal/git ./internal/app ./cmd/pinax -run 'Git|Snapshot|Organize' -count=1`
  - Failure re-check: 如果无 snapshot 仍能真实改文件，修正为失败 envelope。
  - Evidence: 2026-06-06 新增 `internal/git` snapshot adapter；运行聚焦测试和 `go test ./...`，退出码 0；CLI JSON 失败测试确认无 snapshot 时返回 `snapshot_required`。

## 4. 文档和验证

- [x] 4.1 更新 README 和 docs，说明本地整理闭环和真实命令。
  - Owner: `cli/pinax`
  - Lane: C
  - Depends on: 3.2
  - Acceptance: `rg -n "pinax init|metadata plan|organize apply|git snapshot" README.md docs`
  - Failure re-check: 文档不得要求 agent 手写 `.pinax/*.yaml` 或事件 JSONL。
  - Evidence: 2026-06-06 已更新 `README.md`、`docs/product/mvp-scope.md`、`docs/operations/local-development.md`、`docs/README.md`，只展示真实用户可运行命令。

- [x] 4.2 运行完整门禁并记录 evidence。
  - Owner: `cli/pinax`
  - Lane: sequential
  - Depends on: 4.1
  - Acceptance: `task check`
  - Failure re-check: 如果本机无 `task`，运行 `gofmt -w <changed-go-files>`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`、`openspec validate --all`。
  - Evidence: 2026-06-06 运行 `go test ./... && go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax && openspec validate --all`，退出码 0；运行 `task check`，退出码 0，覆盖 test、OpenSpec、fmt-check 和 build。
