package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func testFileSHA(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func initDrivebridgeVault(t *testing.T, svc *Service) string {
	t.Helper()
	root := t.TempDir()
	if _, err := svc.InitVault(context.Background(), InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	return root
}

func TestAttachDrivebridgeLocalZeroCopy(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetLocalStorage(ctx, StorageRequest{VaultPath: root, Root: root}); err != nil {
		t.Fatalf("set local storage: %v", err)
	}
	storageBefore, err := os.ReadFile(filepath.Join(root, ".pinax", "storage.json"))
	if err != nil {
		t.Fatalf("read storage.json: %v", err)
	}
	projection, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"})
	if err != nil {
		t.Fatalf("attach drivebridge: %v", err)
	}
	if projection.Facts["copied"] != "false" || projection.Facts["drivebridge_content_mode"] != "adopt_local" {
		t.Fatalf("attach facts = %#v", projection.Facts)
	}
	calls := fake.calls(t)
	if !strings.Contains(calls, "storage adopt --kind local --consumer pinax --space pinax-vault --root ") {
		t.Fatalf("adopt call missing local root:\n%s", calls)
	}
	storageAfter, err := os.ReadFile(filepath.Join(root, ".pinax", "storage.json"))
	if err != nil || string(storageBefore) != string(storageAfter) {
		t.Fatalf("storage.json must stay untouched by attach")
	}
	raw, err := os.ReadFile(filepath.Join(root, ".pinax", "drivebridge-attach.yaml"))
	if err != nil {
		t.Fatalf("attach record missing: %v", err)
	}
	if !strings.Contains(string(raw), "schema_version: pinax.drivebridge_attach.v1") ||
		!strings.Contains(string(raw), "content_mode: adopt_local") ||
		!strings.Contains(string(raw), "consumer: pinax") {
		t.Fatalf("attach record = \n%s", raw)
	}
}

func TestAttachDrivebridgeS3AndLocationMismatch(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	fake.setState(t, map[string]any{
		"adopts": []map[string]any{
			{"consumer": "pinax", "kind": "s3", "space": "pinax-vault", "location": "notes/other", "copied": false},
		},
	})
	_, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"})
	if !hasCommandCode(err, "drivebridge_location_mismatch") {
		t.Fatalf("expected drivebridge_location_mismatch, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".pinax", "drivebridge-attach.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("mismatch must not leave an attach record")
	}
	profile, err := loadStorageProfile(root)
	if err != nil || profile.Backend != "s3" || profile.S3.Bucket != "notes" || profile.S3.Prefix != "pinax/" {
		t.Fatalf("storage profile changed on mismatch: %#v err=%v", profile, err)
	}
	fake.setState(t, map[string]any{"adopts": []map[string]any{}})
	projection, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"})
	if err != nil {
		t.Fatalf("attach s3: %v", err)
	}
	if projection.Facts["drivebridge_kind"] != "s3" || projection.Facts["drivebridge_content_mode"] != "adopt_s3" {
		t.Fatalf("attach s3 facts = %#v", projection.Facts)
	}
	calls := fake.calls(t)
	if !strings.Contains(calls, "--kind s3") || !strings.Contains(calls, "--remote-path notes/pinax") {
		t.Fatalf("adopt s3 args wrong:\n%s", calls)
	}
	record, attached, err := loadDrivebridgeAttach(root)
	if err != nil || !attached || record.Location.Bucket != "notes" || record.Location.Prefix != "pinax" {
		t.Fatalf("attach record = %#v attached=%v err=%v", record, attached, err)
	}
}

func TestAttachDrivebridgeLocalRootMismatch(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetLocalStorage(ctx, StorageRequest{VaultPath: root, Root: root}); err != nil {
		t.Fatalf("set local storage: %v", err)
	}
	fake.setState(t, map[string]any{
		"adopts": []map[string]any{
			{"consumer": "pinax", "kind": "local", "space": "pinax-vault", "location": "/somewhere/else", "copied": false},
		},
	})
	_, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"})
	if !hasCommandCode(err, "drivebridge_location_mismatch") {
		t.Fatalf("expected drivebridge_location_mismatch, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".pinax", "drivebridge-attach.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("root mismatch must not leave an attach record")
	}
}

func TestAttachDrivebridgeNotInstalledKeepsOwnerCommands(t *testing.T) {
	emptyBin := t.TempDir()
	t.Setenv("PATH", emptyBin)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	_, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"})
	if !hasCommandCode(err, "drivebridge_not_installed") {
		t.Fatalf("expected drivebridge_not_installed, got %v", err)
	}
	if _, err := svc.StorageStatus(ctx, VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("storage status must work without DriveBridge: %v", err)
	}
	doctor, err := svc.StorageDoctor(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("storage doctor must work without DriveBridge: %v", err)
	}
	if doctor.Facts["drivebridge_attached"] != "false" || doctor.Facts["drivebridge_content_mode"] != "none" {
		t.Fatalf("doctor facts = %#v", doctor.Facts)
	}
	if _, err := svc.CapsaBackendSetS3(ctx, CloudBackendSetRequest{VaultPath: root, Bucket: "cap", Region: "us-east-1", WorkspaceID: "ws", DeviceID: "dev", EncryptionSecretRef: "test://enc"}); err != nil {
		t.Fatalf("capsa backend set must work without DriveBridge: %v", err)
	}
	if _, err := svc.AddBackend(ctx, BackendAddRequest{VaultPath: root, Name: "local-1", Kind: "local", Root: root}); err != nil {
		t.Fatalf("backend add must work without DriveBridge: %v", err)
	}
}

func TestAttachDrivebridgeWorkingCopyOptInRequired(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	fake.setState(t, map[string]any{
		"adopts": []map[string]any{
			{"consumer": "other", "kind": "onedrive", "space": "notes-copy", "location": "onedrive:Notes", "copied": false},
		},
	})
	_, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "notes-copy"})
	if !hasCommandCode(err, "drivebridge_working_copy_opt_in_required") {
		t.Fatalf("expected drivebridge_working_copy_opt_in_required, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".pinax", "drivebridge-attach.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("rejected attach must not write a record")
	}
}

func TestBindWorkingCopyProviderPlaintext(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	fake.setState(t, map[string]any{
		"spaces": []map[string]any{{"id": "notes-copy", "backend": "onedrive", "local_root": ""}},
	})
	projection, err := svc.BindWorkingCopy(ctx, DrivebridgeBindWorkingCopyRequest{VaultPath: root, Provider: "onedrive", Space: "notes-copy"})
	if err != nil {
		t.Fatalf("bind working copy: %v", err)
	}
	if projection.Facts["drivebridge_content_mode"] != "provider-plaintext" {
		t.Fatalf("bind facts = %#v", projection.Facts)
	}
	doctor, err := svc.StorageDoctor(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("storage doctor: %v", err)
	}
	if doctor.Facts["drivebridge_content_mode"] != "provider-plaintext" || doctor.Facts["drivebridge_kind"] != "onedrive" {
		t.Fatalf("doctor facts = %#v", doctor.Facts)
	}
	// 日常 note add 只写本地：绑定后 DriveBridge 不应收到任何调用。
	before := fake.calls(t)
	created, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Local only", Body: "body"})
	if err != nil {
		t.Fatalf("note add: %v", err)
	}
	if fake.calls(t) != before {
		t.Fatalf("note add must not call DriveBridge:\n%s", fake.calls(t))
	}
	notePath := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(created.Facts["path"], "./")))
	if _, err := os.Stat(notePath); err != nil {
		t.Fatalf("note file missing: %v (facts %#v)", err, created.Facts)
	}
	// 不存在的网盘空间诚实失败。
	fake.setState(t, map[string]any{"spaces": []map[string]any{}})
	_, err = svc.BindWorkingCopy(ctx, DrivebridgeBindWorkingCopyRequest{VaultPath: root, Provider: "gdrive", Space: "missing"})
	if !hasCommandCode(err, "drivebridge_space_not_found") {
		t.Fatalf("expected drivebridge_space_not_found, got %v", err)
	}
}

func TestDetachDrivebridgeKeepsOwnerStorage(t *testing.T) {
	newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetLocalStorage(ctx, StorageRequest{VaultPath: root, Root: root}); err != nil {
		t.Fatalf("set local storage: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	projection, err := svc.DetachDrivebridge(ctx, DrivebridgeDetachRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("detach: %v", err)
	}
	if len(projection.Warnings) == 0 {
		t.Fatalf("detach should warn that the owner-side release is unavailable: %#v", projection)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".pinax", "drivebridge-attach.yaml")); !os.IsNotExist(statErr) {
		t.Fatalf("detach must remove the attach record")
	}
	status, err := svc.StorageStatus(ctx, VaultRequest{VaultPath: root})
	if err != nil || status.Facts["backend"] != "local" {
		t.Fatalf("storage status after detach = %#v err=%v", status.Facts, err)
	}
	// 再次 detach 诚实失败。
	if _, err := svc.DetachDrivebridge(ctx, DrivebridgeDetachRequest{VaultPath: root}); !hasCommandCode(err, "drivebridge_not_attached") {
		t.Fatalf("expected drivebridge_not_attached, got %v", err)
	}
}

func TestStorageDoctorFactsSeparateModes(t *testing.T) {
	newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	t.Setenv("PINAX_API_URL", "")
	// 未 attach：不要求 DriveBridge，content_mode=none。
	status, err := svc.StorageStatus(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Facts["drivebridge_attached"] != "false" || status.Facts["drivebridge_content_mode"] != "none" {
		t.Fatalf("unattached facts = %#v", status.Facts)
	}
	if status.Facts["capsa_sync_configured"] != "false" || status.Facts["remote_api_configured"] != "false" {
		t.Fatalf("mode facts = %#v", status.Facts)
	}
	// 配置 Capsa s3-direct 与 DriveBridge provider 工作副本：两条路径分开。
	if _, err := svc.CapsaBackendSetS3(ctx, CloudBackendSetRequest{VaultPath: root, Bucket: "cap-bucket", Region: "us-east-1", Prefix: "capsa/", WorkspaceID: "ws", DeviceID: "dev", EncryptionSecretRef: "test://enc"}); err != nil {
		t.Fatalf("capsa backend set: %v", err)
	}
	fakeSpaces := newFakeDrivebridge(t)
	fakeSpaces.setState(t, map[string]any{
		"spaces": []map[string]any{{"id": "notes-copy", "backend": "gdrive", "local_root": ""}},
	})
	if _, err := svc.BindWorkingCopy(ctx, DrivebridgeBindWorkingCopyRequest{VaultPath: root, Provider: "gdrive", Space: "notes-copy"}); err != nil {
		t.Fatalf("bind working copy: %v", err)
	}
	doctor, err := svc.StorageDoctor(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if doctor.Facts["capsa_sync_configured"] != "true" || doctor.Facts["drivebridge_content_mode"] != "provider-plaintext" {
		t.Fatalf("dual-config doctor facts = %#v", doctor.Facts)
	}
	for fact, value := range doctor.Facts {
		if fact == "remote_write" && value == "true" {
			t.Fatalf("storage doctor must never emit Capa remote_write=true: %#v", doctor.Facts)
		}
	}
	vaultDoctor, err := svc.VaultDoctor(ctx, VaultDoctorRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("vault doctor: %v", err)
	}
	if vaultDoctor.Facts["drivebridge_attached"] != "true" || vaultDoctor.Facts["capsa_sync_configured"] != "true" {
		t.Fatalf("vault doctor facts = %#v", vaultDoctor.Facts)
	}
}

func TestStorageDoctorOpaqueEncryptedPrefix(t *testing.T) {
	newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	// Capsa 密文对象落在同一 bucket/prefix：adopt 的前缀是不透明密文，不是笔记。
	if _, err := svc.CapsaBackendSetS3(ctx, CloudBackendSetRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/", WorkspaceID: "ws", DeviceID: "dev", EncryptionSecretRef: "test://enc"}); err != nil {
		t.Fatalf("capsa backend set: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	doctor, err := svc.StorageDoctor(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if doctor.Facts["drivebridge_content_mode"] != "opaque_encrypted" {
		t.Fatalf("expected opaque_encrypted, facts = %#v", doctor.Facts)
	}
	data, _ := doctor.Data.(map[string]any)
	issues, _ := data["issues"].([]domain.Issue)
	found := false
	for _, issue := range issues {
		if issue.Code == "drivebridge_opaque_encrypted" {
			found = true
		}
	}
	if !found {
		t.Fatalf("doctor issues missing opaque_encrypted: %#v", data["issues"])
	}
}

func TestStorageDoctorLocationMatchFact(t *testing.T) {
	newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	doctor, err := svc.StorageDoctor(ctx, VaultRequest{VaultPath: root})
	if err != nil || doctor.Facts["drivebridge_location_match"] != "true" {
		t.Fatalf("matching doctor facts = %#v err=%v", doctor.Facts, err)
	}
	// storage set 改到别的桶后，attach 记录过期：match=false + issue。
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes2", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage again: %v", err)
	}
	doctor, err = svc.StorageDoctor(ctx, VaultRequest{VaultPath: root})
	if err != nil || doctor.Facts["drivebridge_location_match"] != "false" {
		t.Fatalf("stale doctor facts = %#v err=%v", doctor.Facts, err)
	}
}

func TestStorageHydrateS3SecondDevice(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	source := t.TempDir()
	noteBody := "---\nschema_version: pinax.note.v1\ntitle: Remote Note\n---\n\nremote body HYDRATE_MARKER\n"
	notePath := filepath.Join(source, "remote.md")
	writeFile(t, notePath, noteBody)
	attachFile := filepath.Join(source, "storage.json")
	writeFile(t, attachFile, "{}\n")
	fake.setState(t, map[string]any{
		"files": fakeFiles(
			fakeDrivebridgeFile{Space: "pinax-vault", Ref: "pinax-vault:notes/remote.md", Name: "remote.md", Dir: "notes", Version: "v1", SHA256: testFileSHA(t, notePath), Path: notePath},
			fakeDrivebridgeFile{Space: "pinax-vault", Ref: "pinax-vault:.pinax/storage.json", Name: "storage.json", Dir: ".pinax", Version: "v1", SHA256: testFileSHA(t, attachFile), Path: attachFile},
		),
	})
	syncStatePath := filepath.Join(root, ".pinax", "sync-state.json")
	writeFile(t, syncStatePath, "{\"schema_version\":\"pinax.sync_state.v1\",\"revision_id\":\"r0\"}\n")
	syncStateBefore := readFile(t, syncStatePath)
	projection, err := svc.StorageHydrate(ctx, DrivebridgeHydrateRequest{VaultPath: root, Space: "pinax-vault"})
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if projection.Facts["files_downloaded"] != "1" || projection.Facts["protected_skipped"] != "1" {
		t.Fatalf("hydrate facts = %#v", projection.Facts)
	}
	if got := readFile(t, syncStatePath); got != syncStateBefore {
		t.Fatalf("DriveBridge hydrate must not advance Capsa sync-state:\n%s", got)
	}
	if projection.Facts["remote_write"] != "false" {
		t.Fatalf("hydrate must not raise Capa remote_write: %#v", projection.Facts)
	}
	if _, err := os.Stat(filepath.Join(root, "notes", "remote.md")); err != nil {
		t.Fatalf("hydrated note missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "storage.json")); err != nil {
		t.Fatalf("protected .pinax file must not be hydrated over local assets: %v", err)
	}
	if _, err := svc.ValidateVault(ctx, VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("vault validate after hydrate: %v", err)
	}
	if _, err := svc.IndexRefresh(ctx, IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("index refresh after hydrate: %v", err)
	}
	if !strings.Contains(fake.calls(t), "download --ref pinax-vault:notes/remote.md") {
		t.Fatalf("hydrate must transfer through the file plane:\n%s", fake.calls(t))
	}
}

func TestStorageHydrateSkipsNonLocalListingPaths(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	source := t.TempDir()
	notePath := filepath.Join(source, "remote.md")
	writeFile(t, notePath, "---\nschema_version: pinax.note.v1\ntitle: Remote Note\n---\n\nbody\n")
	escapePath := filepath.Join(source, "pwned.md")
	writeFile(t, escapePath, "must never land outside the vault root\n")
	escapeParent := filepath.Dir(root)
	fake.setState(t, map[string]any{
		"files": fakeFiles(
			fakeDrivebridgeFile{Space: "pinax-vault", Ref: "pinax-vault:notes/remote.md", Name: "remote.md", Dir: "notes", Version: "v1", SHA256: testFileSHA(t, notePath), Path: notePath},
			fakeDrivebridgeFile{Space: "pinax-vault", Ref: "pinax-vault:../escape/pwned.md", Name: "pwned.md", Dir: "../escape", Version: "v1", SHA256: testFileSHA(t, escapePath), Path: escapePath},
			fakeDrivebridgeFile{Space: "pinax-vault", Ref: "pinax-vault:/abs/pwned.md", Name: "pwned.md", Dir: "/abs", Version: "v1", SHA256: testFileSHA(t, escapePath), Path: escapePath},
		),
	})
	projection, err := svc.StorageHydrate(ctx, DrivebridgeHydrateRequest{VaultPath: root, Space: "pinax-vault"})
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if projection.Facts["files_downloaded"] != "1" || projection.Facts["unsafe_paths_skipped"] != "2" {
		t.Fatalf("only the in-root file may download, facts = %#v", projection.Facts)
	}
	if _, err := os.Stat(filepath.Join(root, "notes", "remote.md")); err != nil {
		t.Fatalf("legitimate note missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(escapeParent, "escape", "pwned.md")); err == nil {
		t.Fatalf("hydrate must not write outside the vault root")
	}
	if calls := fake.calls(t); strings.Contains(calls, "escape/pwned.md") {
		t.Fatalf("non-local listing entries must be skipped before download:\n%s", calls)
	}
}

func TestStorageHydrateLocalUnreachable(t *testing.T) {
	newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	record := domain.DrivebridgeAttach{
		SchemaVersion: drivebridgeAttachSchemaVersion, Consumer: "pinax", Space: "pinax-vault",
		Kind: "local", Purpose: "vault", Location: domain.DrivebridgeLocation{Root: filepath.Join(t.TempDir(), "missing-dir")},
		ContentMode: drivebridgeContentModeAdoptLocal, AttachedAt: "2026-09-20T00:00:00Z",
	}
	if err := saveDrivebridgeAttach(root, record); err != nil {
		t.Fatalf("save attach record: %v", err)
	}
	_, err := svc.StorageHydrate(ctx, DrivebridgeHydrateRequest{VaultPath: root})
	if !hasCommandCode(err, "drivebridge_local_unreachable") {
		t.Fatalf("expected drivebridge_local_unreachable, got %v", err)
	}
}

func TestStorageHydrateVersionChangedAndConflicts(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	source := t.TempDir()
	changedPath := filepath.Join(source, "changed.md")
	writeFile(t, changedPath, "changed bytes\n")
	conflictPath := filepath.Join(source, "conflict.md")
	writeFile(t, conflictPath, "remote bytes\n")
	localConflict := filepath.Join(root, "notes", "conflict.md")
	writeFile(t, localConflict, "uncommitted local edit LOCAL_EDIT_MARKER\n")
	fake.setState(t, map[string]any{
		"files": fakeFiles(
			fakeDrivebridgeFile{Space: "pinax-vault", Ref: "r:changed.md", Name: "changed.md", Dir: "notes", Version: "v1", SHA256: testFileSHA(t, changedPath), Path: changedPath},
			fakeDrivebridgeFile{Space: "pinax-vault", Ref: "r:conflict.md", Name: "conflict.md", Dir: "notes", Version: "v1", SHA256: testFileSHA(t, conflictPath), Path: conflictPath},
		),
		// 传输后远端相对钉住版本变化：file_version_changed，不留半份副本。
		"stat_bump": map[string]any{"r:changed.md": map[string]any{"version": "v2"}},
	})
	projection, err := svc.StorageHydrate(ctx, DrivebridgeHydrateRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if projection.Facts["files_conflict"] != "1" || projection.Facts["files_failed"] != "1" {
		t.Fatalf("hydrate facts = %#v", projection.Facts)
	}
	if projection.Status != "partial" {
		t.Fatalf("hydrate with conflicts must be partial: %s", projection.Status)
	}
	if got := readFile(t, localConflict); !strings.Contains(got, "LOCAL_EDIT_MARKER") {
		t.Fatalf("uncommitted local edit must be preserved, got %q", got)
	}
	if _, statErr := os.Stat(filepath.Join(root, "notes", "changed.md")); !os.IsNotExist(statErr) {
		t.Fatalf("version-changed file must not be landed from a stale pin")
	}
	handoff := ""
	for _, action := range projection.Actions {
		handoff += action.Command + "\n"
	}
	if !strings.Contains(handoff, "repair plan") {
		t.Fatalf("conflicts must hand off to Pinax repair, actions = %#v", projection.Actions)
	}
}

func TestAssetRegisterAndConsumeDrivebridgeRef(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	attachBytes := filepath.Join(root, "attachments", "report.png")
	writeFile(t, attachBytes, "ATTACHMENT_PAYLOAD_MARKER")
	ref := "drivebridge://pinax-vault/r:report.png@v7"
	fake.setState(t, map[string]any{
		"files": fakeFiles(
			fakeDrivebridgeFile{Space: "pinax-vault", Ref: "r:report.png", Name: "report.png", Dir: "assets", Version: "v7", SHA256: testFileSHA(t, attachBytes), Path: attachBytes},
		),
	})
	// 登记：metadata-only，不传 payload 字节。
	projection, err := svc.AssetAdd(ctx, AssetRequest{VaultPath: root, Source: attachBytes, Register: true, DrivebridgeRef: ref, DrivebridgeSHA256: testFileSHA(t, attachBytes)})
	if err != nil {
		t.Fatalf("asset add: %v", err)
	}
	data, _ := projection.Data.(map[string]any)
	encoded, _ := json.Marshal(data)
	if strings.Contains(string(encoded), "ATTACHMENT_PAYLOAD_MARKER") {
		t.Fatalf("asset add output must not carry payload bytes: %s", encoded)
	}
	manifestRaw := readFile(t, filepath.Join(root, ".pinax", "assets", "manifest.json"))
	var manifest struct {
		Assets []domain.Asset `json:"assets"`
	}
	if err := json.Unmarshal([]byte(manifestRaw), &manifest); err != nil {
		t.Fatalf("manifest invalid: %v", err)
	}
	if len(manifest.Assets) != 1 || manifest.Assets[0].Drivebridge == nil {
		t.Fatalf("manifest asset missing drivebridge ref: %s", manifestRaw)
	}
	pinned := manifest.Assets[0].Drivebridge
	if pinned.FileID != "r:report.png" || pinned.Version != "v7" || pinned.SHA256 != testFileSHA(t, attachBytes) || pinned.Space != "pinax-vault" {
		t.Fatalf("pinned ref = %#v", pinned)
	}
	if manifest.Assets[0].ID == "" || !strings.HasPrefix(manifest.Assets[0].ID, "asset_") {
		t.Fatalf("note/asset identity must stay Pinax-managed: %#v", manifest.Assets[0])
	}
	// 消费成功：本地已落地副本校验通过。
	if _, err := svc.ConsumeDrivebridgeAsset(ctx, AssetRequest{VaultPath: root, Ref: "report.png"}); err != nil {
		t.Fatalf("consume: %v", err)
	}
	// 源文件变化：消费失败 file_version_changed，本地副本不变。
	localCopy := filepath.Join(root, filepath.FromSlash(manifest.Assets[0].Path))
	hashBefore := testFileSHA(t, localCopy)
	fake.setState(t, map[string]any{
		"files": fakeFiles(
			fakeDrivebridgeFile{Space: "pinax-vault", Ref: "r:report.png", Name: "report.png", Dir: "assets", Version: "v8", SHA256: "deadbeef", Path: attachBytes},
		),
	})
	_, err = svc.ConsumeDrivebridgeAsset(ctx, AssetRequest{VaultPath: root, Ref: "report.png"})
	if !hasCommandCode(err, "file_version_changed") {
		t.Fatalf("expected file_version_changed, got %v", err)
	}
	if testFileSHA(t, localCopy) != hashBefore {
		t.Fatalf("landed vault copy must stay unchanged after version change")
	}
	// detach 后新读取失败，已落地副本仍可用。
	if _, err := svc.DetachDrivebridge(ctx, DrivebridgeDetachRequest{VaultPath: root}); err != nil {
		t.Fatalf("detach: %v", err)
	}
	if _, err := svc.ConsumeDrivebridgeAsset(ctx, AssetRequest{VaultPath: root, Ref: "report.png"}); !hasCommandCode(err, "drivebridge_not_attached") {
		t.Fatalf("expected drivebridge_not_attached after detach, got %v", err)
	}
	if _, err := svc.AssetShow(ctx, AssetRequest{VaultPath: root, Ref: "report.png"}); err != nil {
		t.Fatalf("landed asset must remain usable offline: %v", err)
	}
}

func TestAssetConsumeDrivebridgeRefSurvivesIndexRefresh(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	source := filepath.Join(root, "attachments", "report.png")
	writeFile(t, source, "REGRESSION_PAYLOAD")
	fake.setState(t, map[string]any{
		"files": fakeFiles(
			fakeDrivebridgeFile{Space: "pinax-vault", Ref: "r:report.png", Name: "report.png", Dir: "assets", Version: "v7", SHA256: testFileSHA(t, source), Path: source},
		),
	})
	if _, err := svc.AssetAdd(ctx, AssetRequest{VaultPath: root, Source: source, Register: true, DrivebridgeRef: "drivebridge://pinax-vault/r:report.png@v7", DrivebridgeSHA256: testFileSHA(t, source)}); err != nil {
		t.Fatalf("asset add: %v", err)
	}
	// 索引投影没有 drivebridge 列：index refresh 重建资产行后，consume
	// 仍必须能从 manifest 回填 pin 并消费成功。
	if _, err := svc.IndexRefresh(ctx, IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("index refresh: %v", err)
	}
	if _, err := svc.ConsumeDrivebridgeAsset(ctx, AssetRequest{VaultPath: root, Ref: "report.png"}); err != nil {
		t.Fatalf("consume after index refresh: %v", err)
	}
}

func TestInterruptedTransfersLeaveNoPartialCopies(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	source := t.TempDir()
	notePath := filepath.Join(source, "remote.md")
	writeFile(t, notePath, "---\nschema_version: pinax.note.v1\ntitle: Remote Note\n---\n\nbody "+strings.Repeat("x", 512)+"\n")
	noteFiles := fakeFiles(fakeDrivebridgeFile{Space: "pinax-vault", Ref: "pinax-vault:notes/remote.md", Name: "remote.md", Dir: "notes", Version: "v1", SHA256: testFileSHA(t, notePath), Path: notePath})
	// hydrate 第一次：传输中断 → 计入 failed 且不留半截文件。
	fake.setState(t, map[string]any{
		"files":            noteFiles,
		"partial_download": map[string]any{"pinax-vault:notes/remote.md": true},
	})
	projection, err := svc.StorageHydrate(ctx, DrivebridgeHydrateRequest{VaultPath: root, Space: "pinax-vault"})
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if projection.Facts["files_failed"] != "1" {
		t.Fatalf("interrupted transfer must fail, facts = %#v", projection.Facts)
	}
	if _, statErr := os.Stat(filepath.Join(root, "notes", "remote.md")); statErr == nil {
		t.Fatal("interrupted transfer must not leave a partial copy")
	}
	// hydrate 重试（远端健康）：正常落地，不误报本地冲突。
	fake.setState(t, map[string]any{"files": noteFiles})
	projection, err = svc.StorageHydrate(ctx, DrivebridgeHydrateRequest{VaultPath: root, Space: "pinax-vault"})
	if err != nil {
		t.Fatalf("hydrate retry: %v", err)
	}
	if projection.Facts["files_downloaded"] != "1" || projection.Facts["files_conflict"] != "0" {
		t.Fatalf("retry must land the note without a bogus conflict, facts = %#v", projection.Facts)
	}
	// consume：登记后中断 → 不留半截落地副本，重试成功。
	attachSource := filepath.Join(root, "attachments", "report.png")
	fakeSource := filepath.Join(source, "report.png")
	writeFile(t, attachSource, "CONSUME_PARTIAL_PAYLOAD")
	writeFile(t, fakeSource, "CONSUME_PARTIAL_PAYLOAD")
	fake.setState(t, map[string]any{
		"files":            append(noteFiles, fakeFiles(fakeDrivebridgeFile{Space: "pinax-vault", Ref: "r:report.png", Name: "report.png", Dir: "assets", Version: "v7", SHA256: testFileSHA(t, fakeSource), Path: fakeSource})...),
		"partial_download": map[string]any{"r:report.png": true},
	})
	sourceSHA := testFileSHA(t, fakeSource)
	if _, err := svc.AssetAdd(ctx, AssetRequest{VaultPath: root, Source: attachSource, Register: true, DrivebridgeRef: "drivebridge://pinax-vault/r:report.png@v7", DrivebridgeSHA256: sourceSHA}); err != nil {
		t.Fatalf("asset add: %v", err)
	}
	// 登记模式的落地副本就是源文件：删掉它，把 consume 推进下载分支。
	if err := os.Remove(attachSource); err != nil {
		t.Fatalf("remove landed copy: %v", err)
	}
	if _, err := svc.ConsumeDrivebridgeAsset(ctx, AssetRequest{VaultPath: root, Ref: "report.png"}); err == nil {
		t.Fatal("interrupted consume download must fail")
	}
	manifestRaw := readFile(t, filepath.Join(root, ".pinax", "assets", "manifest.json"))
	var manifest struct {
		Assets []domain.Asset `json:"assets"`
	}
	if err := json.Unmarshal([]byte(manifestRaw), &manifest); err != nil {
		t.Fatalf("manifest invalid: %v", err)
	}
	landedPath := filepath.Join(root, filepath.FromSlash(manifest.Assets[0].Path))
	if _, statErr := os.Stat(landedPath); statErr == nil {
		t.Fatal("interrupted consume must not leave a partial landed copy")
	}
	fake.setState(t, map[string]any{
		"files": append(noteFiles, fakeFiles(fakeDrivebridgeFile{Space: "pinax-vault", Ref: "r:report.png", Name: "report.png", Dir: "assets", Version: "v7", SHA256: sourceSHA, Path: fakeSource})...),
	})
	if _, err := svc.ConsumeDrivebridgeAsset(ctx, AssetRequest{VaultPath: root, Ref: "report.png"}); err != nil {
		t.Fatalf("consume retry must land the asset: %v", err)
	}
}

func TestStorageHydrateSkipsUnverifiableListingEntries(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	source := t.TempDir()
	notePath := filepath.Join(source, "remote.md")
	writeFile(t, notePath, "---\nschema_version: pinax.note.v1\ntitle: Remote Note\n---\n\nbody\n")
	// 清单条目缺 sha256：不可校验即不可落地。
	fake.setState(t, map[string]any{
		"files": fakeFiles(fakeDrivebridgeFile{Space: "pinax-vault", Ref: "pinax-vault:notes/remote.md", Name: "remote.md", Dir: "notes", Version: "v1", SHA256: "", Path: notePath}),
	})
	projection, err := svc.StorageHydrate(ctx, DrivebridgeHydrateRequest{VaultPath: root, Space: "pinax-vault"})
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if projection.Facts["unverified_skipped"] != "1" || projection.Facts["files_downloaded"] != "0" {
		t.Fatalf("unverifiable entries must be skipped, facts = %#v", projection.Facts)
	}
	if _, statErr := os.Stat(filepath.Join(root, "notes", "remote.md")); statErr == nil {
		t.Fatal("unverifiable file must not land")
	}
	if calls := fake.calls(t); strings.Contains(calls, "download") {
		t.Fatalf("unverifiable entries must not be downloaded:\n%s", calls)
	}
	warned := false
	for _, warning := range projection.Warnings {
		if warning.Code == "hydrate_unverifiable_file" {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("missing hydrate_unverifiable_file warning: %#v", projection.Warnings)
	}
}

func TestAssetRegisterDerivesDrivebridgeSHA(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	attachSource := filepath.Join(root, "attachments", "report.png")
	fakeSource := filepath.Join(t.TempDir(), "report.png")
	writeFile(t, attachSource, "DERIVED_SHA_PAYLOAD")
	writeFile(t, fakeSource, "DERIVED_SHA_PAYLOAD")
	fake.setState(t, map[string]any{
		"files": fakeFiles(fakeDrivebridgeFile{Space: "pinax-vault", Ref: "r:report.png", Name: "report.png", Dir: "assets", Version: "v7", SHA256: testFileSHA(t, fakeSource), Path: fakeSource}),
	})
	// 不传 --drivebridge-sha256：pin 必须从本地源派生内容 digest。
	if _, err := svc.AssetAdd(ctx, AssetRequest{VaultPath: root, Source: attachSource, Register: true, DrivebridgeRef: "drivebridge://pinax-vault/r:report.png@v7"}); err != nil {
		t.Fatalf("asset add: %v", err)
	}
	manifestRaw := readFile(t, filepath.Join(root, ".pinax", "assets", "manifest.json"))
	var manifest struct {
		Assets []domain.Asset `json:"assets"`
	}
	if err := json.Unmarshal([]byte(manifestRaw), &manifest); err != nil {
		t.Fatalf("manifest invalid: %v", err)
	}
	if manifest.Assets[0].Drivebridge == nil || manifest.Assets[0].Drivebridge.SHA256 != testFileSHA(t, fakeSource) {
		t.Fatalf("derived pin must be content-bound: %#v", manifest.Assets[0].Drivebridge)
	}
	// 派生 pin 真的参与校验：同 version 篡改内容 → file_version_changed。
	tampered := filepath.Join(t.TempDir(), "tampered.png")
	writeFile(t, tampered, "TAMPERED_BYTES")
	fake.setState(t, map[string]any{
		"files": fakeFiles(fakeDrivebridgeFile{Space: "pinax-vault", Ref: "r:report.png", Name: "report.png", Dir: "assets", Version: "v7", SHA256: testFileSHA(t, tampered), Path: tampered}),
	})
	if err := os.Remove(attachSource); err != nil {
		t.Fatalf("remove landed copy: %v", err)
	}
	if _, err := svc.ConsumeDrivebridgeAsset(ctx, AssetRequest{VaultPath: root, Ref: "report.png"}); !hasCommandCode(err, "file_version_changed") {
		t.Fatalf("derived pin must verify content, got %v", err)
	}
}

func TestAttachDifferentSpaceRequiresDetach(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetLocalStorage(ctx, StorageRequest{VaultPath: root, Root: root}); err != nil {
		t.Fatalf("set local storage: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	// 换 space 静默改绑：必须拒绝，旧 adopt 不允许被悄悄泄漏。
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "other-space"}); !hasCommandCode(err, "drivebridge_already_attached") {
		t.Fatalf("expected drivebridge_already_attached, got %v", err)
	}
	record, attached, err := loadDrivebridgeAttach(root)
	if err != nil || !attached || record.Space != "pinax-vault" {
		t.Fatalf("rejected attach must keep the original record: %#v attached=%v err=%v", record, attached, err)
	}
	// 同 space 重挂保持幂等。
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("same-space re-attach must stay idempotent: %v", err)
	}
	// bind-working-copy 换 space 同样拒绝（先确认目标 space 存在，再命中
	// double-attach 守卫，与 space_not_found 的先后语义一致）。
	fake.setState(t, map[string]any{"spaces": []map[string]any{{"id": "other-space", "backend": "onedrive", "local_root": ""}}})
	if _, err := svc.BindWorkingCopy(ctx, DrivebridgeBindWorkingCopyRequest{VaultPath: root, Provider: "onedrive", Space: "other-space"}); !hasCommandCode(err, "drivebridge_already_attached") {
		t.Fatalf("expected drivebridge_already_attached for bind, got %v", err)
	}
}

func TestStorageHydrateNormalizesUppercaseDigests(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	source := t.TempDir()
	notePath := filepath.Join(source, "remote.md")
	writeFile(t, notePath, "---\nschema_version: pinax.note.v1\ntitle: Remote Note\n---\n\nbody\n")
	// 远端清单用大写 hex：大小写不应造成假阳性 file_version_changed。
	fake.setState(t, map[string]any{
		"files": fakeFiles(fakeDrivebridgeFile{Space: "pinax-vault", Ref: "pinax-vault:notes/remote.md", Name: "remote.md", Dir: "notes", Version: "v1", SHA256: strings.ToUpper(testFileSHA(t, notePath)), Path: notePath}),
	})
	projection, err := svc.StorageHydrate(ctx, DrivebridgeHydrateRequest{VaultPath: root, Space: "pinax-vault"})
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if projection.Facts["files_downloaded"] != "1" || projection.Facts["files_failed"] != "0" {
		t.Fatalf("uppercase digests must not fail verification, facts = %#v", projection.Facts)
	}
	if _, err := os.Stat(filepath.Join(root, "notes", "remote.md")); err != nil {
		t.Fatalf("hydrated note missing: %v", err)
	}
}

func TestConsumeDrivebridgeReportsUnreadableAttachRecord(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/"}); err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "pinax-vault"}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	attachSource := filepath.Join(root, "attachments", "report.png")
	fakeSource := filepath.Join(t.TempDir(), "report.png")
	writeFile(t, attachSource, "UNREADABLE_RECORD_PAYLOAD")
	writeFile(t, fakeSource, "UNREADABLE_RECORD_PAYLOAD")
	fake.setState(t, map[string]any{
		"files": fakeFiles(fakeDrivebridgeFile{Space: "pinax-vault", Ref: "r:report.png", Name: "report.png", Dir: "assets", Version: "v7", SHA256: testFileSHA(t, fakeSource), Path: fakeSource}),
	})
	if _, err := svc.AssetAdd(ctx, AssetRequest{VaultPath: root, Source: attachSource, Register: true, DrivebridgeRef: "drivebridge://pinax-vault/r:report.png@v7", DrivebridgeSHA256: testFileSHA(t, fakeSource)}); err != nil {
		t.Fatalf("asset add: %v", err)
	}
	// attach 记录损坏：consume 必须报 unreadable 而不是伪装成未 attach。
	writeFile(t, filepath.Join(root, ".pinax", "drivebridge-attach.yaml"), "{{{{ not-yaml")
	if _, err := svc.ConsumeDrivebridgeAsset(ctx, AssetRequest{VaultPath: root, Ref: "report.png"}); !hasCommandCode(err, "drivebridge_attach_unreadable") {
		t.Fatalf("expected drivebridge_attach_unreadable, got %v", err)
	}
	doctor, err := svc.StorageDoctor(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("storage doctor: %v", err)
	}
	if doctor.Facts["drivebridge_attach_unreadable"] != "true" || doctor.Facts["drivebridge_attached"] != "false" {
		t.Fatalf("doctor must surface the unreadable attach record, facts = %#v", doctor.Facts)
	}
}

func TestParseDrivebridgeRefShapes(t *testing.T) {
	space, fileID, version, err := ParseDrivebridgeRef("drivebridge://pinax-vault/r:note.md@v3")
	if err != nil || space != "pinax-vault" || fileID != "r:note.md" || version != "v3" {
		t.Fatalf("parse = %q %q %q err=%v", space, fileID, version, err)
	}
	for _, bad := range []string{"", "not-a-uri", "drivebridge://space-only", "drivebridge://s/noversion", "s3://s/f@v"} {
		if _, _, _, err := ParseDrivebridgeRef(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
	// 注入形态：组件不得像 flag 或路径，否则会被 DriveBridge CLI 重新
	// 解析成选项或造成路径歧义。
	for _, bad := range []string{
		"drivebridge://--evil/r:note.md@v3",
		"drivebridge://space/--ref@v3",
		"drivebridge://space/r:note.md@-v3",
		"drivebridge://space/../../etc/passwd@v3",
		"drivebridge://space/a/b@v3",
		"drivebridge://./r@v3",
	} {
		if _, _, _, err := ParseDrivebridgeRef(bad); err == nil {
			t.Fatalf("expected injection-shaped ref to be rejected: %q", bad)
		}
	}
}

func TestAttachDrivebridgeRejectsFlagLikeSpace(t *testing.T) {
	fake := newFakeDrivebridge(t)
	ctx := context.Background()
	svc := NewService()
	root := initDrivebridgeVault(t, svc)
	if _, err := svc.SetLocalStorage(ctx, StorageRequest{VaultPath: root, Root: root}); err != nil {
		t.Fatalf("set local storage: %v", err)
	}
	if _, err := svc.AttachDrivebridge(ctx, DrivebridgeAttachRequest{VaultPath: root, Space: "--space=evil"}); !hasCommandCode(err, "invalid_drivebridge_space") {
		t.Fatalf("expected invalid_drivebridge_space, got %v", err)
	}
	if calls := fake.calls(t); strings.Contains(calls, "adopt") {
		t.Fatalf("rejected space must never reach the DriveBridge CLI:\n%s", calls)
	}
}
