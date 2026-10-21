package cli

import (
	"bytes"
	"github.com/yeisme/pinax/internal/domain"
	"strings"
	"testing"
)

func TestResourceCanonicalRegistrationAndErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := NewRootCommand("test")
	for _, verb := range []string{"show", "get", "describe"} {
		cmd, _, err := root.Find([]string{"project", verb})
		if err != nil || cmd.Name() != verb {
			t.Fatalf("%s: %v", verb, err)
		}
		if cmd.Hidden != (verb == "show") {
			t.Fatal("compatibility visibility")
		}
	}
	var out, stderr bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&stderr)
	root.SetArgs([]string{"project", "describe", "--json"})
	if err := root.Execute(); ExitCode(err) != 2 || !strings.Contains(out.String(), "missing_argument") || stderr.Len() != 0 {
		t.Fatalf("%v %s %s", err, out.String(), stderr.String())
	}
	get, _, _ := root.Find([]string{"note", "get"})
	p := resourceProjection(get, domain.Projection{Data: map[string]any{"note": map[string]any{"id": "n1", "body": "PRIVATE BODY", "title": "Title"}}, Facts: map[string]string{"body": "PRIVATE BODY", "note_id": "n1"}})
	if strings.Contains(p.Data.(map[string]any)["note"].(map[string]any)["id"].(string), "PRIVATE") || p.Facts["body"] != "" {
		t.Fatal("get contains body")
	}
}

func TestResourceProjectListIdentityAndNext(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PINAX_API_URL", "")
	vault := t.TempDir()
	if _, _, err := runCLIWithStdin(t, "", "init", vault, "--title", "Resource fixture", "--json"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLIWithStdin(t, "", "project", "create", "research", "--name", "Research", "--vault", vault, "--json"); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"--agent", "--json"} {
		out, stderr, err := runCLIWithStdin(t, "", "project", "list", "--vault", vault, mode)
		if err != nil || stderr != "" || !strings.Contains(out, "project.1.id") || !strings.Contains(out, "project describe research") || !strings.Contains(out, vault) {
			t.Fatalf("%v %s %s", err, out, stderr)
		}
	}
	for _, verb := range []string{"get", "describe"} {
		out, stderr, err := runCLIWithStdin(t, "", "project", verb, "research", "--vault", vault, "--json")
		if err != nil || stderr != "" || !strings.Contains(out, "research") {
			t.Fatalf("%v %s %s", err, out, stderr)
		}
	}
	if _, _, err := runCLIWithStdin(t, "", "project", "create", "archive", "--name", "Archive", "--vault", vault, "--json"); err != nil {
		t.Fatal(err)
	}
	out, stderr, err := runCLIWithStdin(t, "", "project", "list", "--vault", vault, "--agent")
	if err != nil || stderr != "" || !strings.Contains(out, "fact.project.1.id=archive") || !strings.Contains(out, "fact.project.2.id=research") || strings.Contains(out, "fact.project.0.id=") {
		t.Fatalf("%v %s %s", err, out, stderr)
	}
}
