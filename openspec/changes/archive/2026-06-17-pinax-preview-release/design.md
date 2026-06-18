# Design: Pinax v0.1.0-preview Release

## 决策

### License 选择

推荐 **MIT**：
- Pinax 依赖的 Go 生态大部分是 MIT/BSD-3/Apache-2.0，MIT 兼容性最广
- MIT 最简短，降低用户理解成本
- 如果未来需要专利保护再迁移到 Apache-2.0

CEO 需确认：MIT vs Apache-2.0。本 design 默认 MIT。

### Release workflow 设计

```
触发: tag pinax/v* pushed
权限: contents: write (仅 publish job)
步骤:
  1. checkout
  2. setup-go 1.26
  3. task check (质量门禁)
  4. goreleaser release --clean (prerelease: true)
  5. 上传 archives + checksums 到 GitHub Release
```

关键约束：
- prerelease: true（GitHub UI 标记为 Pre-release）
- 不发布到 Homebrew/Scoop/Chocolatey
- snapshot（`workflow_dispatch`）不创建 release，只构建 artifacts

### GoReleaser prerelease 配置

```yaml
release:
  prerelease: true
  name_template: "Pinax {{ .Version }}"
```

### Quickstart 设计

5 分钟流程：
1. 安装（`go install` 或下载 archive）
2. `pinax init ./my-notes`
3. `pinax note add "First Note"`
4. `pinax proof loop run --vault ./my-notes --json`（preview）
5. `pinax repair plan --save` → `pinax version snapshot` → `pinax repair apply --yes`
6. `pinax version restore ...`（证明可回滚）

Quickstart 不覆盖：
- Cloud Sync（preview 阶段非主线）
- MCP server（quickstart 之外）
- Templates、project boards、briefing（advanced）

### 版本号

- 首个 preview release: `pinax/v0.1.0-preview.1`
- 后续: `pinax/v0.1.0-preview.2` ...
- 正式: `pinax/v0.1.0`

### Tag convention

保持现有 `pinax/vX.Y.Z` tag convention，不引入裸 `vX.Y.Z`。

## 验证策略

- `goreleaser check` 通过
- `task release:local` 生成 archives + checksums
- GitHub workflow YAML lint（actionlint 如果可用）
- `task check` 无回归

## 延期项

- Homebrew/Scoop/nFPM（`pinax-release-packaging-distribution` 处理）
- SBOM、cosign 签名
- Release notes 自动生成模板（beyond goreleaser changelog）
- 文档站部署
