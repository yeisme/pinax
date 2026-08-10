package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	defer func() { _ = os.Setenv("XDG_CONFIG_HOME", origXDG) }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("expected empty profiles, got %d", len(cfg.Profiles))
	}
}

func TestLoadAWSStaticCredentials(t *testing.T) {
	path := writeAWSCredentialsFixture(t, `[cos]
aws_access_key_id = AKID-MIGRATE
aws_secret_access_key = SECRET-MIGRATE
aws_session_token = SESSION-MIGRATE
`)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", path)
	credentials, err := LoadAWSStaticCredentials("cos")
	if err != nil {
		t.Fatalf("load credentials: %v", err)
	}
	if credentials.AccessKeyID != "AKID-MIGRATE" || credentials.SecretAccessKey != "SECRET-MIGRATE" || credentials.SessionToken != "SESSION-MIGRATE" {
		t.Fatalf("credentials differ: %#v", credentials)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	defer func() { _ = os.Setenv("XDG_CONFIG_HOME", origXDG) }()

	cfg := &ProfilesConfig{
		Profiles: map[string]Profile{
			"test-s3": {
				Endpoint:  "s3://my-bucket/prefix",
				Workspace: "default",
				Device:    "laptop",
				SecretRef: "env://MY_SECRET",
			},
		},
	}
	cfg.Defaults.Profile = "test-s3"

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(loaded.Profiles))
	}
	p, ok := loaded.Profiles["test-s3"]
	if !ok {
		t.Fatal("expected test-s3 profile")
	}
	if p.Endpoint != "s3://my-bucket/prefix" {
		t.Fatalf("expected endpoint, got %s", p.Endpoint)
	}
	if p.SecretRef != "env://MY_SECRET" {
		t.Fatalf("expected secret_ref, got %s", p.SecretRef)
	}
	if loaded.Defaults.Profile != "test-s3" {
		t.Fatalf("expected default profile, got %s", loaded.Defaults.Profile)
	}
}

func TestSave_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "nonexistent"))
	defer func() { _ = os.Setenv("XDG_CONFIG_HOME", origXDG) }()

	cfg := &ProfilesConfig{Profiles: make(map[string]Profile)}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

func TestResolveSecretRef_Plain(t *testing.T) {
	val, err := ResolveSecretRef("plain:mysecret")
	if err != nil {
		t.Fatalf("ResolveSecretRef: %v", err)
	}
	if val != "mysecret" {
		t.Fatalf("expected mysecret, got %s", val)
	}
}

func TestResolveSecretRef_Env(t *testing.T) {
	_ = os.Setenv("TEST_PINAX_SECRET", "env-value-123")
	defer func() { _ = os.Unsetenv("TEST_PINAX_SECRET") }()

	val, err := ResolveSecretRef("env://TEST_PINAX_SECRET")
	if err != nil {
		t.Fatalf("ResolveSecretRef: %v", err)
	}
	if val != "env-value-123" {
		t.Fatalf("expected env-value-123, got %s", val)
	}
}

func TestResolveSecretRef_EnvNotSet(t *testing.T) {
	_, err := ResolveSecretRef("env://NONEXISTENT_PINAX_VAR_12345")
	if err == nil {
		t.Fatal("expected error for unset env var")
	}
}

func TestResolveSecretRef_Empty(t *testing.T) {
	val, err := ResolveSecretRef("")
	if err != nil {
		t.Fatalf("ResolveSecretRef: %v", err)
	}
	if val != "" {
		t.Fatalf("expected empty, got %s", val)
	}
}

func TestResolveSecretRef_Passthrough(t *testing.T) {
	val, err := ResolveSecretRef("some-random-value")
	if err != nil {
		t.Fatalf("ResolveSecretRef: %v", err)
	}
	if val != "some-random-value" {
		t.Fatalf("expected passthrough, got %s", val)
	}
}

func TestResolveTarget_URI(t *testing.T) {
	ep, ws, dev, sr, err := ResolveTarget("s3://bucket/path")
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if ep != "s3://bucket/path" {
		t.Fatalf("expected s3 URI, got %s", ep)
	}
	if ws != "" || dev != "" || sr != "" {
		t.Fatalf("expected empty workspace/device/secretRef")
	}
}

func TestResolveTarget_KnownTargets(t *testing.T) {
	for _, target := range []string{"cloud", "git", "s3"} {
		ep, _, _, _, err := ResolveTarget(target)
		if err != nil {
			t.Fatalf("ResolveTarget(%s): %v", target, err)
		}
		if ep != target {
			t.Fatalf("expected %s, got %s", target, ep)
		}
	}
}

func TestResolveTarget_ProfileName(t *testing.T) {
	dir := t.TempDir()
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	defer func() { _ = os.Setenv("XDG_CONFIG_HOME", origXDG) }()

	cfg := &ProfilesConfig{
		Profiles: map[string]Profile{
			"my-remote": {
				Endpoint:     "https://cloud.example.com",
				Workspace:    "ws-123",
				Device:       "laptop",
				SecretRef:    "env://MY_TOKEN",
				DefaultScope: "read",
			},
		},
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ep, ws, dev, sr, err := ResolveTarget("my-remote")
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if ep != "https://cloud.example.com" {
		t.Fatalf("expected endpoint, got %s", ep)
	}
	if ws != "ws-123" {
		t.Fatalf("expected ws-123, got %s", ws)
	}
	if dev != "laptop" {
		t.Fatalf("expected laptop, got %s", dev)
	}
	if sr != "env://MY_TOKEN" {
		t.Fatalf("expected env://MY_TOKEN, got %s", sr)
	}
}

func TestResolveTarget_UnknownPassthrough(t *testing.T) {
	dir := t.TempDir()
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	defer func() { _ = os.Setenv("XDG_CONFIG_HOME", origXDG) }()

	ep, _, _, _, err := ResolveTarget("unknown-target")
	if err != nil {
		t.Fatalf("ResolveTarget: %v", err)
	}
	if ep != "unknown-target" {
		t.Fatalf("expected passthrough, got %s", ep)
	}
}

func TestResolveTarget_Empty(t *testing.T) {
	_, _, _, _, err := ResolveTarget("")
	if err == nil {
		t.Fatal("expected error for empty target")
	}
}

func TestProfilesPath(t *testing.T) {
	dir := t.TempDir()
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	defer func() { _ = os.Setenv("XDG_CONFIG_HOME", origXDG) }()

	expected := filepath.Join(dir, "pinax", "profiles.yaml")
	got := ProfilesPath()
	if got != expected {
		t.Fatalf("expected %s, got %s", expected, got)
	}
}

const awsCredentialsFixture = `[default]
aws_access_key_id = AKIADEFAULT
aws_secret_access_key = default-secret

[tencent-cos-pinax]
# inline comment is ignored
aws_access_key_id = AKIATENCENT
aws_secret_access_key = cos-pinax-secret-value
region = ap-shanghai

[other-profile]
aws_access_key_id = AKIAOTHER
aws_secret_access_key = other-secret
`

func writeAWSCredentialsFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestResolveSecretRef_Profile(t *testing.T) {
	path := writeAWSCredentialsFixture(t, awsCredentialsFixture)
	orig := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	_ = os.Setenv("AWS_SHARED_CREDENTIALS_FILE", path)
	defer func() { _ = os.Setenv("AWS_SHARED_CREDENTIALS_FILE", orig) }()

	got, err := ResolveSecretRef("profile://tencent-cos-pinax")
	if err != nil {
		t.Fatalf("resolve profile: %v", err)
	}
	if got != "cos-pinax-secret-value" {
		t.Fatalf("resolved secret = %q, want cos-pinax-secret-value", got)
	}

	// default profile still resolves independently.
	def, err := ResolveSecretRef("profile://default")
	if err != nil {
		t.Fatalf("resolve default profile: %v", err)
	}
	if def != "default-secret" {
		t.Fatalf("default secret = %q", def)
	}
}

func TestResolveSecretRef_ProfileMissing(t *testing.T) {
	path := writeAWSCredentialsFixture(t, awsCredentialsFixture)
	orig := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	_ = os.Setenv("AWS_SHARED_CREDENTIALS_FILE", path)
	defer func() { _ = os.Setenv("AWS_SHARED_CREDENTIALS_FILE", orig) }()

	if _, err := ResolveSecretRef("profile://does-not-exist"); err == nil {
		t.Fatal("expected error for missing profile")
	}
}

func TestResolveSecretRef_ProfileFileUnreadable(t *testing.T) {
	orig := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	_ = os.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "missing-credentials"))
	defer func() { _ = os.Setenv("AWS_SHARED_CREDENTIALS_FILE", orig) }()

	if _, err := ResolveSecretRef("profile://anything"); err == nil {
		t.Fatal("expected error when credentials file is unreadable")
	}
}

func TestResolveSecretRef_ProfileUnchangedPaths(t *testing.T) {
	// env:// and plain: schemes must keep working alongside profile://.
	origEnv := os.Getenv("PINAX_TEST_SECRET")
	_ = os.Setenv("PINAX_TEST_SECRET", "env-value")
	defer func() { _ = os.Setenv("PINAX_TEST_SECRET", origEnv) }()

	if got, err := ResolveSecretRef("env://PINAX_TEST_SECRET"); err != nil || got != "env-value" {
		t.Fatalf("env:// resolution = %q, err=%v", got, err)
	}
	if got, err := ResolveSecretRef("plain:raw-secret"); err != nil || got != "raw-secret" {
		t.Fatalf("plain: resolution = %q, err=%v", got, err)
	}
}

func TestEnsureStoredSecretPersistsWithPrivatePermissions(t *testing.T) {
	dir := t.TempDir()
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	defer func() { _ = os.Setenv("XDG_CONFIG_HOME", origXDG) }()

	ref, err := EnsureStoredSecret("capsa-sync-test")
	if err != nil {
		t.Fatalf("EnsureStoredSecret: %v", err)
	}
	if ref != "stored://capsa-sync-test" {
		t.Fatalf("ref = %q", ref)
	}
	first, err := ResolveSecretRef(ref)
	if err != nil {
		t.Fatalf("ResolveSecretRef: %v", err)
	}
	secondRef, err := EnsureStoredSecret("capsa-sync-test")
	if err != nil || secondRef != ref {
		t.Fatalf("second EnsureStoredSecret: ref=%q err=%v", secondRef, err)
	}
	second, err := ResolveSecretRef(ref)
	if err != nil || first != second || first == "" {
		t.Fatalf("stored secret changed: first=%q second=%q err=%v", first, second, err)
	}
	info, err := os.Stat(SecretsPath())
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secrets permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestResolveSecretRef_StoredMissing(t *testing.T) {
	dir := t.TempDir()
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	defer func() { _ = os.Setenv("XDG_CONFIG_HOME", origXDG) }()

	if _, err := ResolveSecretRef("stored://missing"); err == nil {
		t.Fatal("expected missing stored secret error")
	}
}
