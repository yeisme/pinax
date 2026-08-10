package app

import (
	"fmt"
	"os"
	"path/filepath"
)

// atomicWriteFile writes data to path atomically using a same-directory temp
// file, fsync, and rename (pinax-passphrase-s3-bootstrap task 6.9). On any
// failure the destination is left unchanged, so a declaration or envelope write
// that fails partway cannot corrupt the previous usable file. The temp file
// lives in the same directory so rename is atomic on POSIX filesystems.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s directory: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("stage %s: %w", path, err)
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
		return fmt.Errorf("write staged %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsync staged %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close staged %s: %w", path, err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod staged %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("commit %s: %w", path, err)
	}
	cleanup = false
	return nil
}
