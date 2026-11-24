## Common Local CLI Patterns

### General Search

```bash
firecrawl search "GitHub" --limit 5
firecrawl search "OpenAI Responses API docs" --limit 10
firecrawl view-config
firecrawl search "Open WebUI Research Harness" --api-url http://localhost:32741 --limit 5 --json
```

### Agent Reach Platform Routing

安装与会话配置示例仅用于明确请求该动作的任务，不作为每次联网的准备步骤。凭据不得进入命令参数。

```bash
command -v agent-reach
agent-reach doctor
agent-reach install --env=auto --safe
agent-reach install --env=auto --channels=opencli,twitter,reddit,bilibili
agent-reach configure --from-browser chrome
```

After Agent Reach reports the active backend, call the upstream tool directly instead of treating `agent-reach` as a content wrapper.

### Download Video Or Audio (yt-dlp)

```bash
yt-dlp -f "bv*+ba/b" --merge-output-format mp4 -o "%(uploader)s/%(id)s.%(ext)s" \
  "https://x.com/<user>/status/<id>"          # Confirm access and item scope first
yt-dlp --cookies-from-browser chrome "URL"    # login-gated or age-restricted media
yt-dlp -x --audio-format mp3 "URL"            # audio only
yt-dlp --dump-json "URL" | jq '{title, uploader, duration}'   # probe before downloading
```

失败先分类并核对版本与安装来源，不自动升级；下载交接、权限与验证统一见 [media_download.md](media_download.md)。

### Scrape Or Extract A Known URL

```bash
firecrawl scrape "https://github.com/"
firecrawl scrape "https://docs.firecrawl.dev/"
```

For a JavaScript-rendered page that initially returns only navigation or branding, follow `routing/dynamic_pages.md`. A representative recovery command is:

```bash
firecrawl scrape "https://wetoken.ai/model-docs?model=dreamina-seedance-2-5-filter-off" --only-main-content --wait-for 5000 --max-age 0 --timing -o /tmp/wetoken-model-docs.md
```

### Crawl A Documentation Site

```bash
firecrawl crawl "https://docs.firecrawl.dev/" --limit 20
```

### GitHub-Specific Research

```bash
gh search repos "agent framework language:TypeScript" --limit 10
gh repo view openai/openai-python --json name,description,stargazerCount,pushedAt,url
gh release list --repo openai/openai-python --limit 10
gh issue list --repo openai/openai-python --state open --limit 20
```

### GitHub API Fallback

```bash
curl -L "https://api.github.com/repos/openai/openai-python" | jq '{name, description, stargazers_count, pushed_at, html_url}'
curl -L "https://api.github.com/repos/openai/openai-python/releases?per_page=5" | jq '.[].tag_name'
```

### Package Metadata

```bash
npm view playwright version description repository time --json
python -m pip index versions requests
cargo search tokio --limit 5
go list -m -versions golang.org/x/tools
```

### Browser Interaction Evidence

```bash
agent-browser skills get core --full
agent-browser open "https://example.com"
agent-browser snapshot
agent-browser screenshot /tmp/example.png
browser-use state
npx playwright codegen "https://example.com"
```

