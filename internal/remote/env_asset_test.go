package remote

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDotenv_AcceptsSafeSubset(t *testing.T) {
	cases := map[string]string{
		"bare":        "KEY=value\n",
		"equals":      `KEY=a=b=c`,
		"doubleQuote": `KEY="quoted value"`,
		"singleQuote": "KEY='quoted value'",
		"spaces":      "  KEY = value  \n",
		"comment":     "# header\nKEY=value\n",
		"blank":       "\n\nKEY=value\n\n",
		"spacesValue": `KEY="hello world"`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			vals, err := ParseDotenv([]byte(body))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(vals) != 1 {
				t.Fatalf("expected 1 key, got %d: %v", len(vals), vals)
			}
		})
	}
}

func TestParseDotenv_RejectsShellVectors(t *testing.T) {
	cases := map[string]string{
		"commandSub":    `KEY=$(whoami)`,
		"backtick":      "KEY=`whoami`",
		"braceExpand":   `KEY=${OTHER}`,
		"braceExpandQ":  `KEY="${OTHER}"`,
		"include":       "include other.env\n",
		"source":        "source other.env\n",
		"export":        "export KEY=value\n",
		"dotSource":     ". other.env\n",
		"controlChar":   "KEY=value\x01\n",
		"nul":           "KEY=value\x00\n",
		"duplicate":     "KEY=a\nKEY=b\n",
		"emptyKey":      "=value\n",
		"missingEquals": "just-a-key\n",
		"unterminatedD": `KEY="unterminated`,
		"unterminatedS": "KEY='unterminated",
		"invalidKey":    "1KEY=value\n",
		"invalidKey2":   "KEY-NAME=value\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseDotenv([]byte(body))
			if err == nil {
				t.Fatalf("expected error for %q, got nil", name)
			}
			var derr *DotenvError
			if !errors.As(err, &derr) {
				t.Fatalf("expected *DotenvError, got %T: %v", err, err)
			}
			if derr.Line == 0 {
				t.Fatalf("expected non-zero line number, got 0")
			}
			// Errors must not disclose the rejected secret payload. The control
			// byte / shell command body must never appear in the error text.
			leaked := []string{"whoami", "OTHER"}
			for _, leak := range leaked {
				if strings.Contains(err.Error(), leak) {
					t.Fatalf("error leaked value %q: %v", leak, err)
				}
			}
		})
	}
}

func TestParseDotenv_ErrorReportsLineAndKey(t *testing.T) {
	_, err := ParseDotenv([]byte("A=1\nB=2\nB=3\n"))
	var derr *DotenvError
	if !errors.As(err, &derr) {
		t.Fatalf("expected *DotenvError, got %T", err)
	}
	if derr.Code != "dotenv_duplicate_key" {
		t.Fatalf("expected dotenv_duplicate_key, got %s", derr.Code)
	}
	if derr.Line != 3 {
		t.Fatalf("expected line 3, got %d", derr.Line)
	}
	if derr.Key != "B" {
		t.Fatalf("expected key B, got %s", derr.Key)
	}
}

func TestFormatDotenv_RoundTrip(t *testing.T) {
	original := map[string]string{
		"Z_KEY":   "last",
		"A_KEY":   "first",
		"SPECIAL": "has spaces and # hash",
		"QUOTE":   "has 'single' quote",
	}
	data := FormatDotenv(original)
	parsed, err := ParseDotenv(data)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	for k, v := range original {
		if parsed[k] != v {
			t.Fatalf("round-trip mismatch for %q: got %q want %q", k, parsed[k], v)
		}
	}
	// Verify stable ordering: A_KEY before Z_KEY.
	aIdx := strings.Index(string(data), "A_KEY=")
	zIdx := strings.Index(string(data), "Z_KEY=")
	if aIdx < 0 || zIdx < 0 || aIdx > zIdx {
		t.Fatalf("expected sorted output, data=%q", string(data))
	}
}

func TestEnvDocumentDigest_Stable(t *testing.T) {
	d1 := EnvDocumentDigest([]byte("A=1\nB=2\n"))
	d2 := EnvDocumentDigest([]byte("A=1\nB=2\n"))
	if d1 != d2 {
		t.Fatalf("expected equal digests")
	}
	d3 := EnvDocumentDigest([]byte("A=1\nB=3\n"))
	if d1 == d3 {
		t.Fatalf("expected different digests for different content")
	}
}

func TestSaveLoadEnvAsset_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	asset := EnvAsset{
		Provider:   "fake",
		Ciphertext: "dGVzdA==",
		Digest:     "abc123",
		KeyNames:   []string{"COS_KEY", "COS_SECRET"},
	}
	if err := SaveEnvAsset(tmp, asset); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := LoadEnvAsset(tmp)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Provider != "fake" || loaded.Ciphertext != "dGVzdA==" {
		t.Fatalf("round-trip mismatch: %+v", loaded)
	}
	if loaded.SchemaVersion != EnvAssetSchemaVersion {
		t.Fatalf("expected schema %s, got %s", EnvAssetSchemaVersion, loaded.SchemaVersion)
	}
	// File permissions must be restrictive.
	info, err := os.Stat(EnvAssetPath(tmp))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600, got %v", info.Mode().Perm())
	}
}

func TestLoadEnvAsset_Missing(t *testing.T) {
	tmp := t.TempDir()
	_, err := LoadEnvAsset(tmp)
	if !errors.Is(err, ErrEnvAssetMissing) {
		t.Fatalf("expected ErrEnvAssetMissing, got %v", err)
	}
}

func TestSaveEnvAsset_RejectsMissingFields(t *testing.T) {
	tmp := t.TempDir()
	cases := []struct {
		name  string
		asset EnvAsset
		code  string
	}{
		{"no_provider", EnvAsset{Ciphertext: "x"}, "missing_provider"},
		{"no_ciphertext", EnvAsset{Provider: "fake"}, "missing_ciphertext"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := SaveEnvAsset(tmp, tc.asset)
			var ae *EnvAssetError
			if !errors.As(err, &ae) {
				t.Fatalf("expected *EnvAssetError, got %T", err)
			}
			if ae.Code != tc.code {
				t.Fatalf("expected code %s, got %s", tc.code, ae.Code)
			}
		})
	}
}

func TestLoadEnvAsset_UnsupportedSchema(t *testing.T) {
	tmp := t.TempDir()
	path := EnvAssetPath(tmp)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("schema_version: pinax.sync.env.v999\nprovider: fake\nciphertext: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadEnvAsset(tmp)
	var ae *EnvAssetError
	if !errors.As(err, &ae) || ae.Code != "unsupported_env_schema" {
		t.Fatalf("expected unsupported_env_schema, got %v", err)
	}
}
