package remote

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

// mockStore is a simple in-memory BlobStore for testing CachedBlobStore.
type mockStore struct {
	mu   sync.Mutex
	data map[string][]byte
	revs map[string]string
	gets int
	puts int
	dels int
}

func newMockStore() *mockStore {
	return &mockStore{
		data: make(map[string][]byte),
		revs: make(map[string]string),
	}
}

func (m *mockStore) Get(_ context.Context, key string) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gets++
	data, ok := m.data[key]
	if !ok {
		return nil, "", ErrObjectNotFound
	}
	return data, m.revs[key], nil
}

func (m *mockStore) Put(_ context.Context, key string, data []byte, baseRev string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.puts++
	rev := "rev-" + key
	m.data[key] = data
	m.revs[key] = rev
	return rev, nil
}

func (m *mockStore) Stat(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rev, ok := m.revs[key]
	if !ok {
		return "", ErrObjectNotFound
	}
	return rev, nil
}

func (m *mockStore) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dels++
	delete(m.data, key)
	delete(m.revs, key)
	return nil
}

func TestCachedBlobStore_Get_CacheMiss(t *testing.T) {
	inner := newMockStore()
	inner.data["test-key"] = []byte("hello")
	inner.revs["test-key"] = "rev-1"

	dir := t.TempDir()
	cached := NewCachedBlobStore(inner, dir, 1024*1024)

	data, rev, err := cached.Get(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected hello, got %s", string(data))
	}
	if rev == "" {
		t.Fatal("expected non-empty rev")
	}
	if inner.gets != 1 {
		t.Fatalf("expected 1 inner get, got %d", inner.gets)
	}
}

func TestCachedBlobStore_Get_CacheHit(t *testing.T) {
	inner := newMockStore()
	inner.data["test-key"] = []byte("hello")
	inner.revs["test-key"] = "rev-1"

	dir := t.TempDir()
	cached := NewCachedBlobStore(inner, dir, 1024*1024)

	// First get: miss
	_, _, err := cached.Get(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("Get 1: %v", err)
	}

	// Second get: should be cached
	data, _, err := cached.Get(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("Get 2: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected hello, got %s", string(data))
	}
	if inner.gets != 1 {
		t.Fatalf("expected 1 inner get (cached), got %d", inner.gets)
	}
}

func TestCachedBlobStore_Get_NotFound(t *testing.T) {
	inner := newMockStore()
	dir := t.TempDir()
	cached := NewCachedBlobStore(inner, dir, 1024*1024)

	_, _, err := cached.Get(context.Background(), "nonexistent")
	if !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound, got %v", err)
	}
}

func TestCachedBlobStore_Put_InvalidatesCache(t *testing.T) {
	inner := newMockStore()
	inner.data["key"] = []byte("v1")
	inner.revs["key"] = "rev-1"

	dir := t.TempDir()
	cached := NewCachedBlobStore(inner, dir, 1024*1024)

	// Prime cache
	_, _, _ = cached.Get(context.Background(), "key")

	// Put new data (invalidates cache)
	_, err := cached.Put(context.Background(), "key", []byte("v2"), "")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Get should fetch from inner (cache was invalidated)
	data, _, err := cached.Get(context.Background(), "key")
	if err != nil {
		t.Fatalf("Get after Put: %v", err)
	}
	if string(data) != "v2" {
		t.Fatalf("expected v2, got %s", string(data))
	}
	if inner.gets != 2 {
		t.Fatalf("expected 2 gets, got %d", inner.gets)
	}
}

func TestCachedBlobStore_Delete_InvalidatesCache(t *testing.T) {
	inner := newMockStore()
	inner.data["key"] = []byte("v1")
	inner.revs["key"] = "rev-1"

	dir := t.TempDir()
	cached := NewCachedBlobStore(inner, dir, 1024*1024)

	// Prime cache
	_, _, _ = cached.Get(context.Background(), "key")

	// Delete
	if err := cached.Delete(context.Background(), "key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Get should return not found
	_, _, err := cached.Get(context.Background(), "key")
	if !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound after delete, got %v", err)
	}
}

func TestCachedBlobStore_Stat_DelegatesToInner(t *testing.T) {
	inner := newMockStore()
	inner.data["key"] = []byte("data")
	inner.revs["key"] = "rev-123"

	dir := t.TempDir()
	cached := NewCachedBlobStore(inner, dir, 1024*1024)

	rev, err := cached.Stat(context.Background(), "key")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if rev != "rev-123" {
		t.Fatalf("expected rev-123, got %s", rev)
	}
}

func TestCachedBlobStore_Eviction(t *testing.T) {
	inner := newMockStore()
	dir := t.TempDir()
	// Very small cache to trigger eviction
	cached := NewCachedBlobStore(inner, dir, 50)

	// Put multiple objects to exceed limit
	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		inner.data[key] = []byte("data-" + key + "-padding-to-exceed-size")
		inner.revs[key] = "rev-" + key
	}

	// Get all to cache them
	for i := 0; i < 5; i++ {
		key := string(rune('a' + i))
		_, _, err := cached.Get(context.Background(), key)
		if err != nil {
			t.Fatalf("Get %s: %v", key, err)
		}
	}

	// Verify cache dir has files
	blobDir := filepath.Join(dir, "blobs")
	entries, err := filepath.Glob(filepath.Join(blobDir, "*"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	// Some entries should have been evicted
	if len(entries) == 0 {
		t.Fatal("expected some cached files")
	}
}
