package continuitybinding

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DetectWorktreeRoot 返回 path 所属 Git worktree 的 canonical 顶层目录。
//
// 边界说明（中文）：
//  1. 使用 `git rev-parse --show-toplevel` 解析 worktree 根。该命令在
//     非 Git 目录返回非零退出码，我们据此返回 not_a_repository 语义；
//     不向父目录之外的未知位置搜索，也不扫描其他 vault。
//  2. 返回路径经 CanonicalizeRepoRoot 规范化（Abs + EvalSymlinks），
//     使得通过 symlink 进入同一 worktree 的调用命中同一 exact binding key。
//  3. 超时 5s 防止损坏的 Git 状态挂起短生命周期 CLI 进程。
func DetectWorktreeRoot(ctx context.Context, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		// git 自身会向 stderr 写原因；这里只保留稳定分类，不泄漏 stderr
		// 或调用方提交的绝对路径到 evidence/日志。
		return "", fmt.Errorf("not a git worktree")
	}
	return CanonicalizeRepoRoot(strings.TrimSpace(string(out)))
}
