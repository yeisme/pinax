package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const KBImporterVersion = "pinax.kb.import.v1"

func kbSourceType(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".txt") {
		return "text"
	}
	return "markdown"
}

func kbContentDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// kbSafeSourceRef keeps a source reference useful for local files already
// inside the vault while avoiding absolute paths for external imports.
func kbSafeSourceRef(root, source string) string {
	absRoot, rootErr := filepath.Abs(root)
	absSource, sourceErr := filepath.Abs(source)
	if rootErr == nil && sourceErr == nil {
		rel, err := filepath.Rel(filepath.Clean(absRoot), filepath.Clean(absSource))
		if err == nil {
			rel = filepath.ToSlash(rel)
			if rel != ".." && !strings.HasPrefix(rel, "../") && !filepath.IsAbs(rel) {
				return rel
			}
		}
	}
	// The path itself is not evidence. A stable opaque ref still lets a
	// reviewer correlate repeated imports without exposing the machine path.
	return "external/" + strings.TrimPrefix(kbContentDigest([]byte(filepath.Clean(source))), "sha256:")[:16]
}

func buildKBImportedNoteContent(plan kbImportPlan, body, acquiredAt string) string {
	content := buildNoteContentWithStatus(plan.Title, plan.TargetPath, "", "kb/imports", "reference", []string{"kb", "imported"}, "active", acquiredAt, body)
	metadata := fmt.Sprintf("source_type: %s\nsource_ref: %s\nsource_digest: %s\nsource_version: %s\nacquired_at: %s\nimporter_version: %s\n", plan.SourceType, plan.SourceRef, plan.SourceDigest, plan.SourceVersion, acquiredAt, KBImporterVersion)
	return strings.Replace(content, "\n---\n\n", "\n"+metadata+"---\n\n", 1)
}

func kbReadSourceLineage(root, source string) (sourceType, sourceRef, sourceDigest, sourceVersion string, content []byte, err error) {
	content, err = os.ReadFile(source)
	if err != nil {
		return "", "", "", "", nil, err
	}
	sourceType = kbSourceType(source)
	sourceRef = kbSafeSourceRef(root, source)
	sourceDigest = kbContentDigest(content)
	sourceVersion = sourceDigest
	return sourceType, sourceRef, sourceDigest, sourceVersion, content, nil
}
