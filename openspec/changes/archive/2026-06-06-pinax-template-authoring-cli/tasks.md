# Tasks: Pinax Template Authoring CLI

## 使用规则

- Owner: `cli/pinax`。
- 本任务包只负责 Pinax CLI 模板作者能力；不改根仓库文档，不实现云后端。
- 所有 `.pinax/templates/*.md` 写入、删除和事件记录必须通过 CLI/application service 完成。
- 模板正文是用户可编辑文本资产；模板 metadata、event、projection 是 CLI/service 生成资产。
- 变量替换只做文本替换，不执行脚本、不读环境变量、不访问网络。
- 新增或修改变量解析、fence 校验、路径安全和删除门禁时，需要中文注释解释非显然边界。

## 1. 计划和规格

- [x] 1.1 校验 OpenSpec 设计完整性。
  - Owner: `cli/pinax`
  - Scope: 校验 `proposal.md`、`design.md`、`tasks.md` 和 `specs/pinax/spec.md`。
  - Depends on: none
  - Lane: A
  - Acceptance:
    ```bash
    cd cli/pinax
    openspec validate pinax-template-authoring-cli
    ```
    预期结果：change valid，0 failed。
  - Failure re-check: 如果 spec delta 格式失败，修正 Requirement/Scenario 结构后重跑。

## 2. Service 层测试先行

- [x] 2.1 为模板创建写失败测试。
  - Owner: `cli/pinax`
  - Scope: 修改 `internal/app/service_test.go`，覆盖 `CreateTemplate` 从 body 和 file 创建模板。
  - Depends on: 1.1
  - Lane: A
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'TemplateAuthoring' -count=1
    ```
    预期结果：第一次运行因 `CreateTemplate` 未实现而失败；实现后通过。
  - Failure re-check: 如果测试依赖真实用户 vault，改用 `t.TempDir()` fixture。

- [x] 2.2 为变量渲染写失败测试。
  - Owner: `cli/pinax`
  - Scope: 修改 `internal/app/service_test.go`，覆盖 `RenderTemplate` 支持 `Vars map[string]string` 和缺失变量错误。
  - Depends on: 1.1
  - Lane: B
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'TemplateVariables' -count=1
    ```
    预期结果：第一次运行因 `Vars` 或 missing variable 行为未实现而失败；实现后通过。
  - Failure re-check: 如果缺失变量被静默保留为 `{{client}}`，改为返回 `template_variable_missing`。

- [x] 2.3 为模板校验写失败测试。
  - Owner: `cli/pinax`
  - Scope: 修改 `internal/app/service_test.go`，覆盖 `ValidateTemplate` 对 unclosed code fence、非法变量 token、空模板的 issue 输出。
  - Depends on: 1.1
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'TemplateValidate' -count=1
    ```
    预期结果：第一次运行因 `ValidateTemplate` 未实现而失败；实现后通过。
  - Failure re-check: 如果 validate 写文件或事件以外状态，拆回只读校验。

- [x] 2.4 为模板删除写失败测试。
  - Owner: `cli/pinax`
  - Scope: 修改 `internal/app/service_test.go`，覆盖 `DeleteTemplate` 需要 `Yes`、只删除 `.pinax/templates/<name>.md`、保护内置模板。
  - Depends on: 1.1
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'TemplateDelete' -count=1
    ```
    预期结果：第一次运行因 `DeleteTemplate` 未实现而失败；实现后通过。
  - Failure re-check: 如果无 `--yes` 删除成功，修正 approval gate。

## 3. Service 实现

- [x] 3.1 扩展请求结构。
  - Owner: `cli/pinax`
  - Scope: 修改 `internal/app/service.go` 中 `TemplateRequest` 和 `CreateNoteRequest`，增加 `SourcePath string`、`Body string`、`UseStdin bool`、`Vars map[string]string`、`Yes bool`、`Overwrite bool`。
  - Depends on: 2.1, 2.2
  - Lane: A
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'TemplateAuthoring|TemplateVariables' -count=1
    ```
    预期结果：编译进入具体未实现断言失败，而不是类型缺失。
  - Failure re-check: 如果命令层需要知道模板路径细节，移回 service。

- [x] 3.2 实现 `CreateTemplate`。
  - Owner: `cli/pinax`
  - Scope: 在 `internal/app/service.go` 新增 `Service.CreateTemplate`，固定写 `.pinax/templates/<name>.md`，校验来源互斥、名称安全、冲突和 overwrite。
  - Depends on: 3.1
  - Lane: A
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'TemplateAuthoring' -count=1
    ```
    预期结果：body/file 创建测试通过；unsafe path 和 source conflict 测试通过。
  - Failure re-check: 如果能写出 vault 外路径，修正 `safeJoin` 和模板名校验。

- [x] 3.3 实现安全变量渲染。
  - Owner: `cli/pinax`
  - Scope: 扩展 `renderTemplateBody`，合并内置变量和 `Vars`，扫描 `{{name}}` token，缺失变量返回 `template_variable_missing`。
  - Depends on: 3.1
  - Lane: B
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'TemplateVariables|CoreNoteTemplateIndexAndSyncMVP' -count=1
    ```
    预期结果：自定义变量渲染通过，既有内置模板测试不回退。
  - Failure re-check: 如果变量替换引入执行能力或环境变量读取，删除该能力并重测。

- [x] 3.4 实现 `ValidateTemplate`。
  - Owner: `cli/pinax`
  - Scope: 在 `internal/app/service.go` 新增模板校验，返回 issues：非法变量、缺失变量、frontmatter fence 不闭合、Markdown code fence 不闭合、空模板 warning。
  - Depends on: 3.3
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'TemplateValidate' -count=1
    ```
    预期结果：validate 测试通过，未闭合 fence 返回 `template_fence_unclosed`。
  - Failure re-check: 如果 validate 结果无法通过 JSON 输出定位问题，补充 `domain.Issue` data。

- [x] 3.5 实现 `DeleteTemplate`。
  - Owner: `cli/pinax`
  - Scope: 在 `internal/app/service.go` 新增删除逻辑，要求 `Yes`，保护内置模板，只删除 `.pinax/templates/<name>.md`。
  - Depends on: 3.1
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'TemplateDelete' -count=1
    ```
    预期结果：删除门禁、内置模板保护和路径安全测试通过。
  - Failure re-check: 如果内置模板可被误删，补 `builtin_template_protected`。

## 4. CLI 层

- [x] 4.1 为模板 CLI 写失败测试。
  - Owner: `cli/pinax`
  - Scope: 修改 `cmd/pinax/main_test.go`，覆盖 `template create --body`、`template create --from`、`template render --var`、`template validate`、`template delete --yes`。
  - Depends on: 3.2, 3.3, 3.4, 3.5
  - Lane: sequential
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./cmd/pinax -run 'TemplateAuthoringCLI' -count=1
    ```
    预期结果：第一次运行因命令未 wired 而失败；实现后通过。
  - Failure re-check: 如果 JSON 输出混入 human text，修正 renderer 调用。

- [x] 4.2 Wire `template create`。
  - Owner: `cli/pinax`
  - Scope: 修改 `cmd/pinax/main.go`，新增 `template create <name>`，flags：`--from`、`--body`、`--stdin`、`--overwrite`。
  - Depends on: 4.1
  - Lane: sequential
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./cmd/pinax -run 'TemplateAuthoringCLI' -count=1
    ```
    预期结果：create 相关 CLI 测试通过。
  - Failure re-check: 如果 `--stdin` 与 machine output 冲突，保持 stdin 只读、stdout 只输出 projection。

- [x] 4.3 Wire `template validate` 和 `template delete`。
  - Owner: `cli/pinax`
  - Scope: 修改 `cmd/pinax/main.go`，新增 `template validate <name>` 和 `template delete <name> --yes`。
  - Depends on: 4.2
  - Lane: sequential
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./cmd/pinax -run 'TemplateAuthoringCLI' -count=1
    ```
    预期结果：validate/delete CLI 测试通过。
  - Failure re-check: 如果 delete 无 `--yes` 成功，修正 flag 传递。

- [x] 4.4 扩展 `--var` 到 render 和 note new。
  - Owner: `cli/pinax`
  - Scope: 修改 `cmd/pinax/main.go`，为 `template render` 和 `note new` 增加重复 `--var key=value`，解析为 map 传 service。
  - Depends on: 4.3
  - Lane: sequential
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./cmd/pinax -run 'TemplateAuthoringCLI|CoreMVPCLIJSON' -count=1
    ```
    预期结果：自定义变量生成笔记通过，既有 Core MVP CLI 测试不回退。
  - Failure re-check: 如果 `--var` 解析接受空 key 或无 `=`，返回 `template_variable_invalid`。

## 5. 文档和验收

- [x] 5.1 更新 Pinax 文档。
  - Owner: `cli/pinax`
  - Scope: 修改 `docs/README.md` 或 `docs/operations/local-development.md`，加入模板创建、渲染、校验、生成笔记示例。
  - Depends on: 4.4
  - Lane: sequential
  - Acceptance:
    ```bash
    cd cli/pinax
    rg -n "template create|template validate|note new .*--template|--var" docs README.md
    ```
    预期结果：文档包含真实可运行命令。
  - Failure re-check: 如果文档出现 agent-only wrapper 或不可运行命令，改成真实用户命令。

- [x] 5.2 跑完整质量门禁。
  - Owner: `cli/pinax`
  - Scope: 格式化、测试、构建、OpenSpec 校验。
  - Depends on: 5.1
  - Lane: sequential
  - Acceptance:
    ```bash
    cd cli/pinax
    gofmt -w cmd internal
    go test ./...
    go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
    openspec validate --all
    ```
    预期结果：所有命令退出码 0。
  - Failure re-check: 如果 `task check` 可用，也可以运行 `task check`；失败时先修复本 change 引入的问题，不回滚用户已有无关改动。

- [x] 5.3 归档 OpenSpec change。
  - Owner: `cli/pinax`
  - Scope: 完成实现和验证后归档 `pinax-template-authoring-cli`。
  - Depends on: 5.2
  - Lane: sequential
  - Acceptance:
    ```bash
    cd cli/pinax
    openspec archive pinax-template-authoring-cli --yes
    openspec validate --all
    ```
    预期结果：主 spec 吸收模板作者能力，OpenSpec 校验通过。
  - Failure re-check: 如果归档 spec delta 冲突，修正 delta 后重跑 archive。


## Verification Evidence

- 2026-06-06: `go test ./internal/app ./cmd/pinax -run 'TemplateAuthoring' -count=1` passed.
- 2026-06-06: `go test ./internal/app ./cmd/pinax -run 'TemplateAuthoring|CoreMVP' -count=1` passed.
- 2026-06-06: `go test ./internal/app ./cmd/pinax -run 'Core|Template|Output|Events|Explain|Missing|Flag|ApplyHelp|Project|Storage' -count=1` passed.
- 2026-06-06: `go test ./...` passed.
- 2026-06-06: `go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax` passed.
- 2026-06-06: `task check` passed; it ran OpenSpec validation, full Go tests, gofmt check, and build.
- 2026-06-06: CLI smoke passed for `template create`, `template validate`, `template render --var`, `note new --template --var`, and `template delete --yes`; generated note contained `# 客户会议 - Acme`.
