## Why

Pinax 作为本地优先笔记工具，目前缺乏在多设备（包含移动端）之间进行无缝且安全的双向同步能力。虽然基于 Git 的同步方案相对成熟，但其在移动端实现起来较为沉重，不适合作为移动客户端的首选方案。为了满足用户对端到端加密、易于移动端集成的多端同步需求，我们需要实现“盲存储（Blind Storage）+ 加密清单（Encrypted Manifest）+ 冲突收集箱（Conflict Inbox）”的云端同步机制。

## What Changes

- 实现客户端与盲存储后端的双向同步交互（拉取、比对、推送）。
- 引入加密清单（Manifest）版本管理和并发冲突控制机制。
- 引入统一的存储后端接口抽象（StorageBackend），支持无缝接入 S3、WebDAV 甚至 JuiceFS/本地 FUSE 挂载目录作为同步目标。
- 将多端产生的同步冲突自动在原目录另存为带时间戳和 `.conflict.md` 后缀的副本文件。这不仅保留了文件的目录和项目上下文结构，还不会中断后续的同步流程，便于用户后续就地解决冲突。
- 提供专用的冲突处理 CLI 命令，支持快速查找冲突文件并提供与主干版本的差异对比（diff）辅助。
- 为 AI Agent 设计专门的机器可读输出（如 `--json` 和 `show` 命令），让大模型可以自动读取冲突上下文并执行智能合并（Smart Merge）。
- **BREAKING**: 无，这是在现有本地基础上的新功能层。

## Capabilities

### New Capabilities
<!-- Capabilities being introduced. Replace <name> with kebab-case identifier (e.g., user-auth, data-export, api-rate-limiting). Each creates specs/<name>/spec.md -->

### Modified Capabilities
<!-- Existing capabilities whose REQUIREMENTS are changing (not just implementation).
     Only list here if spec-level behavior changes. Each needs a delta spec file.
     Use existing spec names from openspec/specs/. Leave empty if no requirement changes. -->
- `pinax-cloud-sync`: 完善端到端加密同步协议的需求，明确多端同步与冲突处理的核心工作流。

## Impact

- 核心架构：需要完善 `internal/cloud` 目录下的盲存储加密及网络交互实现。
- 同步逻辑：需要打通 `internal/sync` 目录中的清单生成、版本对比和冲突转换逻辑。
- CLI 命令：将同步功能暴露给用户的 CLI，完善同步执行与状态检查。
- 依赖项：复用 Go 标准库基础加密算法，不再引入其他同步依赖。
