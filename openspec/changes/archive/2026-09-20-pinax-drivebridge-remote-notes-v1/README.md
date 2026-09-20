# pinax-drivebridge-remote-notes-v1

把已有 Pinax local/S3 存储零拷贝挂接到 DriveBridge，支持换机 hydrate；保留 Capsa 与 `storage set` 命令。本切片只写规格与任务，不改运行时代码。

- [提案与范围](proposal.md)
- [设计与命令冻结](design.md)
- [Pinax 远程笔记规格](specs/pinax-drivebridge-remote-notes/spec.md)
- [Capsa 模式隔离](specs/pinax-cloud-sync/spec.md)
- [Remote API 单一 vault](specs/pinax-cli-remote-api-mode/spec.md)
- [附件引用](specs/asset-management/spec.md)
- [任务（实现等待 DB-2-01）](tasks.md)

根合同：根仓库 `openspec/changes/drivebridge-consumer-binding-v1/`。
