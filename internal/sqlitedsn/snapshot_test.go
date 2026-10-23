package sqlitedsn

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSnapshotLeavesSourceUntouched(t *testing.T) {
	for _, live := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed", true: "live-wal"}[live], func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "index.sqlite")
			writer, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.AutoMigrate(&dsnProbeRow{}); err != nil {
				t.Fatal(err)
			}
			if err := writer.Create(&dsnProbeRow{ID: 1, Value: "snapshot"}).Error; err != nil {
				t.Fatal(err)
			}
			if !live {
				if err := CloseAndRelease(writer); err != nil {
					t.Fatal(err)
				}
			} else {
				defer func() { _ = CloseAndRelease(writer) }()
			}
			before, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			data := map[string]string{}
			for _, entry := range before {
				b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				data[entry.Name()] = string(b)
			}
			db, cleanup, err := OpenReadSnapshot(path)
			if err != nil {
				t.Fatal(err)
			}
			var row dsnProbeRow
			if err := db.First(&row, 1).Error; err != nil || row.Value != "snapshot" {
				cleanup()
				t.Fatal("snapshot lost committed row")
			}
			if err := db.Create(&dsnProbeRow{ID: 2, Value: "no"}).Error; err == nil {
				cleanup()
				t.Fatal("snapshot accepted mutation")
			}
			cleanup()
			after, err := os.ReadDir(dir)
			if err != nil || len(after) != len(before) {
				t.Fatal("snapshot changed source tree")
			}
			for _, entry := range after {
				b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
				if err != nil || string(b) != data[entry.Name()] {
					t.Fatal("snapshot changed source bytes")
				}
			}
		})
	}
}
func TestReadSnapshotRejectsUnsafeSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.sqlite")
	if _, _, err := OpenReadSnapshot(path); err == nil {
		t.Fatal("missing source accepted")
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(ReadSnapshotLimit + 1); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()
	if _, _, err := OpenReadSnapshot(path); err == nil {
		t.Fatal("oversized source accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenReadSnapshot(path); err == nil {
		t.Fatal("directory accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	writer, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.AutoMigrate(&dsnProbeRow{}); err != nil {
		t.Fatal(err)
	}
	if err := CloseAndRelease(writer); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+"-journal", []byte("fixture hot journal"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenReadSnapshot(path); err == nil {
		t.Fatal("rollback journal silently ignored")
	}
	if err := os.Remove(path + "-journal"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, path+"-wal"); err != nil {
		t.Skip("symlink unavailable")
	}
	if _, _, err := OpenReadSnapshot(path); err == nil {
		t.Fatal("WAL symlink accepted")
	}
}
