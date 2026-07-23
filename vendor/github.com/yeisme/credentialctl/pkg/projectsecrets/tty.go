package projectsecrets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"

	"golang.org/x/term"
)

// openControllingTTY opens /dev/tty on POSIX so the prompt never competes with
// a stdin that may carry a secret payload. On Windows there is no /dev/tty, so
// the caller falls back to os.Stdin or fails with UnlockRequiredError when no
// console is attached.
func openControllingTTY() (*os.File, error) {
	if runtime.GOOS == "windows" {
		// On Windows, CONIN$ is the console input handle. If the process has no
		// console (e.g. under a service), this fails closed.
		f, err := os.Open("CONIN$")
		if err != nil {
			return nil, fmt.Errorf("no console")
		}
		return f, nil
	}
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("no controlling tty")
	}
	return f, nil
}

// readPassword disables echo on fd, reads a line (until \n or \r) and restores
// the terminal. It mirrors golang.org/x/term.ReadPassword, which is the
// audited pure-Go implementation of cfmakeraw/termios ECHO off.
func readPassword(f *os.File) ([]byte, error) {
	fd := int(f.Fd())
	if !term.IsTerminal(fd) {
		// Not a terminal — fail closed rather than reading a secret from a pipe.
		return nil, fmt.Errorf("not a terminal")
	}
	// term.ReadPassword handles echo disable + line read + restore internally.
	readPasswordMu.Lock()
	defer readPasswordMu.Unlock()
	return term.ReadPassword(fd)
}

// shortDigest returns a non-sensitive 12-hex-char identifier. It is suitable
// for recording "a keychain item exists" or "this account" without leaking the
// account value; it is a truncated SHA-256, not the secret itself.
func shortDigest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:6])
}

// Ensure the imports are used on platforms where openControllingTTY/readPassword
// paths differ.
var _ io.Writer = os.Stderr
