package pinaxclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenFileLoadsSecretOnlyIntoAuthorizationHeader(t *testing.T) {
	t.Parallel()

	const secret = "pinax-owner-token-file-secret"
	tokenPath := secureTokenFixture(t, secret+"\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer "+secret {
			t.Errorf("Authorization header = %q", got)
		}
		writeTestProjection(t, w, "api.manifest", map[string]any{"manifest": Manifest{
			SchemaVersion: TransportManifestSchemaV1,
			Digest:        "sha256:" + strings.Repeat("a", 64),
		}})
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, TokenFile: tokenPath})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Manifest(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTokenFileRejectsUnsafePathsPermissionsAndContents(t *testing.T) {
	t.Parallel()

	secure := secureTokenFixture(t, "valid-token")
	large := secureTokenFixture(t, strings.Repeat("x", int(maxTokenFileBytes)+1))
	empty := secureTokenFixture(t, " \n")
	multiple := secureTokenFixture(t, "first second")
	directory := t.TempDir()
	permissive := secureTokenFixture(t, "valid-token")
	if err := os.Chmod(permissive, 0o640); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(t.TempDir(), "token-link")
	if err := os.Symlink(secure, symlink); err != nil {
		t.Fatal(err)
	}
	unsafeParent := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(unsafeParent, 0o770); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafeParent, 0o770); err != nil {
		t.Fatal(err)
	}
	unsafeAncestor := filepath.Join(unsafeParent, "token")
	if err := os.WriteFile(unsafeAncestor, []byte("valid-token"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		code string
	}{
		{name: "relative", path: filepath.Base(secure), code: CodeTokenFileUnsafe},
		{name: "unclean", path: filepath.Dir(secure) + string(filepath.Separator) + "." + string(filepath.Separator) + filepath.Base(secure), code: CodeTokenFileUnsafe},
		{name: "directory", path: directory, code: CodeTokenFileUnsafe},
		{name: "symlink", path: symlink, code: CodeTokenFileUnsafe},
		{name: "permission", path: permissive, code: CodeTokenFileUnsafe},
		{name: "unsafe ancestor", path: unsafeAncestor, code: CodeTokenFileUnsafe},
		{name: "too large", path: large, code: CodeTokenFileTooLarge},
		{name: "empty", path: empty, code: CodeTokenFileInvalid},
		{name: "multiple values", path: multiple, code: CodeTokenFileInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadTokenFile(test.path)
			var clientErr *Error
			if !errors.As(err, &clientErr) || clientErr.Code != test.code {
				t.Fatalf("LoadTokenFile error = %#v", err)
			}
			if filepath.IsAbs(test.path) && strings.Contains(clientErr.Error(), test.path) {
				t.Fatalf("error leaked path: %s", clientErr)
			}
		})
	}
}

func TestTokenFileRejectsSymlinkAncestor(t *testing.T) {
	t.Parallel()

	realDirectory := t.TempDir()
	tokenPath := filepath.Join(realDirectory, "token")
	if err := os.WriteFile(tokenPath, []byte("valid-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkParent := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(realDirectory, linkParent); err != nil {
		t.Fatal(err)
	}
	_, err := LoadTokenFile(filepath.Join(linkParent, "token"))
	var clientErr *Error
	if !errors.As(err, &clientErr) || clientErr.Code != CodeTokenFileUnsafe {
		t.Fatalf("LoadTokenFile error = %#v", err)
	}
}

func TestTokenFileDescriptorInodeRecheckRejectsReplacement(t *testing.T) {
	t.Parallel()

	tokenPath := secureTokenFixture(t, "first-token")
	openReplacement := func(path string) (*os.File, error) {
		oldPath := path + ".old"
		if err := os.Rename(path, oldPath); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte("replacement-token"), 0o600); err != nil {
			return nil, err
		}
		return os.Open(path)
	}
	_, err := loadTokenFile(tokenPath, openReplacement)
	var clientErr *Error
	if !errors.As(err, &clientErr) || clientErr.Code != CodeTokenFileUnsafe {
		t.Fatalf("loadTokenFile replacement error = %#v", err)
	}
}

func TestTokenFileErrorsAndJSONAreRedacted(t *testing.T) {
	t.Parallel()

	const secret = "pinax-token-file-never-leak"
	tokenPath := secureTokenFixture(t, secret)
	_, err := New(Config{BaseURL: "https://example.com", Token: "inline", TokenFile: tokenPath})
	var clientErr *Error
	if !errors.As(err, &clientErr) || clientErr.Code != CodeInvalidConfig {
		t.Fatalf("New conflict error = %#v", err)
	}
	encoded, marshalErr := json.Marshal(clientErr)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	for _, output := range []string{clientErr.Error(), string(encoded)} {
		if strings.Contains(output, secret) || strings.Contains(output, tokenPath) || strings.Contains(output, "Authorization") {
			t.Fatalf("credential error leaked sensitive input: %s", output)
		}
	}
}

func secureTokenFixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
