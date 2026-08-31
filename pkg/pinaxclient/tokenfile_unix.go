//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package pinaxclient

import (
	"os"
	"syscall"
)

func validateTokenFilePlatform(_ string, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() || info.Mode().Perm() != 0o600 {
		return errTokenFilePolicy
	}
	return nil
}

func validateTokenFilePathEntryPlatform(_ string, info os.FileInfo, ancestor bool) error {
	if !ancestor {
		return nil
	}
	permissions := info.Mode().Perm()
	if permissions&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
		return errTokenFilePolicy
	}
	return nil
}
