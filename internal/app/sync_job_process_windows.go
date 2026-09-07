//go:build windows

package app

import "os"

// processAlive 在 Windows 上经 FindProcess 判定 pid 是否仍存在：Windows
// 实现里 FindProcess 对不存在的进程返回错误。
func processAlive(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}
