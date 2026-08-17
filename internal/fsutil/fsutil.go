// Package fsutil holds small filesystem helpers shared across app packages so
// they stop being re-implemented (with drifting argument order) per package.
package fsutil

import (
	"io"
	"os"
)

// CopyFile streams src into dst, creating or truncating dst with 0644
// permissions. It replaces two package-local copies whose argument orders
// disagreed with each other.
func CopyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
