package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Workbench continuity typed projection 合同（pinax-workbench-continuity-projection-v1）。
//
// Workbench BFF 消费的唯一 machine envelope；全部字段 bounded，不携带 raw note/
// handoff 正文、transcript、provider payload、credential 或绝对路径。

const (
	WorkbenchContinuityProjectionSchemaVersion = "pinax.workbench.continuity_projection.v1"
	WorkbenchProviderPacketSchemaVersion       = "pinax.provider_packet.v1"
	WorkbenchContinuityContractIdentity        = "pinax.workbench.continuity"
	WorkbenchContinuityContractVersion         = "v1"
	// WorkbenchProjectionDefaultTTL 是投影默认时效；basis 是 evidence 观测时间
	// 而非生成时间（refresh-only，消费端不得本地续命）。
	WorkbenchProjectionDefaultTTLSeconds = 600
	WorkbenchProjectionMinTTLSeconds     = 60
)

// WorkbenchProjectionContract 是 envelope/packet 共用的合同标识；digest 对
// canonical JSON（identity/version/actions/errors，无时间戳）稳定。
type WorkbenchProjectionContract struct {
	Identity string `json:"identity"`
	Version  string `json:"version"`
	Digest   string `json:"digest"`
}

type WorkbenchProjectionBinding struct {
	Status        string `json:"status"`
	Ready         bool   `json:"ready"`
	VaultRef      string `json:"vault_ref,omitempty"`
	Scope         string `json:"scope,omitempty"`
	VaultResolved bool   `json:"vault_resolved"`
	ScopeValid    bool   `json:"scope_valid"`
}

// WorkbenchResumeCard 是 ContinuityPack 的显式选字段有界投影：sections 计数化，
// 不透传未来 pack 字段。
type WorkbenchResumeCard struct {
	Objective    string   `json:"objective,omitempty"`
	CurrentState string   `json:"current_state,omitempty"`
	Task         string   `json:"task,omitempty"`
	Sections     []string `json:"sections,omitempty"`
	Sources      string   `json:"sources,omitempty"`
	Handoff      string   `json:"handoff,omitempty"`
	Conflicts    int      `json:"conflicts"`
}

type WorkbenchProjectionFreshness struct {
	Basis      string `json:"basis"`
	ObservedAt string `json:"observed_at"`
	ExpiresAt  string `json:"expires_at"`
	TTLSeconds int    `json:"ttl_seconds"`
}

type WorkbenchProjectionRecovery struct {
	Code   string `json:"code"`
	Action string `json:"action"`
}

type WorkbenchContinuityProjection struct {
	SchemaVersion string                       `json:"schema_version"`
	Contract      WorkbenchProjectionContract  `json:"contract"`
	ProjectRef    string                       `json:"project_ref"`
	Binding       WorkbenchProjectionBinding   `json:"binding"`
	ResumeCard    *WorkbenchResumeCard         `json:"resume_card,omitempty"`
	Freshness     WorkbenchProjectionFreshness `json:"freshness"`
	Recovery      *WorkbenchProjectionRecovery `json:"recovery,omitempty"`
}

// WorkbenchProviderPacket 是根仓消费的紧凑合同描述（pinax.provider_packet.v1）。
type WorkbenchProviderPacket struct {
	SchemaVersion string                      `json:"schema_version"`
	Contract      WorkbenchProjectionContract `json:"contract"`
	Owner         string                      `json:"owner"`
	Availability  WorkbenchPacketAvailability `json:"availability"`
	Actions       []WorkbenchPacketAction     `json:"actions"`
	ScopeRevision string                      `json:"scope_revision"`
	Errors        []string                    `json:"errors"`
	Recovery      WorkbenchPacketRecovery     `json:"recovery"`
	EvidenceRefs  []string                    `json:"evidence_refs"`
}

type WorkbenchPacketAvailability struct {
	Mode  string `json:"mode"`
	Entry string `json:"entry"`
}

type WorkbenchPacketAction struct {
	Name     string `json:"name"`
	Effect   string `json:"effect"`
	Entry    string `json:"entry"`
	Requires string `json:"requires,omitempty"`
	Receipt  string `json:"receipt,omitempty"`
}

type WorkbenchPacketRecovery struct {
	Policy string `json:"policy"`
}

// workbenchContractDigestScope 是 digest 的 canonical 输入域（无时间戳）。
type workbenchContractDigestScope struct {
	Identity string   `json:"identity"`
	Version  string   `json:"version"`
	Actions  []string `json:"actions"`
	Errors   []string `json:"errors"`
}

// ComputeWorkbenchContractDigest 对 canonical 合同域取 sha256；输入排序后序列化，
// 跨运行与跨 envelope/packet 稳定。
func ComputeWorkbenchContractDigest(actions, errors []string) string {
	scope := workbenchContractDigestScope{
		Identity: WorkbenchContinuityContractIdentity,
		Version:  WorkbenchContinuityContractVersion,
		Actions:  sortedCopy(actions),
		Errors:   sortedCopy(errors),
	}
	encoded, err := json.Marshal(scope)
	if err != nil {
		// 纯字符串切片 marshal 不会失败；防御性固定值。
		return "sha256:unavailable"
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// WorkbenchFreshnessFromEvidence 按 evidence 观测时间派生时效（refresh-only 合同）。
func WorkbenchFreshnessFromEvidence(observedAt time.Time, ttlSeconds int, now time.Time) WorkbenchProjectionFreshness {
	if ttlSeconds <= 0 {
		ttlSeconds = WorkbenchProjectionDefaultTTLSeconds
	}
	if ttlSeconds < WorkbenchProjectionMinTTLSeconds {
		ttlSeconds = WorkbenchProjectionMinTTLSeconds
	}
	if observedAt.IsZero() {
		observedAt = now.UTC()
	}
	observedAt = observedAt.UTC()
	return WorkbenchProjectionFreshness{
		Basis:      "evidence_observed_at",
		ObservedAt: observedAt.Format(time.RFC3339),
		ExpiresAt:  observedAt.Add(time.Duration(ttlSeconds) * time.Second).Format(time.RFC3339),
		TTLSeconds: ttlSeconds,
	}
}
