package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestResourceCompletionInstallation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var report resourceCompletionReport
	call := func(args ...string) {
		t.Helper()
		root := &cobra.Command{Use: "resource-test", SilenceErrors: true, SilenceUsage: true}
		completion := &cobra.Command{Use: "completion"}
		completion.AddCommand(resourceCompletionCommands(func(_ *cobra.Command, r resourceCompletionReport) error { report = r; return nil })...)
		root.AddCommand(completion)
		root.SetArgs(append([]string{"completion"}, args...))
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	call("install", "zsh", "--dry-run")
	if report.Installed || report.Changed {
		t.Fatal(report)
	}
	call("install", "zsh", "--yes")
	if !report.Installed || !report.Owned || filepath.Base(report.Target) != "_resource-test" {
		t.Fatal(report)
	}
	body, err := os.ReadFile(report.Target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), "#compdef resource-test") {
		t.Fatalf("zsh autoload header lost")
	}
	call("doctor", "zsh")
	if report.ShellLoaded != "unknown" {
		t.Fatal("file installation cannot prove parent shell state")
	}
	t.Setenv("FPATH", filepath.Dir(report.Target))
	call("doctor", "zsh")
	if report.Discoverability != "available" {
		t.Fatal(report)
	}
	if err := os.WriteFile(report.Target, []byte("foreign content"), 0600); err != nil {
		t.Fatal(err)
	}
	call("install", "zsh", "--yes")
	if report.ErrorCode != "foreign_completion_file" {
		t.Fatal(report)
	}
	call("uninstall", "zsh", "--yes")
	if report.ErrorCode != "foreign_completion_file" {
		t.Fatal(report)
	}
	for _, shell := range []string{"bash", "fish", "powershell"} {
		call("install", shell, "--yes")
		if report.ErrorCode != "" || !report.Installed {
			t.Fatal(report)
		}
		call("uninstall", shell, "--yes")
		if report.Installed {
			t.Fatal(report)
		}
	}
}
