package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/syncwire"
	"github.com/yeisme/pinax/internal/vaultignore"
)

const (
	ManifestSchemaVersionV1 = syncwire.ManifestSchemaVersionV1
	ManifestSchemaVersionV2 = syncwire.ManifestSchemaVersionV2
	ManifestSchemaVersion   = syncwire.ManifestSchemaVersion
)

// Wire types are owned by internal/syncwire; these aliases keep the existing
// remote.* identifiers working while guaranteeing a single on-wire schema.
type (
	Manifest       = syncwire.Manifest
	ManifestEntry  = syncwire.ManifestEntry
	ManifestDelete = syncwire.ManifestDelete
)

const MaxManifestFileBytes = 100 * 1024 * 1024

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
		entries = append(entries, ManifestEntry{Path: rel, PathHash: PathHash(rel), BlobID: blobID, Size: int64(len(b)), SHA256: contentSHA256(b), ObjectKind: manifestObjectKind(rel), Mode: uint32(info.Mode().Perm()), MediaType: mediaType(rel), UpdatedAt: info.ModTime().UTC().Format(time.RFC3339Nano)})
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

type ManifestIdentity struct {
	ObjectID   string
	ObjectKind string
}

// conflictCopyPattern matches sync-preserved conflict copies
// (`<base>.<yyyyMMddHHmmss>.conflict.md`). These are local snapshots for
// manual merge, not managed sync objects: they carry the same canonical
// note_id as the live note, so syncing them would duplicate object identity
// and permanently block manifest v2 pushes.
var conflictCopyPattern = regexp.MustCompile(`\.\d{14}\.conflict\.md$`)

// IsConflictCopyPath reports whether rel is a sync conflict-copy path.
func IsConflictCopyPath(rel string) bool {
	return conflictCopyPattern.MatchString(filepath.ToSlash(strings.TrimSpace(rel)))
}

func BuildManifestV2(root, deviceID string, identities map[string]ManifestIdentity) (Manifest, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return Manifest{}, fmt.Errorf("manifest_device_required")
	}
	manifest, err := BuildManifest(root)
	if err != nil {
		return Manifest{}, err
	}
	// Conflict copies stay local: they preserve pre-pull content for manual
	// merge and duplicate the live note's canonical object id.
	entries := make([]ManifestEntry, 0, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		if IsConflictCopyPath(entry.Path) {
			continue
		}
		entries = append(entries, entry)
	}
	manifest.Entries = entries
	manifest.EntryCount = len(entries)
	seenObjectIDs := make(map[string]string, len(manifest.Entries))
	for index := range manifest.Entries {
		entry := &manifest.Entries[index]
		identityFact, ok := identities[entry.Path]
		if !ok || strings.TrimSpace(identityFact.ObjectID) == "" || strings.TrimSpace(identityFact.ObjectKind) == "" {
			return Manifest{}, fmt.Errorf("manifest_identity_required: %s", entry.Path)
		}
		objectID := strings.TrimSpace(identityFact.ObjectID)
		if previousPath, duplicate := seenObjectIDs[objectID]; duplicate {
			return Manifest{}, fmt.Errorf("manifest_duplicate_object_id: %s: %s, %s", objectID, previousPath, entry.Path)
		}
		seenObjectIDs[objectID] = entry.Path
		entry.ObjectID = objectID
		entry.ObjectKind = strings.TrimSpace(identityFact.ObjectKind)
		entry.RevisionID = manifestEntryRevisionID(objectID, entry.BlobID)
		entry.DeviceID = deviceID
	}
	for index := range manifest.Deletes {
		deleteMarker := &manifest.Deletes[index]
		if strings.TrimSpace(deleteMarker.ObjectID) == "" {
			return Manifest{}, fmt.Errorf("manifest_tombstone_identity_required: %s", deleteMarker.TombstoneID)
		}
		deleteMarker.RevisionID = manifestEntryRevisionID(deleteMarker.ObjectID, deleteMarker.TombstoneID+":"+deleteMarker.TrashBlobID)
		deleteMarker.DeviceID = deviceID
	}
	manifest.SchemaVersion = ManifestSchemaVersionV2
	return manifest, manifest.ValidateV2()
}

func manifestEntryRevisionID(objectID, contentFact string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(objectID) + "\x00" + strings.TrimSpace(contentFact)))
	return "revision_" + hex.EncodeToString(digest[:])
}
