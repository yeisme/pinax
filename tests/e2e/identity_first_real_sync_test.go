package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestIdentityFirstRealSyncSmoke(t *testing.T) {
	endpoint := os.Getenv("PINAX_SYNC_REAL_ENDPOINT")
	if endpoint == "" {
		t.Skip("PINAX_SYNC_REAL_ENDPOINT is required for real sync smoke")
	}
	workspaceID := os.Getenv("PINAX_SYNC_REAL_WORKSPACE")
	if workspaceID == "" {
		workspaceID = "pinax-identity-real-smoke"
	}
	secretRef := os.Getenv("PINAX_SYNC_REAL_SECRET_REF")
	if secretRef == "" {
		secretRef = "env:PINAX_SYNC_SECRET"
	}
	ctx := context.Background()
	deviceA := filepath.Join(t.TempDir(), "device-a")
	deviceB := filepath.Join(t.TempDir(), "device-b")
	svc := app.NewService()
	for _, root := range []string{deviceA, deviceB} {
		if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Real Sync Smoke"}); err != nil {
			t.Fatal(err)
		}
	}
	created, err := svc.CreateNote(ctx, app.CreateNoteRequest{VaultPath: deviceA, Title: "Real Sync Object", Tags: []string{"sync-smoke"}, Body: "credential-safe real transport smoke"})
	if err != nil {
		t.Fatal(err)
	}
	objectID := created.Facts["note_id"]
	loginA := app.CloudLoginRequest{VaultPath: deviceA, Endpoint: endpoint, WorkspaceID: workspaceID, DeviceID: "real-smoke-a", SecretRef: secretRef}
	loginB := app.CloudLoginRequest{VaultPath: deviceB, Endpoint: endpoint, WorkspaceID: workspaceID, DeviceID: "real-smoke-b", SecretRef: secretRef}
	if _, err := svc.CloudLogin(ctx, loginA); err != nil {
		t.Fatal(err)
	}
	promoteManifestV2(t, ctx, svc, deviceA, loginA.DeviceID)
	if _, err := svc.SyncPush(ctx, app.SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CloudLogin(ctx, loginB); err != nil {
		t.Fatal(err)
	}
	promoteManifestV2(t, ctx, svc, deviceB, loginB.DeviceID)
	if _, err := svc.SyncPull(ctx, app.SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true}); err != nil {
		t.Fatal(err)
	}
	assertNoteIdentity(t, ctx, svc, deviceB, objectID)
}
