package agentprotocol

import (
	"fmt"
	"strings"
)

// AdapterSchemaVersion 是 adapter descriptor 的 schema 版本。
const AdapterSchemaVersion = "yeisme.agent_adapter.v1"

// AdapterDescriptor 声明一个 Agent runtime adapter 的能力。
// Core field 100% 共用；runtime-specific 信息放 Metadata。
type AdapterDescriptor struct {
	SchemaVersion     string       `json:"schema_version"`
	AdapterID         string       `json:"adapter_id"`
	Runtime           string       `json:"runtime"`
	SupportedVersions []string     `json:"supported_versions"`
	Capabilities      []Capability `json:"capabilities"`
	MaxContextItems   int          `json:"max_context_items,omitempty"`
	MaxContextChars   int          `json:"max_context_chars,omitempty"`
	SupportsProposal  bool         `json:"supports_proposal"`
	SupportsHandoff   bool         `json:"supports_handoff"`
	SupportsFeedback  bool         `json:"supports_feedback"`
	// Metadata 携带 optional runtime-specific 信息（如 Codex hook version、Cohors team role）。
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Validate 检查 descriptor 必填字段。
func (d AdapterDescriptor) Validate() error {
	if strings.TrimSpace(d.AdapterID) == "" {
		return NewStableError(ErrCodeValidationFailed, "adapter_id is required")
	}
	if strings.TrimSpace(d.Runtime) == "" {
		return NewStableError(ErrCodeValidationFailed, "runtime is required")
	}
	if len(d.SupportedVersions) == 0 {
		return NewStableError(ErrCodeValidationFailed, "supported_versions is required")
	}
	for _, v := range d.SupportedVersions {
		if strings.TrimSpace(v) == "" {
			return NewStableError(ErrCodeValidationFailed, "supported_versions contains empty value")
		}
	}
	for _, cap := range d.Capabilities {
		if !validCapabilities[cap] {
			return NewStableError(ErrCodeValidationFailed, fmt.Sprintf("unknown capability: %q", cap))
		}
	}
	return nil
}

// NegotiateResult 是 capability negotiation 的结果。
type NegotiateResult struct {
	Compatible         bool         `json:"compatible"`
	NegotiatedVersion  string       `json:"negotiated_version,omitempty"`
	SharedCapabilities []Capability `json:"shared_capabilities,omitempty"`
	Reason             string       `json:"reason,omitempty"`
	Degraded           bool         `json:"degraded,omitempty"`
}

// Negotiate 判断本地 runtime 是否与 adapter descriptor 兼容。
// localVersions 是本地支持的 schema 版本列表，requestedCapabilities 是本地需要的能力。
// 版本交集为空或能力不满足时返回 degraded=false，compatible=false。
func Negotiate(d AdapterDescriptor, localVersions []string, requestedCapabilities []Capability) NegotiateResult {
	versionMatch := false
	var matched string
	for _, lv := range localVersions {
		for _, rv := range d.SupportedVersions {
			if lv == rv {
				versionMatch = true
				matched = lv
				break
			}
		}
		if versionMatch {
			break
		}
	}
	if !versionMatch {
		return NegotiateResult{
			Compatible: false,
			Reason:     fmt.Sprintf("no overlapping schema version between local %v and adapter %v", localVersions, d.SupportedVersions),
		}
	}
	// 检查请求的能力是否都被 adapter 支持
	adapterCaps := make(map[Capability]bool, len(d.Capabilities))
	for _, c := range d.Capabilities {
		adapterCaps[c] = true
	}
	var shared []Capability
	for _, req := range requestedCapabilities {
		if adapterCaps[req] {
			shared = append(shared, req)
		}
	}
	allMet := len(shared) == len(requestedCapabilities)
	return NegotiateResult{
		Compatible:         allMet,
		NegotiatedVersion:  matched,
		SharedCapabilities: shared,
		Degraded:           !allMet && len(shared) > 0,
		Reason: func() string {
			if allMet {
				return ""
			}
			return "adapter supports subset of requested capabilities"
		}(),
	}
}
