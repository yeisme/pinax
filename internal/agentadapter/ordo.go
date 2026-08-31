package agentadapter

import "github.com/yeisme/pinax/internal/agentprotocol"

// OrdoDescriptor 返回 Ordo reference adapter 的 descriptor fixture。
// 与 Codex 共用 100% core schema；team-specific 信息放 Metadata。
func OrdoDescriptor() agentprotocol.AdapterDescriptor {
	return agentprotocol.AdapterDescriptor{
		SchemaVersion:     agentprotocol.AdapterSchemaVersion,
		AdapterID:         "ordo-reference",
		Runtime:           "ordo",
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
			"trace_format": "ordo-trace-v1",
			"experimental": "true",
		},
	}
}

// OrdoPrincipal 返回一个典型 Ordo adapter principal fixture。
func OrdoPrincipal(agentID string) agentprotocol.Principal {
	p := agentprotocol.DefaultAdapterPrincipal(agentID, "ordo")
	p.Capabilities = append(p.Capabilities, agentprotocol.CapabilityHandoff)
	return p
}
