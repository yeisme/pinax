package assets

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

type PathDisplayStyle string

const (
	PathStyleVaultRelative PathDisplayStyle = "vault-relative"
	PathStyleNoteRelative  PathDisplayStyle = "note-relative"
	PathStyleAbsolute      PathDisplayStyle = "absolute"
	PathStyleMarkdown      PathDisplayStyle = "markdown"
	PathStyleWiki          PathDisplayStyle = "wiki"
)

type PathDisplayRequest struct {
	Root            string
	AssetPath       string
	ContextNotePath string
	MediaType       string
	Label           string
	Style           PathDisplayStyle
}

func DisplayPath(req PathDisplayRequest) (string, error) {
	style := req.Style
	if style == "" {
		style = PathStyleVaultRelative
	}
	assetPath, err := safeAttachmentRel(req.AssetPath)
	if err != nil {
		return "", err
	}
	switch style {
	case PathStyleVaultRelative:
		return assetPath, nil
	case PathStyleAbsolute:
		return displayAbsolutePath(req.Root, assetPath)
	case PathStyleNoteRelative:
		return noteRelativeAssetPath(req.Root, assetPath, req.ContextNotePath)
	case PathStyleMarkdown:
		rel, err := noteRelativeAssetPath(req.Root, assetPath, req.ContextNotePath)
		if err != nil {
			return "", err
		}
		label := strings.TrimSpace(req.Label)
		if label == "" {
			label = filepath.Base(assetPath)
		}
		if isImageMedia(req.MediaType, assetPath) {
			return fmt.Sprintf("![%s](%s)", label, rel), nil
		}
		return fmt.Sprintf("[%s](%s)", label, rel), nil
	case PathStyleWiki:
		if isImageMedia(req.MediaType, assetPath) {
			return "![[" + assetPath + "]]", nil
		}
		return "[[" + assetPath + "]]", nil
	default:
		return "", &domain.CommandError{Code: "path_style_invalid", Message: "附件路径展示样式无效", Hint: "使用 vault-relative、note-relative、absolute、markdown 或 wiki"}
	}
}

func displayAbsolutePath(root, assetPath string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absTarget := filepath.Join(absRoot, filepath.FromSlash(assetPath))
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil || rel == ".." || strings.HasPrefix(filepath.ToSlash(rel), "../") || filepath.IsAbs(rel) {
		return "", &domain.CommandError{Code: "asset_path_unsafe", Message: "附件路径越过 vault 边界", Hint: "选择 vault 内的附件路径"}
	}
	return absTarget, nil
}

func noteRelativeAssetPath(root, assetPath, contextNotePath string) (string, error) {
	contextNotePath, err := safeAttachmentRel(contextNotePath)
	if err != nil || contextNotePath == "" {
		return "", &domain.CommandError{Code: "path_context_required", Message: "路径展示样式需要 context note", Hint: "提供 --context-note <note>"}
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	fromDir := filepath.Dir(filepath.Join(absRoot, filepath.FromSlash(contextNotePath)))
	toPath := filepath.Join(absRoot, filepath.FromSlash(assetPath))
	rel, err := filepath.Rel(fromDir, toPath)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func isImageMedia(mediaType, assetPath string) bool {
	if strings.HasPrefix(mediaType, "image/") || mediaType == "image" {
		return true
	}
	switch strings.ToLower(filepath.Ext(assetPath)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return true
	default:
		return false
	}
}
