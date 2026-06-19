package versionops

import (
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

func CleanObjectPath(path string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if clean == "" || clean == "." || clean == ".." || filepath.IsAbs(path) || strings.HasPrefix(clean, "../") {
		return "", &domain.CommandError{Code: "version_path_invalid", Message: "version path must be vault-relative", Hint: "Use a path like notes/example.md"}
	}
	return clean, nil
}
