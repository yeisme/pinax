# 本地开发

常用命令：

```bash
task build
task test
task check
```

没有安装 `task` 时，使用 Go 和 OpenSpec 直接命令：

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```

构建产物只用于本地验证，不提交：

```bash
rm -rf dist
```

新增实现前先创建 OpenSpec change：

```bash
openspec new change pinax-<slug>
openspec validate pinax-<slug>
```

## 本地 smoke

构建后可以用临时 vault 验证本地闭环：

```bash
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
rm -rf /tmp/pinax-notes
./dist/pinax init /tmp/pinax-notes --title "我的知识库"
./dist/pinax validate --vault /tmp/pinax-notes --json
./dist/pinax validate --vault /tmp/pinax-notes --agent
./dist/pinax validate --vault /tmp/pinax-notes --events
./dist/pinax validate --vault /tmp/pinax-notes --explain
./dist/pinax project create research --name "研究" --notes-prefix notes/research --vault /tmp/pinax-notes --json
./dist/pinax project list --vault /tmp/pinax-notes --agent

./dist/pinax template init --vault /tmp/pinax-notes --json
./dist/pinax template create "视频学习" --vault /tmp/pinax-notes --json
./dist/pinax template create meeting --body "# {{title}} - {{client}}" --vault /tmp/pinax-notes --json
./dist/pinax template validate meeting --vault /tmp/pinax-notes --json
./dist/pinax template render meeting --title "客户会议" --var client=Acme --vault /tmp/pinax-notes --json
./dist/pinax note new "客户会议" --template meeting --var client=Acme --tags meeting,client --vault /tmp/pinax-notes --json
./dist/pinax note list --tag meeting --vault /tmp/pinax-notes
./dist/pinax note read "客户会议" --vault /tmp/pinax-notes --json
./dist/pinax storage set-s3 --bucket notes --region us-east-1 --prefix pinax/ --profile work --vault /tmp/pinax-notes --json
./dist/pinax storage doctor --vault /tmp/pinax-notes --json
./dist/pinax metadata plan --vault /tmp/pinax-notes --json
./dist/pinax repair plan --vault /tmp/pinax-notes --json
./dist/pinax repair plan --vault /tmp/pinax-notes --save --json
./dist/pinax organize plan --vault /tmp/pinax-notes --json
./dist/pinax mcp serve --vault /tmp/pinax-notes
```

`pinax init` 可以不带参数初始化当前目录；位置参数和 `--vault` 用于指定其它 vault 路径。默认输出给人看，机器消费优先使用 `--agent`、`--json`、`--events` 或 `--explain`。

S3 storage 当前只验证 backend profile 和 credential source 描述，不连接真实 S3，也不保存 access key 或 secret。

Note 命令的 `--recent` 只表示按更新时间排序，不隐式过滤旧笔记。`note edit/open/new --open` 可以使用带参数 editor，例如 `--editor "code --wait"`；默认删除会进入 `.pinax/trash/YYYYMMDD/`，同名冲突时生成后缀路径，不覆盖已有 trash。

真实整理落地必须先创建 Git snapshot，或在 apply 中提供 `--snapshot-message`：

```bash
./dist/pinax git snapshot --vault /tmp/pinax-notes --message "整理前快照"
./dist/pinax organize apply --vault /tmp/pinax-notes --yes
./dist/pinax repair apply --vault /tmp/pinax-notes --plan repair-abc123 --yes --snapshot-message "repair 前快照"
```

`repair plan` 默认不写 Markdown 或 `.pinax/` 资产；只有 `--save` 会创建 `.pinax/repair-plans/<plan_id>.json`。`repair apply` 必须显式 `--yes` 并有 Git snapshot 保护，计划过期或笔记变化后会返回 `plan_stale`，要求重新生成计划。


## 模板工作流

模板保存在 `.pinax/templates/*.md`，可以用 CLI 创建，也可以用普通编辑器修改正文。`pinax template create <name>` 不带来源参数时会创建带 `pinax.template_design.v1` YAML frontmatter 的模板设计文档；创建、删除和事件记录由 Pinax service 负责：

```bash
pinax template create "视频学习" --vault ./my-notes
pinax template create meeting --from ./meeting.md --vault ./my-notes
pinax template create daily-review --from ./daily-review.md --vault ./my-notes
pinax template validate meeting --vault ./my-notes --json
pinax template render meeting --title "客户会议" --var client=Acme --vault ./my-notes --json
pinax note new "客户会议" --template meeting --var client=Acme --tags meeting,client --vault ./my-notes
pinax template delete meeting --vault ./my-notes --yes
```

`--var key=value` 可以重复使用。渲染缺失变量会失败并返回 `template_variable_missing`，防止生成半成品笔记。
