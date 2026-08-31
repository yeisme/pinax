package app

import (
	"context"
	"time"

	"github.com/yeisme/pinax/internal/agentcontinuity"
	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

const AgentGroundingEvidenceSchemaVersion = "pinax.agent_grounding_evidence.v0.1"

type AgentGroundingState string

const (
	AgentGroundingGrounded            AgentGroundingState = "grounded"
	AgentGroundingPartiallyGrounded   AgentGroundingState = "partially_grounded"
	AgentGroundingUngrounded          AgentGroundingState = "ungrounded"
	AgentGroundingUnavailable         AgentGroundingState = "grounding_unavailable"
	AgentGroundingFresh               string              = "fresh"
	AgentGroundingStale               string              = "stale"
	AgentGroundingFreshnessUnmeasured string              = "not_measured"
)

// AgentGroundingEvidence 是 Personal Assistant 可消费的 pre-1.0 readonly
// evidence。Pinax 只证明 source coverage，不拥有 turn 的 canonical state。
type AgentGroundingEvidence struct {
	SchemaVersion        string              `json:"schema_version"`
	State                AgentGroundingState `json:"state"`
	SourceTotal          int                 `json:"source_total"`
	SourceOpenable       int                 `json:"source_openable"`
	CandidateSourceTotal int                 `json:"candidate_source_total"`
	SourceMissing        int                 `json:"source_missing,omitempty"`
	SourceStale          int                 `json:"source_stale,omitempty"`
	SourceAmbiguous      int                 `json:"source_ambiguous,omitempty"`
	FreshnessStatus      string              `json:"freshness_status"`
}

type PersonalAssistantContextProjection struct {
	Pack      agentprotocol.ContextPack `json:"pack"`
	Grounding AgentGroundingEvidence    `json:"grounding"`
}

type PersonalAssistantMemoryRecallProjection struct {
	Memories  []agentprotocol.MemoryRecord `json:"memories"`
	Grounding AgentGroundingEvidence       `json:"grounding"`
}

type PersonalAssistantHandoffReadProjection struct {
	Handoffs  []agentmemory.AgentHandoffRow `json:"handoffs"`
	Grounding AgentGroundingEvidence        `json:"grounding"`
}

type agentGroundingResolution struct {
	Coverage       agentcontinuity.SourceCoverage
	LatestObserved time.Time
	Openable       map[string]bool
}

// AgentGroundingUnavailableEvidence 供 consumer 在 Pinax transport/service
// error 时使用。成功路径不得伪造 unavailable。
func AgentGroundingUnavailableEvidence() AgentGroundingEvidence {
	return AgentGroundingEvidence{
		SchemaVersion:   AgentGroundingEvidenceSchemaVersion,
		State:           AgentGroundingUnavailable,
		FreshnessStatus: AgentGroundingFreshnessUnmeasured,
	}
}

// AgentContextForPersonalAssistant 编译 context 后验证来源；全部来源失效的
// sourced entry 不进入 projection，避免删除 transcript 后继续泄漏摘要。
func (s *AgentMemoryService) AgentContextForPersonalAssistant(
	ctx context.Context,
	req AgentContextRequest,
	repoRoot string,
) (PersonalAssistantContextProjection, error) {
	pack, err := s.AgentContextRuntime(ctx, req)
	if err != nil {
		return PersonalAssistantContextProjection{}, err
	}
	resolution := resolveAgentGroundingSources(ctx, req.VaultPath, pack.Sources, repoRoot)
	pack = filterContextPackForOpenableSources(pack, resolution.Openable)
	return PersonalAssistantContextProjection{Pack: pack, Grounding: groundingEvidence(resolution)}, nil
}

func (s *AgentMemoryService) AgentMemoryRecallForPersonalAssistant(
	ctx context.Context,
	vaultPath string,
	query RecallQuery,
	repoRoot string,
) (PersonalAssistantMemoryRecallProjection, error) {
	memories, err := s.AgentMemoryRecallQuery(ctx, vaultPath, query)
	if err != nil {
		return PersonalAssistantMemoryRecallProjection{}, err
	}
	sources := make(agentprotocol.SourceRefList, 0)
	for _, memory := range memories {
		sources = append(sources, memory.Sources...)
	}
	resolution := resolveAgentGroundingSources(ctx, vaultPath, sources, repoRoot)
	filtered := make([]agentprotocol.MemoryRecord, 0, len(memories))
	for _, memory := range memories {
		openable := filterOpenableSources(memory.Sources, resolution.Openable)
		if len(memory.Sources) > 0 && len(openable) == 0 {
			continue
		}
		memory.Sources = openable
		filtered = append(filtered, memory)
	}
	return PersonalAssistantMemoryRecallProjection{Memories: filtered, Grounding: groundingEvidence(resolution)}, nil
}

func (s *AgentMemoryService) AgentHandoffReadForPersonalAssistant(
	ctx context.Context,
	vaultPath string,
	scope agentprotocol.Scope,
	repoRoot string,
) (PersonalAssistantHandoffReadProjection, error) {
	handoffs, err := s.AgentHandoffList(ctx, vaultPath, scope)
	if err != nil {
		return PersonalAssistantHandoffReadProjection{}, err
	}
	sources := make(agentprotocol.SourceRefList, 0)
	for _, handoff := range handoffs {
		sources = append(sources, handoff.Sources...)
	}
	resolution := resolveAgentGroundingSources(ctx, vaultPath, sources, repoRoot)
	filtered := make([]agentmemory.AgentHandoffRow, 0, len(handoffs))
	for _, handoff := range handoffs {
		openable := filterOpenableSources(handoff.Sources, resolution.Openable)
		if len(handoff.Sources) > 0 && len(openable) == 0 {
			continue
		}
		handoff.Sources = openable
		filtered = append(filtered, handoff)
	}
	return PersonalAssistantHandoffReadProjection{Handoffs: filtered, Grounding: groundingEvidence(resolution)}, nil
}

func resolveAgentGroundingSources(
	ctx context.Context,
	vaultPath string,
	sources agentprotocol.SourceRefList,
	repoRoot string,
) agentGroundingResolution {
	result := agentGroundingResolution{Openable: make(map[string]bool)}
	seen := make(map[string]bool)
	for _, source := range sources {
		key := agentGroundingSourceKey(source)
		if seen[key] {
			continue
		}
		seen[key] = true
		single := resolveContinuitySourceCoverage(ctx, vaultPath, agentprotocol.SourceRefList{source}, repoRoot)
		result.Coverage.Total++
		result.Coverage.Resolved += single.Coverage.Resolved
		result.Coverage.Missing += single.Coverage.Missing
		result.Coverage.Stale += single.Coverage.Stale
		result.Coverage.Ambiguous += single.Coverage.Ambiguous
		if single.Coverage.Resolved == 1 {
			result.Openable[key] = true
		}
		if single.LatestObserved.After(result.LatestObserved) {
			result.LatestObserved = single.LatestObserved
		}
	}
	return result
}

func groundingEvidence(resolution agentGroundingResolution) AgentGroundingEvidence {
	coverage := resolution.Coverage
	evidence := AgentGroundingEvidence{
		SchemaVersion:        AgentGroundingEvidenceSchemaVersion,
		CandidateSourceTotal: coverage.Total,
		SourceMissing:        coverage.Missing,
		SourceStale:          coverage.Stale,
		SourceAmbiguous:      coverage.Ambiguous,
		FreshnessStatus:      AgentGroundingFreshnessUnmeasured,
	}
	if coverage.Stale > 0 {
		evidence.FreshnessStatus = AgentGroundingStale
	} else if !resolution.LatestObserved.IsZero() {
		evidence.FreshnessStatus = AgentGroundingFresh
	}

	switch {
	case coverage.Total == 0 || coverage.Resolved == 0:
		evidence.State = AgentGroundingUngrounded
	case coverage.Resolved == coverage.Total && coverage.Missing == 0 && coverage.Stale == 0 && coverage.Ambiguous == 0:
		evidence.State = AgentGroundingGrounded
		evidence.SourceTotal = coverage.Total
		evidence.SourceOpenable = coverage.Resolved
	default:
		evidence.State = AgentGroundingPartiallyGrounded
		evidence.SourceTotal = coverage.Total
		evidence.SourceOpenable = coverage.Resolved
	}
	return evidence
}

func filterContextPackForOpenableSources(
	pack agentprotocol.ContextPack,
	openable map[string]bool,
) agentprotocol.ContextPack {
	pack.Facts = filterContextEntries(pack.Facts, openable)
	pack.Decisions = filterContextEntries(pack.Decisions, openable)
	pack.Preferences = filterContextEntries(pack.Preferences, openable)
	pack.Procedures = filterContextEntries(pack.Procedures, openable)
	pack.OpenTasks = filterContextEntries(pack.OpenTasks, openable)
	pack.FailedAttempts = filterContextEntries(pack.FailedAttempts, openable)

	retained := make(map[string]bool)
	pack.Sources = nil
	seenSources := make(map[string]bool)
	for _, entries := range [][]agentprotocol.ContextEntry{
		pack.Facts, pack.Decisions, pack.Preferences, pack.Procedures, pack.OpenTasks, pack.FailedAttempts,
	} {
		for _, entry := range entries {
			retained[entry.MemoryID] = true
			for _, source := range entry.Sources {
				key := agentGroundingSourceKey(source)
				if seenSources[key] {
					continue
				}
				seenSources[key] = true
				pack.Sources = append(pack.Sources, source)
			}
		}
	}

	conflicts := make([]agentprotocol.ContextConflict, 0, len(pack.Conflicts))
	for _, conflict := range pack.Conflicts {
		ids := make([]string, 0, len(conflict.MemoryIDs))
		for _, id := range conflict.MemoryIDs {
			if retained[id] {
				ids = append(ids, id)
			}
		}
		if len(ids) >= 2 {
			conflict.MemoryIDs = ids
			conflicts = append(conflicts, conflict)
		}
	}
	pack.Conflicts = conflicts
	return pack
}

func filterContextEntries(entries []agentprotocol.ContextEntry, openable map[string]bool) []agentprotocol.ContextEntry {
	filtered := make([]agentprotocol.ContextEntry, 0, len(entries))
	for _, entry := range entries {
		sources := filterOpenableSources(entry.Sources, openable)
		if len(entry.Sources) > 0 && len(sources) == 0 {
			continue
		}
		entry.Sources = sources
		filtered = append(filtered, entry)
	}
	return filtered
}

func filterOpenableSources(sources agentprotocol.SourceRefList, openable map[string]bool) agentprotocol.SourceRefList {
	filtered := make(agentprotocol.SourceRefList, 0, len(sources))
	seen := make(map[string]bool)
	for _, source := range sources {
		key := agentGroundingSourceKey(source)
		if !openable[key] || seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, source)
	}
	return filtered
}

func agentGroundingSourceKey(source agentprotocol.SourceRef) string {
	return source.Kind + "\x00" + source.Ref
}
