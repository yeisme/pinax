# Design: Pinax Dogfood Demo Vault

## Demo Vault 设计

### Vault 结构

```
examples/messy-vault/
├── notes/
│   ├── research/
│   │   ├── auth-design.md          # 有 frontmatter，有 broken link → [[Nonexistent]]
│   │   ├── api-notes.md            # 有 frontmatter，orphan（无 incoming/outgoing links）
│   │   └── meeting-2026.md         # 缺 tags/kind/status metadata
│   ├── projects/
│   │   ├── pinax-plan.md           # duplicate title with another note
│   │   └── pinax-plan.md           # 第二个同名 note（duplicate title）
│   ├── inbox/
│   │   └── random-thought.md       # empty body，inbox 状态
│   └── archive/
│       └── old-spec.md             # stale note（90天未更新），archive 状态
├── .pinax/
│   └── config.yaml                 # 最小 config
└── README.md                       # demo 说明
```

### 故意制造的问题清单

| 问题 | 对应 diagnose 输出 | 对应 repair plan action |
|---|---|---|
| Broken link `[[Nonexistent]]` | `broken_links` | manual review（不自动修复） |
| Orphan note `api-notes.md` | `orphan_notes` | manual review |
| Missing metadata（tags/kind/status） | `missing_metadata` | auto-fix（低风险 metadata 补全） |
| Duplicate title `pinax-plan.md` | `duplicate_titles` | manual review |
| Empty body `random-thought.md` | `empty_notes` | manual review |
| Stale note `old-spec.md`（>90d） | `stale_notes` | archive suggestion |

### 预期 proof loop 行为

```bash
# 1. Diagnose — 发现 6 类问题
pinax vault doctor --vault ./examples/messy-vault --json
# 预期: broken_links=1, orphan_notes=1, missing_metadata=1, duplicate_titles=1, empty_notes=1, stale_notes=1

# 2. Plan — 生成可审查计划
pinax repair plan --vault ./examples/messy-vault --save --json
# 预期: 低风险 auto-fix = metadata 补全; manual review = broken/orphan/duplicate/empty/stale

# 3. Snapshot — 保护当前状态
pinax version snapshot --vault ./examples/messy-vault --message "before demo apply"

# 4. Apply — 只应用低风险操作
pinax repair apply --vault ./examples/messy-vault --plan <plan-id> --yes
# 预期: metadata 补全成功; manual review 项不变

# 5. Restore — 证明可回滚
pinax version restore notes/research/meeting-2026.md --revision HEAD --plan --vault ./examples/messy-vault
pinax version restore apply --vault ./examples/messy-vault --plan <restore-id> --yes
# 预期: 文件恢复到 snapshot 前状态
```

### MCP bounded context demo（可选段）

```bash
# Agent 通过 MCP 读取 bounded context，不接触原始文件
pinax mcp serve --vault ./examples/messy-vault
# Agent 调用 tools 获取 card/detail projection，不是 full body
```

### Testscript E2E 测试

使用 `testscript` 验证 demo vault 完整 proof loop：
- `demo_diagnose.txt`：doctor 发现预期问题集
- `demo_plan_snapshot_apply.txt`：plan → snapshot → apply 链
- `demo_restore.txt`：restore 回滚链

### Demo 文档结构

`docs/demo-proof-loop.md`：
1. 场景设定（"你的 vault 越来越乱，让 agent 安全整理"）
2. Step-by-step 命令（可复制运行）
3. 预期输出摘要
4. 安全保证说明（plan/snapshot/receipt/restore/no-plaintext）
5. 讲解要点（给做 demo 的人看的 tips）

## 安全约束

- demo vault 不含真实笔记内容、人名、项目名
- 不含 provider credentials、tokens、webhook URL
- `.pinax/config.yaml` 只有最小配置
- testscript 不依赖网络/外部服务

## 验证策略

- `task test` 包含 demo proof loop E2E
- demo vault 文件可以被 `pinax vault doctor` 正确诊断
- plan/apply/restore 链可完整运行
- 不引入需要网络或 credentials 的依赖

## 延期项

- 视频/动画 demo 录制
- 真实 Obsidian vault 导入工具
- Cloud Sync demo（等用户验证 proof loop 后再加）
