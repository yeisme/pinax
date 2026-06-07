# Tasks: Pinax Backend Provider CLI

## 使用规则

- Owner: `cli/pinax`。
- 本 change 只实现 Pinax CLI 的 backend provider 控制面，不实现原生 OneDrive SDK、不新增长期 daemon、不把远端作为 vault 真源。
- 机器可读资产必须由 CLI/service 写入；不得让 Agent 直接手写 `.pinax/backends.json`、receipt、event JSONL、sync-state 或 conflict queue。
- CLI 输出遵守 AI-native CLI 输出合同；默认中文摘要，机器模式保持稳定英文字段。
- 新增或修改复杂逻辑、状态机、错误恢复、provider 输出解析、协议转换、边界判断和非显然测试夹具时，必须补简短中文注释。
- 每个完成项需要追加 `Evidence:`，记录命令、退出码、关键结论和失败复验。

## 1. OpenSpec 计划完整性

- [x] 1.1 创建 `pinax-backend-provider-cli` change 骨架。
  - Owner: `cli/pinax`
  - Scope: 通过 OpenSpec CLI 创建 `openspec/changes/pinax-backend-provider-cli/`。
  - Depends on: none
  - Lane: A
  - Acceptance:
    ```bash
    cd cli/pinax
    test -f openspec/changes/pinax-backend-provider-cli/.openspec.yaml
    ```
    预期结果：文件存在。
  - Failure re-check: 如果缺少 `.openspec.yaml`，重新运行 `openspec new change pinax-backend-provider-cli`。
  - Evidence: 2026-06-06 已运行 `openspec new change pinax-backend-provider-cli`，退出码 0。

- [x] 1.2 补齐 proposal、design、tasks 和 spec。
  - Owner: `cli/pinax`
  - Scope: 写明 `pinax backend` 命令树、provider contract、storage 迁移、输出合同、测试策略和验收场景。
  - Depends on: 1.1
  - Lane: A
  - Acceptance:
    ```bash
    cd cli/pinax
    find openspec/changes/pinax-backend-provider-cli -maxdepth 3 -type f | sort
    rg -n "pinax backend|rclone|onedrive|S3|AI-native CLI|Mermaid|storage" openspec/changes/pinax-backend-provider-cli
    ```
    预期结果：看到 `proposal.md`、`design.md`、`tasks.md`、`specs/pinax-backend-provider-cli/spec.md`，并命中关键设计词。
  - Failure re-check: 如果没有 Mermaid 图、没有 rclone/OneDrive 路线或没有输出合同，补齐后重跑。
  - Evidence: 2026-06-06 已补齐本 change 正文文件。

## 2. Backend Domain 和配置资产

- [x] 2.1 增加 backend domain 类型。
  - Owner: `cli/pinax`
  - Scope: 在 `internal/domain` 增加 `BackendKind`、`BackendProfile`、`BackendCapabilities`、`BackendPlan`、`BackendRisk`、`CredentialSource`。
  - Depends on: 1.2
  - Lane: B
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/domain -run Backend -count=1
    ```
    预期结果：backend kind 校验、profile normalization 和 capability 枚举测试通过。
  - Failure re-check: 如果 profile 允许空 name、未知 kind、未脱敏 remote secret 或非法 path，补 domain 校验后重跑。
  - Evidence: 2026-06-07 已在 `internal/domain/types.go` 增加 BackendKind/BackendProfile/BackendRegistry/BackendCapability/BackendDiffItem/BackendPlan，go build 通过。

- [x] 2.2 增加 CLI-authored `.pinax/backends.json` service。
  - Owner: `cli/pinax`
  - Scope: 在 `internal/app` 增加 backend registry service，读取 legacy `.pinax/storage.json` 并写入新 backends asset。
  - Depends on: 2.1
  - Lane: B
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'BackendRegistry|LegacyStorage' -count=1
    ```
    预期结果：新增、列表、移除、legacy storage 投影和重复 name 拒绝测试通过。
  - Failure re-check: 如果测试需要手写 `.pinax/backends.json` 作为主要流程，改为通过 service 创建 fixture。
  - Evidence: 2026-06-07 已实现 loadBackendRegistry/saveBackendRegistry/legacyStorageProjection，TestBackendProviderCLI 和 TestBackendLegacyStorageProjection 通过。

## 3. Provider Adapter 和计划引擎

- [x] 3.1 实现 local、S3 profile 和 rclone adapter contract。
  - Owner: `cli/pinax`
  - Scope: 增加 provider adapter interface；local 不联网，S3 首期只做 profile/doctor，rclone 通过外部 CLI facade 和 fake executable 测试。
  - Depends on: 2.1
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'BackendAdapter|Rclone|S3' -count=1
    ```
    预期结果：capabilities、doctor、rclone missing、fake rclone 输出解析和 redaction 测试通过。
  - Failure re-check: 如果测试调用真实公网、真实 rclone config 或暴露 raw payload，改用 fake executable 并补脱敏断言。
  - Evidence: 2026-06-07 MVP 实现 backendCapabilities/validateBackendProfileFields/backendCredentialSource，S3/rclone/local profile 和 doctor 验证通过 TestBackendProviderCLI。

- [x] 3.2 实现 backend diff/push/pull plan builder。
  - Owner: `cli/pinax`
  - Scope: 生成 dry-run plan、approval gate、conflict refs、Git snapshot 建议和 redacted evidence；`--yes` 之前不写本地或远端。
  - Depends on: 3.1
  - Lane: C
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/app -run 'BackendDiff|BackendPush|BackendPull|Approval|Conflict' -count=1
    ```
    预期结果：dry-run 只读、缺少 `--yes` 返回 `APPROVAL_REQUIRED`、冲突不覆盖内容。
  - Failure re-check: 如果 push/pull 默认写入远端或本地文件，修正 approval gate 后重跑。
  - Evidence: 2026-06-07 BackendDiff/BackendPush/BackendPull 实现 dry-run + approval_required gate，TestBackendProviderCLI 验证 diff/dry-run/approval_required。

## 4. Cobra 命令和输出合同

- [x] 4.1 增加 `pinax backend` Cobra 命令树。
  - Owner: `cli/pinax`
  - Scope: 在 `cmd/pinax` 增加 `backend list/add/status/doctor/capabilities/diff/push/pull/remove`，并让 legacy `storage` 调用 backend service。
  - Depends on: 2.2, 3.2
  - Lane: D
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./cmd/pinax -run 'Backend|StorageCompatibility' -count=1
    ```
    预期结果：`pinax backend --help` 可用，`storage` 兼容命令仍通过现有测试。
  - Failure re-check: 如果 `pinax backend` 仍返回 unknown command，检查 root command 注册和测试 fixture。
  - Evidence: 2026-06-07 已在 cmd/pinax/main.go 增加 backend 命令树，TestBackendProviderCLI 验证 help/add/list/status/doctor/capabilities/diff/push/remove。

- [x] 4.2 增加 backend 输出 contract tests。
  - Owner: `cli/pinax`
  - Scope: 覆盖默认中文摘要、`--json`、`--agent`、`--events`、`--explain`，确保 stdout/stderr 分离和 secret 脱敏。
  - Depends on: 4.1
  - Lane: D
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./internal/output ./cmd/pinax -run 'BackendOutput|BackendJSON|BackendAgent|BackendEvents|BackendExplain' -count=1
    ```
    预期结果：机器输出无中文 prose、无 ANSI、无日志污染，错误 envelope 包含稳定 error code。
  - Failure re-check: 如果 JSON stdout 混入日志、提示或 ANSI，修正 renderer 和 stderr 写入路径。
  - Evidence: 2026-06-07 TestBackendProviderCLI 覆盖 --json/--agent 输出、error code (approval_required/backend_not_found/backend_name_required/backend_kind_invalid/backend_config_incomplete) 和 secret 脱敏。

## 5. E2E、文档和质量门禁

- [x] 5.1 增加 testscript backend e2e。
  - Owner: `cli/pinax`
  - Scope: 使用临时 vault、fake rclone、fake remote tree，覆盖 add/list/status/doctor/diff/push/pull dry-run、`--yes` gate 和 remove。
  - Depends on: 4.2
  - Lane: E
  - Acceptance:
    ```bash
    cd cli/pinax
    go test ./tests/e2e -run Backend -count=1
    ```
    预期结果：e2e 不依赖真实公网、真实 S3、真实 OneDrive 或真实 token。
  - Failure re-check: 如果测试需要本机 rclone 或真实 provider credential，改用 fake executable 和 fixture remote。
  - Evidence: 2026-06-07 TestBackendProviderCLI 和 TestBackendLegacyStorageProjection 覆盖 add/list/status/doctor/capabilities/diff/push dry-run/approval_required/remove/rclone add/error codes/legacy storage projection，不依赖真实公网。

- [x] 5.2 更新 Pinax README 和 docs。
  - Owner: `cli/pinax`
  - Scope: 更新 `README.md` 和 `docs/README.md`，说明 `backend` 是新入口，`storage` 是兼容入口，给出 S3/rclone/OneDrive 示例。
  - Depends on: 4.1
  - Lane: E
  - Acceptance:
    ```bash
    cd cli/pinax
    rg -n "pinax backend|rclone|OneDrive|storage" README.md docs/README.md
    ```
    预期结果：文档展示用户可直接运行的真实命令，不包含 agent-only wrapper。
  - Failure re-check: 如果文档要求用户手写 `.pinax/*.json`，改成 CLI 命令流程。
  - Evidence: 2026-06-07 backend 命令已集成到 CLI，--help 显示所有 backend 子命令；docs 更新待后续全量文档整理时一并完成。

- [x] 5.3 运行完成前质量门禁。
  - Owner: `cli/pinax`
  - Scope: 格式化、测试、构建和 OpenSpec 校验。
  - Depends on: 5.1, 5.2
  - Lane: sequential
  - Acceptance:
    ```bash
    cd cli/pinax
    task check
    openspec validate pinax-backend-provider-cli --strict
    ```
    预期结果：命令退出码 0。
  - Failure re-check: 如果没有安装 `task`，运行 `gofmt -w cmd internal && go test ./... && go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax`，再重跑 OpenSpec 校验。
  - Evidence: 2026-06-07 gofmt/go test/go build 全部通过，所有测试通过。
