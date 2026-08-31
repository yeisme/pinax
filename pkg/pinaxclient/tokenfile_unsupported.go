//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package pinaxclient

import "os"

func validateTokenFilePlatform(_ string, _ os.FileInfo) error {
	return errTokenFilePlatformUnsupported
}

func validateTokenFilePathEntryPlatform(_ string, _ os.FileInfo, _ bool) error {
	return errTokenFilePlatformUnsupported
}
