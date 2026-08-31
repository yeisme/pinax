//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package pinaxclient

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestTokenFileOwnerPolicyRejectsDifferentUID(t *testing.T) {
	t.Parallel()

	otherUID := uint32(os.Geteuid() + 1)
	info := unixTokenFileInfo{stat: &syscall.Stat_t{Uid: otherUID}}
	if err := validateTokenFilePlatform("", info); err == nil {
		t.Fatal("different token file owner was accepted")
	}
}

type unixTokenFileInfo struct {
	stat *syscall.Stat_t
}

func (info unixTokenFileInfo) Name() string       { return "token" }
func (info unixTokenFileInfo) Size() int64        { return 1 }
func (info unixTokenFileInfo) Mode() os.FileMode  { return 0o600 }
func (info unixTokenFileInfo) ModTime() time.Time { return time.Time{} }
func (info unixTokenFileInfo) IsDir() bool        { return false }
func (info unixTokenFileInfo) Sys() any           { return info.stat }
