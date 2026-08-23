package agentadapter

import "github.com/yeisme/pinax/internal/agentprotocol"

// CohorsDescriptor 返回 Cohors reference adapter 的 descriptor fixture。
// 与 Codex 共用 100% core schema；team-specific 信息放 Metadata。
func CohorsDescriptor() agentprotocol.AdapterDescriptor {
	return agentprotocol.AdapterDescriptor{
		SchemaVersion:     agentprotocol.AdapterSchemaVersion,
		AdapterID:         "cohors-reference",
		Runtime:           "cohors",
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
			"trace_format": "cohors-trace-v1",
			"experimental": "true",
		},
	}
}

// CohorsPrincipal 返回一个典型 Cohors adapter principal fixture。
func CohorsPrincipal(agentID string) agentprotocol.Principal {
	p := agentprotocol.DefaultAdapterPrincipal(agentID, "cohors")
	p.Capabilities = append(p.Capabilities, agentprotocol.CapabilityHandoff)
	return p
}
