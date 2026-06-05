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

## 2. 后续 Handoff

- [ ] 2.1 创建本地 vault MVP change。
  - Owner: `cli/pinax`
  - Scope: `pinax init`、`doctor`、`note new/list/show`、frontmatter、Git snapshot plan 和输出合同。
  - Acceptance:
    ```bash
    openspec new change pinax-local-vault-mvp
    openspec validate pinax-local-vault-mvp
    ```
