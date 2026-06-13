package assets

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

type EmbeddedAssetPreview struct {
	Path       string `json:"path"`
	MediaType  string `json:"media_type"`
	RenderMode string `json:"render_mode"`
	ByteCount  int    `json:"byte_count"`
	Status     string `json:"status"`
	Truncated  bool   `json:"truncated,omitempty"`
	Warning    string `json:"warning,omitempty"`
}

type RenderPreviewRequest struct {
	Root       string
	SourcePath string
	Body       string
	Mode       string
	MaxDepth   int
	MaxBytes   int
}

type RenderPreviewResult struct {
	Body           string
	EmbeddedAssets []EmbeddedAssetPreview
}

func RenderEmbeddedPreview(req RenderPreviewRequest) (RenderPreviewResult, error) {
	mode := strings.TrimSpace(req.Mode)
	if mode == "" || mode == "none" {
		return RenderPreviewResult{Body: req.Body}, nil
	}
	if mode != "markdown" && mode != "text" {
		return RenderPreviewResult{}, &domain.CommandError{Code: "attachment_embed_mode_invalid", Message: "附件内联模式无效", Hint: "使用 markdown、text 或 none"}
	}
	maxDepth := req.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 3
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 8192
	}
	seen := map[string]bool{filepath.ToSlash(req.SourcePath): true}
	body, embedded, err := renderEmbeddedPreviewBody(req.Root, filepath.ToSlash(req.SourcePath), req.Body, mode, maxDepth, maxBytes, 0, seen)
	return RenderPreviewResult{Body: body, EmbeddedAssets: embedded}, err
}

func renderEmbeddedPreviewBody(root, sourcePath, body, mode string, maxDepth, maxBytes, depth int, seen map[string]bool) (string, []EmbeddedAssetPreview, error) {
	links := previewEmbedLinks(sourcePath, body)
	if len(links) == 0 {
		return body, nil, nil
	}
	out := body
	embedded := make([]EmbeddedAssetPreview, 0, len(links))
	for _, link := range links {
		mediaType := previewMediaType(link.AssetPath)
		entry := EmbeddedAssetPreview{Path: link.AssetPath, MediaType: mediaType, RenderMode: mode, Status: "placeholder"}
		if seen[link.AssetPath] {
			entry.Warning = "attachment_embed_cycle"
			out += previewPlaceholder(entry)
			embedded = append(embedded, entry)
			continue
		}
		if depth >= maxDepth {
			entry.Warning = "attachment_embed_depth"
			out += previewPlaceholder(entry)
			embedded = append(embedded, entry)
			continue
		}
		if !previewReadable(link.AssetPath, mode) {
			out += previewPlaceholder(entry)
			embedded = append(embedded, entry)
			continue
		}
		content, truncated, err := readBoundedPreview(filepath.Join(root, filepath.FromSlash(link.AssetPath)), maxBytes)
		if err != nil {
			entry.Status = "missing"
			entry.Warning = "attachment_missing"
			out += previewPlaceholder(entry)
			embedded = append(embedded, entry)
			continue
		}
		entry.Status = "embedded"
		entry.ByteCount = len([]byte(content))
		entry.Truncated = truncated
		childSeen := copySeen(seen)
		childSeen[link.AssetPath] = true
		if strings.EqualFold(filepath.Ext(link.AssetPath), ".md") {
			childBody, childEmbedded, err := renderEmbeddedPreviewBody(root, link.AssetPath, content, mode, maxDepth, maxBytes, depth+1, childSeen)
			if err != nil {
				return "", nil, err
			}
			content = childBody
			embedded = append(embedded, childEmbedded...)
		}
		out += fmt.Sprintf("\n\n<!-- pinax-embedded-asset path=%s -->\n\n%s", link.AssetPath, content)
		embedded = append(embedded, entry)
	}
	return out, embedded, nil
}

func previewEmbedLinks(sourcePath, body string) []AssetLink {
	links := make([]AssetLink, 0)
	for lineNo, line := range strings.Split(body, "\n") {
		for _, match := range markdownReferencePattern.FindAllStringSubmatch(line, -1) {
			if len(match) < 2 || !strings.HasPrefix(match[0], "![") {
				continue
			}
			if assetPath, ok := resolvePreviewReference(sourcePath, normalizeAssetReferenceTarget(match[1])); ok {
				links = append(links, AssetLink{AssetPath: assetPath, SourcePath: sourcePath, RawReference: match[0], LinkStyle: "markdown", LinkKind: "embed", Line: lineNo + 1})
			}
		}
		for _, match := range wikiReferencePattern.FindAllStringSubmatch(line, -1) {
			if len(match) < 2 || !strings.HasPrefix(match[0], "![[") {
				continue
			}
			if assetPath, ok := resolvePreviewReference(sourcePath, normalizeWikiAssetReferenceTarget(match[1])); ok {
				links = append(links, AssetLink{AssetPath: assetPath, SourcePath: sourcePath, RawReference: match[0], LinkStyle: "wiki", LinkKind: "embed", Line: lineNo + 1})
			}
		}
	}
	return links
}

func resolvePreviewReference(sourcePath, target string) (string, bool) {
	target = strings.TrimSpace(filepath.ToSlash(target))
	if target == "" || isExternalAssetReference(target) {
		return "", false
	}
	if strings.HasPrefix(target, "/") {
		target = strings.TrimLeft(target, "/")
	}
	baseDir := pathpkg.Dir(filepath.ToSlash(sourcePath))
	clean := pathpkg.Clean(pathpkg.Join(baseDir, target))
	if strings.HasPrefix(target, "attachments/") || strings.HasPrefix(target, "assets/") || strings.HasPrefix(target, "notes/") {
		clean = pathpkg.Clean(target)
	}
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, ".pinax/") || pathpkg.Ext(clean) == "" {
		return "", false
	}
	return clean, true
}

func previewReadable(path, mode string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if mode == "markdown" && ext == ".md" {
		return true
	}
	return ext == ".txt" || ext == ".text" || ext == ".log" || ext == ".csv" || ext == ".json" || ext == ".yaml" || ext == ".yml"
}

func previewMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md":
		return "text/markdown"
	case ".txt", ".text", ".log":
		return "text/plain"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}

func readBoundedPreview(path string, maxBytes int) (string, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	if len(b) > maxBytes {
		return string(b[:maxBytes]), true, nil
	}
	return string(b), false, nil
}

func previewPlaceholder(entry EmbeddedAssetPreview) string {
	reason := entry.Warning
	if reason == "" {
		reason = "binary_placeholder"
	}
	return fmt.Sprintf("\n\n> [!asset] %s (%s, %s)\n> pinax asset show %s --vault <vault> --json", entry.Path, entry.MediaType, reason, filepath.Base(entry.Path))
}

func copySeen(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
