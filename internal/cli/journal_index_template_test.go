package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

type cliJSONProjection struct {
	Command string            `json:"command"`
	Status  string            `json:"status"`
	Mode    string            `json:"mode"`
	Facts   map[string]string `json:"facts"`
}

func TestJournalTemplateCLIContract(t *testing.T) {
	root := t.TempDir()
	projection := runCLIJSON(t, "journal", "daily", "show", "--date", "2026-06-08", "--template", "journal.daily", "--vault", root, "--json")
	if projection.Command != "daily.show" || projection.Status != "success" || projection.Mode != "json" {
		t.Fatalf("journal projection = %#v", projection)
	}
	if projection.Facts["path"] != "daily/2026-06-08.md" || projection.Facts["template"] != "journal.daily" {
		t.Fatalf("journal facts = %#v", projection.Facts)
	}
}

func TestIndexPageCLIContract(t *testing.T) {
	root := t.TempDir()
	preview := runCLIJSON(t, "index", "page", "preview", "home", "--template", "index.home", "--vault", root, "--json")
	if preview.Command != "index.page.preview" || preview.Facts["writes"] != "false" || preview.Facts["path"] != "index/home.md" {
		t.Fatalf("preview projection = %#v", preview)
	}
	if fileExistsCLI(filepath.Join(root, "index", "home.md")) {
		t.Fatalf("preview wrote index page")
	}
	created := runCLIJSON(t, "index", "page", "create", "home", "--template", "index.home", "--vault", root, "--json")
	if created.Command != "index.page.create" || created.Facts["path"] != "index/home.md" || created.Facts["template"] != "index.home" {
		t.Fatalf("create projection = %#v", created)
	}
	refreshed := runCLIJSON(t, "index", "page", "refresh", "home", "--template", "index.home", "--vault", root, "--json")
	if refreshed.Command != "index.page.refresh" || refreshed.Facts["path"] != "index/home.md" || refreshed.Facts["managed_blocks"] == "" {
		t.Fatalf("refresh projection = %#v", refreshed)
	}
}

func TestTemplateInspectCLIContract(t *testing.T) {
	root := t.TempDir()
	projection := runCLIJSON(t, "template", "inspect", "index.home", "--vault", root, "--json")
	if projection.Command != "template.inspect" || projection.Status != "success" || projection.Mode != "json" {
		t.Fatalf("inspect projection = %#v", projection)
	}
	if projection.Facts["template"] != "index.home" || projection.Facts["kind"] != "index_template" || projection.Facts["path_pattern"] != "index/home.md" || projection.Facts["managed_blocks"] != "1" {
		t.Fatalf("inspect facts = %#v", projection.Facts)
	}
}

func runCLIJSON(t *testing.T, args ...string) cliJSONProjection {
	t.Helper()
	cmd := NewRootCommand("test")
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %v: %v\nstdout:\n%s\nstderr:\n%s", args, err, out.String(), errOut.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr for %v = %s", args, errOut.String())
	}
	var projection cliJSONProjection
	if err := json.Unmarshal(out.Bytes(), &projection); err != nil {
		t.Fatalf("decode json for %v: %v\nstdout:\n%s", args, err, out.String())
	}
	return projection
}

func TestJournalDailyTemplateFlag(t *testing.T) {
	root := t.TempDir()
	projection := runCLIJSON(t, "journal", "daily", "show", "--date", "2026-06-08", "--template", "journal.daily", "--vault", root, "--json")
	if projection.Command != "daily.show" || projection.Facts["template"] != "journal.daily" || projection.Facts["path"] != "daily/2026-06-08.md" {
		t.Fatalf("daily template flag projection = %#v", projection)
	}
}

func TestJournalWeeklyTemplateFlag(t *testing.T) {
	root := t.TempDir()
	projection := runCLIJSON(t, "journal", "weekly", "show", "--date", "2026-W23", "--template", "journal.weekly", "--vault", root, "--json")
	if projection.Command != "weekly.show" || projection.Facts["template"] != "journal.weekly" || projection.Facts["path"] != "weekly/2026-W23.md" {
		t.Fatalf("weekly template flag projection = %#v", projection)
	}
}
