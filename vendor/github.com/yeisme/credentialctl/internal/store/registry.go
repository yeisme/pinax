package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/yeisme/credentialctl/pkg/credentials"
)

const registryVersion = 1

// RegistryStore persists the redacted registry as an atomic 0600 JSON file. It
// never holds secret bytes — only Entry records with redacted digests.
type RegistryStore struct {
	path string
}

// NewRegistryStore returns a registry store at <base>/registry.json.
func NewRegistryStore(base string) *RegistryStore {
	return &RegistryStore{path: filepath.Join(base, "registry.json")}
}

type registryFile struct {
	Version int                          `json:"version"`
	Entries map[string]credentials.Entry `json:"entries"` // keyed by ref.Key()
}

// Load reads all entries. A missing registry is equivalent to an empty set.
func (r *RegistryStore) Load() (map[string]credentials.Entry, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]credentials.Entry{}, nil
		}
		return nil, err
	}
	var rf registryFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("%w: %v", credentials.ErrRegistryCorrupt, err)
	}
	if rf.Entries == nil {
		rf.Entries = map[string]credentials.Entry{}
	}
	return rf.Entries, nil
}

// Save atomically writes all entries. A corrupt/partial write never replaces a
// good registry.
func (r *RegistryStore) Save(entries map[string]credentials.Entry) error {
	if err := mkPrivateDir(filepath.Dir(r.path)); err != nil {
		return err
	}
	rf := registryFile{Version: registryVersion, Entries: entries}
	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return err
	}
	return r.atomicWrite(data)
}

func (r *RegistryStore) atomicWrite(data []byte) error {
	dir := filepath.Dir(r.path)
	tmp, err := os.CreateTemp(dir, ".tmp-registry-*")
	if err != nil {
		// dir may not exist yet on the very first write
		if err2 := mkPrivateDir(dir); err2 != nil {
			return err2
		}
		tmp, err = os.CreateTemp(dir, ".tmp-registry-*")
		if err != nil {
			return err
		}
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpName, r.path); err != nil {
		return err
	}
	cleanup = false
	fsyncDir(dir)
	return nil
}
