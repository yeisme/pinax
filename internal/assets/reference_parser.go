package assets

import (
	"net/url"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

type AssetLink = domain.AssetLink

// LinkExtractionRequest describes a single Markdown note body to scan for asset references.
type LinkExtractionRequest struct {
	SourceNoteID string
	SourcePath   string
	Body         string
}

var (
	markdownReferencePattern = regexp.MustCompile(`!?\[[^\]]*\]\((<[^>]+>|[^)]+)\)`)
	wikiReferencePattern     = regexp.MustCompile(`!?\[\[([^\]]+)\]\]`)
)

// ExtractLinks parses local non-Markdown attachment references without rewriting the note body.
func ExtractLinks(req LinkExtractionRequest) []AssetLink {
	links := make([]AssetLink, 0)
	for lineNo, line := range strings.Split(req.Body, "\n") {
		links = append(links, markdownAssetLinksInLine(req, line, lineNo+1)...)
		links = append(links, wikiAssetLinksInLine(req, line, lineNo+1)...)
	}
	return links
}

func markdownAssetLinksInLine(req LinkExtractionRequest, line string, lineNo int) []AssetLink {
	matches := markdownReferencePattern.FindAllStringSubmatch(line, -1)
	links := make([]AssetLink, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		raw := match[0]
		target := normalizeAssetReferenceTarget(match[1])
		assetPath, ok := resolveAssetReference(req.SourcePath, target)
		if !ok {
			continue
		}
		kind := "link"
		if strings.HasPrefix(raw, "![") {
			kind = "embed"
		}
		links = append(links, AssetLink{AssetPath: assetPath, SourceNoteID: req.SourceNoteID, SourcePath: req.SourcePath, RawReference: raw, LinkStyle: "markdown", LinkKind: kind, Line: lineNo, Status: "unresolved"})
	}
	return links
}

func wikiAssetLinksInLine(req LinkExtractionRequest, line string, lineNo int) []AssetLink {
	matches := wikiReferencePattern.FindAllStringSubmatch(line, -1)
	links := make([]AssetLink, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		raw := match[0]
		// Wiki alias/size hints after "|" are display evidence only; path resolution uses the target part.
		target := normalizeWikiAssetReferenceTarget(match[1])
		assetPath, ok := resolveAssetReference(req.SourcePath, target)
		if !ok {
			continue
		}
		kind := "link"
		if strings.HasPrefix(raw, "![[") {
			kind = "embed"
		}
		links = append(links, AssetLink{AssetPath: assetPath, SourceNoteID: req.SourceNoteID, SourcePath: req.SourcePath, RawReference: raw, LinkStyle: "wiki", LinkKind: kind, Line: lineNo, Status: "unresolved"})
	}
	return links
}

func normalizeAssetReferenceTarget(target string) string {
	target = strings.TrimSpace(target)
	if strings.HasPrefix(target, "<") && strings.HasSuffix(target, ">") {
		target = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(target, "<"), ">"))
	}
	if before, _, ok := strings.Cut(target, "#"); ok {
		target = before
	}
	if before, _, ok := strings.Cut(target, "?"); ok {
		target = before
	}
	return strings.TrimSpace(target)
}

func normalizeWikiAssetReferenceTarget(target string) string {
	target = strings.TrimSpace(target)
	if before, _, ok := strings.Cut(target, "|"); ok {
		target = before
	}
	if before, _, ok := strings.Cut(target, "#"); ok {
		target = before
	}
	return strings.TrimSpace(target)
}

func resolveAssetReference(sourcePath, target string) (string, bool) {
	target = strings.TrimSpace(filepath.ToSlash(target))
	if target == "" || isExternalAssetReference(target) {
		return "", false
	}
	decoded, err := url.PathUnescape(target)
	if err != nil {
		return "", false
	}
	decoded = filepath.ToSlash(strings.TrimSpace(decoded))
	if decoded == "" || isExternalAssetReference(decoded) || strings.EqualFold(pathpkg.Ext(decoded), ".md") || pathpkg.Ext(decoded) == "" {
		return "", false
	}
	if strings.HasPrefix(decoded, "/") {
		decoded = strings.TrimLeft(decoded, "/")
	}
	baseDir := pathpkg.Dir(filepath.ToSlash(sourcePath))
	clean := pathpkg.Clean(pathpkg.Join(baseDir, decoded))
	if strings.HasPrefix(decoded, "attachments/") || strings.HasPrefix(decoded, "assets/") || strings.HasPrefix(decoded, "notes/") {
		clean = pathpkg.Clean(decoded)
	}
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, ".pinax/") || strings.HasPrefix(clean, "//") {
		return "", false
	}
	allowedRoot := strings.HasPrefix(clean, "assets/") || strings.HasPrefix(clean, "attachments/") || strings.HasPrefix(clean, "notes/")
	if !allowedRoot && strings.Contains(decoded, "..") {
		return "", false
	}
	return clean, true
}

func isExternalAssetReference(target string) bool {
	lower := strings.ToLower(strings.TrimSpace(target))
	if strings.HasPrefix(lower, "//") || strings.HasPrefix(lower, "#") {
		return true
	}
	parsed, err := url.Parse(lower)
	if err == nil && parsed.Scheme != "" {
		return true
	}
	return false
}
