package promptbridge

import (
	"context"
	"fmt"
	"sort"
	"strings"

	promptrepo "github.com/yeisme/promptrepo"
)

// Scope levels ordered from broadest to most specific. A more specific scope's
// pin selection overrides broader scopes, but a deny at ANY scope always wins.
const (
	ScopeUser         = "user"
	ScopeOrganization = "organization"
	ScopeProject      = "project"
	ScopeSession      = "session"
)

// ScopePrecedence records the composition order applied to a request.
var ScopePrecedence = []string{ScopeUser, ScopeOrganization, ScopeProject, ScopeSession}

// ScopeBinding is one scope's repository binding and policy. Organization
// bindings come from a Template Registry service projection; project bindings
// come from a Pinax workspace. Both are optional sources; the user scope is
// the shared promptrepo store itself.
type ScopeBinding struct {
	Scope         string   `json:"scope"`
	RepositoryIDs []string `json:"repository_ids,omitempty"`
	Deny          []string `json:"deny,omitempty"`
	// PolicyDigest is an opaque, non-secret digest of the scope's policy for
	// provenance records; it is never a credential or source URI.
	PolicyDigest string `json:"policy_digest,omitempty"`
}

// SessionScope carries per-command overrides that affect only this command.
type SessionScope struct {
	// RepositoryIDs, when set, restricts the effective set to exactly these
	// repositories; a session cannot bypass any scope's deny.
	RepositoryIDs []string `json:"repository_ids,omitempty"`
	Deny          []string `json:"deny,omitempty"`
}

// RepositorySetSource is an optional interface providing a scope binding
// above the user level. Pinax wires organization (Registry projection) and
// project (workspace binding) providers through this port; absent sources
// simply do not constrain the set.
type RepositorySetSource interface {
	ScopeRepositories(ctx context.Context) (ScopeBinding, error)
}

// EffectiveScopeOptions collects the optional sources and session overrides
// for one effective-set evaluation.
type EffectiveScopeOptions struct {
	Organization RepositorySetSource
	Project      RepositorySetSource
	Session      SessionScope
}

// PolicyDecision records how one repository was evaluated across scopes.
type PolicyDecision struct {
	RepositoryID string   `json:"repository_id"`
	Allowed      bool     `json:"allowed"`
	Sources      []string `json:"sources,omitempty"`
	DenialScope  string   `json:"denial_scope,omitempty"`
	DenialReason string   `json:"denial_reason,omitempty"`
}

// RepositorySet is the effective, ordered set of repositories a command may
// read, plus the policy record explaining every allow and deny.
type RepositorySet struct {
	Repositories []promptrepo.RepositoryView `json:"repositories"`
	Decisions    []PolicyDecision            `json:"policy_decisions"`
	Precedence   []string                    `json:"precedence"`
}

// EffectiveRepositorySet composes user, organization, project, and session
// scopes into the repository set a catalog command may read.
//
// Rules, in order:
//  1. The user scope contributes every enabled profile in the shared store.
//  2. Pin selection: the most specific non-empty pin list (session over
//     project over organization) selects the candidate repositories; pinned
//     repositories must still exist in the shared store.
//  3. Deny wins across all scopes: a repository denied anywhere is excluded
//     with the most specific denial recorded.
func (b *Bridge) EffectiveRepositorySet(ctx context.Context, options EffectiveScopeOptions) (RepositorySet, error) {
	page, err := b.ListRepositories(ctx)
	if err != nil {
		return RepositorySet{}, err
	}

	bindings := make([]ScopeBinding, 0, 3)
	for _, source := range []struct {
		scope  string
		source RepositorySetSource
	}{
		{ScopeOrganization, options.Organization},
		{ScopeProject, options.Project},
	} {
		if source.source == nil {
			continue
		}
		binding, err := source.source.ScopeRepositories(ctx)
		if err != nil {
			return RepositorySet{}, fmt.Errorf("scope %s binding failed: %w", source.scope, err)
		}
		binding.Scope = source.scope
		bindings = append(bindings, binding)
	}

	enabled := make(map[string]promptrepo.RepositoryView, len(page.Repositories))
	for _, view := range page.Repositories {
		if view.Profile.Enabled {
			enabled[view.Profile.ID] = view
		}
	}

	// Denies: a deny at any scope excludes the repository; the most specific
	// denial is the one recorded.
	denies := make(map[string]PolicyDecision)
	recordDeny := func(scope string, ids []string) {
		for _, raw := range ids {
			id := strings.TrimSpace(raw)
			if id == "" {
				continue
			}
			existing, exists := denies[id]
			if !exists || scopeOrder(scope) > scopeOrder(existing.DenialScope) {
				denies[id] = PolicyDecision{
					RepositoryID: id,
					Allowed:      false,
					Sources:      append([]string{ScopeUser}, scope),
					DenialScope:  scope,
					DenialReason: fmt.Sprintf("repository denied by %s scope policy", scope),
				}
			}
		}
	}
	for _, binding := range bindings {
		recordDeny(binding.Scope, binding.Deny)
	}
	recordDeny(ScopeSession, options.Session.Deny)

	// Pin selection: start from all enabled user repositories, then apply the
	// most specific non-empty pin list. Pin lists are selections, not unions:
	// a project or session list replaces the organization selection.
	selected := make([]string, 0, len(enabled))
	for id := range enabled {
		selected = append(selected, id)
	}
	sort.Strings(selected)
	pins := pinSelection(bindings, options.Session)
	if pins != nil {
		selected = pins
	}

	set := RepositorySet{Precedence: append([]string{}, ScopePrecedence...)}
	for _, id := range selected {
		view, exists := enabled[id]
		if !exists {
			continue
		}
		if decision, denied := denies[id]; denied {
			set.Decisions = append(set.Decisions, decision)
			continue
		}
		set.Repositories = append(set.Repositories, view)
		set.Decisions = append(set.Decisions, PolicyDecision{
			RepositoryID: id,
			Allowed:      true,
			Sources:      scopesContributing(id, bindings, options.Session),
		})
	}
	sort.Slice(set.Decisions, func(i, j int) bool {
		return set.Decisions[i].RepositoryID < set.Decisions[j].RepositoryID
	})
	return set, nil
}

// pinSelection returns the most specific non-empty pin list across session,
// project, and organization bindings, or nil when no scope pins anything.
func pinSelection(bindings []ScopeBinding, session SessionScope) []string {
	candidates := make([]ScopeBinding, 0, len(bindings)+1)
	candidates = append(candidates, bindings...)
	candidates = append(candidates, ScopeBinding{Scope: ScopeSession, RepositoryIDs: session.RepositoryIDs})

	best := -1
	for _, binding := range candidates {
		if len(binding.RepositoryIDs) == 0 {
			continue
		}
		if order := scopeOrder(binding.Scope); order > best {
			best = order
		}
	}
	if best < 0 {
		return nil
	}
	for _, binding := range candidates {
		if scopeOrder(binding.Scope) == best && len(binding.RepositoryIDs) > 0 {
			ids := make([]string, 0, len(binding.RepositoryIDs))
			for _, id := range binding.RepositoryIDs {
				if trimmed := strings.TrimSpace(id); trimmed != "" {
					ids = append(ids, trimmed)
				}
			}
			sort.Strings(ids)
			return ids
		}
	}
	return nil
}

// scopesContributing lists the scopes whose binding named the repository,
// starting from the user scope that always observes the shared store.
func scopesContributing(id string, bindings []ScopeBinding, session SessionScope) []string {
	sources := []string{ScopeUser}
	for _, binding := range bindings {
		for _, bound := range binding.RepositoryIDs {
			if strings.TrimSpace(bound) == id {
				sources = append(sources, binding.Scope)
			}
		}
	}
	for _, bound := range session.RepositoryIDs {
		if strings.TrimSpace(bound) == id {
			sources = append(sources, ScopeSession)
		}
	}
	return sources
}

func scopeOrder(scope string) int {
	for index, candidate := range ScopePrecedence {
		if candidate == scope {
			return index
		}
	}
	return -1
}
