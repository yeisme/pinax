package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

const drivebridgePinaxVaultMarker = ".drivebridge-pinax-vault.json"

func ensureVaultContentDir(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(path, 0o755)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, statErr := os.Stat(path)
		if statErr != nil {
			return &domain.CommandError{Code: "content_unmounted", Message: "notes path is a dangling symlink", Hint: "Run drivebridge mount vault --preset pinax in a user terminal, then retry pinax init"}
		}
		if !target.IsDir() {
			return fmt.Errorf("notes exists and is not a directory")
		}
		return nil
	}
	if info.IsDir() {
		return nil
	}
	return fmt.Errorf("notes exists and is not a directory")
}

func mountedVaultFacts(root string) map[string]string {
	facts := map[string]string{
		"control_plane_local": "false",
		"content_mount":       "none",
		"content_writable":    "false",
		"drivebridge_preset":  "none",
	}
	pinaxDir := filepath.Join(root, ".pinax")
	if info, err := os.Lstat(pinaxDir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			facts["control_plane_local"] = "false"
		} else if info.IsDir() {
			facts["control_plane_local"] = "true"
		}
	}
	if body, err := os.ReadFile(filepath.Join(root, drivebridgePinaxVaultMarker)); err == nil && strings.Contains(string(body), "pinax-vault") {
		facts["drivebridge_preset"] = "pinax-vault"
	}
	notes := filepath.Join(root, "notes")
	info, err := os.Lstat(notes)
	if err != nil {
		facts["content_mount"] = "unmounted"
		return facts
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		if _, statErr := os.Stat(notes); statErr != nil {
			facts["content_mount"] = "unmounted"
		} else if facts["drivebridge_preset"] == "pinax-vault" {
			facts["content_mount"] = "drivebridge_path"
		} else {
			facts["content_mount"] = "unknown_fuse"
		}
	case info.IsDir():
		if facts["drivebridge_preset"] == "pinax-vault" {
			facts["content_mount"] = "drivebridge_path"
		} else {
			facts["content_mount"] = "none"
		}
	default:
		facts["content_mount"] = "unmounted"
	}
	if facts["content_mount"] != "unmounted" {
		f, createErr := os.CreateTemp(notes, ".pinax-doctor-*")
		if createErr == nil {
			name := f.Name()
			_ = f.Close()
			_ = os.Remove(name)
			facts["content_writable"] = "true"
		}
	}
	return facts
}

func mergeMountedVaultFacts(projection *domain.Projection, root string) {
	for fact, value := range mountedVaultFacts(root) {
		projection.Facts[fact] = value
	}
}

func controlPlaneOnRemoteIssue(root string) []domain.Issue {
	info, err := os.Lstat(filepath.Join(root, ".pinax"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	return []domain.Issue{{
		Code:    "control_plane_on_remote",
		Path:    ".pinax",
		Message: "Pinax control plane is a symlink; keep .pinax on local disk, not on the mounted content tree",
	}}
}
