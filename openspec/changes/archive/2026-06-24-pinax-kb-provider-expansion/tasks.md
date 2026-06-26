# Pinax KB Provider Expansion 任务

## 分组说明

- **Lane A: Registry 重构**，先把现有行为搬到可扩展结构，保持兼容。
- **Lane B: 新 provider**，在 registry 稳定后新增 OpenAI/Ollama。
- **Lane C: CLI 和输出合同**，新增 provider list/doctor，并覆盖 JSON/agent redaction。
- **Lane D: Sidecar 和证据**，验证 LanceDB protocol additive 兼容和 integration evidence。
- **Lane E: 文档和最终门禁**，更新用户文档并跑完整质量门禁。

## Lane A: Registry 重构

- [x] **A1. 为现有 provider 写失败优先测试**
  - Owner: `cli/pinax`
  - Files: `internal/semantic/provider_test.go`, `internal/semantic/semantic_test.go`
  - Scope: 覆盖 `gemini`、`fake`、空 provider 名、未知 provider 的 registry 行为；确认 `NewProvider("", "")` 仍返回 Gemini 默认模型。
  - Depends on: none
  - Parallel lane: A
  - Acceptance: 测试先能证明现有行为；未知 provider 返回 `provider_invalid`，不 panic。
  - Validation command: `go test ./internal/semantic -run 'Provider|Semantic' -count=1`
  - Expected result: 重构前后测试都能固定兼容行为。
  - Failure re-check: 如果测试依赖真实 `GEMINI_API_KEY`，改用只检查 factory metadata 的测试，不访问网络。

- [x] **A2. 拆出 provider registry**
  - Owner: `cli/pinax`
  - Files: `internal/semantic/provider.go`, `internal/semantic/provider_fake.go`, `internal/semantic/provider_gemini.go`, `internal/semantic/semantic.go`
  - Scope: 从 `semantic.go` 拆出 `Provider`、`BatchProvider`、`ProviderInfo`、registry 和 fake/Gemini 实现；保留 `NewProvider` 兼容 wrapper。
  - Depends on: A1
  - Parallel lane: A
  - Acceptance: `BuildChunks` 和 `Search` 通过 registry 获取 provider；默认 provider/model 不变。
  - Validation command: `go test ./internal/semantic -run 'Provider|BuildChunks|Search' -count=1`
  - Expected result: semantic 测试通过，`fake` provider 仍 deterministic。
  - Failure re-check: 如果循环依赖或文件过大，优先拆小文件，不把 CLI/config 引入 provider 层。

- [x] **A3. 拆出 backend registry**
  - Owner: `cli/pinax`
  - Files: `internal/semantic/backend.go`, `internal/semantic/sidecar.go`, `internal/semantic/file_store.go`, `internal/semantic/semantic.go`
  - Scope: 将 `Save`、`Doctor`、`Search` 的 backend 分支移入 backend registry；保留 `lancedb` 和 `fake` 行为。
  - Depends on: A2
  - Parallel lane: A
  - Acceptance: `lancedb` 仍调用 sidecar；`fake` 仍使用 deterministic file store；错误码仍是 `backend_invalid`。
  - Validation command: `go test ./internal/semantic -run 'Backend|Sidecar|Search' -count=1`
  - Expected result: semantic backend 测试通过。
  - Failure re-check: backend 不得读取 provider env var，provider 不得直接写 backend store。

## Lane B: 新 provider

- [x] **B1. 新增 OpenAI embedding provider**
  - Owner: `cli/pinax`
  - Files: `internal/semantic/provider_openai.go`, `internal/semantic/provider_openai_test.go`, `internal/redaction/sensitive.go`
  - Scope: 支持 `--provider openai --model text-embedding-3-small`；通过 fake HTTP server 测试请求 shape、成功响应、错误响应和 redaction。
  - Depends on: A2
  - Parallel lane: B
  - Acceptance: 缺少 `OPENAI_API_KEY` 返回 `provider_not_configured` 或等价稳定错误；错误 hint 不包含 token。
  - Validation command: `go test ./internal/semantic -run 'OpenAI|Provider' -count=1`
  - Expected result: OpenAI provider 单测通过且不访问真实公网。
  - Failure re-check: 如果 provider payload 出现在错误字符串，回到 provider error mapping 和 redaction 修复。

- [x] **B2. 新增 Ollama embedding provider**
  - Owner: `cli/pinax`
  - Files: `internal/semantic/provider_ollama.go`, `internal/semantic/provider_ollama_test.go`, `internal/config/config.go`
  - Scope: 支持 `--provider ollama --model nomic-embed-text`，默认 base URL 为 `http://127.0.0.1:11434`，测试使用 fake HTTP server。
  - Depends on: A2
  - Parallel lane: B
  - Acceptance: provider doctor 能报告本地服务不可达；错误不要求公网或 token。
  - Validation command: `go test ./internal/semantic -run 'Ollama|Provider' -count=1`
  - Expected result: Ollama provider 单测通过。
  - Failure re-check: 如果测试依赖本机真实 Ollama，改成 fake server；真实 Ollama 只做手工 smoke。

- [x] **B3. 支持 batch fallback**
  - Owner: `cli/pinax`
  - Files: `internal/semantic/provider.go`, `internal/semantic/semantic.go`, `internal/semantic/provider_test.go`
  - Scope: 增加可选 `BatchProvider`；`BuildChunks` 对支持 batch 的 provider 使用批量调用，否则逐条调用。
  - Depends on: B1, B2
  - Parallel lane: B
  - Acceptance: fake provider 测试覆盖 batch 和 non-batch 两种路径；输出 chunk 顺序稳定。
  - Validation command: `go test ./internal/semantic -run 'Batch|BuildChunks' -count=1`
  - Expected result: batch fallback 测试通过。
  - Failure re-check: 如果 batch 失败导致全部失败，确认错误码可诊断，不吞掉 provider 原因。

## Lane C: CLI 和输出合同

- [x] **C1. 新增 `pinax kb provider list`**
  - Owner: `cli/pinax`
  - Files: `internal/cli/kb_cmd.go`, `internal/app/kb.go`, `cmd/pinax/kb_command_test.go`
  - Scope: 输出 provider 列表、默认模型、local-only、configured 状态和 credential source 类型。
  - Depends on: A2
  - Parallel lane: C
  - Acceptance: `--json` 是合法 envelope；`--agent` 只包含 stable key=value；不输出 token。
  - Validation command: `go test ./cmd/pinax -run 'TestKBProviderList' -count=1`
  - Expected result: CLI contract 测试通过。
  - Failure re-check: 不允许从 command 层手写 JSON，必须走 projection renderer。

- [x] **C2. 新增 `pinax kb provider doctor`**
  - Owner: `cli/pinax`
  - Files: `internal/cli/kb_cmd.go`, `internal/app/kb.go`, `cmd/pinax/kb_command_test.go`
  - Scope: 检查指定 provider/model 是否可用；OpenAI/Gemini 缺凭据、Ollama 不可达、fake 可用都要可诊断。
  - Depends on: B1, B2, C1
  - Parallel lane: C
  - Acceptance: failure 仍输出 valid JSON envelope 和稳定 error code；stderr 不含 raw provider payload。
  - Validation command: `go test ./cmd/pinax -run 'TestKBProviderDoctor' -count=1`
  - Expected result: doctor 合同测试通过。
  - Failure re-check: 如果 doctor 访问真实公网，改为 provider metadata/readiness 层和 fake transport 测试。

- [x] **C3. 更新 command completion 和 help**
  - Owner: `cli/pinax`
  - Files: `internal/cli/kb_cmd.go`, `cmd/pinax/kb_command_test.go`
  - Scope: `--provider` completion 包含 `gemini`、`openai`、`ollama`、`fake`；help 文案保持英文 CLI 表面。
  - Depends on: B1, B2
  - Parallel lane: C
  - Acceptance: help 不写中文，不推荐 shell credential script。
  - Validation command: `go test ./cmd/pinax -run 'Help|Completion|KB' -count=1`
  - Expected result: help/completion 测试通过。
  - Failure re-check: 不添加小写短 flag；新选项使用长 flag。

## Lane D: Sidecar 和证据

- [x] **D1. 扩展 sidecar request metadata**
  - Owner: `cli/pinax`
  - Files: `internal/semantic/sidecar.go`, `tools/pinax-lancedb-sidecar/src/**`, `tools/pinax-lancedb-sidecar/tests/test_protocol_offline.py`
  - Scope: request/response 支持 optional provider/model/embedding_dim/distance_metric/collection 字段；旧请求继续通过。
  - Depends on: A3, B3
  - Parallel lane: D
  - Acceptance: offline protocol test 同时覆盖旧请求和新 metadata 请求。
  - Validation command: `task kb:sidecar:protocol`
  - Expected result: sidecar protocol 测试通过。
  - Failure re-check: 不升级 `pinax.kb.sidecar.v1` 版本，除非出现真正破坏性字段。

- [x] **D2. 增加 provider redaction integration coverage**
  - Owner: `cli/pinax`
  - Files: `internal/testkit/integrationevidence/**`, `cmd/pinax/kb_command_test.go`, `tests/e2e/**`
  - Scope: 在 integration/e2e 中使用 fake provider payload sentinel，断言 stdout/stderr/evidence 不包含 token、Authorization、raw payload 或 note body。
  - Depends on: C1, C2
  - Parallel lane: D
  - Acceptance: 失败时也保留 redacted evidence，exit code 透传。
  - Validation command: `task test:integration`
  - Expected result: 最新 `temp/integration-test-runs/<run-id>/summary.json` status 为 passed，redaction applied。
  - Failure re-check: 不手写 evidence summary，修复 runner 或 redaction 源头。

## Lane E: 文档和最终门禁

- [x] **E1. 更新 KB 命令文档**
  - Owner: `cli/pinax`
  - Files: `docs/commands/kb.md`, `docs/commands/README.md`
  - Scope: 说明 provider/backend 分离、OpenAI/Ollama/Gemini/Fake 用法、LanceDB sidecar、本地 projection 和 Cloud Sync 边界。
  - Depends on: C1, C2, D1
  - Parallel lane: E
  - Acceptance: 文档只展示真实用户可运行命令；不展示 agent-only wrapper；不推荐把 token 写进 shell 脚本。
  - Validation command: `rg -n "kb provider|openai|ollama|lancedb|OPENAI_API_KEY|OLLAMA" docs/commands/kb.md docs/commands/README.md`
  - Expected result: 文档包含新增 provider 和安全边界。
  - Failure re-check: 如果命令尚未实现，不在文档中宣称 production-ready。

- [x] **E2. OpenSpec 严格验证**
  - Owner: `cli/pinax`
  - Files: `openspec/changes/pinax-kb-provider-expansion/**`
  - Scope: 验证本 change 和全量 specs。
  - Depends on: all A/B/C/D/E1 tasks
  - Parallel lane: sequential
  - Acceptance: 本 change 和全量 OpenSpec 均通过 strict validate。
  - Validation command: `openspec validate pinax-kb-provider-expansion --strict && openspec validate --all --strict`
  - Expected result: 两条命令 exit 0。
  - Failure re-check: 修正 delta spec header 或 requirement 格式，不绕过 validate。

- [x] **E3. 全量质量门禁**
  - Owner: `cli/pinax`
  - Files: project-wide
  - Scope: 跑 Pinax 标准门禁。
  - Depends on: E2
  - Parallel lane: sequential
  - Acceptance: format、lint、unit、build、sidecar protocol、OpenSpec 全部通过。
  - Validation command: `task check`
  - Expected result: `task check` exit 0。
  - Failure re-check: 修源头失败，不跳过 sidecar 或 contract tests。

## 验证记录

- RED: `go test ./internal/semantic -run 'Provider|Semantic|Backend|Sidecar|Batch|BuildChunks|OpenAI|Ollama' -count=1` 初始失败，确认缺少 provider/backend registry、OpenAI/Ollama provider 和 batch fallback。
- GREEN: `go test ./internal/semantic -run 'Provider|Semantic|Backend|Sidecar|Batch|BuildChunks|OpenAI|Ollama' -count=1` 通过。
- GREEN: `go test ./cmd/pinax -run 'TestKB|Help|Completion' -count=1` 通过。
- GREEN: `go test ./internal/semantic ./cmd/pinax ./internal/app -run 'Provider|Backend|Sidecar|Batch|BuildChunks|OpenAI|Ollama|TestKB|KBProvider' -count=1` 通过。
- Sidecar: `task kb:sidecar:protocol` 通过，offline test 覆盖 legacy request 和 provider/model/embedding_dim/distance_metric/collection metadata request。
- 文档检查: `rg -n "kb provider|openai|ollama|lancedb|OPENAI_API_KEY|OLLAMA" docs/commands/kb.md docs/commands/README.md` 覆盖新增 provider、backend、LanceDB 和凭据边界。
- Integration evidence: `task test:integration` 通过，run id `20260624T092209Z-2722467`，`summary.json` 记录 `kb_provider_expansion: true` 和 `redacted: true`。
- OpenSpec: `openspec validate pinax-kb-provider-expansion --strict && openspec validate --all --strict` 通过，48 passed, 0 failed。
- Quality gate: `task check` 通过，包含 `go test ./...`、`golangci-lint run`、`golangci-lint fmt --diff`、sidecar protocol tests、`openspec validate --all` 和 `go build -trimpath`。
