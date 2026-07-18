package app

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestDaemonServiceSlug(t *testing.T) {
	cases := []struct {
		vault string
		want  string
	}{
		{"/home/user/my-notes", "my-notes"},
		{"/home/user/My Notes", "my-notes"},
		{"/tmp/Vault (personal)", "vault-personal"},
		{"/", "vault"},
		{"", "vault"},
	}
	for _, c := range cases {
		if got := daemonServiceSlug(c.vault); got != c.want {
			t.Errorf("daemonServiceSlug(%q) = %q, want %q", c.vault, got, c.want)
		}
	}
}

func TestSystemdUnitContentHasRequiredDirectives(t *testing.T) {
	binary := "/usr/local/bin/pinax"
	vault := "/home/user/my-notes"
	content := systemdUnitContent(binary, vault, "my-notes")

	for _, needle := range []string{
		"Restart=on-failure",
		"RestartSec=10",
		"WantedBy=default.target",
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("systemd unit missing %q\n--- unit ---\n%s", needle, content)
		}
	}

	envFile := filepath.Join(vault, ".pinax", "cloud", "sync-env")
	if !strings.Contains(content, "EnvironmentFile="+envFile) {
		t.Errorf("systemd unit missing EnvironmentFile=%s\n--- unit ---\n%s", envFile, content)
	}

	// The capsa-sync- prefix is part of the unit filename, not the body.
	unitPath := systemdUnitPath("/home/user", "my-notes")
	if !strings.HasSuffix(unitPath, "capsa-sync-my-notes.service") {
		t.Errorf("unit path = %q, want suffix capsa-sync-my-notes.service", unitPath)
	}

	execStart := "ExecStart=" + binary + " sync daemon run --target capsa --vault " + vault + " --yes"
	if !strings.Contains(content, execStart) {
		t.Errorf("systemd unit missing ExecStart directive %q\n--- unit ---\n%s", execStart, content)
	}
}

func TestLaunchdPlistIsValidXML(t *testing.T) {
	binary := "/usr/local/bin/pinax"
	vault := "/Users/user/my-notes"
	content := launchdPlistContent(binary, vault, "my-notes")

	var plist interface{}
	if err := xml.Unmarshal([]byte(content), &plist); err != nil {
		t.Fatalf("launchd plist is not valid XML: %v\n--- plist ---\n%s", err, content)
	}
	for _, needle := range []string{
		"com.yeisme.capsa-sync.my-notes",
		"<key>RunAtLoad</key>",
		"<key>KeepAlive</key>",
		"<true/>",
		"<string>" + binary + "</string>",
		"--vault",
		"<string>" + vault + "</string>",
	} {
		if !strings.Contains(content, needle) {
			t.Errorf("launchd plist missing %q\n--- plist ---\n%s", needle, content)
		}
	}
}

func TestInstallDaemonUnitLinuxWritesUnitAndReturnsEnableCommand(t *testing.T) {
	home := t.TempDir()
	binary := "/usr/local/bin/pinax"
	vault := t.TempDir()
	slug := "my-notes"

	unitPath, enableCommand, err := installDaemonUnit("linux", home, binary, vault, slug)
	if err != nil {
		t.Fatalf("installDaemonUnit linux: %v", err)
	}
	if !strings.HasSuffix(unitPath, "capsa-sync-my-notes.service") {
		t.Errorf("unitPath = %q, want suffix capsa-sync-my-notes.service", unitPath)
	}
	if enableCommand != "systemctl --user enable --now capsa-sync-my-notes" {
		t.Errorf("enableCommand = %q", enableCommand)
	}
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read unit: %v", err)
	}
	if !strings.Contains(string(data), "Restart=on-failure") {
		t.Errorf("written unit missing Restart=on-failure:\n%s", data)
	}
}

func TestInstallDaemonUnitDarwinWritesPlist(t *testing.T) {
	home := t.TempDir()
	binary := "/usr/local/bin/pinax"
	vault := t.TempDir()

	unitPath, enableCommand, err := installDaemonUnit("darwin", home, binary, vault, "my-notes")
	if err != nil {
		t.Fatalf("installDaemonUnit darwin: %v", err)
	}
	if !strings.HasSuffix(unitPath, "com.yeisme.capsa-sync.my-notes.plist") {
		t.Errorf("unitPath = %q", unitPath)
	}
	if !strings.HasPrefix(enableCommand, "launchctl load ") {
		t.Errorf("enableCommand = %q", enableCommand)
	}
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	var plist interface{}
	if err := xml.Unmarshal(data, &plist); err != nil {
		t.Fatalf("written plist is not valid XML: %v", err)
	}
}

func TestInstallDaemonUnitWindowsUnsupported(t *testing.T) {
	home := t.TempDir()
	_, _, err := installDaemonUnit("windows", home, "/pinax", "/vault", "my-notes")
	if err == nil {
		t.Fatal("expected platform_unsupported error for windows, got nil")
	}
	cmdErr, ok := err.(*domain.CommandError)
	if !ok || cmdErr.Code != "platform_unsupported" {
		t.Fatalf("expected platform_unsupported CommandError, got %#v", err)
	}
}

func TestUninstallDaemonUnitRemovesFile(t *testing.T) {
	home := t.TempDir()
	binary := "/usr/local/bin/pinax"
	vault := t.TempDir()
	slug := "my-notes"

	unitPath, _, err := installDaemonUnit("linux", home, binary, vault, slug)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := os.Stat(unitPath); err != nil {
		t.Fatalf("unit should exist before uninstall: %v", err)
	}

	gotPath, removed, err := uninstallDaemonUnit("linux", home, slug)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if gotPath != unitPath {
		t.Errorf("uninstall unitPath = %q, want %q", gotPath, unitPath)
	}
	if !removed {
		t.Error("expected removed=true after uninstalling existing unit")
	}
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Errorf("expected unit file removed, stat err = %v", err)
	}
}

func TestUninstallDaemonUnitMissingFileReportsNotRemoved(t *testing.T) {
	home := t.TempDir()
	_, removed, err := uninstallDaemonUnit("linux", home, "never-installed")
	if err != nil {
		t.Fatalf("uninstall missing file: %v", err)
	}
	if removed {
		t.Error("expected removed=false when no unit file exists")
	}
}

func TestDaemonWriteEnvFile(t *testing.T) {
	vault := t.TempDir()
	t.Setenv("PINAX_SYNC_SECRET", "super-secret-value")
	if !daemonWriteEnvFile(vault) {
		t.Fatal("expected daemonWriteEnvFile to return true when PINAX_SYNC_SECRET is set")
	}
	envPath := filepath.Join(vault, ".pinax", "cloud", "sync-env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	if !strings.Contains(string(data), "PINAX_SYNC_SECRET=super-secret-value") {
		t.Errorf("env file content = %q", data)
	}
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatalf("stat env file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("env file mode = %o, want 0600", perm)
	}
}

func TestDaemonWriteEnvFileAbsentSecret(t *testing.T) {
	vault := t.TempDir()
	t.Setenv("PINAX_SYNC_SECRET", "")
	if daemonWriteEnvFile(vault) {
		t.Error("expected daemonWriteEnvFile to return false when secret is empty")
	}
}

func TestSyncDaemonInstallProjectionFacts(t *testing.T) {
	vault := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PINAX_SYNC_SECRET", "secret-abc")

	svc := NewService()
	projection, err := svc.SyncDaemonInstall(context.Background(), SyncDaemonInstallRequest{VaultPath: vault})
	if err != nil {
		t.Fatalf("SyncDaemonInstall: %v", err)
	}
	for _, key := range []string{"unit_path", "enable_command", "slug", "binary", "env_file"} {
		if projection.Facts[key] == "" {
			t.Errorf("projection missing non-empty fact %q", key)
		}
	}
	if _, err := os.Stat(projection.Facts["unit_path"]); err != nil {
		t.Errorf("unit_path %q does not exist: %v", projection.Facts["unit_path"], err)
	}
}

func TestSyncDaemonUninstallRemovesUnit(t *testing.T) {
	vault := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	svc := NewService()
	if _, err := svc.SyncDaemonInstall(context.Background(), SyncDaemonInstallRequest{VaultPath: vault}); err != nil {
		t.Fatalf("install: %v", err)
	}
	projection, err := svc.SyncDaemonUninstall(context.Background(), SyncDaemonInstallRequest{VaultPath: vault})
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if projection.Facts["removed"] != "true" {
		t.Errorf("removed = %q, want true", projection.Facts["removed"])
	}
	if _, err := os.Stat(projection.Facts["unit_path"]); !os.IsNotExist(err) {
		t.Errorf("expected unit removed after uninstall, stat err = %v", err)
	}
}
