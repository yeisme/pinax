# supervised-local-session Specification

## Purpose

显式受监督的本机会话：私有引导、loopback、秘密不经 argv/日志，以及 EOF 后只关闭本会话。

## Requirements

### Requirement: 显式受监督会话
Pinax SHALL 新增opt-in session-stdin模式，以私有stdin首帧验证绑定、readonly scope及内存bearer，固定loopback随机端口；不改变普通api serve或MCP默认。

#### Scenario: 非法引导或请求开启写模式
- **WHEN** 引导超时/超限/版本未知、root无效或要求mutation/非loopback
- **THEN** 安全失败且无vault初始化、秘密输出或部分可写服务

### Requirement: 秘密不通过日志或argv交付
会话 SHALL 仅通过私有管道传递secret，stdout只返回脱敏ready/error/shutdown，stderr不打印temp token；消费者不能解析普通日志获得认证材料。

#### Scenario: 捕获启动与失败诊断
- **WHEN** 进程正常ready或启动失败
- **THEN** stdout/stderr/事件不含bearer、绝对vault路径、正文或引导原始载荷

### Requirement: EOF排空与其他进程隔离
会话 SHALL 在父管道EOF后停止接收、最多5秒排空并关闭自有连接，不注册daemon；既有SQLite生命周期和其他CLI/Hermes进程保持独立。

#### Scenario: 桌面退出时Hermes仍在读
- **WHEN** DriveBridge关闭它的Pinax会话且另一MCP进程仍活动
- **THEN** 仅当前会话退出，另一个进程不被杀死，不删除其WAL或伪造writer释放

### Requirement: 合同版本失败不降级私有读取
ready SHALL 带session/binding/contract digest及readonly事实；消费者验证当前manifest，未知不兼容版本不能回退到直接vault读取。

#### Scenario: 随包消费者与Pinax合同不兼容
- **WHEN** required reader schema/binding缺失
- **THEN** 报reader_unsupported或unavailable，保留数据并给升级/重连指引，不开启宽权限
