package e2e

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/promptbridge/testfixture"
	promptrepo "github.com/yeisme/promptrepo"
)

// TestPromptCatalogSourceAdaptersE2E covers the file/Git/S3 source fixtures,
// offline cache behavior, catalog quarantine on digest mismatch, and the
// effective-scope policy surface through the compiled pinax binary.
func TestPromptCatalogSourceAdaptersE2E(t *testing.T) {
	t.Parallel()
	pinaxBin := filepath.Join(sharedBinDir, "pinax")
	home := t.TempDir()

	runJSON := func(args ...string) map[string]any {
		t.Helper()
		out, err := runPinaxWithRepoEnv(t, pinaxBin, home, args...)
		if err != nil {
			t.Fatalf("pinax %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
		return parseCatalogEnvelope(t, out)
	}

	t.Run("git_source_full_install", func(t *testing.T) {
		gitRepo := buildGitFixture(t, testfixture.RepositoryOptions{
			RepositoryID: "gitofficial", PackageID: "writing", SolutionID: "outline",
			Title: "中文大纲助手", Body: "GIT-SENTINEL-BODY 请输出大纲 {{subject}}",
			WithContract: true, License: "internal", Permissions: []string{"local_import"},
			Inputs: []promptrepo.InputDefinition{{Name: "subject", Type: promptrepo.InputTypeString, Required: true}},
		})
		source := "git+" + (&url.URL{Scheme: "file", Path: gitRepo}).String()
		runJSON("prompt", "repository", "add", "gitofficial", "--source", source, "--revision", "main", "--json")
		runJSON("prompt", "repository", "sync", "gitofficial", "--json")

		search := runJSON("prompt", "catalog", "search", "大纲", "--json")
		if search["facts"].(map[string]any)["results"] != "1" {
			t.Fatalf("git search facts = %#v", search["facts"])
		}
		ref := "promptrepo://gitofficial/writing/outline@1.0.0?locale=zh-CN"
		vault := t.TempDir()
		values := filepath.Join(t.TempDir(), "values.json")
		if err := os.WriteFile(values, []byte(`{"subject":"推理小说"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		preview := runJSON("prompt", "catalog", "preview", ref, "--values", values, "--json")
		if preview["facts"].(map[string]any)["ready"] != "true" {
			t.Fatalf("git preview facts = %#v", preview["facts"])
		}
		install := runJSON("prompt", "catalog", "install", ref, "--vault", vault, "--yes", "--json")
		if install["facts"].(map[string]any)["prompt_asset_id"] != "catalog_writing_outline" {
			t.Fatalf("git install facts = %#v", install["facts"])
		}

		// Offline cache: the bare clone lives in the shared cache, so reads
		// keep working after the origin disappears.
		if err := os.Rename(gitRepo, gitRepo+".away"); err != nil {
			t.Fatal(err)
		}
		offline := runJSON("prompt", "catalog", "preview", ref, "--values", values, "--json")
		if offline["facts"].(map[string]any)["ready"] != "true" {
			t.Fatalf("offline git preview facts = %#v", offline["facts"])
		}
	})

	t.Run("s3_source_sync_and_inspect", func(t *testing.T) {
		options := testfixture.RepositoryOptions{
			RepositoryID: "bucketofficial", PackageID: "video", SolutionID: "storyboard",
			Title: "中文分镜师", Body: "S3-SENTINEL-BODY {{subject}}",
			WithContract: true, License: "internal", Permissions: []string{"preview"},
			Inputs: []promptrepo.InputDefinition{{Name: "subject", Type: promptrepo.InputTypeString, Required: true}},
		}
		fixtureRoot := t.TempDir()
		if _, err := testfixture.WriteRepository(filepath.Join(fixtureRoot, "repo"), options); err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			serveFixtureFile(t, writer, request, fixtureRoot, "repo")
		}))
		defer server.Close()
		source := "s3://prompt-bucket/catalog?endpoint=" + url.QueryEscape(server.URL) + "&path_style=true"
		runJSON("prompt", "repository", "add", "bucketofficial", "--source", source, "--json")
		runJSON("prompt", "repository", "sync", "bucketofficial", "--json")
		search := runJSON("prompt", "catalog", "search", "分镜", "--json")
		if search["facts"].(map[string]any)["results"] != "1" {
			t.Fatalf("s3 search facts = %#v", search["facts"])
		}
		ref := "promptrepo://bucketofficial/video/storyboard@1.0.0?locale=zh-CN"
		inspect := runJSON("prompt", "catalog", "inspect", ref, "--json")
		if inspect["facts"].(map[string]any)["contract_verified"] != "true" {
			t.Fatalf("s3 inspect facts = %#v", inspect["facts"])
		}
	})

	t.Run("quarantine_keeps_last_good_snapshot", func(t *testing.T) {
		root := t.TempDir()
		options := testfixture.RepositoryOptions{
			RepositoryID: "quarantined", PackageID: "audio", SolutionID: "intro",
			Title: "隔离库旁白", WithContract: false,
		}
		source, err := testfixture.WriteRepository(filepath.Join(root, "repo"), options)
		if err != nil {
			t.Fatal(err)
		}
		runJSON("prompt", "repository", "add", "quarantined", "--source", source, "--json")
		runJSON("prompt", "repository", "sync", "quarantined", "--json")
		before := runJSON("prompt", "catalog", "search", "隔离", "--json")
		if before["facts"].(map[string]any)["results"] != "1" {
			t.Fatalf("pre-quarantine search facts = %#v", before["facts"])
		}

		// Tamper with the catalog digest: the repository must be quarantined
		// at sync time and keep serving the last good snapshot.
		catalogPath := filepath.Join(root, "repo", "catalog.json")
		payload, err := os.ReadFile(catalogPath)
		if err != nil {
			t.Fatal(err)
		}
		tampered := strings.Replace(string(payload), `"digest":"sha256:`, `"digest":"sha256:ff`, 1)
		if tampered == string(payload) {
			t.Fatal("fixture catalog is not tamperable")
		}
		if err := os.WriteFile(catalogPath, []byte(tampered), 0o600); err != nil {
			t.Fatal(err)
		}
		syncEnvelope := runJSON("prompt", "repository", "sync", "quarantined", "--json")
		syncFacts := syncEnvelope["facts"].(map[string]any)
		if syncFacts["failed"] != "1" || syncFacts["synced"] != "0" {
			t.Fatalf("tampered sync facts = %#v", syncFacts)
		}
		receipt := syncEnvelope["data"].(map[string]any)["receipt"].(map[string]any)
		results := receipt["results"].([]any)
		if len(results) != 1 || results[0].(map[string]any)["state"] != "quarantined" || results[0].(map[string]any)["error_code"] != "DIGEST_MISMATCH" {
			t.Fatalf("tampered sync receipt = %#v", receipt)
		}
		after := runJSON("prompt", "catalog", "search", "隔离", "--json")
		if after["facts"].(map[string]any)["results"] != "1" {
			t.Fatalf("quarantined repository must keep its last good snapshot: %#v", after["facts"])
		}
	})

	t.Run("scope_policy_pin_and_deny", func(t *testing.T) {
		homeScoped := t.TempDir()
		pin := func(args ...string) (map[string]any, error) {
			out, err := runPinaxWithRepoEnv(t, pinaxBin, homeScoped, args...)
			return parseCatalogEnvelope(t, out), err
		}
		for _, name := range []string{"alpha", "beta"} {
			root := t.TempDir()
			options := testfixture.RepositoryOptions{
				RepositoryID: name, PackageID: "scope", SolutionID: "probe-" + name,
				Title: "范围探测 " + name, WithContract: false,
			}
			source, err := testfixture.WriteRepository(filepath.Join(root, "repo"), options)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pin("prompt", "repository", "add", name, "--source", source, "--json"); err != nil {
				t.Fatal(err)
			}
			if _, err := pin("prompt", "repository", "sync", name, "--json"); err != nil {
				t.Fatal(err)
			}
		}
		both, err := pin("prompt", "catalog", "search", "范围", "--json")
		if err != nil || both["facts"].(map[string]any)["results"] != "2" {
			t.Fatalf("unscoped search envelope = %#v err=%v", both, err)
		}
		pinned, err := pin("prompt", "catalog", "search", "范围", "--repository", "alpha", "--json")
		if err != nil || pinned["facts"].(map[string]any)["results"] != "1" {
			t.Fatalf("pinned search envelope = %#v err=%v", pinned, err)
		}
		denied, err := pin("prompt", "catalog", "search", "范围", "--deny", "alpha", "--json")
		if err != nil || denied["facts"].(map[string]any)["results"] != "1" {
			t.Fatalf("denied search envelope = %#v err=%v", denied, err)
		}
		if denied["facts"].(map[string]any)["scope_denied"] != "1" {
			t.Fatalf("deny fact missing: %#v", denied["facts"])
		}
	})
}

func serveFixtureFile(t *testing.T, writer http.ResponseWriter, request *http.Request, fixtureRoot, repoDir string) {
	t.Helper()
	relative := strings.TrimPrefix(request.URL.Path, "/prompt-bucket/catalog/")
	if relative == "" {
		http.NotFound(writer, request)
		return
	}
	payload, err := os.ReadFile(filepath.Join(fixtureRoot, repoDir, relative))
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("ETag", `"fixture-etag"`)
	writer.Header().Set("Content-Type", "application/octet-stream")
	if _, err := writer.Write(payload); err != nil {
		t.Logf("serve fixture: %v", err)
	}
}

// buildGitFixture creates a local Git repository whose root carries the
// fixture catalog, prompts, and contract companion.
func buildGitFixture(t *testing.T, options testfixture.RepositoryOptions) string {
	t.Helper()
	gitRepo := t.TempDir()
	if _, err := testfixture.WriteRepository(gitRepo, options); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = gitRepo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=pinax-e2e",
			"GIT_AUTHOR_EMAIL=e2e@pinax.invalid",
			"GIT_COMMITTER_NAME=pinax-e2e",
			"GIT_COMMITTER_EMAIL=e2e@pinax.invalid",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	git("init", "--initial-branch=main", ".")
	git("add", ".")
	git("commit", "--no-gpg-sign", "-m", "prompt catalog fixture")
	return gitRepo
}
