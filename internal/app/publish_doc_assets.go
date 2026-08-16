package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/provider"
)

// remoteAssetClient 下载远程图片/资产。仅允许 HTTPS，10s 超时，10MB 上限。
// 不下载私有/内网 IP（基础 SSRF 防护）。下载结果写入 vault 内 .pinax/publish/doc/cache/remote/，
// 使 lark-cli media-insert 能以 cwd-relative 路径访问。
var remoteAssetClient = &http.Client{Timeout: 10 * time.Second}

var remoteAssetLookupIP = net.DefaultResolver.LookupIPAddr

const remoteAssetMaxBytes = 10 * 1024 * 1024 // 10MB

// publishDocDownloadRemoteAsset 下载远程 SVG URL，返回 SVG 源码文本（供 whiteboard --input_format svg）。
// 以 Content-Type 为权威判定（image/svg*）；无 Content-Type 时回退到 URL 扩展名。
// 非 SVG 返回稳定 error；调用方降级为 warning，不阻塞发布。
func publishDocDownloadRemoteAsset(ctx context.Context, assetURL string) (string, error) {
	body, mediaType, err := publishDocFetchRemote(ctx, assetURL)
	if err != nil {
		return "", err
	}
	if mediaType != "" {
		if !strings.HasPrefix(mediaType, "image/svg") {
			return "", fmt.Errorf("remote asset is not SVG (content-type %s)", mediaType)
		}
	} else if !strings.EqualFold(filepath.Ext(assetURL), ".svg") {
		return "", fmt.Errorf("remote asset is not SVG (no content-type, extension %s)", filepath.Ext(assetURL))
	}
	return string(body), nil
}

// publishDocDownloadRemoteAssetToFile 下载远程图片到 vault 缓存，返回 vault-relative 路径。
// 供 docs +media-insert --file 使用（cwd=vault 时传相对路径）。
func publishDocDownloadRemoteAssetToFile(ctx context.Context, root, assetURL string) (string, error) {
	body, mediaType, err := publishDocFetchRemote(ctx, assetURL)
	if err != nil {
		return "", err
	}
	ext := remoteAssetExt(assetURL, mediaType)
	relDir := filepath.ToSlash(filepath.Join(".pinax", "publish", "doc", "cache", "remote"))
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(relDir)), 0o755); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(assetURL))
	name := hex.EncodeToString(sum[:])[:16] + ext
	relPath := filepath.ToSlash(filepath.Join(relDir, name))
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(relPath)), body, 0o644); err != nil {
		return "", err
	}
	return relPath, nil
}

// publishDocFetchRemote 执行带安全校验的 HTTP GET，返回 body 和 Content-Type。
func publishDocFetchRemote(ctx context.Context, assetURL string) ([]byte, string, error) {
	parsed, err := url.Parse(assetURL)
	if err != nil {
		return nil, "", err
	}
	if err := validateRemoteAssetURL(ctx, parsed); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return nil, "", err
	}
	client := *remoteAssetClient
	previousCheckRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := validateRemoteAssetURL(req.Context(), req.URL); err != nil {
			return err
		}
		if previousCheckRedirect != nil {
			return previousCheckRedirect(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("remote asset download returned status %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, remoteAssetMaxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", err
	}
	if len(body) > remoteAssetMaxBytes {
		return nil, "", fmt.Errorf("remote asset exceeds %d bytes", remoteAssetMaxBytes)
	}
	return body, resp.Header.Get("Content-Type"), nil
}

func validateRemoteAssetURL(ctx context.Context, parsed *url.URL) error {
	if parsed == nil {
		return fmt.Errorf("remote asset URL is empty")
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("unsupported remote asset scheme %q", parsed.Scheme)
	}
	if parsed.User != nil {
		return fmt.Errorf("remote asset URL must not include user info")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("remote asset URL must include a host")
	}
	if strings.Contains(host, "%") {
		return fmt.Errorf("remote asset host must not include an IPv6 zone")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isUnsafeRemoteAssetIP(ip) {
			return fmt.Errorf("remote asset host resolves to unsafe address %s", ip.String())
		}
		return nil
	}
	addrs, err := remoteAssetLookupIP(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve remote asset host %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("remote asset host %q has no addresses", host)
	}
	for _, addr := range addrs {
		if isUnsafeRemoteAssetIP(addr.IP) {
			return fmt.Errorf("remote asset host %q resolves to unsafe address %s", host, addr.IP.String())
		}
	}
	return nil
}

func isUnsafeRemoteAssetIP(ip net.IP) bool {
	return ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

// remoteAssetExt 从 URL 路径或 Content-Type 推断文件扩展名。
func remoteAssetExt(assetURL, mediaType string) string {
	if ext := strings.ToLower(filepath.Ext(assetURL)); ext != "" {
		return ext
	}
	switch {
	case strings.HasPrefix(mediaType, "image/png"):
		return ".png"
	case strings.HasPrefix(mediaType, "image/jpeg"):
		return ".jpg"
	case strings.HasPrefix(mediaType, "image/gif"):
		return ".gif"
	case strings.HasPrefix(mediaType, "image/webp"):
		return ".webp"
	case strings.HasPrefix(mediaType, "image/svg"):
		return ".svg"
	default:
		return ".bin"
	}
}

// publishDocInsertAttachmentAsset 用 docs +media-insert --type file 上传本地非图片附件（PDF 等）。
// 与图片共用 media-insert，仅 type 参数不同；file-view=card 让飞书显示文件卡片（可预览的格式自动预览）。
func publishDocInsertAttachmentAsset(ctx context.Context, root string, profile domain.PublishDocProfile, docToken, relPath, alt string) (string, *domain.PublishDocRenderWarning) {
	relPath = strings.TrimSpace(filepath.ToSlash(relPath))
	if relPath == "" {
		return "", nil
	}
	args := publishDocLarkArgs(profile, "docs", "+media-insert", "--doc", docToken, "--file", "./"+relPath, "--type", "file", "--file-view", "card", "--json")
	if alt != "" {
		args = append(args, "--caption", alt)
	}
	out, err := runPublishDocCLIDir(ctx, root, "lark-cli", args, "")
	if err != nil {
		return "", &domain.PublishDocRenderWarning{Code: "attachment_upload_failed", Message: "Local attachment upload failed", Detail: relPath}
	}
	return parsePublishDocMediaInsertBlockID(out), nil
}

func parsePublishDocMediaInsertBlockID(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(provider.ExtractJSON(body), &payload); err != nil {
		return ""
	}
	if id := provider.FirstString(payload, "block_id"); id != "" {
		return id
	}
	data, _ := payload["data"].(map[string]any)
	return provider.FirstString(data, "block_id")
}

// publishDocReadImageDimensions 读取本地图片的显示宽高（像素），供 media-insert --width/--height。
// 当前只解析 PNG 头（首版覆盖最常见的截图类型）；无法识别时返回 0,0（lark-cli 自动按宽高比计算）。
func publishDocReadImageDimensions(root, relPath string) (int, int) {
	abs := filepath.Join(root, filepath.FromSlash(relPath))
	f, err := os.Open(abs)
	if err != nil {
		return 0, 0
	}
	defer func() { _ = f.Close() }()
	header := make([]byte, 24)
	n, err := io.ReadFull(f, header)
	if err != nil && n < 24 {
		return 0, 0
	}
	// PNG: 8-byte signature + IHDR chunk (width/height big-endian uint32 at offset 16/20).
	if header[0] == 0x89 && header[1] == 'P' && header[2] == 'N' && header[3] == 'G' {
		width := int(header[16])<<24 | int(header[17])<<16 | int(header[18])<<8 | int(header[19])
		height := int(header[20])<<24 | int(header[21])<<16 | int(header[22])<<8 | int(header[23])
		return width, height
	}
	return 0, 0
}
