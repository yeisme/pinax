## Context

Pinax 同时拥有本地 Prompt Vault 和外部联邦 Catalog，两者必须保持不同 identity 和 lifecycle。

## Decisions

升级 vendor 以消费当前 SDK。官方 promptrepo 条目只有经过现有 catalog install --yes rights gate 后才成为 Pinax draft。

## Compatibility

变更为 additive；旧本地 asset 与 pinax://prompt/ 引用不迁移。
