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

func run(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
