package syncdaemon

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

type Lock struct {
	Path string
	PID  int `json:"pid"`
}

type lockFile struct {
	PID        int    `json:"pid"`
	Owner      string `json:"owner"`
	AcquiredAt string `json:"acquired_at"`
	ExpiresAt  string `json:"expires_at"`
}

func AcquireRunnerLock(root string) (Lock, error) {
	return acquireLock(filepath.Join(root, ".pinax", "sync-daemon", "daemon.lock"), "sync.daemon", 6*time.Hour)
}

func AcquireOperationLock(root, owner string) (Lock, error) {
	return acquireLock(filepath.Join(root, ".pinax", "sync", "operation.lock"), owner, 30*time.Minute)
}

func acquireLock(path, owner string, ttl time.Duration) (Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Lock{}, err
	}
	if existing, err := readLock(path); err == nil && !lockStale(existing) {
		return Lock{}, &domain.CommandError{Code: "lock_held", Message: "sync lock is already held", Hint: "Wait for the running sync operation or inspect sync daemon status"}
	} else if err == nil {
		_ = os.Remove(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Lock{}, err
	}
	now := time.Now().UTC()
	payload, err := json.MarshalIndent(lockFile{PID: os.Getpid(), Owner: owner, AcquiredAt: now.Format(time.RFC3339), ExpiresAt: now.Add(ttl).Format(time.RFC3339)}, "", "  ")
	if err != nil {
		return Lock{}, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return Lock{}, &domain.CommandError{Code: "lock_held", Message: "sync lock is already held", Hint: "Retry after the current sync operation finishes"}
		}
		return Lock{}, err
	}
	if _, err := file.Write(append(payload, '\n')); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return Lock{}, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return Lock{}, err
	}
	return Lock{Path: path, PID: os.Getpid()}, nil
}

func (l Lock) Release() { _ = os.Remove(l.Path) }

func readLock(path string) (lockFile, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return lockFile{}, err
	}
	var lock lockFile
	if err := json.Unmarshal(payload, &lock); err != nil {
		return lockFile{}, err
	}
	return lock, nil
}

func lockStale(lock lockFile) bool {
	if expires, err := time.Parse(time.RFC3339, lock.ExpiresAt); err == nil && time.Now().UTC().After(expires) {
		return true
	}
	return lock.PID > 0 && !pidAlive(lock.PID)
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func PIDAlive(pid int) bool { return pidAlive(pid) }
