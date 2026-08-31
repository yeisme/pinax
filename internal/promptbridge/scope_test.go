package promptbridge

import (
	"context"
	"testing"

	promptrepo "github.com/yeisme/promptrepo"
)

type staticSource struct {
	binding ScopeBinding
	err     error
}

func (s staticSource) ScopeRepositories(context.Context) (ScopeBinding, error) {
	return s.binding, s.err
}

func view(id string, enabled bool) promptrepo.RepositoryView {
	return promptrepo.RepositoryView{Profile: promptrepo.RepositoryProfile{ID: id, Enabled: enabled}}
}

func ids(set RepositorySet) []string {
	result := make([]string, 0, len(set.Repositories))
	for _, repository := range set.Repositories {
		result = append(result, repository.Profile.ID)
	}
	return result
}

// TestEffectiveRepositorySetPrecedence locks the user/organization/project/
// session precedence and deny-wins rules from the change spec.
func TestEffectiveRepositorySetPrecedence(t *testing.T) {
	bridge := New(&fakeClient{page: promptrepo.RepositoryPage{Repositories: []promptrepo.RepositoryView{
		view("official", true), view("team", true), view("sandbox", true), view("disabled", false),
	}}})

	cases := []struct {
		name   string
		opts   EffectiveScopeOptions
		want   []string
		denied map[string]string
	}{
		{
			name: "user scope only lists enabled repositories",
			opts: EffectiveScopeOptions{},
			want: []string{"official", "sandbox", "team"},
		},
		{
			name: "organization pins restrict the selection",
			opts: EffectiveScopeOptions{
				Organization: staticSource{binding: ScopeBinding{RepositoryIDs: []string{"official", "team"}}},
			},
			want: []string{"official", "team"},
		},
		{
			name: "project pins override organization pins",
			opts: EffectiveScopeOptions{
				Organization: staticSource{binding: ScopeBinding{RepositoryIDs: []string{"official"}}},
				Project:      staticSource{binding: ScopeBinding{RepositoryIDs: []string{"team", "sandbox"}}},
			},
			want: []string{"sandbox", "team"},
		},
		{
			name: "session pins override project pins",
			opts: EffectiveScopeOptions{
				Project: staticSource{binding: ScopeBinding{RepositoryIDs: []string{"team", "sandbox"}}},
				Session: SessionScope{RepositoryIDs: []string{"sandbox"}},
			},
			want: []string{"sandbox"},
		},
		{
			name: "deny at any scope wins over every pin",
			opts: EffectiveScopeOptions{
				Organization: staticSource{binding: ScopeBinding{RepositoryIDs: []string{"official", "team"}, Deny: []string{"official"}}},
				Project:      staticSource{binding: ScopeBinding{RepositoryIDs: []string{"official", "team"}}},
				Session:      SessionScope{RepositoryIDs: []string{"official", "team"}},
			},
			want:   []string{"team"},
			denied: map[string]string{"official": ScopeOrganization},
		},
		{
			name: "session deny is the most specific denial",
			opts: EffectiveScopeOptions{
				Organization: staticSource{binding: ScopeBinding{Deny: []string{"team"}}},
				Session:      SessionScope{Deny: []string{"team"}},
			},
			want:   []string{"official", "sandbox"},
			denied: map[string]string{"team": ScopeSession},
		},
		{
			name: "session deny cannot be overridden by pins",
			opts: EffectiveScopeOptions{
				Project: staticSource{binding: ScopeBinding{RepositoryIDs: []string{"team"}}},
				Session: SessionScope{RepositoryIDs: []string{"team"}, Deny: []string{"team"}},
			},
			want:   []string{},
			denied: map[string]string{"team": ScopeSession},
		},
		{
			name: "pins naming unknown repositories are ignored",
			opts: EffectiveScopeOptions{
				Organization: staticSource{binding: ScopeBinding{RepositoryIDs: []string{"ghost", "official"}}},
			},
			want: []string{"official"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set, err := bridge.EffectiveRepositorySet(context.Background(), tc.opts)
			if err != nil {
				t.Fatalf("effective set failed: %v", err)
			}
			if got := ids(set); !equalStrings(got, tc.want) {
				t.Fatalf("repositories = %v, want %v", got, tc.want)
			}
			for id, scope := range tc.denied {
				decision, ok := decisionFor(set, id)
				if !ok {
					t.Fatalf("repository %s missing a policy decision", id)
				}
				if decision.Allowed {
					t.Fatalf("repository %s must be denied", id)
				}
				if decision.DenialScope != scope {
					t.Fatalf("repository %s denial scope = %s, want %s", id, decision.DenialScope, scope)
				}
			}
		})
	}
}

func TestEffectiveRepositorySetRecordsPrecedenceAndSources(t *testing.T) {
	bridge := New(&fakeClient{page: promptrepo.RepositoryPage{Repositories: []promptrepo.RepositoryView{view("official", true)}}})
	set, err := bridge.EffectiveRepositorySet(context.Background(), EffectiveScopeOptions{
		Project: staticSource{binding: ScopeBinding{RepositoryIDs: []string{"official"}, PolicyDigest: "sha256:policy"}},
	})
	if err != nil {
		t.Fatalf("effective set failed: %v", err)
	}
	if !equalStrings(set.Precedence, ScopePrecedence) {
		t.Fatalf("precedence = %v", set.Precedence)
	}
	decision, ok := decisionFor(set, "official")
	if !ok || !decision.Allowed {
		t.Fatal("official must be allowed with a decision")
	}
	if !containsScope(decision.Sources, ScopeProject) {
		t.Fatalf("decision sources = %v, want project contribution", decision.Sources)
	}
}

func decisionFor(set RepositorySet, id string) (PolicyDecision, bool) {
	for _, decision := range set.Decisions {
		if decision.RepositoryID == id {
			return decision, true
		}
	}
	return PolicyDecision{}, false
}

func containsScope(scopes []string, want string) bool {
	for _, scope := range scopes {
		if scope == want {
			return true
		}
	}
	return false
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
