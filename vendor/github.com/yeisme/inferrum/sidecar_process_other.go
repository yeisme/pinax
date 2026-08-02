//go:build !linux

package inferrum

import "os/exec"

// Non-Linux builds retain subprocess execution but do not claim Linux process-
// group cleanup semantics.
func configureSidecarProcess(*exec.Cmd) {}
func cleanupSidecarProcess(*exec.Cmd)   {}
