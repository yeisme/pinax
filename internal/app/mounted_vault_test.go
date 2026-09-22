package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestInitVaultKeepsExistingNotesSymlink(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewService()
	content := t.TempDir()
	notes := filepath.Join(content, "notes")
	if err := os.Mkdir(notes, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notes, "kept.md"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	link := filepath.Join(root, "notes")
	if err := os.Symlink(notes, link); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Mounted"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	got, err := os.Readlink(link)
	if err != nil || got != notes {
		t.Fatalf("notes link changed: %q %v", got, err)
	}
	body, err := os.ReadFile(filepath.Join(root, "notes", "kept.md"))
	if err != nil || string(body) != "keep" {
		t.Fatalf("content lost: %s %v", body, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "config.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestInitVaultDanglingNotesSymlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "missing-target"), filepath.Join(root, "notes")); err != nil {
		t.Fatal(err)
	}
	_, err := NewService().InitVault(context.Background(), InitVaultRequest{VaultPath: root, Title: "Broken"})
	if !hasCommandCode(err, "content_unmounted") {
		t.Fatalf("err=%v", err)
	}
}

func TestStorageDoctorMountedVaultFacts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewService()
	root := t.TempDir()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Local"}); err != nil {
		t.Fatal(err)
	}
	doctor, err := svc.StorageDoctor(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatal(err)
	}
	if doctor.Facts["control_plane_local"] != "true" || doctor.Facts["content_mount"] != "none" || doctor.Facts["drivebridge_preset"] != "none" || doctor.Facts["content_writable"] != "true" {
		t.Fatalf("plain local facts=%#v", doctor.Facts)
	}

	content := t.TempDir()
	if err := os.Mkdir(filepath.Join(content, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	mounted := t.TempDir()
	if err := os.Symlink(filepath.Join(content, "notes"), filepath.Join(mounted, "notes")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: mounted, Title: "Mounted"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mounted, ".drivebridge-pinax-vault.json"), []byte(`{"preset":"pinax-vault"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	mountedDoctor, err := svc.StorageDoctor(ctx, VaultRequest{VaultPath: mounted})
	if err != nil {
		t.Fatal(err)
	}
	if mountedDoctor.Facts["drivebridge_preset"] != "pinax-vault" || mountedDoctor.Facts["content_mount"] != "drivebridge_path" || mountedDoctor.Facts["control_plane_local"] != "true" {
		t.Fatalf("mounted facts=%#v", mountedDoctor.Facts)
	}
}

func TestStorageDoctorControlPlaneOnRemote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewService()
	root := t.TempDir()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "X"}); err != nil {
		t.Fatal(err)
	}
	real := t.TempDir()
	pinax := filepath.Join(root, ".pinax")
	if err := os.Rename(pinax, filepath.Join(real, ".pinax")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(real, ".pinax"), pinax); err != nil {
		t.Fatal(err)
	}
	doctor, err := svc.StorageDoctor(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatal(err)
	}
	if doctor.Facts["control_plane_local"] != "false" {
		t.Fatalf("facts=%#v", doctor.Facts)
	}
	found := false
	data, _ := doctor.Data.(map[string]any)
	switch issues := data["issues"].(type) {
	case []domain.Issue:
		for _, issue := range issues {
			if issue.Code == "control_plane_on_remote" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("missing control_plane_on_remote in %#v", doctor.Data)
	}
}

func TestInitVaultRefusesPinaxSymlink(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := NewService()
	root := t.TempDir()
	remote := t.TempDir()
	if err := os.Symlink(remote, filepath.Join(root, ".pinax")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Mounted"}); !hasCommandCode(err, "control_plane_symlink") {
		t.Fatalf("expected control_plane_symlink, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(remote, "config.yaml")); err == nil {
		t.Fatal("init must not write the control plane through the symlink")
	}
}

func TestMountedVaultFactsSymlinkToFileAndBoundedMarker(t *testing.T) {
	t.Parallel()
	// notes 符号链接指向普通文件：如实报 unmounted，不伪装 unknown_fuse。
	root := t.TempDir()
	targetFile := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(targetFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetFile, filepath.Join(root, "notes")); err != nil {
		t.Fatal(err)
	}
	facts := mountedVaultFacts(root)
	if facts["content_mount"] != "unmounted" || facts["content_writable"] != "false" {
		t.Fatalf("symlink-to-file must report unmounted, facts = %#v", facts)
	}
	// 超 64KiB 的 marker 视为无效：preset 不生效。
	root2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root2, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root2, drivebridgePinaxVaultMarker), bytes.Repeat([]byte("pinax-vault junk "), 5000), 0o644); err != nil {
		t.Fatal(err)
	}
	facts2 := mountedVaultFacts(root2)
	if facts2["drivebridge_preset"] != "none" {
		t.Fatalf("oversized marker must be ignored, facts = %#v", facts2)
	}
}
