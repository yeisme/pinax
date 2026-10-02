package e2e

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/mcpserver"
)

func TestNoteProposalWorkflow(t *testing.T) {
	testscript.Run(t, testscript.Params{Dir: "testdata/note_specs/scripts", Setup: func(env *testscript.Env) error {
		env.Vars = append(env.Vars, "PATH="+sharedBinDir+string(os.PathListSeparator)+os.Getenv("PATH"), "XDG_CONFIG_HOME="+filepath.Join(env.WorkDir, "xdg"))
		return nil
	}, Cmds: map[string]func(*testscript.TestScript, bool, []string){"vault-digest": researchVaultDigest, "contract-fact": func(ts *testscript.TestScript, neg bool, args []string) {
		if neg || len(args) != 3 {
			ts.Fatalf("usage: contract-fact <output-file> <fact> <env>")
		}
		var p struct {
			Facts map[string]string `json:"facts"`
		}
		if err := json.Unmarshal([]byte(ts.ReadFile(args[0])), &p); err != nil || p.Facts[args[1]] == "" {
			ts.Fatalf("missing contract fact")
		}
		ts.Setenv(args[2], p.Facts[args[1]])
	}}})
}

func noteSpecCLI(t *testing.T, root, body string, args ...string) map[string]any {
	t.Helper()
	args = append(args, "--vault", root, "--json")
	cmd := exec.Command(filepath.Join(sharedBinDir, "pinax"), args...)
	cmd.Stdin = strings.NewReader(body)
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+t.TempDir(), "PINAX_API_URL=")
	b, err := cmd.Output()
	if err != nil {
		t.Fatal("fixture CLI command failed", err)
	}
	var p map[string]any
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal("invalid CLI envelope")
	}
	return p
}

func noteSpecMCP(t *testing.T, s *mcpserver.Server, name string, args map[string]any) map[string]any {
	t.Helper()
	r, err := s.Handle(context.Background(), mcpserver.Request{Method: "tools/call", Params: map[string]any{"name": name, "arguments": args}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(r.Result)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestNoteProposalTransportParity(t *testing.T) {
	root := t.TempDir()
	svc := app.NewService()
	ctx := context.Background()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Parity"}); err != nil {
		t.Fatal(err)
	}
	p, err := svc.CreateNote(ctx, app.CreateNoteRequest{VaultPath: root, Title: "Target", Dir: "index", Body: "original"})
	if err != nil {
		t.Fatal(err)
	}
	id := p.Facts["note_id"]
	_, rev, err := svc.CollaborationRead(ctx, root, id, "read", app.CollaborationPolicy{AllowBody: true})
	if err != nil {
		t.Fatal(err)
	}
	cli := noteSpecCLI(t, root, "replacement", "agent", "note", "propose", "--kind", "note_edit", "--target", id, "--expected-revision", rev, "--rationale", "Fixture review", "--stdin")
	server := mcpserver.NewServerWithOptions(svc, root, mcpserver.ServerOptions{NoteProposals: true})
	mcp := noteSpecMCP(t, server, "pinax.agent.propose_note_edit", map[string]any{"target_object_id": id, "expected_revision": rev, "body": "replacement", "rationale": "Fixture review"})
	a := cli["data"].(map[string]any)
	b := mcp["data"].(map[string]any)
	for _, key := range []string{"kind", "risk", "status", "target_object_id", "expected_revision", "rationale", "added_lines", "removed_lines", "hunks"} {
		if !reflect.DeepEqual(a[key], b[key]) {
			t.Fatalf("proposal parity mismatch in %s", key)
		}
	}
	cliList := noteSpecCLI(t, root, "", "agent", "note", "proposal", "list")
	mcpList := noteSpecMCP(t, server, "pinax.agent.note_proposals", nil)
	if !reflect.DeepEqual(cliList["data"], mcpList["data"]) {
		t.Fatal("list transport parity mismatch")
	}
}

func TestRenderedViewTransportParity(t *testing.T) {
	root := t.TempDir()
	svc := app.NewService()
	ctx := context.Background()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Parity"}); err != nil {
		t.Fatal(err)
	}
	p, err := svc.CreateNote(ctx, app.CreateNoteRequest{VaultPath: root, Title: "Render", Dir: "index", Body: "# Heading\n\n[[Missing]]\n\n```pinax-board missing\n```\n"})
	if err != nil {
		t.Fatal(err)
	}
	id := p.Facts["note_id"]
	cli := noteSpecCLI(t, root, "", "note", "preview", id, "--view", "rendered", "--display", "context")
	server := mcpserver.NewServer(svc, root)
	mcp := noteSpecMCP(t, server, "pinax.render_note", map[string]any{"note_ref": id, "display": "context"})
	a := cli["data"].(map[string]any)
	b := mcp["data"].(map[string]any)
	delete(a["badge"].(map[string]any), "rendered_at")
	delete(b["badge"].(map[string]any), "rendered_at")
	if !reflect.DeepEqual(a, b) {
		t.Fatal("render transport parity mismatch")
	}
}

func assertDefaultMCPReadTools(t *testing.T, tools []any) {
	t.Helper()
	registry := mcpserver.ToolInventory()
	if len(tools) != len(registry) {
		t.Fatalf("default tool inventory=%d, canonical registry=%d", len(tools), len(registry))
	}
	names := map[string]bool{}
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			t.Fatal("invalid tool descriptor")
		}
		name, _ := tool["name"].(string)
		names[name] = true
		annotations, _ := tool["annotations"].(map[string]any)
		if annotations["readOnlyHint"] != true {
			t.Fatal("default discovery exposed a mutation")
		}
	}
	for _, tool := range registry {
		if !names[tool.Name] {
			t.Fatalf("canonical tool %s missing", tool.Name)
		}
	}
}
