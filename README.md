# Pinax

Pinax 是本地优先的 Markdown 笔记 CLI。Markdown vault 是用户知识资产真源，`.pinax/` 保存由 CLI 或 application service 创建的配置、索引、映射、事件和审计投影。

当前仓库状态是本地 notebook core 闭环：已支持初始化、校验、daily/inbox、note 创建/列表/读取/编辑/单笔维护、组织维度浏览、saved views、SQLite/GORM 索引、搜索、链接/反链/孤儿笔记、附件、Markdown 导入导出、统计、健康检查、本地 dashboard、模板、metadata 补齐、repair 维护计划、agent organize 计划、受 Git snapshot 保护的整理和 repair 应用，以及只读 MCP surface。后续 provider、sync、briefing 和 cloud 能力仍必须先进入 `openspec/changes/<change-id>/`，再按任务验收落地。

## 本地 vault 工作流

初始化一个 Markdown vault：

```bash
pinax init
pinax init ./my-notes --title "我的知识库"
pinax validate --vault ./my-notes --json
```

`pinax init` 不带参数时初始化当前目录；也可以用 `--vault <path>` 或位置参数指定 vault 路径。
常规命令支持默认中文摘要、`--agent`、`--json`、`--events` 和 `--explain` 输出模式；这些模式一次只能选择一个。

查看 vault 统计、健康问题和只读本地 dashboard：

```bash
pinax stats --vault ./my-notes
pinax stats --vault ./my-notes --json
pinax doctor --vault ./my-notes --stale-after 90d --agent
pinax dashboard --vault ./my-notes --port 0
```

`stats` 和 `doctor` 默认只读，不修改 Markdown、`.pinax/`、Git 或远端服务；`dashboard` 只绑定 localhost，并复用同一组 application service projection。

把健康问题转换为可审查、可保存、受 snapshot 保护的维护动作：

```bash
pinax repair plan --vault ./my-notes --json
pinax repair plan --vault ./my-notes --save --json
pinax git snapshot --vault ./my-notes --message "repair 前快照"
pinax repair apply --vault ./my-notes --plan repair-abc123 --yes
pinax repair apply --vault ./my-notes --plan repair-abc123 --yes --snapshot-message "repair 前快照"
```

`repair plan` 默认只读；只有 `--save` 会通过 service 写入 `.pinax/repair-plans/<plan_id>.json`。`repair apply` 只执行低风险 metadata、tags、index rebuild 和 archive status 修复，重复标题、空笔记和孤立笔记只生成 manual review，不自动删除、合并或改写正文。Dashboard 的 `/api/repair-plans` 只读展示 saved plans 和 CLI apply 命令。

补齐 note metadata：

```bash
pinax metadata plan --vault ./my-notes --json
pinax metadata apply --vault ./my-notes --yes
```

管理一个 vault 内的多个项目：

```bash
pinax project create research --name "研究" --notes-prefix notes/research --vault ./my-notes
pinax project list --vault ./my-notes --json
pinax project switch research --vault ./my-notes
```

管理日常 Markdown 笔记：

```bash
pinax note create "研究日志" --body "今天的观察" --tags research --status active --dir work --vault ./my-notes
pinax note create "会议记录" --stdin --vault ./my-notes
pinax note list --tag research --status active --recent --limit 20 --vault ./my-notes
pinax note read "研究日志" --vault ./my-notes --json
pinax note edit "研究日志" --editor "$EDITOR" --vault ./my-notes
pinax note rename "研究日志" "Pinax 研究日志" --vault ./my-notes
pinax note move "Pinax 研究日志" archive --vault ./my-notes
pinax note archive "Pinax 研究日志" --vault ./my-notes
pinax note tag add "Pinax 研究日志" important --vault ./my-notes
pinax note delete "Pinax 研究日志" --yes --vault ./my-notes
```

`note show/read/edit/rename/move/archive/delete/tag` 都支持 note id、vault 内路径或唯一标题；标题有多个候选时返回 `note_ref_ambiguous`，避免误改。`note edit/open/new --open` 支持带参数 editor，例如 `--editor "code --wait"`；`note list --recent` 表示按更新时间排序，不隐式过滤旧笔记；`note delete` 默认移动到 `.pinax/trash/YYYYMMDD/` 并在同名冲突时生成唯一目标，真实删除必须同时传 `--hard --yes`。

使用 notebook core 工作流捕获、索引、浏览和搜索：

```bash
pinax daily open --vault ./my-notes --editor "$EDITOR"
pinax daily append --body "今日复盘" --vault ./my-notes
pinax inbox capture "临时想法" --body "先放 inbox" --tags idea --vault ./my-notes
pinax inbox triage "临时想法" --group work --folder ideas --kind reference --status active --vault ./my-notes

pinax index init --vault ./my-notes --json
pinax index rebuild --vault ./my-notes --json
pinax index status --vault ./my-notes --agent
pinax search "认证" --tag auth --group work --folder architecture --kind reference --status active --vault ./my-notes --json

pinax tag list --vault ./my-notes --json
pinax folder list --vault ./my-notes --json
pinax kind list --vault ./my-notes --json
pinax group list --vault ./my-notes --json
pinax view save active-work --group work --status active --kind reference --sort updated --vault ./my-notes --json
pinax view show active-work --vault ./my-notes --json
```

检查本地 Markdown 关系和附件：

```bash
pinax note links "认证方案" --vault ./my-notes --json
pinax note backlinks "认证方案" --vault ./my-notes --json
pinax note orphans --vault ./my-notes --json
pinax note attach "认证方案" ./diagram.png --vault ./my-notes --json
pinax note attachments "认证方案" --vault ./my-notes --json
```

`note attach` 会复制文件到 vault 内 `attachments/` 并追加 Markdown 引用；源文件缺失返回 `attachment_source_missing`，不会修改笔记正文或附件目录。

导入和导出本地 Markdown bundle：

```bash
pinax import markdown ./source --group research --tags imported --dry-run --vault ./my-notes --json
pinax import markdown ./source --group research --kind reference --status active --conflict rename --yes --vault ./my-notes --json
pinax import markdown ./source/beta.md --group research --conflict overwrite --yes --vault ./my-notes --json
pinax export markdown ./out --tag imported --vault ./my-notes --json
```

`import markdown --dry-run` 不写 notes、receipt、Git 或 provider 状态；apply 通过 service 写 `.pinax/receipts/import-*.json`。`export markdown` 按 note filters 导出 Markdown 和引用附件，并写 `.pinax/receipts/export-*.json`。


管理 Markdown 模板并用模板生成笔记：

```bash
pinax template init --vault ./my-notes
pinax template create "视频学习" --vault ./my-notes
pinax template create meeting --body "# {{title}} - {{client}}" --vault ./my-notes
pinax template validate meeting --vault ./my-notes --json
pinax template render meeting --title "客户会议" --var client=Acme --vault ./my-notes --json
pinax note new "客户会议" --template meeting --var client=Acme --tags meeting,client --vault ./my-notes
pinax template delete meeting --vault ./my-notes --yes
```

模板保存在 `.pinax/templates/*.md`，是普通 Markdown 文本。`pinax template create <name>` 不带 `--from`、`--body` 或 `--stdin` 时会创建一篇带 `pinax.template_design.v1` YAML frontmatter 的模板设计文档。变量只做 `{{name}}` 安全文本替换，不执行脚本、不读取环境变量、不访问网络。

配置 storage backend。S3 当前是 backend profile 和诊断能力，不会连接公网或保存 secret：

```bash
pinax storage set-local --root ./my-notes --vault ./my-notes
pinax storage set-s3 --bucket notes --region us-east-1 --prefix pinax/ --profile work --vault ./my-notes --json
pinax storage doctor --vault ./my-notes --json
```

整理结构前先预览并创建显式 Git snapshot：

```bash
pinax organize plan --vault ./my-notes --json
pinax git snapshot --vault ./my-notes --message "整理前快照"
pinax organize apply --vault ./my-notes --yes
```

也可以在 apply 时提供 snapshot message，让 Pinax 先创建保护快照再落地：

```bash
pinax organize apply --vault ./my-notes --yes --snapshot-message "整理前快照"
```

Agent 可生成可审查 organize plan，而不是直接改笔记：

```bash
pinax organize suggest --vault ./my-notes --json
pinax organize suggest --vault ./my-notes --save --agent
pinax organize list --vault ./my-notes --json
pinax organize apply --vault ./my-notes --plan organize-abc123 --yes --snapshot-message "整理前快照" --json
```

`organize suggest` 会生成 move、tag_patch、kind_patch、status_patch、link_resolution、attachment_repair 和 manual_review 操作；`organize apply --plan` 只执行受 snapshot 保护的低风险 move，其它操作保留给人工或后续专门 apply。

启动只读 MCP surface：

```bash
pinax mcp serve --vault ./my-notes
```

## 本地验证

```bash
task build
task test
task check
```

没有安装 `task` 时，使用等价命令：

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```

## 文档入口

- [子项目指令](./AGENTS.md)
- [文档地图](./docs/README.md)
- [Go 开发生态设计](./docs/architecture/go-development-ecosystem.md)
- [OpenSpec](./openspec/config.yaml)
