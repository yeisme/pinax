# Pinax 完整使用样例

本文给出一条从空目录开始的完整 Pinax 使用路线。核心流程只依赖本地 Markdown vault；Capsa Sync、API、plugin、Feishu 发布和 backend inspection 是可选扩展。示例命令使用真实 `pinax` 命令，不使用本地 agent wrapper。

## 0. 准备

从源码构建或安装 release 后确认命令可用：

```bash
pinax version
```

如果在源码 checkout 内开发，可以先构建本地二进制：

```bash
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
./dist/pinax version
```

下文默认 `pinax` 已在 `PATH` 中。

## 1. 创建本地 vault 并写入内容

```bash
pinax init ./my-notes --title "My Knowledge Base"
pinax vault validate --vault ./my-notes --json
pinax inbox capture "Investigate local-first notes" --vault ./my-notes --json
pinax note add "Research Log" --body "First note" --tags research --vault ./my-notes --json
pinax journal daily append "Reviewed Pinax proof loop" --vault ./my-notes --json
```

这些命令只写本地 vault 和 CLI-authored `.pinax/` 资产，不连接云端或 provider。

## 2. 建索引、检索和关系检查

```bash
pinax index refresh --vault ./my-notes --json
pinax search "First note" --vault ./my-notes --json
pinax note links "Research Log" --vault ./my-notes --json
pinax note backlinks "Research Log" --vault ./my-notes --json
pinax note orphans --mode full --vault ./my-notes --json
```

SQLite/GORM index 是可重建投影，Markdown note 才是真源。结构异常时先查看诊断，再显式 rebuild：

```bash
pinax index doctor --vault ./my-notes --agent
pinax index rebuild --vault ./my-notes --json
```

## 3. Proof Loop：诊断、计划、快照、应用、回滚

先运行只读预览：

```bash
pinax proof loop run --vault ./my-notes --json
pinax vault doctor --vault ./my-notes --json
```

把问题转为可审阅计划，并在应用前创建 snapshot：

```bash
pinax repair plan --vault ./my-notes --save --json
pinax version snapshot --vault ./my-notes --message "before repair" --json
pinax repair apply --vault ./my-notes --plan <plan-id> --yes --json
```

如果需要回滚单个文件，先生成 restore plan，再显式 apply：

```bash
pinax version restore research-log.md --revision <snapshot-id> --plan --vault ./my-notes --json
pinax version restore apply --vault ./my-notes --plan <restore-id> --yes --json
```

`repair plan` 默认只读；只有 `--save` 写计划资产。`repair apply` 必须有明确计划和 `--yes`，高风险 rewrite 不会自动执行。

## 4. 项目工作区和数据库视图

```bash
pinax project create research --name "Research" --notes-prefix notes/research --vault ./my-notes --json
pinax project subproject create research stock-learning --title "Stock Learning" --template scenario --vault ./my-notes --json
pinax project item add research "Read annual report" --subproject stock-learning --column next --vault ./my-notes --json
pinax project board show research --subproject stock-learning --vault ./my-notes --json
pinax project board export research --format markdown --vault ./my-notes --json
```

保存并渲染一个 database view：

```bash
pinax database view save active-table --display table --query 'SELECT title, status FROM notes WHERE status = "active" LIMIT 20' --vault ./my-notes --json
pinax database view render active-table --vault ./my-notes --json
```

项目和 database view 仍以本地 Markdown 和 CLI-authored metadata 为真源，不把 TaskBridge、provider 或远端平台当作主数据源。

## 5. 本地 API 和 MCP 只读投影

查看 API surface：

```bash
pinax api routes --vault ./my-notes --json
pinax api schema export --format openapi --vault ./my-notes --json
```

启动只读本地 API：

```bash
pinax api serve --vault ./my-notes --readonly --port 8787
curl -s http://127.0.0.1:8787/v1/capabilities
```

需要写入 API 时，必须显式启用写能力和 token，且 token 存在用户级位置或环境变量中，不写入仓库：

```bash
pinax token create --label local-agent --scope read --expires 30d --vault ./my-notes --json
pinax api serve --vault ./my-notes --allow-write --port 8787 --token-file ~/.config/pinax/local-agent.token
curl -X POST 'http://127.0.0.1:8787/v1/memory:capture?yes=true' -H 'Content-Type: application/json' -H "Authorization: Bearer $PINAX_API_TOKEN" -d '{"type":"fact","subject":"pinax","object":"confirmed write through API"}'
```

MCP surface 是只读 bounded projection：

```bash
pinax mcp serve --vault ./my-notes
```

## 6. 双设备 Capsa Sync（file backend 示例）

这个示例使用本地 file backend 模拟两个设备。真实凭据通过环境变量或用户级 secret store 提供，不写入 vault 或仓库。

```bash
export PINAX_SYNC_SECRET="replace-with-local-test-secret"
pinax init ./device-a --title "Device A"
pinax init ./device-b --title "Device B"
pinax note add "Alpha" --body "# Alpha\n\nfrom device A" --vault ./device-a --json
pinax index refresh --vault ./device-a --json
pinax capsa login --endpoint "file://$PWD/.capsa-sync-store" --workspace personal --device laptop --secret-ref env://PINAX_SYNC_SECRET --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./device-a --json
pinax capsa login --endpoint "file://$PWD/.capsa-sync-store" --workspace personal --device desktop --secret-ref env://PINAX_SYNC_SECRET --encryption-secret-ref env://PINAX_SYNC_SECRET --vault ./device-b --json
pinax sync push --target capsa --vault ./device-a --yes --json
pinax sync pull --target capsa --vault ./device-b --yes --json
pinax sync conflicts list --vault ./device-b --json
```

`remote_write=true` 只有在远端 revision durable commit 且本地 sync-state evidence 写入后才成立。dry-run、plan、失败写入和 pull 不应报告远端写成功。

## 7. Backend inspection

配置 backend profile 后，可以只读查看对象或 note-like Markdown 对象：

```bash
pinax backend notes summary work-s3 --vault ./my-notes --json
pinax backend notes list work-s3 notes/ --vault ./my-notes --json
pinax backend notes stat work-s3 notes/cloud-sync.md --vault ./my-notes --json
pinax backend object list work-s3 pinax/ --vault ./my-notes --agent
```

这些 inspection 命令不写本地 notes、`.pinax/**` sync state、远端对象或 provider credentials。

## 8. Publish 和 Feishu native doc

静态发布先 plan/dry-run，再显式 apply：

```bash
pinax publish plan --profile public --target local --vault ./my-notes --json
pinax publish apply --profile public --target local --dry-run --vault ./my-notes --json
```

Feishu native Docs 发布需要用户已配置 `lark-cli`，并使用用户授权的文件夹。下面命令使用占位 token，不要把真实 token 写入文档、fixture 或运行证据：

```bash
pinax publish doc profile set lark-doc --folder <folder-token-or-url> --as user --renderer native-docx --vault ./my-notes --json
pinax publish doc provider doctor --target lark-doc --vault ./my-notes --agent
pinax publish doc prepare --note "Research Log" --target lark-doc --vault ./my-notes --json
pinax publish doc push --package <package-id> --target lark-doc --vault ./my-notes --dry-run --json
pinax publish doc push --package <package-id> --target lark-doc --vault ./my-notes --yes --json
pinax publish doc status --note "Research Log" --target lark-doc --vault ./my-notes --json
```

`--dry-run` 不应远端写入。真实 push 应显式使用 `--yes`，并且输出、receipt、fixture、smoke 记录不得包含 provider token、Authorization header、cookie、raw provider payload 或真实 doc token。

## 9. Plugin dry-run

插件必须通过 audited runtime 和 permission grant 路径运行；调试时先 dry-run：

```bash
pinax plugin validate ./plugins/project-dashboard --vault ./my-notes --json
pinax plugin doctor --vault ./my-notes --json
pinax plugin run project-dashboard render_dashboard --vault ./my-notes --dry-run --agent
```

## 10. 开发者验证

只改文档通常不需要跑 Go 全量测试。改 Go 代码、输出合同、provider、sync、evidence 或 OpenSpec 时运行：

```bash
task check
```

涉及 integration/e2e 证据时运行：

```bash
task test:integration
find temp/integration-test-runs -maxdepth 3 -type f | sort
```

没有安装 `task` 时运行 fallback：

```bash
golangci-lint fmt --diff
golangci-lint run
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```
