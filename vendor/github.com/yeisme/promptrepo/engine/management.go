package engine

import (
	"context"
	"sort"
	"strings"

	"github.com/yeisme/promptrepo"
)

var _ promptrepo.RepositorySetReader = (*Manager)(nil)
var _ promptrepo.PolicyEvaluator = (*Manager)(nil)

// EffectiveRepositorySet composes caller-owned external layers with canonical
// user repository state. It is read-only and never persists external bindings.
func (m *Manager) EffectiveRepositorySet(_ context.Context, request promptrepo.EffectiveRepositorySetRequest) (promptrepo.RepositorySet, error) {
	state, err := m.readState()
	if err != nil {
		return promptrepo.RepositorySet{}, err
	}

	localIDs := make([]string, 0, len(state.Profiles))
	localIDSet := make(map[string]struct{}, len(state.Profiles))
	for id := range state.Profiles {
		localIDs = append(localIDs, id)
		localIDSet[id] = struct{}{}
	}
	sort.Strings(localIDs)

	external := make([]promptrepo.RepositorySetCandidate, 0, len(request.Repositories)+len(localIDs))
	for _, candidate := range request.Repositories {
		if candidate.Scope == promptrepo.RepositoryScopeUser {
			continue
		}
		if _, collision := localIDSet[strings.TrimSpace(candidate.RepositoryID)]; collision {
			continue
		}
		external = append(external, candidate)
	}
	for _, id := range localIDs {
		profile := state.Profiles[id]
		health := state.Health[id]
		external = append(external, promptrepo.RepositorySetCandidate{
			RepositoryID: id,
			Scope:        promptrepo.RepositoryScopeUser,
			Trust:        profile.Trust,
			Health:       health.State,
			Enabled:      profile.Enabled,
		})
	}
	request.Repositories = external
	request.Bindings = append(request.Bindings, promptrepo.RepositoryScopeBinding{
		Scope:         promptrepo.RepositoryScopeUser,
		ScopeRef:      "local-user",
		RepositoryIDs: localIDs,
		Priority:      -1 << 30,
		Enabled:       true,
	})
	return promptrepo.ComposeRepositorySet(request)
}

// EvaluatePolicy is a side-effect-free adapter for the public pure evaluator.
func (m *Manager) EvaluatePolicy(_ context.Context, request promptrepo.EvaluatePolicyRequest) (promptrepo.PolicyDecision, error) {
	return promptrepo.EvaluateRepositoryPolicy(request)
}
