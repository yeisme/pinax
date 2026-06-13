package vaultregistry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryRoundTripAndResolveSelector(t *testing.T) {
	root := t.TempDir()
	paths := Paths{ConfigDir: filepath.Join(root, "config"), CacheDir: filepath.Join(root, "cache")}
	vault := filepath.Join(root, "work")
	if err := RegisterLocal(paths, "work", vault, true); err != nil {
		t.Fatalf("register local: %v", err)
	}
	registry, err := LoadRegistry(paths)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if registry.Default != "work" || registry.Locals["work"].Path != vault {
		t.Fatalf("registry = %#v", registry)
	}
	resolved, info, err := ResolveSelector(paths, "work")
	if err != nil {
		t.Fatalf("resolve selector: %v", err)
	}
	if resolved != vault || info.Kind != "local" || info.Name != "work" {
		t.Fatalf("resolved=%q info=%#v", resolved, info)
	}
	pathResolved, info, err := ResolveSelector(paths, "./notes")
	if err != nil || pathResolved != "./notes" || info.Kind != "path" {
		t.Fatalf("path resolved=%q info=%#v err=%v", pathResolved, info, err)
	}
}

func TestRemoteCacheRefreshParsesProjectionAndRedacts(t *testing.T) {
	root := t.TempDir()
	paths := Paths{ConfigDir: filepath.Join(root, "config"), CacheDir: filepath.Join(root, "cache")}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/vaults" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret-value" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"spec_version": "1.0",
			"mode":         "json",
			"command":      "vault.remote.list",
			"status":       "success",
			"data":         map[string]any{"vaults": []map[string]any{{"id": "team", "label": "Team", "workspace": "ws", "selector": "cloud:team", "revision": "rev1"}}},
		})
	}))
	defer server.Close()

	entry, err := RefreshRemote(paths, RemoteRefreshRequest{Profile: "cloud", Endpoint: server.URL, Workspace: "ws", Token: "secret-value"})
	if err != nil {
		t.Fatalf("refresh remote: %v", err)
	}
	if entry.Profile != "cloud" || len(entry.Vaults) != 1 || entry.Vaults[0].Selector != "cloud:team" {
		t.Fatalf("entry = %#v", entry)
	}
	b, err := ReadCacheBytes(paths)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	if stringContains(string(b), "secret-value") || stringContains(string(b), "Authorization") {
		t.Fatalf("cache leaked secret: %s", string(b))
	}
	items, err := CompletionItems(paths)
	if err != nil {
		t.Fatalf("completion items: %v", err)
	}
	if !sliceHasPrefix(items, "cloud:team\tremote vault profile=cloud workspace=ws") {
		t.Fatalf("completion items = %#v", items)
	}

	pathServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/vaults" {
			t.Fatalf("path-prefixed endpoint path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"vaults": []map[string]any{{"id": "path-team"}}})
	}))
	defer pathServer.Close()
	if _, err := RefreshRemote(paths, RemoteRefreshRequest{Profile: "path-cloud", Endpoint: pathServer.URL + "/api"}); err != nil {
		t.Fatalf("refresh path-prefixed endpoint: %v", err)
	}
}

func stringContains(s, sub string) bool {
	return len(sub) == 0 || (len(sub) <= len(s) && contains(s, sub))
}
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
func sliceHasPrefix(values []string, want string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, want) {
			return true
		}
	}
	return false
}
