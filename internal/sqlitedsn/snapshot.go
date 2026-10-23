package sqlitedsn

import (
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"

	"gorm.io/gorm"
)

// ReadSnapshotLimit bounds the complete main+WAL copy. Larger indexes are
// unavailable to this read-only diagnostic instead of consuming unbounded IO.
const ReadSnapshotLimit int64 = 64 << 20

// OpenReadSnapshot inspects a stable private main+WAL copy. SQLite may create
// transient WAL/SHM even with mode=ro; those files must not appear in the vault.
// Never mark an active source immutable, checkpoint it, or remove its sidecars.
// Two identical bounded observations are required; an active writer may cause
// a conservative failure. This is not a replacement for a live DB transaction.
func OpenReadSnapshot(path string) (*gorm.DB, func(), error) {
	fail := func() (*gorm.DB, func(), error) { return nil, nil, errors.New("read_snapshot_unavailable") }
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return fail()
	}
	defer func() { _ = root.Close() }()
	// A hot rollback journal needs owner recovery; never silently omit it.
	if _, err := root.Lstat(filepath.Base(path) + "-journal"); !os.IsNotExist(err) {
		return fail()
	}
	dir, err := os.MkdirTemp("", "pinax-read-index-")
	if err != nil {
		return fail()
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	names := []string{filepath.Base(path), filepath.Base(path) + "-wal"}
	type observation struct {
		exists   bool
		size     int64
		hash     [32]byte
		identity os.FileInfo
	}
	observe := func(name string, dest string, budget int64) (observation, error) {
		var result observation
		info, err := root.Lstat(name)
		if os.IsNotExist(err) {
			return result, nil
		}
		if err != nil || !info.Mode().IsRegular() || info.Size() > budget {
			return result, errors.New("unsafe_snapshot_source")
		}
		source, err := root.Open(name)
		if err != nil {
			return result, err
		}
		defer func() { _ = source.Close() }()
		opened, err := source.Stat()
		if err != nil || !os.SameFile(info, opened) {
			return result, errors.New("snapshot_source_changed")
		}
		output := io.Writer(io.Discard)
		if dest != "" {
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return result, err
			}
			defer func() { _ = f.Close() }()
			output = f
		}
		hash := sha256.New()
		n, err := io.Copy(io.MultiWriter(output, hash), io.LimitReader(source, budget+1))
		after, statErr := root.Lstat(name)
		if err != nil || statErr != nil || n != info.Size() || n > budget || !os.SameFile(info, after) || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
			return result, errors.New("snapshot_source_changed")
		}
		result.exists = true
		result.size = n
		result.identity = info
		copy(result.hash[:], hash.Sum(nil))
		return result, nil
	}
	first := make([]observation, len(names))
	remaining := ReadSnapshotLimit
	for i, name := range names {
		first[i], err = observe(name, filepath.Join(dir, name), remaining)
		if err != nil || i == 0 && !first[i].exists {
			cleanup()
			return fail()
		}
		remaining -= first[i].size
	}
	remaining = ReadSnapshotLimit
	for i, name := range names {
		next, err := observe(name, "", remaining)
		if err != nil || next.exists != first[i].exists || next.size != first[i].size || next.hash != first[i].hash || next.exists && !os.SameFile(next.identity, first[i].identity) {
			cleanup()
			return fail()
		}
		remaining -= next.size
	}
	if _, err := root.Lstat(filepath.Base(path) + "-journal"); !os.IsNotExist(err) {
		cleanup()
		return fail()
	}
	db, err := OpenReadOnly(filepath.Join(dir, names[0]))
	if err != nil {
		cleanup()
		return fail()
	}
	return db, func() { _ = CloseAndRelease(db); cleanup() }, nil
}
