## Why

Pinax 已有 Cobra shell completion，但覆盖是按功能逐步补出来的：vault、note、template、asset、journal、view 等路径已有一部分候选，project、profile、backend、folder、prompt、plugin、collection、sync conflict、全局输出 flag 等仍不系统。用户已经遇到 `pinax project board show stock-trading --subproject <TAB>` 这类高价值场景，因此需要把补全能力整理成工具级体验。

## What Changes

- 补齐高价值 Tab 补全：位置参数、枚举 flag、vault 内对象、用户级 profile、project/subproject、backend、folder、prompt、plugin、collection 和 sync conflict。
- 保持 completion 只读、轻量：不写 vault、不刷新索引、不调用远端、不解析 secret。
- 保持路径参数使用 shell 文件补全；枚举和对象候选返回 `ShellCompDirectiveNoFileComp`。
- 更新 Pinax 文档，记录补全策略和覆盖范围。

## Non Goals

- 不给 `--body`、`--title`、`--query`、URL、secret、纯数字或自然语言输入制造噪声候选。
- 不重命名命令、flag、JSON envelope、`--agent` key 或事件类型。
- 不新增 shell wrapper；继续使用 Cobra 原生 completion。
- 不修复当前工作区中与补全无关的 lint 或 OpenSpec 旧状态问题。

## Compatibility

本变更只新增 completion 行为和测试，是 additive CLI UX 增强。现有命令、flag、输出结构和配置键保持不变，不需要迁移或弃用窗口。
