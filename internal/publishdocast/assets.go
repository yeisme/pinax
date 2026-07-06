package publishdocast

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
)

// AssetOptions 控制 native plan 组装时的资产解析行为。
// 本包只做路径安全校验和资产识别；实际渲染（Mermaid/SVG → 图片）由 provider adapter
// 在 push 阶段完成（lark-cli whiteboard 服务端渲染 / docs +media-insert 本地上传）。
type AssetOptions struct {
	// NoteDir 是当前 note 在 vault 中的相对目录（如 notes/index），用于解析相对图片。
	NoteDir string
}

// resolveRelativePath 把 note 相对目录 + 引用路径合成 vault-relative 路径。
// 拒绝路径逃逸：合成后不得以 ../ 越过 vault 根，也不接受绝对路径。
// package/receipt 只记录 vault-relative 路径，绝不保存本地绝对路径。
func resolveRelativePath(noteDir, ref string) (string, error) {
	ref = filepath.ToSlash(filepath.Clean(strings.TrimSpace(ref)))
	if ref == "" {
		return "", errors.New("empty image reference")
	}
	if filepath.IsAbs(ref) {
		return "", errors.New("absolute image path rejected")
	}
	var joined string
	if noteDir != "" {
		joined = filepath.ToSlash(filepath.Clean(filepath.Join(noteDir, ref)))
	} else {
		joined = ref
	}
	if isPathEscape(joined) {
		return "", errors.New("path escapes vault boundary")
	}
	return joined, nil
}

// isPathEscape 检测合成路径是否越过 vault 根（clean 后仍以 .. 开头）。
func isPathEscape(path string) bool {
	path = filepath.ToSlash(filepath.Clean(path))
	return path == ".." || strings.HasPrefix(path, "../")
}

func mediaKey(kind, seed string) string {
	sum := sha256.Sum256([]byte(kind + ":" + seed))
	return kind + "_" + hex.EncodeToString(sum[:])[:16]
}

func mediaTypeForPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	if len(s) > 80 {
		return s[:80]
	}
	return s
}
