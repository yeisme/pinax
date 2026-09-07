//go:build !windows

package app

import (
	"os"
	"syscall"
)

// processAlive 判定本机 pid 是否仍有活进程（signal 0 探测）。用于判定
// sync scope claim 持有者是否已死，从而允许幂等抢占。
func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}
