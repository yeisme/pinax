# Pinax 项目工作区层级、API 与 CLI 输出美化

## 为什么

用户已经在 Pinax vault 中用项目目录承载研究、学习、写作、复盘和工具候选需求。当前 `pinax project` 已能创建项目、切换项目、展示本地 project board，但还缺一层适合“特殊需求项目”的本地工作单元：它不是新的 Yeisme 工程项目，也不是 GitHub/Gitea issue tracker，而是 vault 内围绕某个目标持续积累资料、运行记录、输出、复盘和工具候选的 workspace。

如果不支持子项目和更强看板，用户会继续手动维护目录、Markdown task、项目状态和复盘结构，agent 也缺少稳定 API/CLI 合同去读取和推进这些工作。Pinax 应该先提供本地项目管理控制面，再考虑是否把反复出现的痛点升级为脚本、skill、CLI 或 agent。

## 做什么

本变更在现有 Pinax project board 基础上增量增加：

1. **Subproject / Scenario 工作单元**：在现有 project 下创建 `subproject`，对应 vault 内标准目录结构。
2. **项目目录模板**：默认创建 `00-charter`、`10-inbox`、`20-sources`、`30-runs`、`40-outputs`、`50-retros`、`90-tool-candidates`。
3. **看板增强**：支持按 project + subproject 过滤看板，提供 item add/move/archive、labels、milestone、priority、due date、blocked_by 等本地字段。
4. **API/RPC surface**：新增 local REST/RPC projection adapter，让 dashboard、agent 和本地工具读取项目、子项目、看板和 item；写操作仍受 `--allow-write`、`yes=true`、snapshot gate 约束。
5. **CLI 输出美化**：默认 human summary 更像项目管理工具；`--json`、`--agent`、`--events` 继续由同一 projection 渲染，新增字段只做 optional additive 扩展。

## 不做什么

- 不创建新的 Yeisme 工程项目、Git 仓库、submodule、CLI 子项目或 agent 子项目。
- 不同步 GitHub、Gitea、GitLab、Linear、Jira 或 TaskBridge 远端写入。
- 不实现多人权限、评论线程、通知系统、PR/commit 关联自动化。
- 不把 Pinax 变成长期 daemon 或完整远端 issue tracker。
- 不删除或重命名现有 `pinax project board show`、`ProjectBoard.Show`、`GET /v1/projects/{slug}/board` 等稳定合同。

## 兼容性策略

本变更必须是增量兼容：

- CLI 只新增子命令、flag 和 optional 输出字段。
- JSON/agent/events 只新增 optional `subproject`、`labels`、`milestone`、`priority`、`due_at`、`blocked_by`、`workspace_path` 等字段。
- REST/RPC 只新增 endpoint/method 或给现有 board endpoint 增加 optional `subproject` query 参数。
- `.pinax/projects.json` 继续可读；新的子项目 registry 作为 additive structured asset 写入。
- 旧项目和旧 board config 没有 subproject 时继续按 project-wide board 工作。

## 成功标准

- 用户能运行 `pinax project subproject create research stock-learning --vault yeisme-notes --json` 创建本地工作区。
- 用户能运行 `pinax project board show research --subproject stock-learning --vault yeisme-notes` 得到清晰看板摘要。
- agent 能用 `--agent` 低 token 读取 project/subproject/board/item facts。
- local API 能读取 project、subproject、board、item projection，且写操作不绕过 app service。
- `task check`、focused command tests、API contract tests、integration evidence 全部通过。

