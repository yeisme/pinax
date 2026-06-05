# 本地开发

常用命令：

```bash
task build
task test
task check
```

没有安装 `task` 时，使用 Go 和 OpenSpec 直接命令：

```bash
go test ./...
go build -trimpath -ldflags="-s -w" -o dist/pinax ./cmd/pinax
openspec validate --all
```

构建产物只用于本地验证，不提交：

```bash
rm -rf dist
```

新增实现前先创建 OpenSpec change：

```bash
openspec new change pinax-<slug>
openspec validate pinax-<slug>
```
