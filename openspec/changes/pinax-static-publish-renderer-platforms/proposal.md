# Pinax 静态发布 Renderer 与多平台部署方案

## 背景

Pinax 已经把 `pinax-web` 定为 static publish renderer，并把 GitHub Pages 发布命令写入 README 和 `publish` 文档，但当前 `pinax publish build` 仍返回 `publish_not_implemented`。用户现在最需要的不是先接独立客户端，而是先把本地 Markdown vault 稳定构建成可预览、可审查、可部署的静态站点；内部 UI 后续进入 `client/yeisme-workbench` Pinax 模块。

## 目标

- 实现 `pinax publish build`，从 publish-safe projection bundle 生成 `dist/site/` 静态站点。
- 实现 `pinax publish serve` 和新增 `pinax publish dev`，优先支持本地构建、loopback 预览、前端构建预览和可选 watch。
- 将本地主动预览变成部署前 gate：用户必须能先看到站点效果、检查选中/跳过/阻塞项，并留下 preview receipt，才能执行 deploy。
- 新增内网只读分享场景：`pinax share start` 可同时启动 Web 预览和 bounded API，让同一局域网的人看已发布范围或受控只读 vault 投影。
- 建立 `pinax-web` 前端 renderer 包，支持 Markdown、frontmatter、wikilink、附件占位、标签页、搜索索引和安全 HTML 输出。
- 将 GitHub Pages、Vercel、Cloudflare Pages 作为同一份静态产物的部署 target，而不是三套 renderer。
- 保留 Pinax Go sidecar 对 vault、selection、redaction scan、receipt、部署安全 gate 的所有权。

## 非目标

- 不在本变更中实现 Electron/React 客户端源码、Electron main process、preload bridge 或完整桌面应用。
- 不把 Vercel/Cloudflare/GitHub token 写入项目仓库、`.pinax/**`、stdout、stderr、receipt 或测试 fixture。
- 不让 renderer 直接读取 vault、SQLite、`.pinax/**`、provider config、token 文件或 sync state。
- 不在第一阶段实现评论、登录、协作、服务端搜索或动态云笔记后端。
- 不把内网分享做成公网 tunnel、Cloudflare Tunnel、Tailscale ACL 管理器或反向代理配置工具；Pinax 只负责本地进程和自身访问边界。

## 交付切片

1. **本地闭环优先**：`plan -> build -> serve -> preview approve`，能在本机看到 HTML 并确认效果。
2. **GitHub Pages 首发部署**：只有已预览并确认的输出能部署到独立发布目录或发布仓库。
3. **内网只读分享**：`share start` 同时启动 Web 和 API，默认 scope 为 `published`，必须显式 `--allow-lan` 才能绑定内网地址。
4. **Vercel/Cloudflare Pages 适配**：调用系统 CLI，Pinax 只校验产物和命令边界。
5. **Workbench 复用准备**：renderer fixture 固化 AST/HTML 语义，后续 Workbench 模块通过 Pinax contracts 复用投影，不重新定义 Markdown 语义。

## OpenSpec 归属

本变更属于 `cli/pinax`，因为它修改 Pinax CLI 命令、Go application service、publish projection、渲染产物、部署适配和本地预览服务。未来内部 UI 应由 `client/yeisme-workbench` Pinax 模块拥有；本变更只为其提供 renderer contract 和 publish-safe data contract。
