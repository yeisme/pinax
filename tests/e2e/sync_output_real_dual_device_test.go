package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

// TestSyncOutputRealDualDeviceRegression runs the pinax.sync.output.v1 view
// through a real object store: convergence, up-to-date, bidirectional edits,
// conflict preservation, and delete markers. It is gated on the same env vars
// as TestIdentityFirstRealSyncSmoke and shares its credential rules.
func TestSyncOutputRealDualDeviceRegression(t *testing.T) {
	t.Parallel()
	endpoint := os.Getenv("PINAX_SYNC_REAL_ENDPOINT")
	if endpoint == "" {
		t.Skip("PINAX_SYNC_REAL_ENDPOINT is required for real dual-device regression")
	}
	workspaceID := os.Getenv("PINAX_SYNC_REAL_WORKSPACE")
	if workspaceID == "" {
		workspaceID = "pinax-sync-output-regression"
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
		if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Sync Output Regression"}); err != nil {
			t.Fatal(err)
		}
	}
	created, err := svc.CreateNote(ctx, app.CreateNoteRequest{VaultPath: deviceA, Title: "Regression Note", Tags: []string{"regression"}, Body: "shared body v1"})
	if err != nil {
		t.Fatal(err)
	}
	noteRel := created.Facts["path"]
	noteID := created.Facts["note_id"]

	loginA := app.CloudLoginRequest{VaultPath: deviceA, Endpoint: endpoint, WorkspaceID: workspaceID, DeviceID: "sync-ux-a", SecretRef: secretRef, EncryptionSecretRef: "plain:test-secret"}
	loginB := app.CloudLoginRequest{VaultPath: deviceB, Endpoint: endpoint, WorkspaceID: workspaceID, DeviceID: "sync-ux-b", SecretRef: secretRef, EncryptionSecretRef: "plain:test-secret"}
	if _, err := svc.CloudLogin(ctx, loginA); err != nil {
		t.Fatal(err)
	}
	promoteManifestV2(t, ctx, svc, deviceA, loginA.DeviceID)

	// Initial push applies through the real transport and reports the shared view.
	pushed, err := svc.SyncPush(ctx, app.SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	assertSyncViewFact(t, pushed, "sync.result", "applied")
	assertSyncViewFact(t, pushed, "sync.scope", "remote-aware")
	assertSyncViewFact(t, pushed, "remote_write", "true")

	if _, err := svc.CloudLogin(ctx, loginB); err != nil {
		t.Fatal(err)
	}
	promoteManifestV2(t, ctx, svc, deviceB, loginB.DeviceID)
	pulled, err := svc.SyncPull(ctx, app.SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	assertSyncViewFact(t, pulled, "sync.result", "applied")
	// The vault template seeds extra starter notes, so the exact added count
	// varies; lock the floor and let the body check prove convergence.
	assertSyncViewFactAtLeast(t, pulled, "sync.added", 1)
	assertNoteBody(t, deviceB, noteRel, "shared body v1")

	// A converged second push reports up_to_date from real manifest comparison.
	converged, err := svc.SyncPush(ctx, app.SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	assertSyncViewFact(t, converged, "up_to_date", "true")
	assertSyncViewFact(t, converged, "sync.result", "up_to_date")

	// A converged second pull reports up_to_date, aligned with push facts.
	convergedPull, err := svc.SyncPull(ctx, app.SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	assertSyncViewFact(t, convergedPull, "up_to_date", "true")
	assertSyncViewFact(t, convergedPull, "sync.result", "up_to_date")
	assertSyncViewFact(t, convergedPull, "files_applied", "0")

	// Bidirectional edit: device B modifies, pushes; device A pulls and converges.
	rewriteNoteBody(t, deviceB, noteRel, "shared body v2 from B")
	pushedB, err := svc.SyncPush(ctx, app.SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	assertSyncViewFact(t, pushedB, "sync.result", "applied")
	pulledA, err := svc.SyncPull(ctx, app.SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	assertSyncViewFact(t, pulledA, "sync.modified", "1")
	assertNoteBody(t, deviceA, noteRel, "shared body v2 from B")
	// A's local matched the base, so the pull must fast-forward without
	// leaving a noise conflict copy.
	if copies := listConflictCopies(t, deviceA, noteRel); len(copies) != 0 {
		t.Fatalf("sequential edit produced noise conflict copies: %v", copies)
	}

	// Divergent edits converge with a preserved conflict copy, not data loss.
	rewriteNoteBody(t, deviceA, noteRel, "device A divergent edit")
	rewriteNoteBody(t, deviceB, noteRel, "device B divergent edit")
	if _, err := svc.SyncPush(ctx, app.SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true}); err != nil {
		t.Fatal(err)
	}
	conflictPull, err := svc.SyncPull(ctx, app.SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if conflictPull.Facts["sync.conflicts"] != "1" {
		t.Fatalf("conflict count fact = %q, want 1 (facts=%v)", conflictPull.Facts["sync.conflicts"], conflictPull.Facts)
	}
	conflictCopies := listConflictCopies(t, deviceB, noteRel)
	if len(conflictCopies) != 1 {
		t.Fatalf("diverged edit must preserve exactly one conflict copy, got %v", conflictCopies)
	}
	for _, copy := range conflictCopies {
		assertFileContains(t, copy, "device B divergent edit")
	}

	// Delete markers converge across devices through the real transport.
	if _, err := svc.DeleteNote(ctx, app.NoteDeleteRequest{VaultPath: deviceA, NoteRef: noteID, Yes: true}); err != nil {
		t.Fatal(err)
	}
	deletePush, err := svc.SyncPush(ctx, app.SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if deletePush.Facts["delete_markers"] != "1" {
		t.Fatalf("delete marker fact = %q, want 1 (facts=%v)", deletePush.Facts["delete_markers"], deletePush.Facts)
	}
	deletePull, err := svc.SyncPull(ctx, app.SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(deviceB, filepath.FromSlash(noteRel))); !os.IsNotExist(statErr) {
		t.Fatalf("deleted note still present on device B after pull (facts=%v)", deletePull.Facts)
	}
}

func assertSyncViewFact(t *testing.T, projection domain.Projection, key, want string) {
	t.Helper()
	if got := projection.Facts[key]; got != want {
		t.Fatalf("fact %s = %q, want %q (facts=%v)", key, got, want, projection.Facts)
	}
	if _, ok := projection.Data.(map[string]any)["sync_view"]; !ok {
		t.Fatalf("projection data missing sync_view for %s assertion (command=%s)", key, projection.Command)
	}
}

func assertSyncViewFactAtLeast(t *testing.T, projection domain.Projection, key string, min int) {
	t.Helper()
	got, err := strconv.Atoi(projection.Facts[key])
	if err != nil || got < min {
		t.Fatalf("fact %s = %q, want >= %d (facts=%v)", key, projection.Facts[key], min, projection.Facts)
	}
}

func assertNoteBody(t *testing.T, root, rel, want string) {
	t.Helper()
	assertFileContains(t, filepath.Join(root, filepath.FromSlash(rel)), want)
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), want) {
		t.Fatalf("%s does not contain %q", filepath.Base(path), want)
	}
}

// rewriteNoteBody replaces the markdown body below the frontmatter divider,
// preserving frontmatter so note identity survives the edit.
func rewriteNoteBody(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(raw), "---\n", 3)
	if len(parts) < 3 {
		t.Fatalf("note %s has no frontmatter divider", rel)
	}
	frontmatter := parts[0] + "---\n" + parts[1] + "---\n"
	if err := os.WriteFile(path, []byte(frontmatter+body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func listConflictCopies(t *testing.T, root, rel string) []string {
	t.Helper()
	dir := filepath.Join(root, filepath.Dir(filepath.FromSlash(rel)))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var copies []string
	base := filepath.Base(rel)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, strings.TrimSuffix(base, ".md")+".") && strings.Contains(name, "conflict") {
			copies = append(copies, filepath.Join(dir, name))
		}
	}
	return copies
}
