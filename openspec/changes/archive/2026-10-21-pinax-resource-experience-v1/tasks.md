# 任务

- [x] 1.1 从已注册命令树为单身份详情补 canonical `describe`/`get`，并把既有 `show` 标为隐藏兼容。 | evidence: `internal/cli/resource_views.go` `registerResourceViews`；`go test ./internal/cli -run TestResourceCanonicalRegistrationAndErrors -count=1` 通过。
- [x] 1.2 缺身份返回 `missing_argument`、稳定退出码 2、可执行发现命令，stdout/stderr 分离。 | evidence: `TestResourceCanonicalRegistrationAndErrors` 断言 `project describe --json` 退出码 2 且 stderr 为空。
- [x] 1.3 `get` 只保留身份字段，不回传正文等非身份载荷。 | evidence: 同测试用含 `body` 的 note 投影断言 `get` 投影不含正文。
- [x] 1.4 列表输出保留 1-based `fact.<kind>.N.id`，不从 JSON 数组按 0 重写并覆盖第一条。 | evidence: `resourceProjection` 在 owner 已发布 `kind.1.id` 时跳过合成；`TestResourceProjectListIdentityAndNext` 断言 archive/research 两条身份不串号。
- [x] 1.5 补全安装不改 rc，拒绝覆盖非受管文件，doctor 分开报告文件存在与 shell 可发现。 | evidence: `internal/cli/resource_completion_install.go`；`go test ./internal/cli -run TestResourceCompletionInstallation -count=1` 通过。
- [x] 1.6 资源补全只读、最多 50 项、250ms 超时，backend 与无本地查询返回零候选。 | evidence: `boundedResourceCompletion`。
- [x] 1.7 产品说明与源码候选一致。 | evidence: `docs/commands/resource-views.md`。
