package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	flagpkg "github.com/spf13/pflag"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/transportcatalog"
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
	registry := remoteCommandRegistrySnapshot()
	commands := map[string]commandStub{}
	collectCommandStub(root, "", commands)
	if len(commands) == 0 {
		t.Fatal("command tree walk found no commands; harness broken")
	}
	for path, spec := range registry {
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
	NewRootCommand("test")
	registry := remoteCommandRegistrySnapshot()
	methods := remoteSupportedRPCMethods()
	if len(methods) != len(registry) {
		t.Fatalf("methods map has %d entries, registry has %d", len(methods), len(registry))
	}
	for path, spec := range registry {
		if methods[path] != spec.Method {
			t.Errorf("methods[%q] = %q, want %q", path, methods[path], spec.Method)
		}
	}
}

func TestRemoteCLITransportBindingsDeriveFromDispatchRegistry(t *testing.T) {
	t.Parallel()
	NewRootCommand("test")
	registry := remoteCommandRegistrySnapshot()

	bindings, err := RemoteCLITransportBindings()
	if err != nil {
		t.Fatalf("RemoteCLITransportBindings() error = %v", err)
	}
	if len(bindings) != len(registry) {
		t.Fatalf("bindings = %d, registry entries = %d", len(bindings), len(registry))
	}

	bindingByCommand := make(map[string]transportcatalog.TransportBinding, len(bindings))
	for _, binding := range bindings {
		if binding.Transport != transportcatalog.TransportCLI || binding.Availability != transportcatalog.AvailabilityAvailable || binding.BackingRef == "" {
			t.Fatalf("remote CLI binding is not available and backed: %#v", binding)
		}
		if _, exists := bindingByCommand[binding.ProtocolName]; exists {
			t.Fatalf("duplicate remote CLI command binding %q", binding.ProtocolName)
		}
		bindingByCommand[binding.ProtocolName] = binding
	}
	for commandPath, spec := range registry {
		binding, ok := bindingByCommand[commandPath]
		if !ok {
			t.Fatalf("missing transport binding for remote command %q", commandPath)
		}
		if binding.Method != spec.Method {
			t.Fatalf("binding method for %q = %q, want %q", commandPath, binding.Method, spec.Method)
		}
	}
}

func TestTransportManifestIncludesRESTRPCMCPAndRemoteCLI(t *testing.T) {
	root := NewRootCommand("test")
	if root == nil {
		t.Fatal("root command is nil")
	}

	manifest, err := TransportManifest()
	if err != nil {
		t.Fatalf("TransportManifest() error = %v", err)
	}
	seen := map[transportcatalog.Transport]bool{}
	for _, capability := range manifest.Capabilities {
		for _, binding := range capability.Bindings {
			if binding.Availability == transportcatalog.AvailabilityAvailable {
				seen[binding.Transport] = true
			}
		}
	}
	for _, transport := range []transportcatalog.Transport{
		transportcatalog.TransportCLI,
		transportcatalog.TransportREST,
		transportcatalog.TransportRPC,
		transportcatalog.TransportMCPTool,
		transportcatalog.TransportMCPResource,
	} {
		if !seen[transport] {
			t.Fatalf("manifest has no available %s binding", transport)
		}
	}
	if manifest.Digest == "" {
		t.Fatal("manifest digest is empty")
	}
}

func TestTransportManifestConformsToRemoteCLIRegistry(t *testing.T) {
	t.Parallel()
	NewRootCommand("test")
	registry := remoteCommandRegistrySnapshot()

	manifest, err := TransportManifest()
	if err != nil {
		t.Fatalf("TransportManifest() error = %v", err)
	}
	capabilityByRPCMethod := make(map[string]string)
	for _, route := range app.RemoteRoutes() {
		if route.Surface == "rpc" {
			capabilityByRPCMethod[route.RPCMethod] = route.CapabilityID
		}
	}
	operations := make([]transportcatalog.AdapterOperation, 0, len(registry))
	for commandPath, spec := range registry {
		capabilityID, ok := capabilityByRPCMethod[spec.Method]
		if !ok {
			t.Fatalf("remote command %q references unknown RPC method %q", commandPath, spec.Method)
		}
		operations = append(operations, transportcatalog.AdapterOperation{
			BindingID:    "cli.remote." + strings.ReplaceAll(commandPath, " ", "."),
			CapabilityID: capabilityID,
			Transport:    transportcatalog.TransportCLI,
			BackingRef:   "internal/cli/remote:" + commandPath,
			Method:       spec.Method,
			ProtocolName: commandPath,
		})
	}
	if err := transportcatalog.ValidateConformance(manifest, operations, transportcatalog.TransportCLI); err != nil {
		t.Fatalf("remote CLI registry conformance failed: %v", err)
	}
}

func TestRemoteCommandCoverageExplainsEveryNonRemoteCommand(t *testing.T) {
	t.Parallel()

	for _, entry := range RemoteCommandCoverage(NewRootCommand("test")) {
		switch entry.Status {
		case "remote_supported":
			if entry.RPCMethod == "" || entry.Reason != "" {
				t.Fatalf("remote supported coverage is incomplete: %#v", entry)
			}
		case "local_only", "unsupported":
			if entry.Reason == "" || entry.RPCMethod != "" {
				t.Fatalf("non-remote coverage lacks an explicit reason: %#v", entry)
			}
		default:
			t.Fatalf("unknown remote coverage status: %#v", entry)
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
