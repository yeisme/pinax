# Tasks: pinax-knowledge-source-adapter-v1

## 1. 合同承接（已完成）

- [x] 1.1 创建本 change（4/4 artifacts，strict PASS）。来源：root `inferrum-enterprise-multimodal-knowledge-v1` task 3.1（pinax-source-adapter lane）。

## 2. 导出实现

- [ ] 2.1 `knowledge export-projection` 命令（allowlist 双条件、默认零导出、审计行数）。
- [ ] 2.2 投影条目 schema（refs/digest/permission/citation/freshness）+ tombstone revocation。
- [ ] 2.3 增量导出（digest 差集）。

## 3. 验证

- [ ] 3.1 allowlist/默认空/删除 tombstone/只读边界测试 + integration evidence。
- [ ] 3.2 `go test ./... && go vet ./... && go build ./... && openspec validate --changes --strict --no-interactive` 全绿。
