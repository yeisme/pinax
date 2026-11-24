# 媒体下载交接

用户同时要求理解、拆解或相似发现时，下载是 [媒体研究](media_research.md) 的一个阶段；下载成功后在既有授权内继续交接，不能只给文件就结束整个请求。

媒体文件由当前环境可用的 `media-download` Skill 承接；只加载一个版本，保留用户 URL、目标目录、分集范围、格式和已授权会话。该 Skill 缺失时使用下方最小回退，不自动安装 Skill。

专项提供格式与平台细节，但不扩大授权。若专项要求自动更新工具、导出 cookie、把 cookie 展开进命令参数，或将 412 归因于单一原因，改用当前权限边界和下列恢复规则。不要照搬不安全命令。

## 最小回退

1. 检查 yt-dlp 版本、安装来源及合并需要的 ffmpeg。只有明确的 extractor 兼容性证据才考虑更新；不能遇到 4xx 就升级。全局安装/更新须有相应授权。
2. 先取得目标条目 ID、分集范围、时长、可用格式；只保留必要字段，不输出签名流 URL、cookie 或原始 provider payload。
3. 使用稳定 ID 文件名，在用户指定目录下载；未指定时选择本地任务目录并告知。单个 URL 若解析为播放列表，不自动扩张到整频道或无关合集。
4. 对照 metadata 验证身份、时长、非零大小和预期音视频流。静音视频、纯音频、仅字幕按各自目标验收，不一律要求双流。

```bash
command -v yt-dlp
yt-dlp --version
command -v ffmpeg
yt-dlp --dump-single-json "URL" | jq '{id, title, duration, width, height}'
yt-dlp -F "URL"
yt-dlp -f "bv*+ba/b" --merge-output-format mp4 -o "%(id)s.%(ext)s" "URL"
ffprobe -v error -show_entries format=duration,size -show_entries stream=codec_name,codec_type,width,height -of json "FILE"
```

示例只适用于已确认的单条目；命令实际选项以安装版本为准。登录媒体优先使用用户授权的现有会话或受保护凭据文件，不能把凭据放入参数或产物。

## 平台与恢复

- Bilibili 历史上遇到过 yt-dlp 412；这不是永久禁用，也不能断言都是旧版本导致。记录当前版本、阶段、状态码和账号条件，优先已有可用 bili-cli / OpenCLI，按入口的限流恢复上限处理。
- X 公共视频是否可匿名下载取决于当前内容与提取器；不要将历史成功视为保证。返回媒体 ID 可能与 status ID 不同，多视频帖子须核对范围。
- 会员格式缺失、DRM、权限不足不靠重试解决，不绕过访问控制。
- 批量任务核对预期/实际条目、身份、顺序、缺失、大小和抽样内容。重复 hash 先调查是否重复 URL 或合法相同内容；存在同名文件不代表下载已验证完成。
- 只恢复失败或未验证条目；报告已完成与缺失，不静默覆盖原产物或重新下载成功项。

交付时先给本地文件链接，再给格式/时长验证和实际缺口。平台事实应附检查日期和工具版本；未在线验证就不宣称该平台当前可用。
