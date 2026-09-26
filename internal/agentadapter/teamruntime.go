package agentadapter

import "github.com/yeisme/pinax/internal/agentprotocol"

// TeamRuntimeDescriptor 返回 team-runtime reference adapter 的 descriptor fixture。
// 与 Codex 共用 100% core schema；team-specific 信息放 Metadata。
func TeamRuntimeDescriptor() agentprotocol.AdapterDescriptor {
	return agentprotocol.AdapterDescriptor{
		SchemaVersion:     agentprotocol.AdapterSchemaVersion,
		AdapterID:         "team-runtime-reference",
		Runtime:           "teamRuntime",
		SupportedVersions: []string{agentprotocol.SchemaVersion, agentprotocol.ContextSchemaVersion},
		Capabilities: []agentprotocol.Capability{
			agentprotocol.CapabilityRead,
			agentprotocol.CapabilityPropose,
			agentprotocol.CapabilityFeedback,
			agentprotocol.CapabilityHandoff,
		},
		SupportsProposal: true,
		SupportsHandoff:  true,
		SupportsFeedback: true,
		MaxContextItems:  20,
		MaxContextChars:  8000,
		Metadata: map[string]string{
			"team_role":    "implementation-worker",
			"trace_format": "team-runtime-trace-v1",
			"experimental": "true",
		},
	}
}

// TeamRuntimePrincipal 返回一个典型 team-runtime adapter principal fixture。
func TeamRuntimePrincipal(agentID string) agentprotocol.Principal {
	p := agentprotocol.DefaultAdapterPrincipal(agentID, "teamRuntime")
	p.Capabilities = append(p.Capabilities, agentprotocol.CapabilityHandoff)
	return p
}
