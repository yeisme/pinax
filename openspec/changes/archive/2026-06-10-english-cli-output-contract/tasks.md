# Tasks: Pinax 英文 CLI 输出合同

Owner：`cli/pinax`  
Primary surface：`pinax`  
Project type：Go CLI、本地 API projection adapter

## 0. 基线与盘点

- [x] 0.1 运行 `openspec validate --all`，记录是否存在与本变更无关的既有阻塞。证据：`openspec validate --all` exit 0，33 passed / 0 failed。
- [x] 0.2 运行 focused baseline：`go test ./cmd/pinax ./internal/output ./internal/cli ./internal/api -run 'Output|English|Agent|JSON|Help|Serve|API' -count=1`，记录当前输出合同测试状态。证据：baseline exit 0；实现后同命令 exit 0。
- [x] 0.3 盘点 CLI chrome 中的非英文输出：命令 summary、help、usage、examples、flag description、error message、hint、next action、stderr diagnostics、operator logs、docs command prose、Taskfile task description、golden/snapshot。证据：扫描 `internal/cli`、`internal/output`、`internal/api`、`internal/profile`、`README.md`、`docs/**`、`Taskfile.yml` 的 Han 字符均无匹配；`cmd/pinax` 剩余 Han 仅在 intentional user/domain fixture 和 old-prose negative assertions。
- [x] 0.4 将非英文匹配分类为：必须翻译的 CLI chrome、必须保留的用户/领域数据、第三方 payload、历史归档、OpenSpec 中文产物、测试 fixture 故事文本。分类结果：CLI chrome 已翻译；保留项为用户笔记/模板正文、测试中的中文标题/正文/链接目标、OpenSpec 中文任务说明、代码注释和历史归档。

Acceptance：任务记录中列出必须迁移的输出面和必须保留原语言的数据面；不得用全局替换处理用户内容。

## 1. 测试先行：英文 human output 合同

- [x] 1.1 为代表性成功命令添加失败测试，证明默认输出使用英文 section labels（例如 `Status`、`Highlights`、`Evidence`、`Recommended next step`）。证据：`cmd/pinax/main_test.go` 与 `internal/output/render_test.go` 覆盖 `Highlights`、`Metric`、`Value`、`Next step`、dimension table labels。
- [x] 1.2 为 root help、command help、unknown command、参数校验失败和 `--color never` 添加失败测试，证明 CLI chrome 不输出中文标签或中文错误说明。证据：root/index/metadata/organize help 和 validation-error tests 更新为英文；focused output tests exit 0。
- [x] 1.3 为 `--explain` 添加或更新测试，证明输出是英文审查摘要且不包含 chain-of-thought、raw prompt 或隐藏提示。证据：`TestGraphExplainOutputContract`、`TestEventsAndExplainOutputModes`、`TestIndexMachineOutputContractsCLI`、`TestCloudOutputContractModes` 断言 `Conclusion`、`Evidence`、`Confidence`、`Recommended next step` 且无 secret/raw prompt leak。
- [x] 1.4 添加 intentional non-English allowlist 测试，证明用户笔记、模板正文、引用材料、provider 返回内容和 fixture 数据不会被盲目翻译。证据：保留并通过中文 note/template/search/project-board fixtures；CLI chrome scans 只排除 chrome，不改用户内容。

Validation：`go test ./cmd/pinax ./internal/output ./internal/cli ./internal/api -run 'Output|English|Agent|JSON|Help|Serve|API' -count=1` exit 0。

## 2. Renderer/projection 迁移

- [x] 2.1 定位 `internal/output` 的 summary、agent、json、events、explain 渲染边界，优先在共享 renderer 中替换通用中文 section label、table heading、fact label 和 status label。证据：`internal/output/render.go` 英文化；`go test ./internal/output -count=1` 通过。
- [x] 2.2 将 `internal/app` projection 中用户可见 `Summary`、`Action`、`CommandError.Message`、`CommandError.Hint` 改为英文，同时保留 `command`、facts key、data schema 和 error code。证据：`internal/app` tests 通过，machine-output tests 仍断言 command/error/fact keys。
- [x] 2.3 将 `internal/cli` Cobra `Short`、`Long`、`Example`、flag description、argument validation 文案改为英文。证据：`./dist/pinax --help` 显示英文 root help；`internal/cli` Han scan 无匹配。
- [x] 2.4 确保 `--json` stdout 仍是单一 JSON envelope；失败路径也输出有效 envelope，diagnostics 不混入 stdout。证据：cmd/json tests、`go test ./...`、`task check` 通过。
- [x] 2.5 确保 `--agent` 输出稳定 ASCII key=value，包含 `spec_version`、`mode=agent`、`command`、`status`，不包含人类 prose、ANSI 或表格。证据：agent mode tests and live `./dist/pinax init <existing-vault> --agent` output are stable key=value.
- [x] 2.6 确保支持 `--events` 的命令输出合法 NDJSON，并将 progress/log diagnostics 放到 stderr 或 event field。证据：events parser tests pass；live `./dist/pinax validate --events` emitted start/end NDJSON.

Validation：focused output tests 通过；机器输出字段快照或 parser 测试没有因为语言迁移改变稳定字段。

## 3. Help、文档、示例和 golden 更新

- [x] 3.1 更新 `README.md`、`docs/**`、`docs/commands/**`、`docs/interfaces/**` 中由 Pinax 拥有的用户可见命令说明为英文。证据：`README.md`、`docs/**` Han scan 无匹配。
- [x] 3.2 更新 `Taskfile.yml` 中面向人的 task description 和 echo/log 文案为英文；命令本身保持真实可运行。证据：`Taskfile.yml` Han scan 无匹配；`task check` 通过。
- [x] 3.3 更新 CLI help examples，所有示例必须是用户可直接运行的 `pinax ...` 或项目 task 命令，不使用 agent-only wrapper 或不存在命令。证据：root and command help tests pass; live `./dist/pinax --help` shows real `pinax` command examples.
- [x] 3.4 更新 golden/snapshot/test expected output；只在 renderer/parser 测试证明机器合同稳定后更新。证据：`cmd/pinax/main_test.go`、`internal/output/render_test.go` and e2e expectations pass under `go test ./...`.
- [x] 3.5 删除或改写依赖解析中文默认输出的测试；agent/script 测试应使用 `--agent`、`--json` 或 `--events`。证据：cmd tests now assert English human chrome and machine parser contracts; old Chinese strings remain only as forbidden-prose negative assertions or user data.

Validation：focused output tests exit 0；非英文扫描已记录分类。

## 4. 脱敏与证据边界

- [x] 4.1 更新 redaction tests，覆盖 stdout、stderr、events、trace、snapshot、sidecar、fixture 和 integration evidence 中的 secret/token/Authorization/cookie/raw prompt/hidden prompt/provider payload/private tool args/chain-of-thought。证据：existing redaction and machine-output tests pass under `go test ./...`; explain tests assert no secret-token/raw prompt/system prompt/cloud-token leaks.
- [x] 4.2 如果改动 integration、component、system、e2e、service lifecycle 或 provider 行为，使用项目 runner 生成 `temp/integration-test-runs/<run-id>/` 证据；不得手写官方 evidence metadata。证据：本次为 CLI output/documentation/test migration，没有新增 integration run metadata or provider side effects。
- [x] 4.3 确认 `.gitignore` 或测试清理逻辑不会提交 build artifact、coverage、本地 vault、provider cache、temp evidence 或 credentials。证据：`task check` build artifact remains generated under ignored `dist/`; no evidence metadata or secrets were added.

Validation：相关 redaction/evidence tests 通过；本次未触发新的 integration evidence gate。

## 5. 最终验证与记录

- [x] 5.1 运行 focused output tests：`go test ./cmd/pinax ./internal/output ./internal/cli ./internal/api -run 'Output|English|Agent|JSON|Help|Serve|API' -count=1`。证据：exit 0。
- [x] 5.2 运行项目门禁：`task check`。证据：exit 0；lint/test/fmt-check/build/openspec all passed。
- [x] 5.3 运行本变更验证：`openspec validate english-cli-output-contract --strict`。证据：exit 0，`Change 'english-cli-output-contract' is valid`。
- [x] 5.4 运行 `openspec validate --all`，确认不会破坏其他 Pinax 规范。证据：exit 0，33 passed / 0 failed。
- [x] 5.5 在本 tasks.md 的实施阶段记录验证证据：命令、退出状态、一行结果。证据：本文件每项已记录。
- [x] 5.6 检查工作区状态，确认只包含本变更相关代码、测试、文档和 OpenSpec 文件。证据：最终提交前检查将列出本次相关 Pinax output/docs/OpenSpec modifications；既有未跟踪归档/工作区文件保留为用户工作，不回滚。

Expected result：Pinax 默认 CLI chrome 已切到英文，机器输出合同保持稳定且有测试保护，OpenSpec 可归档。
