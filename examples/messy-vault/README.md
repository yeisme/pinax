# Messy Vault (Pinax Dogfood Demo Fixture)

这是 Pinax agent-safe proof loop 的标准 demo fixture：一个故意被弄乱的合成 Markdown vault，包含 6 类典型问题，用于演示 **诊断 → 计划 → 快照 → 应用 → 回滚** 的完整 proof loop。

## 设计原则

- **合成数据**：所有人名、项目名、内容都是虚构占位符，不含真实数据、credentials、tokens 或 webhook URL。
- **可复现**：所有 Pinax reviewer、CI、销售 demo、用户访谈都跑同一个 fixture，保证 aha moment 一致。
- **本地可运行**：所有命令只读 / 写本地 vault，不依赖网络、provider 或 Cloud Sync。

## 包含的 6 类问题

| 问题类型 | 对应 note | `vault doctor` issue code | repair plan action |
|---|---|---|---|
| Broken link | `notes/research/auth-design.md`（含 `[[Nonexistent]]`） | `broken_link` | manual review |
| Orphan note | `notes/research/api-notes.md`（无入链/出链） | `orphan_note` | manual review |
| Missing metadata | `notes/research/meeting-2026.md`（缺 tags/kind/status） | `missing_tags` | automatic（低风险 metadata 补全） |
| Duplicate title | `notes/projects/pinax-plan.md` 与 `notes/projects/pinax-plan-2.md` 同名 | `duplicate_title` | manual review |
| Empty body | `notes/inbox/random-thought.md`（frontmatter 后无正文） | `empty_note` | manual review |
| Stale note | `notes/archive/old-spec.md`（>90 天未更新） | `stale_note` | archive suggestion |

## 快速开始

从仓库根目录运行（假设已经 `go build -o dist/pinax ./cmd/pinax` 或使用 `go run`）：

```bash
# 1. （首次运行）把 stale note 的修改时间固定到 120 天前
touch -d "120 days ago" examples/messy-vault/notes/archive/old-spec.md

# 2. 诊断 — 发现全部 6 类问题
go run ./cmd/pinax vault doctor --vault ./examples/messy-vault --json

# 3. 完整 proof loop 见 docs/demo-proof-loop.md
```

> Stale note 检测依赖文件 mtime，Git 不保留 mtime，所以 `old-spec.md` 在 clone 后需要用 `touch -d "120 days ago"` 把 mtime 回拨，才能稳定复现 `stale_note`。E2E 测试 (`tests/e2e`) 的 Setup 会自动做这件事。

## 安全保证

这个 fixture 本身不含敏感数据；用 Pinax 跑 proof loop 时，`--json` / `--agent` / `--events` 默认只返回 bounded projection（事实 + 下一步动作），不泄漏完整正文、token 或 provider payload。所有写入都经过 plan → snapshot → apply → receipt → restore 控制链。

详细 demo 脚本、预期输出和讲解 tips 见 [`docs/demo-proof-loop.md`](../../docs/demo-proof-loop.md)。
