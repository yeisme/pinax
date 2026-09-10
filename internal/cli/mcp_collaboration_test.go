package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestMCPCollaborationFlagsAndDefaultPermissions(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
		fail bool
	}{
		{[]string{"mcp", "serve", "--help"}, "--allow-note-body", false},
		{[]string{"mcp", "serve", "--allow-note-body"}, "require --collaboration", true},
		{[]string{"mcp", "serve", "--collaboration", "--allow-note-write"}, "requires --allow-note-body", true},
		{[]string{"mcp", "serve", "-w"}, "unknown shorthand", true},
	} {
		cmd := NewRootCommand("test")
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetErr(&output)
		cmd.SetArgs(tc.args)
		err := cmd.Execute()
		if (err != nil) != tc.fail {
			t.Fatalf("args=%v err=%v", tc.args, err)
		}
		text := output.String()
		if err != nil {
			text += err.Error()
		}
		if !strings.Contains(text, tc.want) {
			t.Fatalf("missing expected help/error %q", tc.want)
		}
	}
}
