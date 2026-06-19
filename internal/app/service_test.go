package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app/planningops"
	pinaxassets "github.com/yeisme/pinax/internal/assets"
	"github.com/yeisme/pinax/internal/cloudclient"
	"github.com/yeisme/pinax/internal/cloudclient/mlptest"
	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
	pinaxcloud "github.com/yeisme/pinax/internal/remote"
	pinaxversion "github.com/yeisme/pinax/internal/version"
	"github.com/yeisme/pinax/internal/version/versiontest"
)

func TestAttachmentReferenceParserFeedsNoteAttachmentsFromBody(t *testing.T) {
	root := t.TempDir()
	writeAppFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	writeAppFixture(t, filepath.Join(root, "attachments", "My Spec.pdf"), "pdf")
	writeAppFixture(t, filepath.Join(root, "notes", "media", "demo.mp4"), "mp4")
	note := domain.Note{ID: "note_alpha", Path: "notes/note.md", Body: "![Diagram](../assets/diagram.png)\n[Spec](<attachments/My%20Spec.pdf>)\n![[media/demo.mp4|demo]]\n[Plan](project-plan.md)\n[External](https://example.com/a.png)\n![Secret](../../secret.png)\n"}

	attachments := noteAttachmentsFromBody(root, note)

	if len(attachments) != 3 {
		t.Fatalf("expected 3 parsed attachments, got %d: %#v", len(attachments), attachments)
	}
	assertNoteAttachment(t, attachments[0], "notes/note.md", "![Diagram](../assets/diagram.png)", "assets/diagram.png", "image", true)
	assertNoteAttachment(t, attachments[1], "notes/note.md", "[Spec](<attachments/My%20Spec.pdf>)", "attachments/My Spec.pdf", "document", true)
	assertNoteAttachment(t, attachments[2], "notes/note.md", "![[media/demo.mp4|demo]]", "notes/media/demo.mp4", "video", true)
}

func assertNoteAttachment(t *testing.T, attachment domain.NoteAttachment, notePath, reference, targetPath, mediaType string, exists bool) {
	t.Helper()
	if attachment.NotePath != notePath || attachment.ReferenceText != reference || attachment.TargetPath != targetPath || attachment.MediaType != mediaType || attachment.Exists != exists {
		t.Fatalf("unexpected attachment:\n got: %#v\nwant: note=%q reference=%q target=%q media=%q exists=%v", attachment, notePath, reference, targetPath, mediaType, exists)
	}
}

func writeAppFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func recordLedgerSize(t *testing.T, root string) int64 {
	t.Helper()
	info, err := os.Stat(filepath.Join(root, ".pinax", "records", "ledger.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatalf("stat ledger: %v", err)
	}
	return info.Size()
}

func TestVaultInitValidateSearchAndShow(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "我的知识库"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Pinax 推进", Slug: "project", Body: "整理本地知识库。"}); err != nil {
		t.Fatalf("create note: %v", err)
	}

	validation, err := svc.ValidateVault(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("validate vault: %v", err)
	}
	if validation.Facts["notes"] != "1" {
		t.Fatalf("notes fact = %#v", validation.Facts)
	}
	data, ok := validation.Data.(map[string]any)
	if !ok {
		t.Fatalf("validation data = %#v", validation.Data)
	}
	issues, ok := data["issues"].([]domain.Issue)
	if !ok {
		t.Fatalf("validation issues = %#v", data["issues"])
	}
	if len(issues) != 0 {
		t.Fatalf("validation issues = %#v", issues)
	}

	search, err := svc.SearchNotes(ctx, SearchRequest{VaultPath: root, Query: "知识库"})
	if err != nil {
		t.Fatalf("search notes: %v", err)
	}
	if len(search.Notes) != 1 || search.Notes[0].Title != "Pinax 推进" {
		t.Fatalf("search notes = %#v", search.Notes)
	}

	shown, err := svc.ShowNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "project.md"})
	if err != nil {
		t.Fatalf("show note: %v", err)
	}
	if !strings.Contains(shown.Body, "整理本地知识库") {
		t.Fatalf("show body = %q", shown.Body)
	}
}

func TestInitVaultRejectsAlreadyInitializedVault(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()

	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Other"}); !hasCommandCode(err, "vault_already_initialized") {
		t.Fatalf("second init err = %v", err)
	}

	events := readFile(t, filepath.Join(root, ".pinax", "events.jsonl"))
	if got := strings.Count(events, `"type":"vault.init"`); got != 1 {
		t.Fatalf("vault.init events = %d\n%s", got, events)
	}
	config := readFile(t, filepath.Join(root, ".pinax", "config.yaml"))
	if !strings.Contains(config, `title: "Vault"`) || strings.Contains(config, "Other") {
		t.Fatalf("config was rewritten:\n%s", config)
	}
}

func TestNoteProjectionRequiresPinaxNoteFrontmatter(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "raw.md"), "# Raw Markdown\n\nraw-only marker\n")
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Managed", Body: "managed marker"}); err != nil {
		t.Fatalf("create note: %v", err)
	}

	listed, err := svc.ListNotesQuery(ctx, NoteListRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if listed.Facts["total"] != "1" || strings.Contains(fmt.Sprint(listed.Data), "notes/raw.md") {
		t.Fatalf("raw markdown leaked into note list: %#v", listed)
	}

	rawSearch, err := svc.SearchNotes(ctx, SearchRequest{VaultPath: root, Query: "raw-only marker"})
	if err != nil {
		t.Fatalf("search raw marker: %v", err)
	}
	if len(rawSearch.Notes) != 0 {
		t.Fatalf("raw markdown leaked into search: %#v", rawSearch.Notes)
	}

	indexed, err := svc.RebuildIndex(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	if indexed.Facts["notes"] != "1" {
		t.Fatalf("raw markdown leaked into index: %#v", indexed.Facts)
	}

	if _, err := svc.ShowNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "raw.md"}); !hasCommandCode(err, "note_not_found") {
		t.Fatalf("raw markdown resolved as note, err=%v", err)
	}
}

func TestProjectRegistryCreateListAndSwitch(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	created, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", Description: "长期研究", NotesPrefix: "notes/research"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if created.Command != "project.create" || created.Facts["project"] != "research" {
		t.Fatalf("created projection = %#v", created)
	}

	listed, err := svc.ListProjects(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if listed.Facts["projects"] != "1" || listed.Facts["current_project"] != "research" {
		t.Fatalf("list facts = %#v", listed.Facts)
	}

	if _, err := svc.SwitchProject(ctx, ProjectRequest{VaultPath: root, Slug: "research"}); err != nil {
		t.Fatalf("switch project: %v", err)
	}
	asset := readFile(t, filepath.Join(root, ".pinax", "projects.json"))
	var registry map[string]any
	if err := json.Unmarshal([]byte(asset), &registry); err != nil {
		t.Fatalf("projects asset invalid: %v\n%s", err, asset)
	}
	if registry["schema_version"] != "pinax.projects.v1" || registry["current_project"] != "research" {
		t.Fatalf("projects registry = %#v", registry)
	}
}

func TestProjectCreateRejectsUnsafePrefixAndConflicts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "bad", Name: "Bad", NotesPrefix: "../outside"}); err == nil {
		t.Fatalf("unsafe project prefix succeeded")
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", NotesPrefix: "notes/research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "冲突", NotesPrefix: "notes/other"}); err == nil {
		t.Fatalf("conflicting project create succeeded")
	}
}

func TestStorageS3ConfigurationAndDoctor(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	configured, err := svc.SetS3Storage(ctx, StorageRequest{VaultPath: root, Bucket: "notes", Region: "us-east-1", Prefix: "pinax/", Profile: "work"})
	if err != nil {
		t.Fatalf("set s3 storage: %v", err)
	}
	if configured.Facts["backend"] != "s3" || configured.Facts["bucket"] != "notes" {
		t.Fatalf("configured facts = %#v", configured.Facts)
	}
	asset := readFile(t, filepath.Join(root, ".pinax", "storage.json"))
	if strings.Contains(strings.ToLower(asset), "secret") || strings.Contains(strings.ToLower(asset), "access_key") {
		t.Fatalf("storage asset appears to contain secret material:\n%s", asset)
	}

	doctor, err := svc.StorageDoctor(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("storage doctor: %v", err)
	}
	if doctor.Facts["backend"] != "s3" || doctor.Facts["credential_source"] != "profile:work" {
		t.Fatalf("doctor facts = %#v", doctor.Facts)
	}
	data, ok := doctor.Data.(map[string]any)
	if !ok {
		t.Fatalf("doctor data = %#v", doctor.Data)
	}
	profile, ok := data["storage"].(domain.StorageProfile)
	if !ok {
		t.Fatalf("doctor storage data = %#v", data["storage"])
	}
	if profile.Local != nil {
		t.Fatalf("s3 storage doctor retained local profile: %#v", profile.Local)
	}
}

func TestMetadataApplyRequiresApprovalAndWritesFrontmatter(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	note := filepath.Join(root, "notes", "raw.md")
	writeFile(t, note, "---\nschema_version: pinax.note.v1\ntitle: Raw Note\n---\n\n# Raw Note\n\nbody\n")

	plan, err := svc.PlanMetadata(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("plan metadata: %v", err)
	}
	if plan.Facts["planned_updates"] != "1" {
		t.Fatalf("metadata plan facts = %#v", plan.Facts)
	}

	if _, err := svc.ApplyMetadata(ctx, ApplyRequest{VaultPath: root}); err == nil {
		t.Fatalf("metadata apply without approval succeeded")
	}
	if _, err := svc.ApplyMetadata(ctx, ApplyRequest{VaultPath: root, Yes: true}); err != nil {
		t.Fatalf("metadata apply: %v", err)
	}
	content := readFile(t, note)
	for _, want := range []string{"schema_version: pinax.note.v1", "note_id:", "title: Raw Note", "tags: []"} {
		if !strings.Contains(content, want) {
			t.Fatalf("metadata content missing %q:\n%s", want, content)
		}
	}
}

func TestOrganizeApplyRequiresSnapshotAndMovesInsideVault(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	source := filepath.Join(root, "Inbox Note.md")
	writeFile(t, source, "---\nschema_version: pinax.note.v1\nnote_id: note_inbox\ntitle: Inbox Note\ntags: []\n---\n\n# Inbox Note\n\nbody\n")

	plan, err := svc.PlanOrganize(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("plan organize: %v", err)
	}
	if plan.Facts["planned_moves"] != "1" {
		t.Fatalf("organize plan facts = %#v", plan.Facts)
	}

	if _, err := svc.ApplyOrganize(ctx, ApplyRequest{VaultPath: root, Yes: true}); err == nil {
		t.Fatalf("organize apply without snapshot succeeded")
	}
	if _, err := svc.ApplyOrganize(ctx, ApplyRequest{VaultPath: root, Yes: true, SnapshotMessage: "整理前快照"}); err != nil {
		t.Fatalf("organize apply with snapshot: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source still exists or stat error was %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "notes", "inbox-note.md")); err != nil {
		t.Fatalf("organized note missing: %v", err)
	}
}

func TestCoreNoteTemplateIndexAndSyncMVP(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", NotesPrefix: "notes/research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := svc.InitTemplates(ctx, VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("init templates: %v", err)
	}
	rendered, err := svc.RenderTemplate(ctx, TemplateRequest{VaultPath: root, Name: "mermaid", Title: "架构", Project: "research", Tags: []string{"pinax", "sync"}})
	if err != nil {
		t.Fatalf("render template: %v", err)
	}
	if rendered.Facts["template"] != "mermaid" || !strings.Contains(fmt.Sprint(rendered.Data), "```mermaid") {
		t.Fatalf("rendered mermaid projection = %#v", rendered)
	}
	created, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "研究日志", Project: "research", Tags: []string{"pinax", "sync"}, Template: "mermaid"})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if created.Facts["project"] != "research" || created.Facts["tags"] != "pinax,sync" {
		t.Fatalf("created facts = %#v", created.Facts)
	}
	path := filepath.Join(root, created.Facts["path"])
	content := readFile(t, path)
	for _, want := range []string{"schema_version: pinax.note.v1", "title: 研究日志", "tags: [pinax, sync]", "project: research", "```mermaid"} {
		if !strings.Contains(content, want) {
			t.Fatalf("note missing %q:\n%s", want, content)
		}
	}
	writeFile(t, filepath.Join(root, "notes", "research", "source.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_source\ntitle: Source\ntags: [pinax]\n---\n\n# Source\n\n链接 [[研究日志]] #pinax\n")
	indexed, err := svc.RebuildIndex(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	if indexed.Facts["notes"] != "2" || indexed.Facts["links"] != "1" {
		t.Fatalf("index facts = %#v", indexed.Facts)
	}
	search, err := svc.SearchProjection(ctx, SearchRequest{VaultPath: root, Query: "研究日志"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if search.Facts["matches"] == "0" || search.Facts["engine"] == "" {
		t.Fatalf("search facts = %#v", search.Facts)
	}
	syncPlan, err := svc.SyncDiff(ctx, SyncRequest{VaultPath: root, Target: "cloud"})
	if err != nil {
		t.Fatalf("sync diff: %v", err)
	}
	if syncPlan.Facts["backend_required"] != "true" || syncPlan.Facts["target"] != "cloud" {
		t.Fatalf("sync facts = %#v", syncPlan.Facts)
	}
	if _, err := svc.SyncPush(ctx, SyncRequest{VaultPath: root, Target: "cloud"}); err == nil {
		t.Fatalf("sync push without approval succeeded")
	}
}

func TestServerBackedSyncPushRegistersObjectRefMetadataBeforeCommit(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Device A"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "alpha.md"), "# Alpha\n\nserver backed push metadata\n")
	server := mlptest.New(mlptest.Config{VaultID: "personal", SessionToken: "session-token"})
	defer server.Close()
	if _, err := svc.CloudLogin(ctx, CloudLoginRequest{VaultPath: root, Endpoint: server.Endpoint(), WorkspaceID: "personal", DeviceID: "dev_laptop", SecretRef: "plain:session-token"}); err != nil {
		t.Fatalf("cloud login: %v", err)
	}
	push, err := svc.SyncPush(ctx, SyncRequest{VaultPath: root, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatalf("server sync push: %v", err)
	}
	if push.Facts["remote_write"] != "true" {
		t.Fatalf("server push did not commit remotely: facts=%#v data=%#v", push.Facts, push.Data)
	}
}

func TestServerBackedSyncPushUpdatesExistingUploadOnlyMetadata(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Device A"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	body := "# Alpha\n\nexisting blob metadata repair\n"
	writeFile(t, filepath.Join(root, "notes", "alpha.md"), body)
	server := mlptest.New(mlptest.Config{VaultID: "personal", SessionToken: "session-token"})
	defer server.Close()
	if _, err := svc.CloudLogin(ctx, CloudLoginRequest{VaultPath: root, Endpoint: server.Endpoint(), WorkspaceID: "personal", DeviceID: "dev_laptop", SecretRef: "plain:session-token"}); err != nil {
		t.Fatalf("cloud login: %v", err)
	}
	manifest, err := pinaxcloud.BuildManifest(root)
	if err != nil || len(manifest.Entries) != 1 {
		t.Fatalf("build manifest entries=%#v err=%v", manifest.Entries, err)
	}
	client, err := cloudclient.New(cloudclient.Config{Endpoint: server.Endpoint(), VaultID: "personal", DeviceID: "dev_laptop", Token: server.Token()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	transport := cloudclient.NewTransport(client)
	envelope, err := pinaxcloud.EncryptBlob(mustCloudKey(t), []byte(body), []byte(manifest.Entries[0].BlobID))
	if err != nil {
		t.Fatalf("encrypt blob: %v", err)
	}
	if _, _, err := transport.PutBlobWithEnvelopeMetadata(ctx, manifest.Entries[0].BlobID, cloudEnvelope(envelope)); err != nil {
		t.Fatalf("seed existing blob metadata: %v", err)
	}
	push, err := svc.SyncPush(ctx, SyncRequest{VaultPath: root, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatalf("server sync push with stale metadata: %v", err)
	}
	if push.Facts["remote_write"] != "true" {
		t.Fatalf("server push did not commit remotely: facts=%#v data=%#v", push.Facts, push.Data)
	}
}

func TestServerBackedSyncPushAllowsEmptyNoteObjectRef(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Device A"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "empty.md"), "")
	server := mlptest.New(mlptest.Config{VaultID: "personal", SessionToken: "session-token"})
	defer server.Close()
	if _, err := svc.CloudLogin(ctx, CloudLoginRequest{VaultPath: root, Endpoint: server.Endpoint(), WorkspaceID: "personal", DeviceID: "dev_laptop", SecretRef: "plain:session-token"}); err != nil {
		t.Fatalf("cloud login: %v", err)
	}
	push, err := svc.SyncPush(ctx, SyncRequest{VaultPath: root, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatalf("empty note server sync push: %v", err)
	}
	if push.Facts["remote_write"] != "true" {
		t.Fatalf("empty note push did not commit remotely: facts=%#v data=%#v", push.Facts, push.Data)
	}
}

func mustCloudKey(t *testing.T) pinaxcloud.CryptoKey {
	t.Helper()
	key, err := pinaxcloud.DeriveKey("plain:session-token")
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	return key
}

func TestRcloneDirectSyncUsesObjectStoreEngineAndLockRecovery(t *testing.T) {
	ctx := context.Background()
	objectRoot := installAppFakeRclone(t)
	deviceA := t.TempDir()
	deviceB := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: deviceA, Title: "Device A"}); err != nil {
		t.Fatalf("init device A: %v", err)
	}
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: deviceB, Title: "Device B"}); err != nil {
		t.Fatalf("init device B: %v", err)
	}
	writeFile(t, filepath.Join(deviceA, "notes", "alpha.md"), "# Alpha\n\nfrom rclone device A\n")
	if _, err := svc.CloudBackendSetRclone(ctx, CloudBackendSetRequest{VaultPath: deviceA, Remote: "onedrive:PinaxSync", WorkspaceID: "personal", DeviceID: "laptop"}); err != nil {
		t.Fatalf("set rclone A: %v", err)
	}
	lockPath := filepath.Join(objectRoot, "onedrive", "PinaxSync", "workspaces", "personal", "vaults", "personal", "locks", "commit.lock")
	writeFile(t, lockPath, `{"device_id":"other","request_id":"held","expires_at":"2099-01-01T00:00:00Z"}`)
	lockedProjection, err := svc.SyncPush(ctx, SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true})
	if !hasCommandCode(err, "lock_held") {
		t.Fatalf("locked rclone push err = %v", err)
	}
	if strings.Contains(fmt.Sprint(lockedProjection.Data), "remote_write:true") || strings.Contains(fmt.Sprint(lockedProjection.Facts), "remote_write:true") {
		t.Fatalf("locked push claimed remote_write=true: %#v", lockedProjection)
	}
	writeFile(t, lockPath, `{"device_id":"dead","request_id":"stale","expires_at":"2000-01-01T00:00:00Z"}`)
	push, err := svc.SyncPush(ctx, SyncRequest{VaultPath: deviceA, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatalf("rclone push after stale lock: %v", err)
	}
	if push.Facts["remote_write"] != "true" || push.Facts["backend_kind"] != "rclone-direct" || strings.Contains(fmt.Sprint(push.Data), "cloud_api_unimplemented") {
		t.Fatalf("rclone push did not use direct engine: facts=%#v data=%#v", push.Facts, push.Data)
	}
	if _, err := svc.CloudBackendSetRclone(ctx, CloudBackendSetRequest{VaultPath: deviceB, Remote: "onedrive:PinaxSync", WorkspaceID: "personal", DeviceID: "desktop"}); err != nil {
		t.Fatalf("set rclone B: %v", err)
	}
	pull, err := svc.SyncPull(ctx, SyncRequest{VaultPath: deviceB, Target: "cloud", Yes: true})
	if err != nil {
		t.Fatalf("rclone pull: %v", err)
	}
	if pull.Facts["files_applied"] != "1" || pull.Facts["backend_kind"] != "rclone-direct" {
		t.Fatalf("rclone pull facts = %#v", pull.Facts)
	}
	if got := readFile(t, filepath.Join(deviceB, "notes", "alpha.md")); !strings.Contains(got, "from rclone device A") {
		t.Fatalf("pulled rclone note missing body:\n%s", got)
	}
}

func TestCreateNoteRejectsUnsafeTagsBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	unsafeTags := []string{"bad]", "status:archived", "line\nbreak", "comma,split", string([]byte{0x1f})}
	for _, tag := range unsafeTags {
		if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Unsafe", Tags: []string{tag}}); !hasCommandCode(err, "invalid_tag") {
			t.Fatalf("tag %q should fail with invalid_tag, got %v", tag, err)
		}
	}
	for _, rel := range []string{"Unsafe.md", filepath.Join(".pinax", "index.sqlite"), filepath.Join(".pinax", "records", "ledger.jsonl")} {
		if fileExistsApp(filepath.Join(root, rel)) {
			t.Fatalf("unsafe tag created %s", rel)
		}
	}
}

func TestTagNoteRejectsUnsafeTagsBeforeMutation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	created, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Safe", Tags: []string{"safe"}})
	if err != nil {
		t.Fatalf("create safe note: %v", err)
	}
	path := filepath.Join(root, created.Facts["path"])
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	beforeLedger := recordLedgerSize(t, root)

	unsafeTags := []string{"bad]", "status:archived", "line\nbreak", "comma,split", string([]byte{0x1f})}
	for _, operation := range []string{"add", "set"} {
		for _, tag := range unsafeTags {
			if _, err := svc.TagNote(ctx, NoteTagRequest{VaultPath: root, NoteRef: "Safe", Operation: operation, Tags: []string{tag}}); !hasCommandCode(err, "invalid_tag") {
				t.Fatalf("operation %s tag %q should fail with invalid_tag, got %v", operation, tag, err)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != string(before) {
				t.Fatalf("unsafe tag mutation changed note for %s/%q: err=%v\n%s", operation, tag, err, string(got))
			}
			if got := recordLedgerSize(t, root); got != beforeLedger {
				t.Fatalf("unsafe tag mutation changed ledger size: got %d want %d", got, beforeLedger)
			}
		}
	}
}

func TestTagNoteWritesRecordAndRefreshesIndexFacts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	created, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Taggable", Tags: []string{"safe"}})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}

	projection, err := svc.TagNote(ctx, NoteTagRequest{VaultPath: root, NoteRef: created.Facts["path"], Operation: "add", Tags: []string{"research"}})
	if err != nil {
		t.Fatalf("tag note: %v", err)
	}
	for key, want := range map[string]string{
		"record_event":   string(domain.RecordEventNoteMetadataUpdated),
		"ledger_seq":     "2",
		"index_updated":  "true",
		"ledger_status":  "updated",
		"record_version": "2",
	} {
		if projection.Facts[key] != want {
			t.Fatalf("fact %s = %q, want %q; facts=%#v", key, projection.Facts[key], want, projection.Facts)
		}
	}
	if !fileExistsApp(filepath.Join(root, ".pinax", "index.sqlite")) {
		t.Fatalf("tag note did not refresh index")
	}
}

func TestImportMarkdownRejectsUnsafeDefaultTagsBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	source := t.TempDir()
	writeAppFixture(t, filepath.Join(source, "import.md"), "# Import\n\nbody")
	svc := NewService()
	if _, err := svc.ImportMarkdown(ctx, ImportMarkdownRequest{VaultPath: root, Source: source, Tags: []string{"bad]"}, Yes: true}); !hasCommandCode(err, "invalid_tag") {
		t.Fatalf("unsafe import tag should fail with invalid_tag, got %v", err)
	}
	for _, rel := range []string{"import.md", filepath.Join(".pinax", "index.sqlite"), filepath.Join(".pinax", "events.jsonl")} {
		if fileExistsApp(filepath.Join(root, rel)) {
			t.Fatalf("unsafe import tag created %s", rel)
		}
	}
}

func TestVaultStatsAndDoctorServices(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "active.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_active\ntitle: Active\ntags: [pinax, cli]\ncreated_at: 2026-01-01T00:00:00Z\nupdated_at: 2026-01-02T00:00:00Z\n---\n\n# Active\n\n链接 [[Missing]] #cli\n")
	writeFile(t, filepath.Join(root, "notes", "duplicate.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_duplicate\ntitle: Active\ntags: []\n---\n\n# Active\n\n")
	writeFile(t, filepath.Join(root, "notes", "raw.md"), "# Raw\n\n")
	writeFile(t, filepath.Join(root, "notes", "empty.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_empty\ntitle: Empty\ntags: [inbox]\n---\n\n")
	old := time.Now().Add(-120 * 24 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, "notes", "raw.md"), old, old); err != nil {
		t.Fatalf("chtimes raw note: %v", err)
	}

	stats, err := svc.VaultStats(ctx, VaultStatsRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("vault stats: %v", err)
	}
	if stats.Command != "vault.stats" || stats.Facts["notes"] != "3" || stats.Facts["index_status"] != "missing" {
		t.Fatalf("stats projection = %#v", stats)
	}
	statsData, ok := stats.Data.(domain.VaultStats)
	if !ok {
		t.Fatalf("stats data = %#v", stats.Data)
	}
	if statsData.TagCount != 3 || statsData.FrontmatterCoverage != 100 {
		t.Fatalf("stats data = %#v", statsData)
	}

	doctor, err := svc.VaultDoctor(ctx, VaultDoctorRequest{VaultPath: root, StaleAfter: 90 * 24 * time.Hour})
	if err != nil {
		t.Fatalf("vault doctor: %v", err)
	}
	if doctor.Command != "vault.doctor" || doctor.Status != "partial" {
		t.Fatalf("doctor projection = %#v", doctor)
	}
	doctorData, ok := doctor.Data.(domain.VaultDoctorReport)
	if !ok {
		t.Fatalf("doctor data = %#v", doctor.Data)
	}
	for _, want := range []string{"missing_tags", "duplicate_title", "empty_note", "orphan_note", "index_stale"} {
		if !hasVaultIssue(doctorData.Issues, want) {
			t.Fatalf("doctor missing issue %q: %#v", want, doctorData.Issues)
		}
	}
	for _, notWant := range []string{"missing_pinax_metadata", "stale_note"} {
		if hasVaultIssue(doctorData.Issues, notWant) {
			t.Fatalf("doctor reported ignored raw markdown issue %q: %#v", notWant, doctorData.Issues)
		}
	}
}

func hasVaultIssue(issues []domain.VaultIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestNoteUXServiceResolverListCreateAndMutate(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "work", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\ntags: [research]\nproject: work\nstatus: active\n---\n\n# Alpha\n")
	writeFile(t, filepath.Join(root, "notes", "work", "meeting-a.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_meeting_a\ntitle: Meeting\ntags: []\n---\n\n# Meeting\n")
	writeFile(t, filepath.Join(root, "notes", "work", "meeting-b.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_meeting_b\ntitle: Meeting\ntags: []\n---\n\n# Meeting\n")

	resolved, err := svc.ResolveNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "Alpha"})
	if err != nil || resolved.Path != "notes/work/alpha.md" || resolved.ID != "note_alpha" {
		t.Fatalf("resolve title note=%#v err=%v", resolved, err)
	}
	_, err = svc.ResolveNote(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "Meeting"})
	if !hasCommandCode(err, "note_ref_ambiguous") {
		t.Fatalf("ambiguous err = %v", err)
	}

	listed, err := svc.ListNotesQuery(ctx, NoteListRequest{VaultPath: root, Tags: []string{"research"}, Project: "work", Status: "active", Recent: true, Limit: 10})
	if err != nil {
		t.Fatalf("list query: %v", err)
	}
	if listed.Facts["total"] != "1" || !strings.Contains(fmt.Sprint(listed.Data), "notes/work/alpha.md") {
		t.Fatalf("listed projection = %#v", listed)
	}

	dryRun, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Draft", Body: "body", Dir: "work", Slug: "draft", Status: "draft", DryRun: true})
	if err != nil {
		t.Fatalf("dry run create: %v", err)
	}
	if dryRun.Command != "note.new" || dryRun.Facts["planned_path"] != "notes/work/draft.md" || fileExistsApp(filepath.Join(root, "notes", "work", "draft.md")) {
		t.Fatalf("dry run projection = %#v", dryRun)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Bad", Body: "a", SourcePath: filepath.Join(root, "missing.md")}); !hasCommandCode(err, "note_source_conflict") {
		t.Fatalf("source conflict err = %v", err)
	}
	created, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Created", Body: "body", Dir: "work", Slug: "created", Status: "active", Tags: []string{"inbox"}})
	if err != nil || created.Facts["path"] != "notes/work/created.md" {
		t.Fatalf("create projection=%#v err=%v", created, err)
	}

	renamed, err := svc.RenameNote(ctx, NoteMutationRequest{VaultPath: root, NoteRef: "Created", Title: "Created Renamed"})
	if err != nil || renamed.Facts["path"] != "notes/work/created-renamed.md" {
		t.Fatalf("rename projection=%#v err=%v", renamed, err)
	}
	moved, err := svc.MoveNote(ctx, NoteMutationRequest{VaultPath: root, NoteRef: "Created Renamed", TargetDir: "archive"})
	if err != nil || moved.Facts["path"] != "notes/archive/created-renamed.md" {
		t.Fatalf("move projection=%#v err=%v", moved, err)
	}
	archived, err := svc.ArchiveNote(ctx, NoteMutationRequest{VaultPath: root, NoteRef: "Created Renamed"})
	if err != nil || archived.Facts["status"] != "archived" {
		t.Fatalf("archive projection=%#v err=%v", archived, err)
	}
	tagged, err := svc.TagNote(ctx, NoteTagRequest{VaultPath: root, NoteRef: "Created Renamed", Operation: "add", Tags: []string{"important"}})
	if err != nil || !strings.Contains(fmt.Sprint(tagged.Data), "important") {
		t.Fatalf("tag projection=%#v err=%v", tagged, err)
	}
	if _, err := svc.DeleteNote(ctx, NoteDeleteRequest{VaultPath: root, NoteRef: "Created Renamed", Hard: true}); !hasCommandCode(err, "approval_required") {
		t.Fatalf("hard delete without yes err = %v", err)
	}
	deleted, err := svc.DeleteNote(ctx, NoteDeleteRequest{VaultPath: root, NoteRef: "Created Renamed", Yes: true})
	if err != nil || deleted.Facts["trash_path"] == "" {
		t.Fatalf("delete projection=%#v err=%v", deleted, err)
	}
}

func TestNoteHardeningEditorCommandParser(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		executable string
		args       []string
	}{
		{name: "code wait", input: "code --wait", executable: "code", args: []string{"--wait"}},
		{name: "vim no swap", input: "vim -n", executable: "vim", args: []string{"-n"}},
		{name: "quoted arg", input: `fake-editor --label "work notes"`, executable: "fake-editor", args: []string{"--label", "work notes"}},
		{name: "shell metachar token", input: `fake-editor --literal ';rm -rf /'`, executable: "fake-editor", args: []string{"--literal", ";rm -rf /"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := parseEditorCommand(tt.input)
			if err != nil {
				t.Fatalf("parse editor: %v", err)
			}
			if cmd.Executable != tt.executable || strings.Join(cmd.Args, "\x00") != strings.Join(tt.args, "\x00") {
				t.Fatalf("command = %#v", cmd)
			}
		})
	}
	if _, err := parseEditorCommand("   "); !hasCommandCode(err, "editor_not_configured") {
		t.Fatalf("empty editor err = %v", err)
	}
}

func TestNoteHardeningCommitKeepsOriginalWhenPrepareFails(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "notes", "current.md")
	writeFile(t, current, "original")
	blocker := filepath.Join(root, "blocked")
	writeFile(t, blocker, "not a directory")
	if err := commitNoteContent(current, filepath.Join(blocker, "target.md"), "new content"); err == nil {
		t.Fatalf("commit unexpectedly succeeded")
	}
	if got := readFile(t, current); got != "original" {
		t.Fatalf("original changed after failed prepare: %q", got)
	}
}

func TestNoteHardeningTrashAndRecentSemantics(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	notePath := filepath.Join(root, "notes", "work", "note.md")
	writeFile(t, notePath, "---\nschema_version: pinax.note.v1\nnote_id: note_work\ntitle: Work Note\ntags: [work]\nstatus: active\nupdated_at: 2026-01-02T00:00:00Z\n---\n\n# Work Note\n")
	writeFile(t, filepath.Join(root, "notes", "old.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_old\ntitle: Old Note\ntags: []\n---\n\n# Old Note\n")

	trashDate := time.Now().UTC().Format("20060102")
	existingTrash := filepath.Join(root, ".pinax", "trash", trashDate, "work", "note.md")
	writeFile(t, existingTrash, "existing trash")
	deleted, err := svc.DeleteNote(ctx, NoteDeleteRequest{VaultPath: root, NoteRef: "Work Note", Yes: true})
	if err != nil {
		t.Fatalf("delete note: %v", err)
	}
	if deleted.Facts["trash_path"] == ".pinax/trash/"+trashDate+"/work/note.md" {
		t.Fatalf("trash path overwrote existing target: %#v", deleted.Facts)
	}
	if got := readFile(t, existingTrash); got != "existing trash" {
		t.Fatalf("existing trash was overwritten: %q", got)
	}
	if !strings.Contains(deleted.Facts["trash_path"], "note-2.md") {
		t.Fatalf("trash path should use numeric suffix: %#v", deleted.Facts)
	}

	listed, err := svc.ListNotesQuery(ctx, NoteListRequest{VaultPath: root, Recent: true})
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if listed.Facts["recent"] != "true" || listed.Facts["sort"] != "updated" || listed.Facts["total"] != "1" {
		t.Fatalf("recent facts = %#v", listed.Facts)
	}
	hardPath := filepath.Join(root, "notes", "hard.md")
	writeFile(t, hardPath, "---\nschema_version: pinax.note.v1\nnote_id: note_hard\ntitle: Hard\ntags: []\n---\n\n# Hard\n")
	hardDeleted, err := svc.DeleteNote(ctx, NoteDeleteRequest{VaultPath: root, NoteRef: "Hard", Yes: true, Hard: true})
	if err != nil {
		t.Fatalf("hard delete: %v", err)
	}
	if hardDeleted.Facts["hard"] != "true" || hardDeleted.Facts["trash_path"] != "" || fileExistsApp(hardPath) {
		t.Fatalf("hard delete projection/path = %#v exists=%v", hardDeleted.Facts, fileExistsApp(hardPath))
	}
}

func TestNoteHardeningFrontmatterPatchPreservesUserMetadata(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	path := filepath.Join(root, "notes", "annotated.md")
	writeFile(t, path, "---\n# user comment\nschema_version: pinax.note.v1\nnote_id: note_annotated\ntitle: Annotated\nowner: alice\ntags: [inbox]\n---\n\n# Annotated\n\nbody\n")
	if _, err := svc.TagNote(ctx, NoteTagRequest{VaultPath: root, NoteRef: "Annotated", Operation: "add", Tags: []string{"research"}}); err != nil {
		t.Fatalf("tag note: %v", err)
	}
	content := readFile(t, path)
	for _, want := range []string{"# user comment", "owner: alice", "tags: [inbox, research]", "# Annotated\n\nbody"} {
		if !strings.Contains(content, want) {
			t.Fatalf("patched content missing %q:\n%s", want, content)
		}
	}
	if _, err := svc.ArchiveNote(ctx, NoteMutationRequest{VaultPath: root, NoteRef: "Annotated"}); err != nil {
		t.Fatalf("archive note: %v", err)
	}
	archived := readFile(t, path)
	for _, want := range []string{"# user comment", "owner: alice", "status: archived", "# Annotated\n\nbody"} {
		if !strings.Contains(archived, want) {
			t.Fatalf("archived content missing %q:\n%s", want, archived)
		}
	}
}

func TestNoteHardeningPatchFrontmatterFields(t *testing.T) {
	content := "---\n# keep me\ntitle: Old\nowner: alice\ntags: [inbox]\n---\n\n# Old\n"
	patched, normalized := patchFrontmatterFields(content, map[string]string{"title": "New", "tags": "[inbox, research]"})
	if normalized {
		t.Fatalf("ordinary frontmatter patch should not normalize")
	}
	for _, want := range []string{"# keep me", "title: New", "owner: alice", "tags: [inbox, research]", "# Old"} {
		if !strings.Contains(patched, want) {
			t.Fatalf("patched content missing %q:\n%s", want, patched)
		}
	}
}

func fileExistsApp(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestSavedViewRegistryV2Compatibility(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pinax"), 0o755); err != nil {
		t.Fatalf("mkdir .pinax: %v", err)
	}
	writeFile(t, filepath.Join(root, ".pinax", "views.json"), `{
  "schema_version": "pinax.views.v2",
  "views": [{
    "id": "view_active",
    "name": "active",
    "kind": "table",
    "query": "SELECT title FROM notes LIMIT 20",
    "columns": ["title", "status"],
    "filters": {"status": "active"},
    "sorts": ["updated_at desc"],
    "limit": 20,
    "display": {"density": "compact"}
  }]
}`)

	registry, err := loadSavedViews(root)
	if err != nil {
		t.Fatalf("load saved views: %v", err)
	}
	if registry.SchemaVersion != "pinax.views.v2" || len(registry.Views) != 1 {
		t.Fatalf("registry = %#v", registry)
	}
	view := registry.Views[0]
	if view.ID != "view_active" || view.Query != "SELECT title FROM notes LIMIT 20" || len(view.Columns) != 2 || view.Filters["status"] != "active" || view.Sorts[0] != "updated_at desc" || view.Display["density"] != "compact" {
		t.Fatalf("v2 view not preserved: %#v", view)
	}
}

func TestSearchLazyIndexRebuildsMissingIndex(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Lazy Note", Body: "searchable body", Status: "active"}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	if err := os.Remove(filepath.Join(root, ".pinax", "index.sqlite")); err != nil {
		t.Fatalf("remove index: %v", err)
	}

	projection, err := svc.SearchProjection(ctx, SearchRequest{VaultPath: root, Query: "searchable"})
	if err != nil {
		t.Fatalf("search projection: %v", err)
	}
	if projection.Facts["engine"] != "index" || projection.Facts["index_status"] != "fresh" || projection.Facts["index_loaded"] != "lazy_rebuild" || projection.Facts["returned"] != "1" {
		t.Fatalf("search facts = %#v", projection.Facts)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "index.sqlite")); err != nil {
		t.Fatalf("index not rebuilt: %v", err)
	}
}

func TestTemplateAuthoringCreateRenderValidateDelete(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	design, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "视频学习"})
	if err != nil {
		t.Fatalf("create template design: %v", err)
	}
	if design.Facts["template"] != "视频学习" || design.Facts["kind"] != "template_design" {
		t.Fatalf("design projection = %#v", design)
	}
	designContent := readFile(t, filepath.Join(root, ".pinax", "templates", "视频学习.md"))
	for _, want := range []string{"---\n", "schema_version: pinax.template_design.v1", "kind: template_design", "title: 视频学习", "## Template Body"} {
		if !strings.Contains(designContent, want) {
			t.Fatalf("template design missing %q:\n%s", want, designContent)
		}
	}

	created, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "meeting", Body: "# {{title}}\n客户: {{client}}\n负责人: {{owner}}\n"})
	if err != nil {
		t.Fatalf("create template body: %v", err)
	}
	if created.Command != "template.create" || created.Facts["template"] != "meeting" {
		t.Fatalf("created projection = %#v", created)
	}
	content := readFile(t, filepath.Join(root, ".pinax", "templates", "meeting.md"))
	if !strings.Contains(content, "客户: {{client}}") {
		t.Fatalf("template content = %q", content)
	}

	source := filepath.Join(root, "source-template.md")
	writeFile(t, source, "# {{title}}\n\n```mermaid\nflowchart TD\n  A --> B\n```\n")
	if _, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "diagram", SourcePath: source}); err != nil {
		t.Fatalf("create template file: %v", err)
	}

	rendered, err := svc.RenderTemplate(ctx, TemplateRequest{VaultPath: root, Name: "meeting", Title: "客户会议", Vars: map[string]string{"client": "Acme", "owner": "张三"}})
	if err != nil {
		t.Fatalf("render template vars: %v", err)
	}
	if body := fmt.Sprint(rendered.Data); !strings.Contains(body, "客户: Acme") || !strings.Contains(body, "负责人: 张三") {
		t.Fatalf("rendered body = %#v", rendered.Data)
	}

	if _, err := svc.RenderTemplate(ctx, TemplateRequest{VaultPath: root, Name: "meeting", Title: "客户会议"}); !hasCommandCode(err, "template_variable_missing") {
		t.Fatalf("missing var err = %v", err)
	}

	valid, err := svc.ValidateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "diagram"})
	if err != nil {
		t.Fatalf("validate diagram: %v", err)
	}
	if valid.Facts["issues"] != "0" || valid.Facts["template"] != "diagram" {
		t.Fatalf("valid facts = %#v", valid.Facts)
	}

	writeFile(t, filepath.Join(root, ".pinax", "templates", "bad.md"), "# Bad\n\n```mermaid\nflowchart TD\n")
	bad, err := svc.ValidateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "bad"})
	if err != nil {
		t.Fatalf("validate bad should report partial projection, got err: %v", err)
	}
	if bad.Status != "partial" || bad.Facts["issues"] == "0" || !strings.Contains(fmt.Sprint(bad.Data), "template_fence_unclosed") {
		t.Fatalf("bad validation = %#v", bad)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "坏模板", Template: "bad"}); !hasCommandCode(err, "template_invalid") {
		t.Fatalf("bad template note err = %v", err)
	}

	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "客户会议", Template: "meeting", Tags: []string{"meeting"}, Vars: map[string]string{"client": "Acme", "owner": "张三"}}); err != nil {
		t.Fatalf("create note from custom template: %v", err)
	}
	note := readFile(t, filepath.Join(root, "客户会议.md"))
	if !strings.Contains(note, "客户: Acme") || !strings.Contains(note, "负责人: 张三") {
		t.Fatalf("note from template = %s", note)
	}

	if _, err := svc.DeleteTemplate(ctx, TemplateRequest{VaultPath: root, Name: "meeting"}); !hasCommandCode(err, "approval_required") {
		t.Fatalf("delete without approval err = %v", err)
	}
	if _, err := svc.DeleteTemplate(ctx, TemplateRequest{VaultPath: root, Name: "meeting", Yes: true}); err != nil {
		t.Fatalf("delete template: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "templates", "meeting.md")); !os.IsNotExist(err) {
		t.Fatalf("template still exists or stat error = %v", err)
	}
	if _, err := svc.DeleteTemplate(ctx, TemplateRequest{VaultPath: root, Name: "mermaid", Yes: true}); !hasCommandCode(err, "builtin_template_protected") {
		t.Fatalf("delete builtin err = %v", err)
	}
}

func TestTemplateDesignDraftIsNotExecutable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "draft"}); err != nil {
		t.Fatalf("create design template: %v", err)
	}

	checks := []struct {
		name string
		run  func() (domain.Projection, error)
	}{
		{name: "preview", run: func() (domain.Projection, error) {
			return svc.PreviewTemplate(ctx, TemplateRequest{VaultPath: root, Name: "draft"})
		}},
		{name: "render", run: func() (domain.Projection, error) {
			return svc.RenderTemplate(ctx, TemplateRequest{VaultPath: root, Name: "draft"})
		}},
		{name: "note", run: func() (domain.Projection, error) {
			return svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Draft Note", Template: "draft"})
		}},
	}
	for _, check := range checks {
		if _, err := check.run(); !hasCommandCode(err, "template_design_not_executable") {
			t.Fatalf("%s should fail with template_design_not_executable, got %v", check.name, err)
		}
	}
	for _, rel := range []string{"Draft Note.md", filepath.Join(".pinax", "index.sqlite"), filepath.Join(".pinax", "records", "ledger.jsonl")} {
		if fileExistsApp(filepath.Join(root, rel)) {
			t.Fatalf("design execution wrote %s", rel)
		}
	}
}

func TestTemplateAuthoringGoTemplateIntegration(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	body := strings.Join([]string{
		"---",
		"schema_version: pinax.template.v2",
		"engine: go-template",
		"kind: note",
		"defaults:",
		"  owner: Pinax",
		"variables:",
		"  client:",
		"    required: true",
		"---",
		"# {{ .Title | upper }}",
		"客户: {{ .Vars.client }}",
		"负责人: {{ .Vars.owner }}",
		"{{ range .Tags }}- {{ . }}{{ end }}",
	}, "\n")
	if _, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "go-meeting", Body: body}); err != nil {
		t.Fatalf("create go template: %v", err)
	}

	rendered, err := svc.RenderTemplate(ctx, TemplateRequest{VaultPath: root, Name: "go-meeting", Title: "weekly", Tags: []string{"pinax", "sync"}, Vars: map[string]string{"client": "Acme"}})
	if err != nil {
		t.Fatalf("render go template: %v", err)
	}
	if got := fmt.Sprint(rendered.Data); !strings.Contains(got, "# WEEKLY") || !strings.Contains(got, "客户: Acme") || !strings.Contains(got, "负责人: Pinax") || !strings.Contains(got, "- pinax") {
		t.Fatalf("rendered data = %#v", rendered.Data)
	}

	if _, err := svc.RenderTemplate(ctx, TemplateRequest{VaultPath: root, Name: "go-meeting", Title: "weekly"}); !hasCommandCode(err, "template_variable_missing") {
		t.Fatalf("missing go template var err = %v", err)
	}

	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "weekly", Template: "go-meeting", Vars: map[string]string{"client": "Acme"}}); err != nil {
		t.Fatalf("create note from go template: %v", err)
	}
	note := readFile(t, filepath.Join(root, "weekly.md"))
	if !strings.Contains(note, "# WEEKLY") || !strings.Contains(note, "客户: Acme") || !strings.Contains(note, "负责人: Pinax") {
		t.Fatalf("note from go template = %s", note)
	}
}

func TestTemplatePreviewUsesExampleContextAndExplicitOverrides(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	body := strings.Join([]string{
		"---",
		"schema_version: pinax.template.v2",
		"kind: note_template",
		"engine: go-template",
		"example:",
		"  title: Example Title",
		"  project: example-project",
		"  tags: [example, pinax]",
		"  vars:",
		"    client: Acme",
		"---",
		"# {{ .Title }}",
		"project={{ .Project }}",
		"tags={{ .Tags }}",
		"client={{ .Vars.client }}",
	}, "\n")
	if _, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "example-note", Body: body}); err != nil {
		t.Fatalf("create template: %v", err)
	}
	preview, err := svc.PreviewTemplate(ctx, TemplateRequest{VaultPath: root, Name: "example-note"})
	if err != nil {
		t.Fatalf("preview example: %v", err)
	}
	previewBody := fmt.Sprint(preview.Data)
	for _, want := range []string{"# Example Title", "project=example-project", "tags=[example pinax]", "client=Acme"} {
		if !strings.Contains(previewBody, want) {
			t.Fatalf("preview missing example %q:\n%s", want, previewBody)
		}
	}
	override, err := svc.PreviewTemplate(ctx, TemplateRequest{VaultPath: root, Name: "example-note", Title: "Override", Project: "work", Tags: []string{"custom"}, Vars: map[string]string{"client": "Beta"}})
	if err != nil {
		t.Fatalf("preview override: %v", err)
	}
	overrideBody := fmt.Sprint(override.Data)
	for _, want := range []string{"# Override", "project=work", "tags=[custom]", "client=Beta"} {
		if !strings.Contains(overrideBody, want) {
			t.Fatalf("preview missing override %q:\n%s", want, overrideBody)
		}
	}
}

func TestTemplateMissingVariableIncludesSafeAction(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	body := strings.Join([]string{
		"---",
		"schema_version: pinax.template.v2",
		"engine: go-template",
		"variables:",
		"  url:",
		"    required: true",
		"---",
		"link={{ .Vars.url }}",
	}, "\n")
	if _, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "needs-url", Body: body}); err != nil {
		t.Fatalf("create template: %v", err)
	}
	projection, err := svc.RenderTemplate(ctx, TemplateRequest{VaultPath: root, Name: "needs-url", Vars: map[string]string{"secret": "secret-token"}})
	if !hasCommandCode(err, "template_variable_missing") {
		t.Fatalf("missing variable should fail with template_variable_missing: projection=%#v err=%v", projection, err)
	}
	if len(projection.Actions) == 0 || !strings.Contains(projection.Actions[0].Command, "--var url=...") || strings.Contains(projection.Actions[0].Command, "secret-token") {
		t.Fatalf("missing variable action = %#v", projection.Actions)
	}
}

func TestTemplateQueryBackedPreviewInspectAndCreate(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "A", Body: "priority:: 1\n", Status: "active"}); err != nil {
		t.Fatalf("create active note: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "B", Body: "priority:: 2\n", Status: "done"}); err != nil {
		t.Fatalf("create done note: %v", err)
	}

	body := strings.Join([]string{
		"---",
		"schema_version: pinax.template.v2",
		"engine: go-template",
		"kind: note",
		"queries:",
		"  active:",
		"    language: sql",
		"    sql: SELECT title, status FROM notes WHERE status = \"active\" LIMIT 5",
		"    kind: table",
		"    max_rows: 5",
		"    required: true",
		"---",
		"# Active",
		"{{ table .Queries.active }}",
	}, "\n")
	if _, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "active-report", Body: body}); err != nil {
		t.Fatalf("create query template: %v", err)
	}

	inspected, err := svc.InspectTemplate(ctx, TemplateRequest{VaultPath: root, Name: "active-report"})
	if err != nil {
		t.Fatalf("inspect query template: %v", err)
	}
	if inspected.Facts["queries"] != "1" || !strings.Contains(fmt.Sprint(inspected.Data), "query_explain") {
		t.Fatalf("inspect projection = %#v", inspected)
	}

	preview, err := svc.PreviewTemplate(ctx, TemplateRequest{VaultPath: root, Name: "active-report", Title: "Active Report"})
	if err != nil {
		t.Fatalf("preview query template: %v", err)
	}
	if got := fmt.Sprint(preview.Data); !strings.Contains(got, "| title | status |") || !strings.Contains(got, "| A | active |") || strings.Contains(got, "| B | done |") {
		t.Fatalf("preview data = %#v", preview.Data)
	}

	createdProjection, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Active Report", Template: "active-report"})
	if err != nil {
		t.Fatalf("create query note: %v", err)
	}
	created := readFile(t, filepath.Join(root, createdProjection.Facts["path"]))
	if !strings.Contains(created, "| A | active |") {
		t.Fatalf("created note = %s", created)
	}
}

func TestNoteShowRenderedSourceAndRefreshManagedBlock(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "A", Body: "priority:: 1\n", Status: "active"}); err != nil {
		t.Fatalf("create active note: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "B", Body: "priority:: 2\n", Status: "done"}); err != nil {
		t.Fatalf("create done note: %v", err)
	}
	dashboardBody := strings.Join([]string{
		"Dashboard intro",
		"",
		"```pinax-sql active",
		"SELECT title, status FROM notes WHERE status = \"active\" LIMIT 5",
		"```",
		"",
		"<!-- pinax:render active start -->",
		"stale",
		"<!-- pinax:render active end -->",
	}, "\n")
	created, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Dashboard", Body: dashboardBody, Status: "active"})
	if err != nil {
		t.Fatalf("create dashboard note: %v", err)
	}

	source, err := svc.ShowNoteProjection(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "Dashboard", View: "source"})
	if err != nil {
		t.Fatalf("show source: %v", err)
	}
	if source.Facts["view"] != "source" || source.Facts["query_count"] != "0" || !strings.Contains(fmt.Sprint(source.Data), "```pinax-sql active") {
		t.Fatalf("source projection = %#v", source)
	}

	rendered, err := svc.ShowNoteProjection(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "Dashboard", View: "rendered"})
	if err != nil {
		t.Fatalf("show rendered: %v", err)
	}
	if rendered.Facts["view"] != "rendered" || rendered.Facts["query_count"] != "1" || !strings.Contains(fmt.Sprint(rendered.Data), "| A | active |") || strings.Contains(fmt.Sprint(rendered.Data), "| B | done |") {
		t.Fatalf("rendered projection = %#v", rendered)
	}
	before := readFile(t, filepath.Join(root, created.Facts["path"]))
	if !strings.Contains(before, "stale") {
		t.Fatalf("source before refresh = %s", before)
	}

	if _, err := svc.RefreshNoteRendered(ctx, NoteRefreshRequest{VaultPath: root, NoteRef: "Dashboard", Yes: false}); !hasCommandCode(err, "approval_required") {
		t.Fatalf("refresh without approval err = %v", err)
	}
	refreshed, err := svc.RefreshNoteRendered(ctx, NoteRefreshRequest{VaultPath: root, NoteRef: "Dashboard", Yes: true})
	if err != nil {
		t.Fatalf("refresh rendered: %v", err)
	}
	if refreshed.Facts["changed_blocks"] != "1" || refreshed.Facts["query_count"] != "1" {
		t.Fatalf("refresh projection = %#v", refreshed)
	}
	after := readFile(t, filepath.Join(root, created.Facts["path"]))
	if !strings.Contains(after, "```pinax-sql active") || !strings.Contains(after, "| A | active |") || strings.Contains(after, "stale") {
		t.Fatalf("source after refresh = %s", after)
	}
}

func TestTemplateAuthoringRejectsUnsafeInput(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "../bad", Body: "x"}); !hasCommandCode(err, "invalid_template_name") {
		t.Fatalf("unsafe name err = %v", err)
	}
	if _, err := svc.CreateTemplate(ctx, TemplateRequest{VaultPath: root, Name: "conflict", Body: "x", SourcePath: filepath.Join(root, "missing.md")}); !hasCommandCode(err, "template_source_conflict") {
		t.Fatalf("source conflict err = %v", err)
	}
	if _, err := svc.RenderTemplate(ctx, TemplateRequest{VaultPath: root, Name: "note", Vars: map[string]string{"bad key": "x"}}); !hasCommandCode(err, "template_variable_invalid") {
		t.Fatalf("invalid var err = %v", err)
	}
}

func TestPlanningActionDraftBuildsTaskBridgeActionsSchema(t *testing.T) {
	now := time.Date(2026, 6, 7, 8, 30, 0, 0, time.UTC)
	snapshot := domain.PlanningSnapshot{SnapshotID: "plan_snap_test"}
	decision := domain.PlanningDecision{
		DecisionID:  "plan_dec_test",
		Period:      domain.PlanningDaily,
		Deferred:    []string{"task_123"},
		Reasons:     []domain.PlanningReason{{Kind: "capacity", Summary: "今日容量不足，推迟低优先级任务"}},
		CreatedAt:   now.Format(time.RFC3339),
		NextActions: []domain.Action{{Name: "open", Command: "pinax daily open --vault ./notes"}},
	}

	draft := planningops.BuildActionDraft("daily", snapshot, decision, now)

	if draft.SchemaVersion != "taskbridge.actions.v1" {
		t.Fatalf("schema = %q", draft.SchemaVersion)
	}
	if draft.SourceDecision != "plan_dec_test" || draft.SourceSnapshot != "plan_snap_test" {
		t.Fatalf("source refs = %#v", draft)
	}
	if !draft.RequiresConfirmation {
		t.Fatalf("draft should require confirmation: %#v", draft)
	}
	if len(draft.Tasks) != 1 {
		t.Fatalf("tasks = %#v", draft.Tasks)
	}
	task := draft.Tasks[0]
	if task.ActionID == "" || task.TaskID != "task_123" || task.Kind != "defer" || !task.RequiresConfirmation {
		t.Fatalf("task draft = %#v", task)
	}
	if task.Reason == "" || len(draft.EvidenceRefs) == 0 {
		t.Fatalf("draft missing reason/evidence: %#v", draft)
	}
}

func TestActionDraftDryRunDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	projection, err := svc.PlanActions(ctx, PlanningRequest{VaultPath: root, FromPeriod: "daily"})
	if err != nil {
		t.Fatalf("plan actions dry-run: %v", err)
	}
	if projection.Facts["dry_run"] != "true" || projection.Facts["saved_path"] != "" {
		t.Fatalf("dry-run facts = %#v", projection.Facts)
	}
	data, ok := projection.Data.(map[string]any)
	if !ok {
		t.Fatalf("data = %#v", projection.Data)
	}
	draft, ok := data["draft"].(domain.PlanningActionDraft)
	if !ok {
		t.Fatalf("draft data = %#v", data["draft"])
	}
	if draft.SchemaVersion != "taskbridge.actions.v1" || draft.SourceDecision == "" || draft.SourceSnapshot == "" {
		t.Fatalf("draft refs = %#v", draft)
	}
	if fileExistsApp(filepath.Join(root, ".pinax", "planning", "actions")) {
		t.Fatalf("dry-run created actions directory")
	}
}

func TestActionDraftSaveWritesAssetAndReceipt(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	projection, err := svc.PlanActions(ctx, PlanningRequest{VaultPath: root, FromPeriod: "weekly", Save: true})
	if err != nil {
		t.Fatalf("plan actions save: %v", err)
	}
	rel := projection.Facts["saved_path"]
	if rel == "" || !strings.HasPrefix(rel, ".pinax/planning/actions/") {
		t.Fatalf("saved_path = %#v", projection.Facts)
	}
	asset := readFile(t, filepath.Join(root, rel))
	var draft domain.PlanningActionDraft
	if err := json.Unmarshal([]byte(asset), &draft); err != nil {
		t.Fatalf("draft json invalid: %v\n%s", err, asset)
	}
	if draft.SchemaVersion != "taskbridge.actions.v1" || draft.SourcePeriod != "weekly" || draft.SourceDecision == "" || draft.SourceSnapshot == "" {
		t.Fatalf("draft asset = %#v", draft)
	}
	if !strings.Contains(readFile(t, filepath.Join(root, ".pinax", "events.jsonl")), "plan.actions") {
		t.Fatalf("plan action save did not append receipt event")
	}
	if len(projection.Actions) != 1 || !strings.Contains(projection.Actions[0].Command, "taskbridge agent execute --action-file") || !strings.Contains(projection.Actions[0].Command, "--dry-run") {
		t.Fatalf("next actions = %#v", projection.Actions)
	}
}

func installAppFakeRclone(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	binDir := t.TempDir()
	script := `#!/usr/bin/env python3
import os, pathlib, shutil, sys, time
root = pathlib.Path(os.environ["FAKE_RCLONE_ROOT"])
sleep = os.environ.get("FAKE_RCLONE_SLEEP", "")
if sleep:
    time.sleep(float(sleep.rstrip("s")))

def local_path(target):
    if ":" not in target:
        print("bad target", file=sys.stderr)
        sys.exit(7)
    remote, path = target.split(":", 1)
    return root / remote / path.strip("/")

args = sys.argv[1:]
if not args:
    print("missing command", file=sys.stderr)
    sys.exit(2)
cmd = args[0]
if cmd == "cat":
    p = local_path(args[1])
    if not p.exists():
        print("object not found path=notes/alpha.md Authorization: Bearer raw-token", file=sys.stderr)
        sys.exit(3)
    sys.stdout.buffer.write(p.read_bytes())
elif cmd == "copyto":
    src = pathlib.Path(args[1])
    dst = local_path(args[2])
    dst.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(src, dst)
elif cmd == "lsf":
    base = local_path(args[-1])
    if base.is_file():
        parent = base.parent
        files = [base]
    else:
        parent = base
        files = sorted([p for p in base.rglob("*") if p.is_file()]) if base.exists() else []
    for p in files:
        rel = p.relative_to(parent).as_posix()
        print(f"{p.stat().st_size};{rel};2026-06-12T00:00:00Z")
elif cmd == "deletefile":
    try:
        local_path(args[1]).unlink()
    except FileNotFoundError:
        pass
else:
    print("unsupported fake rclone command " + cmd, file=sys.stderr)
    sys.exit(2)
`
	path := filepath.Join(binDir, "rclone")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake rclone: %v", err)
	}
	t.Setenv("FAKE_RCLONE_ROOT", root)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return root
}

func hasCommandCode(err error, code string) bool {
	if err == nil {
		return false
	}
	var commandErr *domain.CommandError
	if errors.As(err, &commandErr) {
		return commandErr.Code == code
	}
	return false
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestNoteListPropertyStrictProperties(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "A", Body: "priority:: 2\n", Status: "active"}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	projection, err := svc.ListNotesQuery(ctx, NoteListRequest{VaultPath: root, Properties: []string{"priority"}})
	if err != nil {
		t.Fatalf("list notes property: %v", err)
	}
	if projection.Facts["properties"] != "priority" || !strings.Contains(fmt.Sprint(projection.Data), "priority") {
		t.Fatalf("projection = %#v", projection)
	}
	if _, err := svc.ListNotesQuery(ctx, NoteListRequest{VaultPath: root, Properties: []string{"missing"}, StrictProperties: true}); !hasCommandCode(err, "property_not_found") {
		t.Fatalf("strict property err = %v", err)
	}
}

func TestFrontmatterPropertiesAreSelectable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "frontmatter-props.md"), strings.Join([]string{
		"---",
		"schema_version: pinax.note.v1",
		"note_id: note_frontmatter_props",
		"title: Frontmatter Props",
		"tags: [pinax]",
		"rating: 5",
		"done: false",
		"due: 2026-06-09",
		"---",
		"",
		"# Frontmatter Props",
	}, "\n"))

	projection, err := svc.ListNotesQuery(ctx, NoteListRequest{VaultPath: root, Properties: []string{"rating", "done", "due"}, StrictProperties: true})
	if err != nil {
		t.Fatalf("list frontmatter properties: %v", err)
	}
	if projection.Facts["properties"] != "rating,done,due" || !strings.Contains(fmt.Sprint(projection.Data), "rating") || !strings.Contains(fmt.Sprint(projection.Data), "2026-06-09") {
		t.Fatalf("frontmatter properties not projected: %#v", projection)
	}
}

func TestAssetListShowPrefersIndexProjectionAndFallsBackToManifest(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	manifestAsset := domain.Asset{ID: "asset_manifest", Path: "assets/manifest.txt", Filename: "manifest.txt", Stem: "manifest", Extension: "txt", MediaType: "text/plain", Size: 8, SHA256: "manifest-sha", ManagedStatus: domain.ManagedStatusManaged}
	if err := pinaxassets.Save(root, pinaxassets.Manifest{Assets: []pinaxassets.Asset{manifestAsset}}); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
	indexAsset := domain.Asset{ID: "asset_index", Path: "assets/index.png", Filename: "index.png", Stem: "index", Extension: "png", MediaType: "image/png", Size: 12, SHA256: "index-sha", ManagedStatus: domain.ManagedStatusManaged, Width: 3, Height: 2}
	if err := noteindex.ReplaceAssetProjection(root, []domain.Asset{indexAsset}); err != nil {
		t.Fatalf("replace index assets: %v", err)
	}

	list, err := svc.AssetList(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("asset list: %v", err)
	}
	if list.Facts["engine"] != "index" || list.Facts["index_status"] != "fresh" || list.Facts["assets"] != "1" || list.Facts["asset.1.path"] != indexAsset.Path {
		t.Fatalf("list projection = %#v", list)
	}
	showIndex, err := svc.AssetShow(ctx, AssetRequest{VaultPath: root, Ref: "index"})
	if err != nil {
		t.Fatalf("asset show index: %v", err)
	}
	if showIndex.Facts["engine"] != "index" || showIndex.Facts["index_status"] != "fresh" || showIndex.Facts["asset_path"] != indexAsset.Path {
		t.Fatalf("show index projection = %#v", showIndex)
	}
	showManifest, err := svc.AssetShow(ctx, AssetRequest{VaultPath: root, Ref: "manifest"})
	if err != nil {
		t.Fatalf("asset show manifest fallback: %v", err)
	}
	if showManifest.Facts["engine"] != "manifest" || showManifest.Facts["index_status"] != "fresh" || showManifest.Facts["asset_path"] != manifestAsset.Path {
		t.Fatalf("show manifest fallback projection = %#v", showManifest)
	}
}

func TestAssetMovePlanAndSharedRemovePlanAreNoWrite(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\n![Diagram](../assets/diagram.png)\n")
	writeAppFixture(t, filepath.Join(root, "notes", "beta.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\n---\n\n# Beta\n\n![[../assets/diagram.png]]\n")
	if _, err := svc.RebuildIndex(ctx, VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}

	move, err := svc.AssetMovePlan(ctx, AssetRequest{VaultPath: root, Ref: "diagram", Target: "attachments/archive/diagram.png"})
	if err != nil {
		t.Fatalf("asset move plan: %v", err)
	}
	if move.Command != "asset.move" || move.Facts["writes"] != "false" || move.Facts["linked_notes"] != "2" || move.Facts["requires_snapshot"] != "true" {
		t.Fatalf("move projection facts = %#v", move.Facts)
	}
	moveData := fmt.Sprint(move.Data)
	if !strings.Contains(moveData, "asset_move") || !strings.Contains(moveData, "asset_reference_rewrite") || !strings.Contains(moveData, "![Diagram](../assets/diagram.png)") || !strings.Contains(moveData, "![[../assets/diagram.png]]") {
		t.Fatalf("move data = %#v", move.Data)
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", "archive", "diagram.png")); !os.IsNotExist(err) {
		t.Fatalf("move plan wrote target: %v", err)
	}

	remove, err := svc.AssetRemovePlan(ctx, AssetRequest{VaultPath: root, Ref: "diagram"})
	if err != nil {
		t.Fatalf("asset remove plan: %v", err)
	}
	if remove.Command != "asset.remove" || remove.Facts["writes"] != "false" || remove.Facts["shared"] != "true" || remove.Facts["delete_allowed"] != "false" || remove.Facts["requires_snapshot"] != "true" {
		t.Fatalf("remove projection facts = %#v", remove.Facts)
	}
	removeData := fmt.Sprint(remove.Data)
	if strings.Contains(removeData, "asset_delete") || !strings.Contains(removeData, "asset_reference_review") {
		t.Fatalf("remove data = %#v", remove.Data)
	}
	if _, err := os.Stat(filepath.Join(root, "assets", "diagram.png")); err != nil {
		t.Fatalf("remove plan deleted asset: %v", err)
	}
}
func TestAssetLinkPlanIsNoWrite(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	notePath := filepath.Join(root, "notes", "alpha.md")
	writeAppFixture(t, notePath, "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\nbody before link\n")
	if _, err := svc.RebuildIndex(ctx, VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	beforeBytes, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("read note before link: %v", err)
	}
	before := string(beforeBytes)
	projection, err := svc.AssetLink(ctx, AssetRequest{VaultPath: root, Ref: "diagram", ContextNote: "Alpha"})
	if err != nil {
		t.Fatalf("asset link plan: %v", err)
	}
	if projection.Command != "asset.link" || projection.Facts["writes"] != "false" || projection.Facts["asset_path"] != "assets/diagram.png" || projection.Facts["note_path"] != "notes/alpha.md" || projection.Facts["operations"] != "1" {
		t.Fatalf("asset link projection facts = %#v", projection.Facts)
	}
	data := fmt.Sprint(projection.Data)
	if !strings.Contains(data, "asset_link") || !strings.Contains(data, "notes/alpha.md") || !strings.Contains(data, "assets/diagram.png") {
		t.Fatalf("asset link data = %#v", projection.Data)
	}
	afterBytes, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("read note after link: %v", err)
	}
	if after := string(afterBytes); after != before {
		t.Fatalf("asset link plan modified note body:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestAssetVerifyReportsUnmanagedAndOrphanFacts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "unmanaged.txt"), []byte("unmanaged"), 0o644); err != nil {
		t.Fatalf("write unmanaged: %v", err)
	}
	manifest := pinaxassets.Manifest{Assets: []pinaxassets.Asset{{ID: "asset_missing", Path: "assets/missing.txt", Filename: "missing.txt", Stem: "missing", Extension: "txt", MediaType: "text/plain", Size: 7, SHA256: "missing-sha", ManagedStatus: domain.ManagedStatusManaged}}}
	if err := pinaxassets.Save(root, manifest); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
	projection, err := svc.AssetVerify(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("asset verify: %v", err)
	}
	if projection.Facts["missing"] != "1" || projection.Facts["orphan"] != "1" || projection.Facts["unmanaged"] != "1" {
		t.Fatalf("verify facts = %#v", projection.Facts)
	}
}
func TestRepairPlanIncludesAssetAndVersionConsistencyOperations(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	changedPath := filepath.Join(root, "assets", "changed.png")
	orphanPath := filepath.Join(root, "assets", "orphan.png")
	writeAppFixture(t, changedPath, "original")
	writeAppFixture(t, orphanPath, "orphan")
	if _, err := pinaxassets.AddWithOptions(root, changedPath, pinaxassets.AddOptions{Mode: pinaxassets.AddModeRegister}); err != nil {
		t.Fatalf("register changed asset: %v", err)
	}
	if _, err := pinaxassets.AddWithOptions(root, orphanPath, pinaxassets.AddOptions{Mode: pinaxassets.AddModeRegister}); err != nil {
		t.Fatalf("register orphan asset: %v", err)
	}
	writeAppFixture(t, changedPath, "changed")
	manifest, err := pinaxassets.Load(root)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	manifest.Assets = append(manifest.Assets, pinaxassets.Asset{ID: "asset_missing", Path: "assets/missing.png", Filename: "missing.png", Stem: "missing", Extension: "png", MediaType: "image/png", Size: 7, SHA256: "missing-sha", ManagedStatus: domain.ManagedStatusManaged})
	if err := pinaxassets.Save(root, manifest); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\ntags: [asset]\n---\n\n# Alpha\n\n![Dangling](../assets/dangling.png)\n")
	if _, err := svc.RebuildIndex(ctx, VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	beforeChanged := readFile(t, changedPath)
	beforeNote := readFile(t, filepath.Join(root, "notes", "alpha.md"))

	projection, err := svc.PlanRepair(ctx, RepairPlanRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("repair plan: %v", err)
	}
	plan, ok := projection.Data.(domain.RepairPlan)
	if !ok {
		t.Fatalf("repair data type = %T", projection.Data)
	}
	if got := readFile(t, changedPath); got != beforeChanged {
		t.Fatalf("repair plan modified asset: %q", got)
	}
	if got := readFile(t, filepath.Join(root, "notes", "alpha.md")); got != beforeNote {
		t.Fatalf("repair plan modified note: %q", got)
	}
	kinds := map[string]bool{}
	for _, op := range plan.Operations {
		kinds[op.Kind] = true
	}
	for _, want := range []string{"asset_missing", "asset_hash_changed", "orphan_manifest_entry", "dangling_asset_link", "version_evidence_missing"} {
		if !kinds[want] {
			t.Fatalf("repair operation %s missing; kinds=%#v operations=%#v", want, kinds, plan.Operations)
		}
	}
}

func TestIndexRefreshChangedSinceUsesVersionBackendWithoutDeletingUnchangedNotes(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fake := &versiontest.FakeBackend{ChangedSinceResult: []pinaxversion.ChangedPath{{Path: "notes/a.md", ChangeKind: "modified", ObjectKind: domain.VaultObjectKindNote}}}
	svc := NewServiceWithVersionBackend(fake)
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "A", Slug: "a", Body: "before"}); err != nil {
		t.Fatalf("create note a: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "B", Slug: "b", Body: "unchanged"}); err != nil {
		t.Fatalf("create note b: %v", err)
	}
	notePath := filepath.Join(root, "a.md")
	body, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("read note a: %v", err)
	}
	writeAppFixture(t, notePath, strings.Replace(string(body), "before", "after", 1))

	projection, err := svc.IndexRefresh(ctx, IndexRefreshRequest{VaultPath: root, ChangedSince: "rev_1"})
	if err != nil {
		t.Fatalf("changed-since refresh: %v", err)
	}
	if fake.LastChangedSinceRequest.Root != root || fake.LastChangedSinceRequest.SinceRevision != "rev_1" {
		t.Fatalf("changed-since request = %#v", fake.LastChangedSinceRequest)
	}
	if projection.Facts["changed_since"] != "rev_1" || projection.Facts["changed_candidates"] != "1" || projection.Facts["scanned"] != "1" {
		t.Fatalf("changed-since facts = %#v", projection.Facts)
	}
	lookup, err := noteindex.Lookup(root, noteindex.LookupRequest{Query: "b.md", Scope: "registered", Kind: "note"})
	if err != nil {
		t.Fatalf("lookup unchanged note: %v", err)
	}
	if len(lookup.Candidates) != 1 || lookup.Candidates[0].Path != "b.md" {
		t.Fatalf("unchanged note projection was removed: %#v", lookup)
	}
}

func TestNoteReadPathsUseSharedResolverCandidates(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\nunique resolver body [[Beta]].\n")
	writeAppFixture(t, filepath.Join(root, "notes", "beta.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\n---\n\n# Beta\n\nBacklink target.\n")
	if _, err := svc.IndexRefresh(ctx, IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("refresh index: %v", err)
	}

	show, err := svc.ShowNoteProjection(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "unique resolver body"})
	if err != nil {
		t.Fatalf("show by resolver content: %v", err)
	}
	if show.Facts["path"] != "notes/alpha.md" || show.Facts["resolver.match_field"] != "content" || show.Facts["resolver.candidates"] != "1" {
		t.Fatalf("show resolver facts = %#v", show.Facts)
	}

	links, err := svc.NoteLinks(ctx, NoteLinkRequest{VaultPath: root, NoteRef: "unique resolver body"})
	if err != nil {
		t.Fatalf("links by resolver content: %v", err)
	}
	if links.Facts["path"] != "notes/alpha.md" || links.Facts["note_id"] != "note_alpha" || links.Facts["links"] != "1" {
		t.Fatalf("links resolver facts = %#v", links.Facts)
	}

	backlinks, err := svc.NoteBacklinks(ctx, NoteLinkRequest{VaultPath: root, NoteRef: "Backlink target"})
	if err != nil {
		t.Fatalf("backlinks by resolver content: %v", err)
	}
	if backlinks.Facts["path"] != "notes/beta.md" || backlinks.Facts["note_id"] != "note_beta" || backlinks.Facts["backlinks"] != "1" {
		t.Fatalf("backlinks resolver facts = %#v", backlinks.Facts)
	}
}

func TestNoteWritePathsUseResolverGuardBeforeWrite(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "target-a.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_target_a\ntitle: Target A\n---\n\n# Target A\n")
	writeAppFixture(t, filepath.Join(root, "notes", "target-b.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_target_b\ntitle: Target B\n---\n\n# Target B\n")
	writeAppFixture(t, filepath.Join(root, "source.txt"), "attachment")
	if _, err := svc.IndexRefresh(ctx, IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("refresh index: %v", err)
	}

	checks := []struct {
		name string
		run  func() (domain.Projection, error)
	}{
		{name: "rename", run: func() (domain.Projection, error) {
			return svc.RenameNote(ctx, NoteMutationRequest{VaultPath: root, NoteRef: "target", Title: "Changed"})
		}},
		{name: "move", run: func() (domain.Projection, error) {
			return svc.MoveNote(ctx, NoteMutationRequest{VaultPath: root, NoteRef: "target", TargetDir: "archive"})
		}},
		{name: "archive", run: func() (domain.Projection, error) {
			return svc.ArchiveNote(ctx, NoteMutationRequest{VaultPath: root, NoteRef: "target"})
		}},
		{name: "delete", run: func() (domain.Projection, error) {
			return svc.DeleteNote(ctx, NoteDeleteRequest{VaultPath: root, NoteRef: "target", Yes: true})
		}},
		{name: "tag", run: func() (domain.Projection, error) {
			return svc.TagNote(ctx, NoteTagRequest{VaultPath: root, NoteRef: "target", Operation: "add", Tags: []string{"x"}})
		}},
		{name: "attach", run: func() (domain.Projection, error) {
			return svc.AttachNoteFile(ctx, NoteAttachRequest{VaultPath: root, NoteRef: "target", SourcePath: filepath.Join(root, "source.txt"), Mode: "copy"})
		}},
	}
	for _, check := range checks {
		projection, err := check.run()
		if err == nil || !hasCommandCode(err, domain.ErrorCodeVaultObjectRefAmbiguous) {
			t.Fatalf("%s guard err=%v projection=%#v", check.name, err, projection)
		}
		if projection.Facts["candidates"] != "2" || !strings.Contains(fmt.Sprint(projection.Data), "notes/target-a.md") || !strings.Contains(fmt.Sprint(projection.Data), "notes/target-b.md") {
			t.Fatalf("%s guard projection=%#v", check.name, projection)
		}
	}
	if !fileExistsApp(filepath.Join(root, "notes", "target-a.md")) || !fileExistsApp(filepath.Join(root, "notes", "target-b.md")) || !fileExistsApp(filepath.Join(root, "source.txt")) {
		t.Fatalf("write guard changed files")
	}
	if fileExistsApp(filepath.Join(root, ".pinax", "records", "events.jsonl")) {
		t.Fatalf("write guard wrote record ledger")
	}
}

func TestMetadataPlanQueryUsesRegisteredOrAdoptableResolver(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Needs Alias", Slug: "needs-alias", Body: "unique metadata resolver body"}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	if _, err := svc.IndexRefresh(ctx, IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatalf("refresh index: %v", err)
	}
	notePath := filepath.Join(root, "notes", "needs-alias.md")
	writeAppFixture(t, notePath, "---\nnote_id: note_needs_alias\ntitle: Needs Alias\n---\n\n# Needs Alias\n\nunique metadata resolver body\n")

	projection, err := svc.PlanMetadata(ctx, VaultRequest{VaultPath: root, Query: "unique metadata resolver body"})
	if err != nil {
		t.Fatalf("metadata plan resolver query: %v", err)
	}
	if projection.Facts["candidates"] != "1" || projection.Facts["planned_updates"] != "1" || projection.Facts["writes"] != "false" {
		t.Fatalf("metadata resolver facts = %#v data=%#v", projection.Facts, projection.Data)
	}
	if !strings.Contains(fmt.Sprint(projection.Data), "needs-alias.md") {
		t.Fatalf("metadata resolver data = %#v", projection.Data)
	}
}

func TestVersionApplicationServicesUseInjectedBackend(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fake := &versiontest.FakeBackend{
		ChangedSinceResult: []pinaxversion.ChangedPath{{Path: "notes/a.md", ChangeKind: "modified", ObjectKind: domain.VaultObjectKindNote}},
		ReadFileResult:     pinaxversion.VersionedFile{Path: "notes/a.md", Revision: "rev_1", Backend: "fake", Content: "# A\n", SizeBytes: 4, ContentHash: "sha256:a"},
		DiffSummaryResult:  pinaxversion.DiffSummary{BaseRevision: "HEAD", TargetRevision: "rev_1", FilesChanged: 1, ChangedPaths: []domain.ChangedPath{{Path: "notes/a.md", ChangeKind: "modified"}}},
	}
	svc := NewServiceWithVersionBackend(fake)
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	changed, err := svc.VersionChanged(ctx, VersionChangedRequest{VaultPath: root, SinceRevision: "rev_0"})
	if err != nil {
		t.Fatalf("version changed: %v", err)
	}
	if changed.Command != "version.changed" || changed.Facts["changed"] != "1" || changed.Facts["since_revision"] != "rev_0" {
		t.Fatalf("changed projection = %#v", changed)
	}
	if fake.LastChangedSinceRequest.Root != root || fake.LastChangedSinceRequest.SinceRevision != "rev_0" {
		t.Fatalf("changed request = %#v", fake.LastChangedSinceRequest)
	}

	show, err := svc.VersionShow(ctx, VersionShowRequest{VaultPath: root, Path: "notes/a.md", Revision: "rev_1"})
	if err != nil {
		t.Fatalf("version show: %v", err)
	}
	if show.Command != "version.show" || show.Facts["path"] != "notes/a.md" || show.Facts["revision"] != "rev_1" || show.Facts["bytes"] != "4" {
		t.Fatalf("show projection = %#v", show)
	}
	if fake.LastReadFileRequest.Path != "notes/a.md" || fake.LastReadFileRequest.Revision != "rev_1" {
		t.Fatalf("read request = %#v", fake.LastReadFileRequest)
	}

	restore, err := svc.VersionRestorePlan(ctx, VersionRestorePlanRequest{VaultPath: root, Path: "notes/a.md", Revision: "rev_1"})
	if err != nil {
		t.Fatalf("version restore plan: %v", err)
	}
	if restore.Command != "version.restore" || restore.Facts["writes"] != "false" || restore.Facts["operations"] != "1" || restore.Facts["requires_snapshot"] != "true" {
		t.Fatalf("restore projection = %#v", restore)
	}
	if fake.LastReadFileRequest.Path != "notes/a.md" || fake.LastDiffSummaryRequest.TargetRevision != "rev_1" {
		t.Fatalf("restore backend requests read=%#v diff=%#v", fake.LastReadFileRequest, fake.LastDiffSummaryRequest)
	}
	if !fileExistsApp(filepath.Join(root, "a.md")) {
		// Restore planning must not create the target file.
	} else {
		t.Fatalf("restore plan wrote note file")
	}
}

func TestVersionRestorePlanUsesResolverInputAndDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fake := &versiontest.FakeBackend{
		ReadFileResult:    pinaxversion.VersionedFile{Path: "notes/restorable.md", Revision: "rev_1", Backend: "fake", Content: "# Restorable old\n", SizeBytes: 17, ContentHash: "sha256:old"},
		DiffSummaryResult: pinaxversion.DiffSummary{BaseRevision: "HEAD", TargetRevision: "rev_1", FilesChanged: 1, ChangedPaths: []domain.ChangedPath{{Path: "notes/restorable.md", ChangeKind: "modified"}}},
	}
	svc := NewServiceWithVersionBackend(fake)
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	notePath := filepath.Join(root, "notes", "restorable.md")
	writeAppFixture(t, notePath, "---\nschema_version: pinax.note.v1\nnote_id: note_restorable\ntitle: Restorable\ntags: []\n---\n\n# Restorable\n\ncurrent body\n")
	before := readFile(t, notePath)

	projection, err := svc.VersionRestorePlan(ctx, VersionRestorePlanRequest{VaultPath: root, Path: "Restorable", Revision: "rev_1"})
	if err != nil {
		t.Fatalf("version restore resolver plan: %v", err)
	}
	if projection.Facts["path"] != "notes/restorable.md" || projection.Facts["writes"] != "false" || projection.Facts["requires_snapshot"] != "true" {
		t.Fatalf("restore resolver facts = %#v", projection.Facts)
	}
	if fake.LastReadFileRequest.Path != "notes/restorable.md" || fake.LastReadFileRequest.Revision != "rev_1" {
		t.Fatalf("restore read request = %#v", fake.LastReadFileRequest)
	}
	if got := readFile(t, notePath); got != before {
		t.Fatalf("restore plan modified note:\n%s", got)
	}
	if fileExistsApp(filepath.Join(root, ".pinax", "records", "events.jsonl")) {
		t.Fatalf("restore plan wrote record event")
	}
}

func TestVersionRestorePlanRejectsAmbiguousCandidatesBeforeBackendRead(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fake := &versiontest.FakeBackend{ReadFileResult: pinaxversion.VersionedFile{Path: "notes/target-a.md", Revision: "rev_1", Backend: "fake"}}
	svc := NewServiceWithVersionBackend(fake)
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "target-a.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_target_a\ntitle: Target\ntags: []\n---\n\n# Target\n")
	writeAppFixture(t, filepath.Join(root, "notes", "target-b.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_target_b\ntitle: Target\ntags: []\n---\n\n# Target\n")
	beforeA := readFile(t, filepath.Join(root, "notes", "target-a.md"))
	beforeB := readFile(t, filepath.Join(root, "notes", "target-b.md"))

	projection, err := svc.VersionRestorePlan(ctx, VersionRestorePlanRequest{VaultPath: root, Path: "Target", Revision: "rev_1"})
	if err == nil || !hasCommandCode(err, domain.ErrorCodeVaultObjectRefAmbiguous) {
		t.Fatalf("restore ambiguous err=%v projection=%#v", err, projection)
	}
	if projection.Facts["candidates"] != "2" || !strings.Contains(fmt.Sprint(projection.Data), "notes/target-a.md") || !strings.Contains(fmt.Sprint(projection.Data), "notes/target-b.md") {
		t.Fatalf("restore ambiguous projection=%#v", projection)
	}
	if fake.LastReadFileRequest.Path != "" {
		t.Fatalf("ambiguous restore called backend: %#v", fake.LastReadFileRequest)
	}
	if got := readFile(t, filepath.Join(root, "notes", "target-a.md")); got != beforeA {
		t.Fatalf("target-a changed: %s", got)
	}
	if got := readFile(t, filepath.Join(root, "notes", "target-b.md")); got != beforeB {
		t.Fatalf("target-b changed: %s", got)
	}
}
