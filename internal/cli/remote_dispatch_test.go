package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	flagpkg "github.com/spf13/pflag"
)

type commandStub struct {
	method string
	flags  map[string]bool
}

// TestRemoteCommandRegistryMatchesCommandTree guards the registry against
// drift: every registered command path must resolve to a real command in the
// root tree, and every registered flag must exist on that command.
func TestRemoteCommandRegistryMatchesCommandTree(t *testing.T) {
	t.Parallel()
	root := NewRootCommand("test")
	commands := map[string]commandStub{}
	collectCommandStub(root, "", commands)
	if len(commands) == 0 {
		t.Fatal("command tree walk found no commands; harness broken")
	}
	for path, spec := range remoteCommandRegistry {
		stub, ok := commands[path]
		if !ok {
			t.Errorf("registry path %q does not match any command in the root tree", path)
			continue
		}
		for _, param := range spec.Flags {
			if !stub.flags[param.Flag] {
				t.Errorf("registry flag --%s for %q is not defined on the command", param.Flag, path)
			}
		}
	}
}

// TestRemoteSupportedRPCMethodsDerivesFromRegistry ensures the coverage map and
// the dispatch registry cannot disagree.
func TestRemoteSupportedRPCMethodsDerivesFromRegistry(t *testing.T) {
	t.Parallel()
	methods := remoteSupportedRPCMethods()
	if len(methods) != len(remoteCommandRegistry) {
		t.Fatalf("methods map has %d entries, registry has %d", len(methods), len(remoteCommandRegistry))
	}
	for path, spec := range remoteCommandRegistry {
		if methods[path] != spec.Method {
			t.Errorf("methods[%q] = %q, want %q", path, methods[path], spec.Method)
		}
	}
}

func collectCommandStub(cmd *cobra.Command, prefix string, out map[string]commandStub) {
	path := cmd.Name()
	if prefix != "" {
		path = prefix + " " + cmd.Name()
	}
	flags := map[string]bool{}
	cmd.Flags().VisitAll(func(flag *flagpkg.Flag) {
		flags[flag.Name] = true
	})
	out[strings.TrimPrefix(path, "pinax ")] = commandStub{method: cmd.Name(), flags: flags}
	for _, sub := range cmd.Commands() {
		collectCommandStub(sub, path, out)
	}
}
