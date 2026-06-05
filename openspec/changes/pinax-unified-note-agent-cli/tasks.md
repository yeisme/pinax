# Tasks: Pinax Unified Note Agent CLI Bootstrap

## 使用规则

- 本任务包只记录 Pinax 子项目底座落地。
- 不实现 vault、provider、sync、briefing、MCP 或 Feishu 业务能力。
- 每个完成项需要追加 `Evidence:`，记录命令、退出码或关键结论。

## 1. 子项目底座

- [x] 1.1 创建 Go CLI 工程骨架。
  - Owner: `cli/pinax`
  - Scope: `go.mod`、`cmd/pinax`、最小 `version` / `doctor`、internal ownership marker。
  - Acceptance:
    ```bash
    go test ./...
    go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
    ```
  - Evidence: 2026-06-05 运行 `gofmt -w cmd/pinax/main.go cmd/pinax/main_test.go internal/app/doc.go internal/domain/doc.go internal/output/doc.go internal/redaction/doc.go internal/testkit/doc.go`，退出码 0；运行 `go mod tidy`，退出码 0；运行 `go test ./...`，退出码 0；运行 `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`，退出码 0；运行 `./dist/pinax version` 和 `./dist/pinax doctor`，退出码 0。

- [x] 1.2 创建子项目指令和文档入口。
  - Owner: `cli/pinax`
  - Scope: `AGENTS.md`、`CLAUDE.md`、`docs/README.md` 和初始文档分区。
  - Acceptance:
    ```bash
    test -f AGENTS.md
    test -f CLAUDE.md
    test -f docs/README.md
    ```
  - Evidence: 2026-06-05 已创建。

- [x] 1.3 创建子项目 OpenSpec 底座。
  - Owner: `cli/pinax`
  - Scope: `openspec/config.yaml`、baseline spec 和本 change。
  - Acceptance:
    ```bash
    openspec validate --all
    ```
  - Evidence: 2026-06-05 运行 `openspec validate --all`，退出码 0，输出 `2 passed, 0 failed`。

## 2. Go 开发生态

- [x] 2.1 增加 Go 开发生态和 Taskfile 入口。
  - Owner: `cli/pinax`
  - Scope: 参考 Cohors `task build` 的使用体验，新增 Go CLI 版 `Taskfile.yml`，覆盖 `build`、`test`、`fmt`、`fmt-check`、`tidy`、`openspec`、`check`、`clean`。
  - Depends on: 1.1
  - Lane: A
  - Acceptance:
    ```bash
    task --list
    task build
    task check
    ```
  - Failure re-check: 如果 `task build` 报 `No Taskfile found`，确认当前目录是 `cli/pinax` 且存在 `Taskfile.yml`；如果未安装 `task`，运行底层 Go/OpenSpec 命令。
  - Evidence: 2026-06-05 已新增 `Taskfile.yml`；验证命令见 5.1。

- [x] 2.2 增加 Go 开发生态文档。
  - Owner: `cli/pinax`
  - Scope: 新增 `docs/architecture/go-development-ecosystem.md`，记录 Taskfile、Go 包边界、依赖默认值、输出合同、测试分层和任务切片顺序。
  - Depends on: 2.1
  - Lane: A
  - Acceptance:
    ```bash
    rg -n "Taskfile|GORM|testscript|internal/config|internal/provider|task check" docs/architecture/go-development-ecosystem.md
    ```
  - Failure re-check: 如果文档要求 agent 手写结构化 metadata，改为 CLI/service authored；如果文档推荐非 GORM 业务持久化，改回 GORM repository。
  - Evidence: 2026-06-05 已新增文档，并更新 `README.md`、`docs/README.md`、`docs/operations/local-development.md`、`AGENTS.md`。

## 3. 实现任务矩阵

本节是后续 `pinax-*` 实现 change 的拆分输入。每一行都应在对应 change 中展开为更细任务、测试证据和 closeout。

| 能力任务 | Owner | Scope | Depends on | Lane | Acceptance | Failure re-check |
| --- | --- | --- | --- | --- | --- | --- |
| Dev task surface | `cli/pinax` | Taskfile、Go build/test/fmt/check、OpenSpec 校验入口、README/docs 同步 | Bootstrap | A | `task build && task check` | 如果 `task` 不存在，文档必须给出 `go test ./...`、`go build ...`、`openspec validate --all` 等价命令 |
| CLI command layer | `cli/pinax` | `internal/cli` command factory、version injection、context、root flags、completion strategy | Dev task surface | A | `go test ./cmd/pinax ./internal/cli -count=1` | 如果业务逻辑进入 Cobra `RunE`，抽回 app service |
| Config foundation | `cli/pinax` | `internal/config`、Viper defaults、env prefix `PINAX_`、project/local config、typed validate、secret_ref | CLI command layer | B | `go test ./internal/config ./cmd/pinax -run Config -count=1` | 如果普通只读命令隐式写 config，改为显式 `pinax config set` |
| Output projection foundation | `cli/pinax` | `internal/domain.CommandProjection`、`internal/output` summary/agent/json/events/explain renderer、error envelope | CLI command layer | B | `go test ./internal/output ./cmd/pinax -run 'Output|JSON|Agent|Events|Explain' -count=1` | 如果机器输出解析中文摘要，改为同源 projection 多 renderer |
| Redaction foundation | `cli/pinax` | `internal/redaction` 规则、token/webhook/raw payload/path redaction、fixture tests | Output projection | B | `go test ./internal/redaction ./internal/output -count=1` | 如果 stdout/stderr/event 泄漏 token、webhook 或 raw provider payload，阻塞发布 |
| Runtime adapters | `cli/pinax` | `internal/runtime` clock、filesystem、process runner、context cancellation、timeout、stderr capture | CLI command layer | C | `go test ./internal/runtime -count=1` | 如果外部命令无 timeout 或 cancellation，补 runner 边界 |
| Testscript harness | `cli/pinax` | `tests/e2e`、`testdata/script`、二进制构建 fixture、stdout/stderr golden | Dev task surface | C | `go test ./tests/e2e -run TestScripts -count=1` | 如果 e2e 依赖真实用户目录、真实 token 或公网，改用 fixture/fake executable |
| Local vault workbench | `cli/pinax` | `pinax init`、vault layout、`note new/list/show`、frontmatter、path safety、validate | Config + Output + Testscript | D | `go test ./internal/vault ./internal/notes ./tests/e2e -run 'Vault|Note' -count=1` | 如果 agent 需要手写 `.pinax/config.yaml`，补 CLI/service |
| GORM index and search | `cli/pinax` | SQLite/GORM index、tag/link/backlink/search、rebuild projection、migration notes | Vault workbench | E | `go test ./internal/index ./internal/vault -run 'Index|Search|Link' -count=1` | 如果业务层出现硬编码 SQL，改为 GORM repository 或记录允许例外 |
| Git safety adapter | `cli/pinax` | Git status、changed paths、snapshot plan、rollback hint、temp repo tests | Vault workbench | E | `go test ./internal/git ./cmd/pinax -run Git -count=1` | 如果命令层直接解析复杂 Git porcelain，移动到 adapter/service |
| Provider capability probe | `cli/pinax` | `internal/provider` interface、`ntn` / `lark-cli` fake executable、capability schema、provider doctor | Runtime + Output + Redaction | F | `go test ./internal/provider/... ./cmd/pinax -run Provider -count=1` | 如果测试需要真实 Notion/Lark token，改用 fake executable |
| Sync engine | `cli/pinax` | diff/pull/push/conflict queue、mapping、sync-state、dry-run/yes、idempotency、event evidence | Provider + Index + Git | sequential | `go test ./internal/sync ./tests/e2e -run Sync -count=1` | 如果冲突静默覆盖本地或远端，阻塞实现 |
| Agent output contract | `cli/pinax` | `--agent`、`--json`、`--events`、`--explain` 全命令 contract tests | Output + Vault + Sync | G | `go test ./internal/output ./tests/e2e -run OutputContract -count=1` | 如果 machine stdout 混入 progress/log，修正 stdout/stderr 分离 |
| Local MCP stdio | `cli/pinax` | `pinax mcp serve`、resources/tools/prompts、read-only tools、write dry-run/approval-required | Agent output + App services | H | `go test ./internal/mcpserver ./tests/e2e -run MCP -count=1` | 如果 MCP tool 绕过 app service 或默认远端写入，退回 dry-run/approval gate |
| Daily hot notes briefing | `cli/pinax` | recipe、research request、evidence ledger、dedupe/scoring、review queue、Feishu delivery receipt、feedback | Vault + Index + Provider + Output | I | `go test ./internal/briefing ./tests/e2e -run Briefing -count=1` | 如果低来源候选进入飞书 top notes 或反馈不回写 Pinax，阻塞发布 |
| Release and CI baseline | `cli/pinax` | GitHub Actions、Go cache、`task check`、artifact naming、version injection、release notes gate | Dev task + stable CLI | J | `go test ./... && task build` | 如果 CI 使用未文档化 secret 或跳过 OpenSpec/测试，补 gate |

## 4. 产品切片路线

| Phase | Owner | Deliverable | Validation |
| --- | --- | --- | --- |
| Phase 0: Go Dev Ecosystem | `cli/pinax` | Taskfile、docs、output/config/runtime/testscript skeleton | `task check` |
| Phase 1: Local Vault Workbench | `cli/pinax` | init/doctor/note/search 基础本地体验 | `go test ./tests/e2e -run LocalVault -count=1` |
| Phase 2: Safe Knowledge Index | `cli/pinax` | GORM index、tags、backlinks、search、validate | `go test ./internal/index ./internal/vault -count=1` |
| Phase 3: Git + Provider Pull | `cli/pinax` | Git snapshot plan、provider doctor、sync diff/pull dry-run | `go test ./tests/e2e -run 'Git|Provider|SyncDiff' -count=1` |
| Phase 4: Agent/MCP Read and Plan | `cli/pinax` | agent/json/events/explain、MCP read-only resources/tools | `go test ./tests/e2e -run 'OutputContract|MCP' -count=1` |
| Phase 5: Controlled Apply | `cli/pinax` | action apply、sync pull --yes、本地事件、conflict queue | `go test ./tests/e2e -run ControlledApply -count=1` |
| Phase 6: Daily Hot Notes | `cli/pinax` + research/delivery owner | evidence ledger -> review queue -> delivery receipt -> feedback | `go test ./tests/e2e -run Briefing -count=1` |

## 5. 验证

- [x] 5.1 校验 Go 开发生态入口。
  - Owner: `cli/pinax`
  - Scope: 确认 `task build` 不再报 `No Taskfile found`，并且 `task check` 覆盖 format、test、build、OpenSpec。
  - Depends on: 2.1, 2.2
  - Lane: sequential
  - Acceptance:
    ```bash
    task --list
    task build
    task check
    ```
  - Failure re-check: 如果本机未安装 `task`，运行 `gofmt -w cmd internal`、`go test ./...`、`go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`、`openspec validate --all` 并记录原因。
  - Evidence: 2026-06-05 运行 `task --version && task --list`，退出码 0，显示 `build`、`test`、`fmt`、`fmt-check`、`openspec`、`check`、`clean`；运行 `task build`，退出码 0，生成 `dist/pinax`；运行 `task check`，退出码 0，覆盖 `openspec validate --all`、`go test ./...`、`fmt-check` 和 build。

- [x] 5.2 校验 OpenSpec 结构。
  - Owner: `cli/pinax`
  - Scope: 确认扩展后的 design/spec/tasks 能通过 OpenSpec 校验。
  - Depends on: 5.1
  - Lane: sequential
  - Acceptance:
    ```bash
    openspec validate --all
    ```
  - Failure re-check: 根据校验错误修正 spec 或任务结构后重跑。
  - Evidence: 2026-06-05 运行 `openspec validate --all`，退出码 0，输出 `spec/pinax` 和 `change/pinax-unified-note-agent-cli` 均通过，`2 passed, 0 failed`。

- [x] 5.3 校验文档覆盖。
  - Owner: `cli/pinax`
  - Scope: 确认 README、docs、AGENTS 和 Go 生态设计都出现 Taskfile、Go build、OpenSpec 和 fallback 命令。
  - Depends on: 2.2
  - Lane: sequential
  - Acceptance:
    ```bash
    rg -n "task build|task check|go build|openspec validate --all|Go 开发生态" README.md AGENTS.md docs openspec/changes/pinax-unified-note-agent-cli
    ```
  - Failure re-check: 如果只在 OpenSpec 写任务但没有用户可读文档，补 `docs/**`。
  - Evidence: 2026-06-05 运行 `rg -n "task build|task check|go build|openspec validate --all|Go 开发生态" README.md AGENTS.md docs openspec/changes/pinax-unified-note-agent-cli`，退出码 0，命中 README、AGENTS、docs 和 OpenSpec。
