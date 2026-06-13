package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

func Evidence(ctx context.Context, root, relPath string) domain.VersionEvidence {
	if _, err := os.Stat(filepath.Join(root, ".git")); errors.Is(err, os.ErrNotExist) {
		return domain.VersionEvidence{Backend: "none", WorktreeState: "unknown"}
	}
	evidence := domain.VersionEvidence{Backend: "git", WorktreeState: "unknown"}
	if head, err := gitOutput(ctx, root, "rev-parse", "HEAD"); err == nil {
		evidence.RevisionID = head
	}
	if status, err := gitOutput(ctx, root, "status", "--porcelain", "--", relPath); err == nil {
		if strings.TrimSpace(status) == "" {
			evidence.WorktreeState = "clean"
		} else {
			evidence.WorktreeState = "dirty"
		}
	}
	if blob, err := gitOutput(ctx, root, "hash-object", filepath.FromSlash(relPath)); err == nil {
		evidence.FileBlobID = blob
	}
	return evidence
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
