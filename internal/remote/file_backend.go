package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	Register("file", func(ctx context.Context, endpoint string) (BlobStore, error) {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		path := u.Path
		if u.Host != "" && u.Host != "localhost" {
			path = filepath.Join(u.Host, path)
		}
		return NewFileBackend(path)
	})
}

type FileBackend struct {
	baseDir string
}

func NewFileBackend(baseDir string) (*FileBackend, error) {
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	return &FileBackend{baseDir: abs}, nil
}

func (b *FileBackend) SupportsConditionalWrites() bool { return true }

func (b *FileBackend) Get(ctx context.Context, key string) ([]byte, string, error) {
	path, err := b.objectPath(key)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", ErrObjectNotFound
		}
		return nil, "", err
	}
	return data, computeRev(data), nil
}

func (b *FileBackend) Put(ctx context.Context, key string, data []byte, baseRev string) (string, error) {
	path, err := b.objectPath(key)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}

	lockPath := path + ".lock"
	if err := acquireLock(lockPath); err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(lockPath) }()

	if baseRev != "" {
		existingData, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if baseRev == CreateIfAbsentRevision {
			if err == nil {
				return "", ErrConflict
			}
		} else {
			currentRev := ""
			if err == nil {
				currentRev = computeRev(existingData)
			}
			if currentRev != baseRev {
				return "", ErrConflict
			}
		}
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}

	return computeRev(data), nil
}

func (b *FileBackend) Stat(ctx context.Context, key string) (string, error) {
	path, err := b.objectPath(key)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrObjectNotFound
		}
		return "", err
	}
	return computeRev(data), nil
}

func (b *FileBackend) Delete(ctx context.Context, key string) error {
	path, err := b.objectPath(key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// List returns objects under the given prefix.
func (b *FileBackend) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	dir, err := b.objectPath(prefix)
	if err != nil {
		return nil, err
	}
	var objects []ObjectInfo
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(b.baseDir, path)
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		objects = append(objects, ObjectInfo{
			Key:          rel,
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return objects, nil
}

// Exists checks if an object exists.
func (b *FileBackend) Exists(ctx context.Context, key string) (bool, error) {
	path := filepath.Join(b.baseDir, key)
	_, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// BatchStat returns revisions for multiple keys.
func (b *FileBackend) BatchStat(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		rev, err := b.Stat(ctx, key)
		if err == nil {
			result[key] = rev
		}
	}
	return result, nil
}

func (b *FileBackend) objectPath(key string) (string, error) {
	if strings.ContainsRune(key, '\x00') || filepath.IsAbs(key) {
		return "", fmt.Errorf("unsafe object key: %q", key)
	}
	slashKey := strings.ReplaceAll(key, "\\", "/")
	if path.IsAbs(slashKey) {
		return "", fmt.Errorf("unsafe object key: %q", key)
	}
	cleaned := path.Clean(slashKey)
	if cleaned == "." {
		cleaned = ""
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("unsafe object key: %q", key)
	}
	full := filepath.Join(b.baseDir, filepath.FromSlash(cleaned))
	rel, err := filepath.Rel(b.baseDir, full)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe object key: %q", key)
	}
	return full, nil
}

func computeRev(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func acquireLock(path string) error {
	for i := 0; i < 50; i++ { // try for 5 seconds
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			return nil
		}
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout acquiring lock %s", path)
}
