package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/credentialctl/pkg/credentials"
)

// FileBackend stores secrets as 0600 files under a 0700 private directory. It
// is the fallback when no system Keychain is available. Writes are atomic:
// each Put writes a same-directory temp file, fsyncs, applies 0600 and renames
// over the target so a partial write never replaces a good secret.
type FileBackend struct {
	base string // e.g. <config>/yeisme/credentialctl
}

func assertResolvedParentContained(root, target string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedParent)
	if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("secret target resolves outside secrets root")
	}
	return nil
}

// NewFileBackend returns a file backend rooted at base. The directory and its
// secrets/ subtree are created with restrictive permissions on first use.
func NewFileBackend(base string) *FileBackend {
	return &FileBackend{base: base}
}

// DefaultBase returns the default backend root under the user config dir.
func DefaultBase() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "yeisme", "credentialctl"), nil
}

func (f *FileBackend) Name() string { return "file" }

func (f *FileBackend) Available() bool { return true }

func (f *FileBackend) ensureRoot() error {
	// 0700 root + secrets dir.
	if err := mkPrivateDir(f.base); err != nil {
		return err
	}
	return mkPrivateDir(filepath.Join(f.base, "secrets"))
}

// secretPath returns the on-disk path for a registry key (provider/account).
// key is validated so a crafted key cannot escape the secrets root.
func (f *FileBackend) secretPath(key string) (string, error) {
	ref, err := credentials.ParseRef(credentials.Scheme + "://" + key)
	if err != nil {
		return "", fmt.Errorf("invalid secret key %q: %w", key, err)
	}
	root := filepath.Join(f.base, "secrets")
	target := filepath.Join(root, ref.Provider, ref.Account)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid secret key %q: target escapes secrets root", key)
	}
	return target, nil
}

func (f *FileBackend) Get(key string) ([]byte, error) {
	path, err := f.secretPath(key)
	if err != nil {
		return nil, err
	}
	if err := assertResolvedParentContained(filepath.Join(f.base, "secrets"), path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, credentials.ErrNotFound
		}
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, credentials.ErrNotFound
		}
		return nil, err
	}
	return data, nil
}

func (f *FileBackend) Put(key string, secret []byte) error {
	if err := f.ensureRoot(); err != nil {
		return err
	}
	path, err := f.secretPath(key)
	if err != nil {
		return err
	}
	if err := mkPrivateDir(filepath.Dir(path)); err != nil {
		return err
	}
	if err := assertResolvedParentContained(filepath.Join(f.base, "secrets"), path); err != nil {
		return err
	}
	// temp file in the same directory so rename is atomic on the same filesystem
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-secret-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(secret); err != nil {
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
	// enforce 0600 before the rename reveals the file
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	// best-effort: fsync the directory so the rename is durable
	fsyncDir(filepath.Dir(path))
	// fail closed if the file ended up more permissive than 0600
	if err := assertSecretPerms(path); err != nil {
		return err
	}
	return nil
}

func (f *FileBackend) Delete(key string) error {
	path, err := f.secretPath(key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // removing a missing key is not an error
		}
		return err
	}
	return nil
}

// mkPrivateDir creates dir with 0700 (or verifies it). On Unix this enforces
// owner-only access; on other platforms the call still creates the directory.
func mkPrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return assertDirPerms(dir)
}

func assertDirPerms(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	// Only validate the owner-permission bits on Unix; group/world bits being
	// set is what we want to forbid.
	if info.Mode().Perm()&0o077 != 0 {
		// tighten rather than fail, so a previously-loose dir self-heals
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func assertSecretPerms(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(path, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func fsyncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	defer d.Close()
	_ = d.Sync()
}
