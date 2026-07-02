package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	SchemaVersion string           `json:"schema_version"`
	GeneratedAt   string           `json:"generated_at"`
	EntryCount    int              `json:"entry_count"`
	Entries       []ManifestEntry  `json:"entries"`
	Deletes       []ManifestDelete `json:"deletes,omitempty"`
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

type ManifestDelete struct {
	PathHash    string `json:"path_hash"`
	ObjectKind  string `json:"object_kind"`
	ObjectID    string `json:"object_id,omitempty"`
	TombstoneID string `json:"tombstone_id"`
	DeletedAt   string `json:"deleted_at,omitempty"`
	TrashBlobID string `json:"trash_blob_id,omitempty"`
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
	deletes, err := buildManifestDeletes(root)
	if err != nil {
		return Manifest{}, err
	}
	return Manifest{SchemaVersion: ManifestSchemaVersion, GeneratedAt: time.Now().UTC().Format(time.RFC3339), EntryCount: len(entries), Entries: entries, Deletes: deletes}, nil
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

func buildManifestDeletes(root string) ([]ManifestDelete, error) {
	path := filepath.Join(root, ".pinax", "records", "tombstones.json")
	payload, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	tombstones := map[string]struct {
		ObjectKind  string `json:"object_kind"`
		ObjectID    string `json:"object_id"`
		TombstoneID string `json:"tombstone_id"`
		OldPath     string `json:"old_path"`
		TrashPath   string `json:"trash_path"`
		DeletedAt   string `json:"deleted_at"`
	}{}
	if err := json.Unmarshal(payload, &tombstones); err != nil {
		return nil, err
	}
	deletes := make([]ManifestDelete, 0, len(tombstones))
	for key, tombstone := range tombstones {
		objectID := strings.TrimSpace(tombstone.ObjectID)
		if objectID == "" {
			objectID = key
		}
		objectKind := strings.TrimSpace(tombstone.ObjectKind)
		if objectKind == "" {
			objectKind = "note"
		}
		tombstoneID := strings.TrimSpace(tombstone.TombstoneID)
		if tombstoneID == "" {
			tombstoneID = "trash_" + strings.TrimPrefix(PathHash(objectID), "path_")[:12]
		}
		pathHashSource := objectID
		if strings.TrimSpace(tombstone.OldPath) != "" && objectKind == "note" {
			pathHashSource = tombstone.OldPath
		}
		deleteMarker := ManifestDelete{PathHash: PathHash(pathHashSource), ObjectKind: objectKind, ObjectID: objectID, TombstoneID: tombstoneID, DeletedAt: tombstone.DeletedAt}
		if strings.TrimSpace(tombstone.TrashPath) != "" {
			trashPath, joinErr := safeManifestJoin(root, tombstone.TrashPath)
			if joinErr != nil {
				return nil, joinErr
			}
			if info, statErr := os.Stat(trashPath); statErr == nil && !info.IsDir() {
				content, readErr := os.ReadFile(trashPath)
				if readErr != nil {
					return nil, readErr
				}
				deleteMarker.TrashBlobID = BlobID(content)
				if err := writeBlobCache(root, deleteMarker.TrashBlobID, content); err != nil {
					return nil, err
				}
			} else if statErr == nil && info.IsDir() {
				deleteMarker.TrashBlobID = PathHash(tombstone.TrashPath)
			}
		}
		deletes = append(deletes, deleteMarker)
	}
	sort.Slice(deletes, func(i, j int) bool { return deletes[i].PathHash < deletes[j].PathHash })
	return deletes, nil
}

func safeManifestJoin(root, rel string) (string, error) {
	if filepath.IsAbs(rel) || strings.Contains(filepath.ToSlash(rel), "../") || strings.HasPrefix(filepath.ToSlash(rel), "..") {
		return "", &ManifestUnsafePathError{Path: rel}
	}
	path := filepath.Clean(filepath.Join(root, filepath.FromSlash(rel)))
	if path != root && !strings.HasPrefix(path, root+string(os.PathSeparator)) {
		return "", &ManifestUnsafePathError{Path: rel}
	}
	return path, nil
}

type ManifestUnsafePathError struct{ Path string }

func (e *ManifestUnsafePathError) Error() string { return "unsafe_manifest_path: " + e.Path }

func writeBlobCache(root, blobID string, content []byte) error {
	path := filepath.Join(root, ".pinax", "cloud", "blob-cache", blobID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o600)
}
