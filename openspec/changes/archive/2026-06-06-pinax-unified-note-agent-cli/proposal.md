## Why

根仓库已经完成 `pinax-unified-note-agent-cli` 的设计归档，明确 Pinax 必须作为 `cli/pinax` 独立 CLI 子项目落地。现在需要先建立本地开发底座，让后续业务实现、测试证据、文档和 closeout 都有正确 owner。

根级来源：

- `openspec/specs/pinax-project-routing/spec.md`
- `openspec/changes/archive/2026-06-05-pinax-unified-note-agent-cli/`

## What Changes

- 新建 Pinax Go CLI 子项目底座。
- 增加 `AGENTS.md`、`CLAUDE.md`、`docs/README.md` 和初始 docs 分区。
- 增加子项目 OpenSpec config、baseline spec 和本实现 change。
- 增加最小 `pinax version` / `pinax doctor` 命令，用于验证 Go/Cobra 工程可构建。
- 后续业务能力仍必须拆到新的 `pinax-*` OpenSpec change。

## Non-Goals

- 不实现 vault、sync、provider、briefing、MCP 或 Feishu 业务能力。
- 不接入真实 Notion、飞书、Hermes、internet-access 或外部网络凭据。
- 不创建 `.pinax/` 运行数据或机器可读 vault metadata。

## Impact

- 子项目影响：`cli/pinax`。
- 根仓库影响：`.gitmodules`、submodule gitlink、`.skills/profiles/targets/cli/pinax.txt` 和生成的 runtime skills 副本。

