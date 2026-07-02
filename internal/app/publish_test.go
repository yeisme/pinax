package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

func TestPublishProfileFacadeWritesAndValidatesProfile(t *testing.T) {
	svc := NewService()
	ctx := context.Background()
	root := t.TempDir()
	req := PublishRequest{VaultPath: root, Profile: "public", Target: "github-pages", Renderer: "hugo", Title: "Knowledge Base", BaseURL: "https://example.github.io/kb/"}

	initProjection, err := svc.PublishProfileInit(ctx, req)
	if err != nil {
		t.Fatalf("profile init: %v", err)
	}
	if initProjection.Command != "publish.profile.init" || initProjection.Facts["profile"] != "public" {
		t.Fatalf("init projection = %#v", initProjection)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "publish", "profiles", "public.yaml")); err != nil {
		t.Fatalf("profile file missing: %v", err)
	}

	validateProjection, err := svc.PublishProfileValidate(ctx, PublishRequest{VaultPath: root, Profile: "public"})
	if err != nil {
		t.Fatalf("profile validate: %v", err)
	}
	if validateProjection.Command != "publish.profile.validate" || validateProjection.Facts["issues"] != "0" {
		t.Fatalf("validate projection = %#v", validateProjection)
	}

	listProjection, err := svc.PublishProfileList(ctx, PublishRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("profile list: %v", err)
	}
	if listProjection.Command != "publish.profile.list" || listProjection.Facts["profiles"] != "1" {
		t.Fatalf("list projection = %#v", listProjection)
	}
}

func TestPublishProfileValidateExposesLegacyRendererMigrationPlan(t *testing.T) {
	svc := NewService()
	ctx := context.Background()
	root := t.TempDir()

	if _, err := svc.PublishProfileInit(ctx, PublishRequest{VaultPath: root, Profile: "legacy", Target: "github-pages", Renderer: "hugo"}); err != nil {
		t.Fatalf("legacy profile init: %v", err)
	}

	projection, err := svc.PublishProfileValidate(ctx, PublishRequest{VaultPath: root, Profile: "legacy"})
	if err != nil {
		t.Fatalf("legacy profile should remain valid: %v", err)
	}
	if projection.Facts["renderer"] != "hugo" || projection.Facts["migration.recommended"] != "true" {
		t.Fatalf("legacy migration facts = %#v", projection.Facts)
	}
	data, ok := projection.Data.(map[string]any)
	if !ok {
		t.Fatalf("projection data = %#v", projection.Data)
	}
	plan, ok := data["migration_plan"].(domain.PublishProfileMigrationPlan)
	if !ok {
		t.Fatalf("migration plan missing from data: %#v", data)
	}
	if !plan.Recommended || plan.FromRenderer != domain.PublishRendererHugo || plan.ToRenderer != domain.PublishRendererPinaxWeb {
		t.Fatalf("migration plan = %#v", plan)
	}
	if !strings.Contains(plan.Command, "pinax publish profile init legacy --target github-pages --renderer pinax-web") {
		t.Fatalf("migration command = %q", plan.Command)
	}
}

func TestPublishPlanFacadeSelectsAndBlocksNotes(t *testing.T) {
	svc := NewService()
	ctx := context.Background()
	root := t.TempDir()
	if _, err := svc.PublishProfileInit(ctx, PublishRequest{VaultPath: root, Profile: "public", Target: "github-pages", Renderer: "hugo"}); err != nil {
		t.Fatalf("profile init: %v", err)
	}
	writeAppPublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Public", "kind": "concept", "status": "active", "publish": "public"}, "# Public\n")
	writeAppPublishNoteFixture(t, root, "notes/private.md", map[string]string{"note_id": "note_private", "title": "Private", "kind": "concept", "status": "active", "publish": "public", "privacy": "private"}, "PRIVATE_BODY_SENTINEL\n")
	writeAppPublishNoteFixture(t, root, "notes/secret.md", map[string]string{"note_id": "note_secret", "title": "Secret", "kind": "concept", "status": "active", "publish": "public"}, "Authorization: Bearer SECRET_SENTINEL\n")

	projection, err := svc.PublishPlan(ctx, PublishRequest{VaultPath: root, Profile: "public", Target: "github-pages"})
	if err != nil {
		t.Fatalf("publish plan: %v", err)
	}
	if projection.Command != "publish.plan" || projection.Status != "partial" {
		t.Fatalf("plan projection = %#v", projection)
	}
	if projection.Facts["selected_count"] != "1" || projection.Facts["skipped_count"] != "1" || projection.Facts["blocking_count"] != "1" {
		t.Fatalf("plan facts = %#v", projection.Facts)
	}
}

func TestPublishPlanFacadeClassifiesLinkedAssets(t *testing.T) {
	svc := NewService()
	ctx := context.Background()
	root := t.TempDir()
	if _, err := svc.PublishProfileInit(ctx, PublishRequest{VaultPath: root, Profile: "public", Target: "github-pages", Renderer: "hugo"}); err != nil {
		t.Fatalf("profile init: %v", err)
	}
	writeAppPublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Assets", "kind": "concept", "status": "active", "publish": "public"}, "![Diagram](../assets/diagram.png)\n![Raw](../assets/raw.exe)\n")
	writeAppPublishFileFixture(t, root, "assets/diagram.png", "fake image")
	writeAppPublishFileFixture(t, root, "assets/raw.exe", "not publishable")

	projection, err := svc.PublishPlan(ctx, PublishRequest{VaultPath: root, Profile: "public", Target: "github-pages"})
	if err != nil {
		t.Fatalf("publish plan: %v", err)
	}
	if projection.Status != "partial" {
		t.Fatalf("plan projection = %#v", projection)
	}
	if projection.Facts["selected_count"] != "1" || projection.Facts["selected_asset_count"] != "1" || projection.Facts["asset_violation_count"] != "1" || projection.Facts["blocking_count"] != "1" {
		t.Fatalf("plan facts = %#v", projection.Facts)
	}
}

func TestPublishGitErrorRedactionCoversCredentialsAndPaths(t *testing.T) {
	root := t.TempDir()
	raw := "fatal: https://user:raw-token@example.invalid/repo.git Authorization: Bearer raw-token token=raw " + root + "/dist"
	redacted := publishRedactGitOutput(raw, root)
	for _, leak := range []string{"raw-token", "Authorization: Bearer", "token=raw", root, "user:"} {
		if strings.Contains(redacted, leak) {
			t.Fatalf("git redaction leaked %q in %q", leak, redacted)
		}
	}
	for _, want := range []string{"[REDACTED_URL]", "[REDACTED]", "[REDACTED_PATH]"} {
		if !strings.Contains(redacted, want) {
			t.Fatalf("git redaction missing marker %q in %q", want, redacted)
		}
	}
}

func TestPublishDeployFacadeExposesStableMissingProfileError(t *testing.T) {
	svc := NewService()
	ctx := context.Background()
	req := PublishRequest{VaultPath: t.TempDir(), Profile: "public", Target: "github-pages", Renderer: "hugo"}

	cases := []struct {
		name    string
		command string
		call    func() (domain.Projection, error)
	}{
		{name: "deploy", command: "publish.deploy", call: func() (domain.Projection, error) { return svc.PublishDeploy(ctx, req) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projection, err := tc.call()
			if err == nil {
				t.Fatalf("expected missing profile error")
			}
			var cmdErr *domain.CommandError
			if !errors.As(err, &cmdErr) {
				t.Fatalf("expected CommandError, got %T", err)
			}
			if cmdErr.Code != "publish_profile_not_found" {
				t.Fatalf("error code = %q", cmdErr.Code)
			}
			if projection.Command != tc.command || projection.Status != "failed" {
				t.Fatalf("unexpected projection: %#v", projection)
			}
		})
	}
}

func TestPublishDevWatchOnceRebuildsAfterVaultMarkdownChange(t *testing.T) {
	svc := NewService()
	root := t.TempDir()
	outDir := filepath.Join(root, "dist", "site")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := svc.PublishProfileInit(ctx, PublishRequest{VaultPath: root, Profile: "public", Target: "github-pages", Renderer: "pinax-web"}); err != nil {
		t.Fatalf("profile init: %v", err)
	}
	writeAppPublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Public", "kind": "concept", "status": "active", "publish": "public"}, "# Public\n\nFirst body.\n")

	result := make(chan domain.Projection, 1)
	errs := make(chan error, 1)
	go func() {
		projection, err := svc.PublishDev(ctx, PublishRequest{VaultPath: root, Profile: "public", Out: outDir, Host: "127.0.0.1", Port: 0, Watch: true, Once: true})
		result <- projection
		errs <- err
	}()

	waitForFile(t, filepath.Join(outDir, "index.html"), 10*time.Second)
	time.Sleep(300 * time.Millisecond)
	writeAppPublishNoteFixture(t, root, "notes/public.md", map[string]string{"note_id": "note_public", "title": "Public", "kind": "concept", "status": "active", "publish": "public"}, "# Public\n\nSecond body.\n")

	select {
	case projection := <-result:
		if err := <-errs; err != nil {
			t.Fatalf("publish dev watch: %v projection=%#v", err, projection)
		}
		if projection.Command != "publish.dev" || projection.Facts["watched"] != "true" || projection.Facts["rebuilds"] != "1" || projection.Facts["served"] != "true" {
			t.Fatalf("watch projection = %#v", projection)
		}
	case <-ctx.Done():
		t.Fatalf("publish dev watch did not rebuild before timeout: %v", ctx.Err())
	}
}

func writeAppPublishNoteFixture(t *testing.T, root, rel string, meta map[string]string, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nschema_version: pinax.note.v1\n"
	for _, key := range []string{"note_id", "title", "kind", "status", "publish", "privacy"} {
		if meta[key] != "" {
			content += key + ": " + meta[key] + "\n"
		}
	}
	content += "---\n\n" + body
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeAppPublishFileFixture(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
