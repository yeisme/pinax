# Tasks

## 1. 合同冻结

- [x] 1.1 冻结 typed facade machine envelope（schema identity/version/digest、错误码、恢复 action）与 safe `projectRef` ↔ binding 映射语义；Validation: `openspec validate pinax-workbench-continuity-projection-v1 --strict --no-interactive`。
- [x] 1.2 冻结投影时效（evidence freshness/revision 推导、TTL、refresh-only 语义）与 provider packet 输出格式。

## 2. 实现与验证

- [x] 2.1 实现 facade/映射/packet 并以 owner-local 六件套 evidence 收口；根仓只接收紧凑 verdict。
