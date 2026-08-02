## Why

Pinax KB 语义层已经把 provider、Record、sidecar client 和检索管线交给共享模块，但当前代码、tests、Taskfile、文档和 active OpenSpec 仍绑定 `github.com/yeisme/lance` 与 `lance.sidecar.v2`。根级 [`openspec/changes/inferrum-one-step-cutover-v1/`](../../../../../openspec/changes/inferrum-one-step-cutover-v1/) 将删除这些旧合同，因此 Pinax 必须在同一交付中完整切换到 Inferrum。

## What Changes

- **BREAKING** 将 Go dependency/import/replace 从 `github.com/yeisme/lance` / `../lance` 改为 `github.com/yeisme/inferrum` / `../inferrum`。
- **BREAKING** 将 sidecar executable、协议、错误码和 fake fixture 改为 `inferrum-lancedb-sidecar`、`inferrum.sidecar.v1`、`inferrum_sidecar_*`。
- 将 `lance_backend.go`、`provider_lance.go`、`lance_*_test.go` 等 Pinax-owned 文件与符号命名改为 Inferrum。
- 更新 KB generation、permission、evaluation、shadow/compat tests、Taskfile、integration evidence、docs 和 active OpenSpec 中的产品引用。
- 保持 Pinax vault 真源、generation state、permission fail-closed、evaluation receipt、citation/redaction、LanceDB store layout 和 CLI output contract 不变。
- 删除 Pinax 内部旧 Lance v2 写路径命名；仅保留真正的 `legacy_v1_readonly` Pinax 历史投影语义时，以 “legacy Pinax KB projection” 描述，不再把它作为现行 Lance 产品合同。

## Capabilities

### New Capabilities

- `pinax-inferrum-consumer`：Pinax KB 通过 Inferrum module/sidecar/protocol 执行 generation、permission、evaluation 和 retrieval。

### Modified Capabilities

- 无；现有 `personal-kb`、evaluation 和 Web 合同只变更依赖身份，不改变业务语义。

## Impact

- `go.mod`、`internal/semantic/**`、KB-owned `internal/app/**` tests、`cmd/pinax/kb_command_test.go`、`Taskfile.yml`、`internal/testkit/kblocalevidence/**`、KB docs 和 active OpenSpec。
- 不修改 vault 数据、GORM schema、CLI command/flags/envelope、provider credential policy、Workbench API 或真实用户数据。
