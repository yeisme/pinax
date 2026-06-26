package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/vaultignore"
)

const ManifestSchemaVersion = "pinax.cloud.manifest.v1"

const MaxManifestFileBytes = 100 * 1024 * 1024

type Manifest struct {
	SchemaVersion string          `json:"schema_version"`
	GeneratedAt   string          `json:"generated_at"`
	EntryCount    int             `json:"entry_count"`
	Entries       []ManifestEntry `json:"entries"`
}

type ManifestEntry struct {
	Path       string `json:"path"`
	PathHash   string `json:"path_hash"`
	BlobID     string `json:"blob_id"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	ObjectKind string `json:"object_kind,omitempty"`
	Mode       uint32 `json:"mode,omitempty"`
	MediaType  string `json:"media_type,omitempty"`
}

func BuildManifest(root string) (Manifest, error) {
	root, err := cleanRoot(root)
	if err != nil {
		return Manifest{}, err
	}
	matcher, err := vaultignore.Load(root)
	if err != nil {
		return Manifest{}, err
	}
	entries := make([]ManifestEntry, 0)
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if matcher.Ignored(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || matcher.Ignored(rel, false) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > MaxManifestFileBytes {
			return &ManifestFileTooLargeError{Path: rel, Size: info.Size(), Limit: MaxManifestFileBytes}
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		blobID := BlobID(b)
		if err := writeBlobCache(root, blobID, b); err != nil {
			return err
		}
		entries = append(entries, ManifestEntry{Path: rel, PathHash: PathHash(rel), BlobID: blobID, Size: int64(len(b)), SHA256: contentSHA256(b), ObjectKind: manifestObjectKind(rel), Mode: uint32(info.Mode().Perm()), MediaType: mediaType(rel)})
		return nil
	}); err != nil {
		return Manifest{}, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].PathHash < entries[j].PathHash
	})
	return Manifest{SchemaVersion: ManifestSchemaVersion, GeneratedAt: time.Now().UTC().Format(time.RFC3339), EntryCount: len(entries), Entries: entries}, nil
}

type ManifestFileTooLargeError struct {
	Path  string
	Size  int64
	Limit int64
}

func (e *ManifestFileTooLargeError) Error() string {
	return "content_file_too_large: " + e.Path
}

func PathHash(path string) string {
	normalized := normalizeManifestPath(path)
	h := sha256.Sum256([]byte(normalized))
	return "path_" + hex.EncodeToString(h[:])
}

func BlobID(content []byte) string {
	h := sha256.Sum256(content)
	return "blob_" + hex.EncodeToString(h[:])
}

func normalizeManifestPath(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	path = filepath.ToSlash(filepath.Clean(path))
	path = strings.TrimPrefix(path, "./")
	return strings.ToLower(path)
}

func contentSHA256(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

func manifestObjectKind(rel string) string {
	lower := strings.ToLower(rel)
	if strings.HasSuffix(lower, ".md") {
		return "note"
	}
	if strings.HasPrefix(rel, "assets/") || strings.HasPrefix(rel, "attachments/") {
		return "asset"
	}
	return "file"
}

func mediaType(rel string) string {
	if mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(rel))); mt != "" {
		return mt
	}
	return "application/octet-stream"
}

func writeBlobCache(root, blobID string, content []byte) error {
	path := filepath.Join(root, ".pinax", "cloud", "blob-cache", blobID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o600)
}
