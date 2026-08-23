# pinax-manifest-v2-conflict-copy-identity

修复 manifest v2 vault 在 pull 冲突保留后因冲突副本重复 canonical object ID 而永久无法 push 的缺陷：冲突副本定义为本地保留快照，不参与 v2 同步与 identity 审计。
