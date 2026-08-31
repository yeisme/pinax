package cli

import (
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/pkg/pinaxclient"
)

// remoteFlagKind selects how a cobra flag value is converted into an RPC param.
type remoteFlagKind int

const (
	remoteFlagString remoteFlagKind = iota
	remoteFlagBool
	remoteFlagInt
	remoteFlagStringArray
	remoteFlagCSV
)

// remoteParamSpec maps one cobra flag onto one RPC param key.
type remoteParamSpec struct {
	Key          string
	Flag         string
	Kind         remoteFlagKind
	FallbackFlag string // used when Flag is empty (see note list --group)
}

// remoteCommandSpec describes how one CLI command path maps onto one RPC
// method. ArgParams maps positional arguments to param keys (its length is the
// command's arity); Const carries fixed params; Flags maps cobra flags.
type remoteCommandSpec struct {
	CommandPath string
	Method      string
	ArgParams   []string
	Const       map[string]string
	Flags       []remoteParamSpec
}

// remoteCommandRegistry is the single source of truth for remote-dispatchable
// commands. It replaces the remoteRPCRequestForCommand switch and feeds
// remoteSupportedRPCMethods, so the RPC mapping, the coverage report, and the
// command definitions can no longer drift apart silently.
var (
	remoteCommandRegistryMu sync.RWMutex
	remoteCommandRegistry   = map[string]remoteCommandSpec{}
)

// registerRemoteCommand registers a spec; specs live next to the command
// definitions in the *_cmd.go files that own them.
func registerRemoteCommand(spec remoteCommandSpec) {
	remoteCommandRegistryMu.Lock()
	defer remoteCommandRegistryMu.Unlock()
	remoteCommandRegistry[spec.CommandPath] = spec
}

func remoteCommandRegistrySnapshot() map[string]remoteCommandSpec {
	remoteCommandRegistryMu.RLock()
	defer remoteCommandRegistryMu.RUnlock()
	snapshot := make(map[string]remoteCommandSpec, len(remoteCommandRegistry))
	for path, spec := range remoteCommandRegistry {
		snapshot[path] = spec
	}
	return snapshot
}

// remoteSupportedRPCMethods returns the command path → RPC method mapping used
// by the coverage report.
func remoteSupportedRPCMethods() map[string]string {
	registry := remoteCommandRegistrySnapshot()
	methods := make(map[string]string, len(registry))
	for path, spec := range registry {
		methods[path] = spec.Method
	}
	return methods
}

// remoteRPCRequestForCommand assembles the RPC request for a command instance
// from the registry, enforcing positional arity.
func remoteRPCRequestForCommand(cmd *cobra.Command, args []string) (pinaxclient.RPCRequest, bool) {
	commandPath := strings.TrimPrefix(cmd.CommandPath(), "pinax ")
	remoteCommandRegistryMu.RLock()
	spec, ok := remoteCommandRegistry[commandPath]
	remoteCommandRegistryMu.RUnlock()
	if !ok {
		return pinaxclient.RPCRequest{}, false
	}
	if len(args) != len(spec.ArgParams) {
		return pinaxclient.RPCRequest{}, false
	}
	params := map[string]any{}
	for key, value := range spec.Const {
		params[key] = value
	}
	for i, key := range spec.ArgParams {
		params[key] = args[i]
	}
	for _, param := range spec.Flags {
		value, ok := remoteFlagValue(cmd, param)
		if !ok {
			continue
		}
		params[param.Key] = value
	}
	return pinaxclient.RPCRequest{Method: spec.Method, Params: params}, true
}

func remoteFlagValue(cmd *cobra.Command, param remoteParamSpec) (any, bool) {
	switch param.Kind {
	case remoteFlagBool:
		return boolFlag(cmd, param.Flag), true
	case remoteFlagInt:
		return intFlag(cmd, param.Flag), true
	case remoteFlagStringArray:
		return stringArrayFlag(cmd, param.Flag), true
	case remoteFlagCSV:
		return splitCSV(stringFlag(cmd, param.Flag)), true
	default:
		value := stringFlag(cmd, param.Flag)
		if value == "" && param.FallbackFlag != "" {
			value = stringFlag(cmd, param.FallbackFlag)
		}
		return value, true
	}
}

// sfb are shorthand constructors for remoteParamSpec entries, keeping the
// registrations next to command definitions compact.
func s(key, flag string) remoteParamSpec { return remoteParamSpec{Key: key, Flag: flag} }
func b(key, flag string) remoteParamSpec {
	return remoteParamSpec{Key: key, Flag: flag, Kind: remoteFlagBool}
}
func i(key, flag string) remoteParamSpec {
	return remoteParamSpec{Key: key, Flag: flag, Kind: remoteFlagInt}
}
func sa(key, flag string) remoteParamSpec {
	return remoteParamSpec{Key: key, Flag: flag, Kind: remoteFlagStringArray}
}
func csv(key, flag string) remoteParamSpec {
	return remoteParamSpec{Key: key, Flag: flag, Kind: remoteFlagCSV}
}
