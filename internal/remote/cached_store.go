package remote

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// CachedBlobStore decorates a BlobStore with local file caching for Get operations.
type CachedBlobStore struct {
	inner    BlobStore
	cacheDir string
	maxSize  int64 // max cache size in bytes
	mu       sync.Mutex
}

// NewCachedBlobStore creates a new caching decorator.
func NewCachedBlobStore(inner BlobStore, cacheDir string, maxSize int64) *CachedBlobStore {
	return &CachedBlobStore{
		inner:    inner,
		cacheDir: cacheDir,
		maxSize:  maxSize,
	}
}

// Get retrieves the object, using cache when available.
func (c *CachedBlobStore) Get(ctx context.Context, key string) ([]byte, string, error) {
	cachePath := c.cachePath(key)

	// Check cache
	if data, err := os.ReadFile(cachePath); err == nil {
		rev := computeCacheRev(data)
		// Update access time for LRU
		_ = os.Chtimes(cachePath, time.Now(), time.Now())
		return data, rev, nil
	}

	// Cache miss: fetch from inner
	data, rev, err := c.inner.Get(ctx, key)
	if err != nil {
		return nil, "", err
	}

	// Cache the result
	_ = os.MkdirAll(filepath.Dir(cachePath), 0o700)
	_ = os.WriteFile(cachePath, data, 0o600)

	// Evict if over limit
	go c.evictIfNeeded()

	return data, rev, nil
}

// Put delegates to inner store and invalidates cache.
func (c *CachedBlobStore) Put(ctx context.Context, key string, data []byte, baseRev string) (string, error) {
	newRev, err := c.inner.Put(ctx, key, data, baseRev)
	if err != nil {
		return "", err
	}
	_ = os.Remove(c.cachePath(key))
	return newRev, nil
}

// Stat delegates to inner store.
func (c *CachedBlobStore) Stat(ctx context.Context, key string) (string, error) {
	return c.inner.Stat(ctx, key)
}

// Delete delegates to inner store and invalidates cache.
func (c *CachedBlobStore) Delete(ctx context.Context, key string) error {
	err := c.inner.Delete(ctx, key)
	_ = os.Remove(c.cachePath(key))
	return err
}

func (c *CachedBlobStore) cachePath(key string) string {
	return filepath.Join(c.cacheDir, "blobs", key)
}

func computeCacheRev(data []byte) string {
	// Simple hash for cache hit identification
	h := 0
	for _, b := range data {
		h = h*31 + int(b)
	}
	return "cache"
}

type cacheEntry struct {
	path  string
	atime time.Time
	size  int64
}

func (c *CachedBlobStore) evictIfNeeded() {
	c.mu.Lock()
	defer c.mu.Unlock()

	var totalSize int64
	var entries []cacheEntry

	blobDir := filepath.Join(c.cacheDir, "blobs")
	_ = filepath.WalkDir(blobDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		totalSize += info.Size()
		entries = append(entries, cacheEntry{
			path:  path,
			atime: info.ModTime(),
			size:  info.Size(),
		})
		return nil
	})

	if totalSize <= c.maxSize || len(entries) == 0 {
		return
	}

	// Sort by access time, oldest first
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].atime.Before(entries[j].atime)
	})

	// Remove oldest entries until under limit
	for _, entry := range entries {
		if totalSize <= c.maxSize {
			break
		}
		_ = os.Remove(entry.path)
		totalSize -= entry.size
	}
}
