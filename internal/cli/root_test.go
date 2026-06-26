package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestNewRootCommandFactoryIsolatedState(t *testing.T) {
	first := NewRootCommand("test-one")
	var firstOut bytes.Buffer
	first.SetOut(&firstOut)
	first.SetArgs([]string{"version", "--json"})
	if err := first.Execute(); err != nil {
		t.Fatalf("execute first: %v", err)
	}
	if !strings.Contains(firstOut.String(), "test-one") || !strings.Contains(firstOut.String(), `"mode":"json"`) {
		t.Fatalf("first output = %s", firstOut.String())
	}

	second := NewRootCommand("test-two")
	var secondOut bytes.Buffer
	second.SetOut(&secondOut)
	second.SetArgs([]string{"version", "--agent"})
	if err := second.Execute(); err != nil {
		t.Fatalf("execute second: %v", err)
	}
	if !strings.Contains(secondOut.String(), "test-two") || !strings.Contains(secondOut.String(), "mode=agent") || strings.Contains(secondOut.String(), `"mode":"json"`) {
		t.Fatalf("second output = %s", secondOut.String())
	}
}

func TestRemoteModeUsesConfiguredAPIURL(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "xdg")
	var seenMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rpc" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var rpc struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&rpc); err != nil {
			t.Fatalf("decode rpc: %v", err)
		}
		seenMethod = rpc.Method
		_ = json.NewEncoder(w).Encode(domain.NewProjection("folder.list", "remote folders listed"))
	}))
	defer server.Close()
	writeCLITestFile(t, filepath.Join(xdg, "pinax", "config.yaml"), "remote:\n  api_url: "+server.URL+"\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("PINAX_API_URL", "")
	t.Setenv("NO_COLOR", "")

	cmd := NewRootCommand("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"folder", "list", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote folder list: %v\n%s", err, out.String())
	}
	if seenMethod != "Pinax.Folder.List" || !strings.Contains(out.String(), `"command":"folder.list"`) {
		t.Fatalf("seen method=%q output=%s", seenMethod, out.String())
	}
}

func TestRemoteModeMapsProjectSubprojectCommands(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "xdg")
	var seenMethod string
	var seenParams map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rpc" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var rpc struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&rpc); err != nil {
			t.Fatalf("decode rpc: %v", err)
		}
		seenMethod = rpc.Method
		seenParams = rpc.Params
		_ = json.NewEncoder(w).Encode(domain.NewProjection("project.subproject.list", "remote subprojects listed"))
	}))
	defer server.Close()
	writeCLITestFile(t, filepath.Join(xdg, "pinax", "config.yaml"), "remote:\n  api_url: "+server.URL+"\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("PINAX_API_URL", "")

	cmd := NewRootCommand("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"project", "subproject", "list", "research", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote subproject list: %v\n%s", err, out.String())
	}
	if seenMethod != "Pinax.Project.Subproject.List" || seenParams["project"] != "research" || !strings.Contains(out.String(), `"command":"project.subproject.list"`) {
		t.Fatalf("seen method=%q params=%#v output=%s", seenMethod, seenParams, out.String())
	}
}

func TestRemoteModeMapsNoteListFlags(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "xdg")
	var seenMethod string
	var seenParams map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rpc" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var rpc struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&rpc); err != nil {
			t.Fatalf("decode rpc: %v", err)
		}
		seenMethod = rpc.Method
		seenParams = rpc.Params
		_ = json.NewEncoder(w).Encode(domain.NewProjection("note.list", "remote notes listed"))
	}))
	defer server.Close()
	writeCLITestFile(t, filepath.Join(xdg, "pinax", "config.yaml"), "remote:\n  api_url: "+server.URL+"\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("PINAX_API_URL", "")
	t.Setenv("NO_COLOR", "")

	cmd := NewRootCommand("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"note", "list", "--tag", "research,go", "--project", "work", "--status", "active", "--limit", "5", "--sort", "updated", "--property", "priority", "--strict-properties", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote note list: %v\n%s", err, out.String())
	}
	if seenMethod != "Pinax.Note.List" || !strings.Contains(out.String(), `"command":"note.list"`) {
		t.Fatalf("seen method=%q output=%s", seenMethod, out.String())
	}
	if seenParams["status"] != "active" || seenParams["project"] != "work" || seenParams["group"] != "work" || seenParams["sort"] != "updated" || seenParams["limit"] != float64(5) || seenParams["strict_properties"] != true {
		t.Fatalf("note list params = %#v", seenParams)
	}
	tags, ok := seenParams["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "research" || tags[1] != "go" {
		t.Fatalf("note list tags = %#v", seenParams["tags"])
	}
	properties, ok := seenParams["properties"].([]any)
	if !ok || len(properties) != 1 || properties[0] != "priority" {
		t.Fatalf("note list properties = %#v", seenParams["properties"])
	}
}
func TestRemoteModeMapsCreateApprovalPreviewFlags(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "xdg")
	seen := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rpc" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var rpc struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&rpc); err != nil {
			t.Fatalf("decode rpc: %v", err)
		}
		seen[rpc.Method] = rpc.Params
		_ = json.NewEncoder(w).Encode(domain.NewProjection("note.new", "remote note previewed"))
	}))
	defer server.Close()
	writeCLITestFile(t, filepath.Join(xdg, "pinax", "config.yaml"), "remote:\n  api_url: "+server.URL+"\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("PINAX_API_URL", "")
	t.Setenv("NO_COLOR", "")

	cmd := NewRootCommand("test")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"inbox", "capture", "Remote Inbox", "--body", "body", "--dry-run", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote inbox capture: %v", err)
	}
	cmd = NewRootCommand("test")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"draft", "create", "Remote Draft", "--body", "draft body", "--yes", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("remote draft create: %v", err)
	}
	if seen["Pinax.Inbox.Capture"]["dry_run"] != true || seen["Pinax.Inbox.Capture"]["body"] != "body" {
		t.Fatalf("inbox capture params = %#v", seen["Pinax.Inbox.Capture"])
	}
	if seen["Pinax.Draft.Create"]["yes"] != true || seen["Pinax.Draft.Create"]["body"] != "draft body" {
		t.Fatalf("draft create params = %#v", seen["Pinax.Draft.Create"])
	}
}

func TestConfiguredRemoteModeLeavesConfigCommandLocal(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "xdg")
	writeCLITestFile(t, filepath.Join(xdg, "pinax", "config.yaml"), "remote:\n  api_url: http://127.0.0.1:1\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("PINAX_API_URL", "")
	t.Setenv("NO_COLOR", "")

	cmd := NewRootCommand("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"config", "get", "remote.api_url", "--agent"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config get should remain local: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "fact.value=http://127.0.0.1:1") {
		t.Fatalf("config get output = %s", out.String())
	}
}

func TestConfiguredRemoteModeLeavesCloudSyncCommandsLocal(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "xdg")
	vault := filepath.Join(root, "vault")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("cloud sync commands should not call remote API mode: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()
	writeCLITestFile(t, filepath.Join(xdg, "pinax", "config.yaml"), "remote:\n  api_url: "+server.URL+"\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("PINAX_API_URL", "")
	t.Setenv("NO_COLOR", "")

	cmd := NewRootCommand("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"cloud", "backend", "set", "s3", "--bucket", "notes", "--region", "us-east-1", "--prefix", "pinax-sync/", "--workspace", "ec", "--device", "dev", "--vault", vault, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cloud backend set should remain local: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), `"command":"cloud.backend.set"`) || !strings.Contains(out.String(), `"backend_kind":"s3-direct"`) {
		t.Fatalf("cloud backend set output = %s", out.String())
	}

	cmd = NewRootCommand("test")
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"sync", "init", "--vault", vault, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync init should remain local: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), `"command":"sync.init"`) || !strings.Contains(out.String(), `"status":"success"`) || !strings.Contains(out.String(), `"workspace":"ec"`) {
		t.Fatalf("sync init output = %s", out.String())
	}
}

func TestEnvironmentRemoteModeLeavesCloudSyncCommandsLocal(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("cloud sync commands should not call remote API mode: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()
	t.Setenv("PINAX_API_URL", server.URL)
	t.Setenv("NO_COLOR", "")

	cmd := NewRootCommand("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"cloud", "backend", "set", "s3", "--bucket", "notes", "--region", "us-east-1", "--prefix", "pinax-sync/", "--workspace", "ec", "--device", "dev", "--vault", vault, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cloud backend set should remain local with PINAX_API_URL: %v\n%s", err, out.String())
	}

	cmd = NewRootCommand("test")
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"sync", "init", "--vault", vault, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("sync init should remain local with PINAX_API_URL: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), `"status":"success"`) || !strings.Contains(out.String(), `"workspace":"ec"`) {
		t.Fatalf("sync init output = %s", out.String())
	}
}

func TestConfiguredRemoteModeRejectsUnsupportedBusinessCommand(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "xdg")
	writeCLITestFile(t, filepath.Join(xdg, "pinax", "config.yaml"), "remote:\n  api_url: http://127.0.0.1:1\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("PINAX_API_URL", "")
	t.Setenv("NO_COLOR", "")

	cmd := NewRootCommand("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"note", "tags", "--agent"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("unsupported remote command succeeded:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "error.code=remote_command_unsupported") {
		t.Fatalf("unsupported remote output = %s", out.String())
	}
}

func TestRemoteCommandCoverageClassifiesEveryVisibleRunnableCommand(t *testing.T) {
	coverage := RemoteCommandCoverage(NewRootCommand("test"))
	if len(coverage) == 0 {
		t.Fatalf("expected command coverage entries")
	}

	byPath := map[string]RemoteCommandCoverageEntry{}
	for _, entry := range coverage {
		if entry.CommandPath == "" {
			t.Fatalf("coverage entry missing command path: %#v", entry)
		}
		if entry.Status != "remote_supported" && entry.Status != "local_only" && entry.Status != "unsupported" {
			t.Fatalf("coverage entry %s has invalid status %q", entry.CommandPath, entry.Status)
		}
		if entry.Status == "remote_supported" && entry.RPCMethod == "" {
			t.Fatalf("remote-supported entry missing RPC method: %#v", entry)
		}
		if entry.Status == "local_only" && entry.Reason == "" {
			t.Fatalf("local-only entry missing reason: %#v", entry)
		}
		if entry.Status == "unsupported" && entry.Reason == "" {
			t.Fatalf("unsupported entry missing reason: %#v", entry)
		}
		byPath[entry.CommandPath] = entry
	}

	for _, want := range []struct {
		path   string
		status string
	}{
		{path: "pinax folder list", status: "remote_supported"},
		{path: "pinax note tags", status: "unsupported"},
		{path: "pinax config get", status: "local_only"},
		{path: "pinax sync init", status: "local_only"},
	} {
		entry, ok := byPath[want.path]
		if !ok {
			t.Fatalf("missing coverage for %s", want.path)
		}
		if entry.Status != want.status {
			t.Fatalf("coverage %s status = %s, want %s (entry %#v)", want.path, entry.Status, want.status, entry)
		}
	}
}

func TestConfiguredRemoteModeLeavesRootHelpLocal(t *testing.T) {
	root := t.TempDir()
	xdg := filepath.Join(root, "xdg")
	writeCLITestFile(t, filepath.Join(xdg, "pinax", "config.yaml"), "remote:\n  api_url: http://127.0.0.1:1\n")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("PINAX_API_URL", "")
	t.Setenv("NO_COLOR", "")

	cmd := NewRootCommand("test")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("root help should remain local: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Pinax manages local Markdown vault notes") {
		t.Fatalf("root help output = %s", out.String())
	}
}
