package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func Snapshot(ctx context.Context, root, message string) error {
	if _, err := os.Stat(filepath.Join(root, ".git")); errors.Is(err, os.ErrNotExist) {
		if err := run(ctx, root, "init"); err != nil {
			return err
		}
	}
	_ = run(ctx, root, "config", "user.email", "pinax@example.local")
	_ = run(ctx, root, "config", "user.name", "Pinax")
	if err := run(ctx, root, "add", "-A"); err != nil {
		return err
	}
	if err := run(ctx, root, "commit", "-m", message); err != nil {
		if !strings.Contains(err.Error(), "nothing to commit") && !strings.Contains(err.Error(), "no changes added") {
			return err
		}
	}
	return os.WriteFile(filepath.Join(root, ".pinax", "last_snapshot"), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)
}

func HasSnapshot(root string) bool {
	_, err := os.Stat(filepath.Join(root, ".pinax", "last_snapshot"))
	return err == nil
}

// HeadCommit 返回当前 HEAD commit hash；没有提交时返回空串和 nil。
func HeadCommit(ctx context.Context, root string) (string, error) {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return "", nil
	}
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// RestorePathFromCommit 把指定路径的工作区内容恢复到给定 commit。
// 它是 proof loop 可逆 apply 的底层恢复原语：apply 前已 git commit，
// 坏 apply 后用 commit hash 把单个文件 checkout 回来，不发明内容。
func RestorePathFromCommit(ctx context.Context, root, commit, path string) error {
	if commit == "" {
		return fmt.Errorf("git restore requires a commit")
	}
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if clean == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("git restore path must be vault-relative")
	}
	return run(ctx, root, "checkout", commit, "--", clean)
}

func run(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
