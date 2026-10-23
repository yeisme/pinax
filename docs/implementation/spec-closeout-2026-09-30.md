# Pinax 本地 OpenSpec 收尾（2026-09-30）

笔记提案与渲染预览共 40 项任务完成。两项能力保持 experimental，owner 为 `cli/pinax`。本次授权提前实施，continuity 原样本、门槛与最终决策保留。

## 交付

- 笔记提案：四种 typed kind、低风险自动应用、编辑人工审批、revision 防漂移、snapshot/apply/readback/receipt、原 proposal 查询与 metadata 恢复；MCP 写能力默认关闭。
- 渲染预览：共享 CLI/MCP projection、稳定锚点、四类 bounded fence、正文权限、固定与 floating 引用、HTML 转义；读取不创建索引 sidecar、快照或引用状态。
- 文档、命令手册和 router 源技能已同步，现有 memory proposal 与 legacy preview 行为保留。

## 归档与 canonical specs

通过 `openspec archive <change> --yes` 同步 canonical specs 并归档：

- [agent-note-proposal](../../openspec/changes/archive/2026-09-30-pinax-agent-note-proposal-v1/tasks.md)：16 项完成。
- [rendered-view](../../openspec/changes/archive/2026-09-30-pinax-rendered-view-v1/tasks.md)：24 项完成。
- [inbox-judgment](../../openspec/changes/archive/2026-09-30-pinax-inbox-judgment-v1/tasks.md)：已有完成项经本地回归核验。
- [mounted-vault](../../openspec/changes/archive/2026-09-30-pinax-mounted-vault-local-v1/tasks.md)：已有完成项经本地回归核验。

## 验证

下列路径均位于本项目 `temp/integration-test-runs/`，包含 summary.json、command.txt、stdout.log、stderr.log、env.json 与 artifacts/；脱敏扫描通过。

| 检查 | 证据目录 | 结果 |
| --- | --- | --- |
| agent-note-proposal | `20260930T034433Z-1199919/` | passed，exit 0 |
| rendered-view-parity | `20260930T034551Z-1273832/` | passed，exit 0 |
| 最终 task check | `20260930T041030Z-2741326/` | passed，exit 0 |

最终 `task check` 包含 Go 测试、CLI/E2E/MCP SDK 与 legacy 回归、格式、lint（0 issues）、构建、renderer Vitest（4/4）与类型检查、OpenSpec 校验。未运行 race。首次全量检查失败证据 `20260930T035718Z-1939160/` 保留；修复 typed-nil fence 错误、Goldmark 废弃接口和新增工具数量断言后通过。OpenAPI 回归同时验证去除两个新增 schema 后仍精确匹配原 digest。

源技能与 root、cli/pinax、data/yeisme-notes 定向 profile 校验、同步和双运行目录比对通过。全局 profile 校验仍受既有缺失 `apigateway/aigora-web-console` profile 阻塞，未改动该无关项目。

## 保留门禁

五个活跃 change 的 11 项真实验收任务继续未勾选；本地 fixture 不代表自然使用或发布验收。

| Change | 保留证据 |
| --- | --- |
| daily-knowledge-loop | 主力环境两周自然使用 |
| agent-memory-runtime | 连续四周观察及真实 adapter 覆盖 |
| trusted-continuity-dogfood | 至少 30 个真实 loops 与最终 Go/Iterate/Stop |
| real-judgment-acceptance | 真实模型执行与用户复核 |
| passphrase-s3-bootstrap | Mac 发布候选、双向 canary 与恢复矩阵 |

本次交付仅为本地实现、验证与归档；未执行远端推送、发布或真实外部操作。
