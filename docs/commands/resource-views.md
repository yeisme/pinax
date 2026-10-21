# pinax 资源查询与补全

## 1. 何时使用

`pinax project list` 用于发现身份；`get` 返回身份、状态和关键字段；`describe` 展开详情。列表第一列的 ID 是后续命令直接接受的值。写入继续使用 owner 的显式业务命令。

本页对应当前源码候选。使用前核对 `pinax --help` 与详情命令的 help；若安装版没有 canonical 入口，应报告版本不匹配，不能把源码验证当作已安装验证。

## 2. 从目录开始

```bash
pinax project list
pinax project list --agent
pinax project describe --help
```

在同一个项目和配置范围下，从列表取真实 ID，再执行 `pinax project get <id> --json` 或 `pinax project describe <id> --agent`。`<id>` 表示需要从列表复制的值；它不是可以直接执行的 Next。空目录先按返回的恢复动作配置资源。

## 3. 默认输出与 Agent

默认输出保留完整身份，不以总数代替记录。机器结果使用单一 envelope；本轮添加的逐条身份字段形如 `fact.project.N.id`，已有机器字段继续保留。索引从该 owner 实际输出取值，不假定序号或目录排序等同于 readiness。

```text
ID  ...
<directory-id>  ...
Next: pinax project list
```

上面是列与动作的布局示意。真实状态与计数以当前查询为准。Agent 应读取 `pinax project list --agent` 的身份字段；解析失败可检查 `pinax project list --json` 的合同，不能改猜人读表。保留已有范围、过滤及分页参数。

## 4. Tab 补全

```bash
pinax completion install zsh --yes
pinax completion doctor zsh --agent
```

安装文件为 `_命令名`，通过 `fpath` 加载；不修改 shell rc，不把生成脚本 source 进 rc。doctor 分开报告文件已安装、shell 可发现、当前父 shell 是否加载（无法判断时为 unknown）。按 doctor 给出的 activation command 在当前 shell 配置 `fpath` 后，使用 `pinax project describe <Tab>`。

资源补全读取本地目录，最多 50 个前缀匹配项，预算 250ms；远程目录没有可用本地快照时返回零候选。显式路径 flag 才启用路径补全。

## 5. 错误与恢复

缺 ID 返回 `missing_argument`，未知 ID 返回该资源的稳定错误码和非零退出。先执行 `pinax project list --agent` 获取当前合法值；损坏或不可读目录必须先修复原配置，不新建目录掩盖错误。

旧对象详情 `show` 是隐藏兼容路径（owner 原本没有该路径时不新增）。正常 help、推荐命令和 Skills 使用 canonical 入口。兼容窗口为首次功能发布结束；下一次功能发布删除，补丁发布不提前删除。已发布 raw 文件、流、导出与独立业务动作保持自己的合同。
