package agentadapter

import "github.com/yeisme/pinax/internal/agentprotocol"

// CodexDescriptor 返回 Codex reference adapter 的 descriptor fixture。
// 只使用 common schema + optional metadata；不引入 Codex-specific required field。
func CodexDescriptor() agentprotocol.AdapterDescriptor {
	return agentprotocol.AdapterDescriptor{
		SchemaVersion:     agentprotocol.AdapterSchemaVersion,
		AdapterID:         "codex-reference",
		Runtime:           "codex",
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
			"hook_version": "codex-hooks-v1",
			"transport":    "cli-subprocess",
			"experimental": "true",
		},
	}
}

// CodexPrincipal 返回一个典型 Codex adapter principal fixture。
func CodexPrincipal(agentID string) agentprotocol.Principal {
	p := agentprotocol.DefaultAdapterPrincipal(agentID, "codex")
	p.Capabilities = append(p.Capabilities, agentprotocol.CapabilityHandoff)
	return p
}
