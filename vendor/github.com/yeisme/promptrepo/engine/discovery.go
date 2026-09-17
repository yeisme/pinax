package engine

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yeisme/promptrepo"
	"github.com/yeisme/promptrepo/source"
)

const discoveryTTL = 5 * time.Minute

func (m *Manager) SetRepository(_ context.Context, id string, patch promptrepo.RepositoryPatch) (promptrepo.RepositoryView, error) {
	var view promptrepo.RepositoryView
	err := m.withWriteState(func(state *stateFile) error {
		p, ok := state.Profiles[id]
		if !ok {
			return promptrepo.NewError(promptrepo.CodeNotFound, "repository was not found", false, nil)
		}
		invalidate := patch.Source != nil || patch.Revision != nil || patch.CredentialRef != nil
		if patch.Source != nil {
			kind, err := source.DetectKind(*patch.Source)
			if err != nil {
				return err
			}
			p.Source = *patch.Source
			p.SourceKind = kind
		}
		if patch.Revision != nil {
			p.Revision = *patch.Revision
		}
		if patch.Trust != nil {
			p.Trust = *patch.Trust
		}
		if patch.CredentialRef != nil {
			p.CredentialRef = *patch.CredentialRef
		}
		p.UpdatedAt = m.now().UTC()
		state.Profiles[id] = p
		if invalidate {
			delete(state.Snapshots, id)
			state.Health[id] = promptrepo.RepositoryHealth{State: "configured", CheckedAt: p.UpdatedAt}
		}
		view = viewFor(state, id)
		return nil
	})
	return view, err
}

// refreshDiscovery performs network I/O outside the state lock. Each result is
// committed only if its profile and previous snapshot still match, so disabling,
// removing or editing a source during refresh cannot resurrect obsolete state.
func (m *Manager) refreshDiscovery(ctx context.Context, profiles []promptrepo.RepositoryProfile, before *stateFile) error {
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	// Adapter I/O is bounded; the Git adapter serializes access to shared mirrors.
	for _, p := range profiles {
		wg.Add(1)
		go func(p promptrepo.RepositoryProfile) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			sub, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			adapter, e := m.sources.Resolve(p.Source)
			var result source.SyncResult
			if e == nil {
				result, e = adapter.Sync(sub, p, m.cacheRoot)
			}
			if sub.Err() != nil || errors.Is(e, context.DeadlineExceeded) {
				e = promptrepo.NewError("SOURCE_TIMEOUT", "source refresh timed out", true, nil)
			}
			err := m.withWriteState(func(state *stateFile) error {
				if !reflect.DeepEqual(state.Profiles[p.ID], p) || !reflect.DeepEqual(state.Snapshots[p.ID], before.Snapshots[p.ID]) {
					return nil
				}
				if e != nil {
					m.recordSyncFailure(state, p.ID, e)
					return nil
				}
				for i := range result.Catalog.Solutions {
					result.Catalog.Solutions[i].RepositoryID = p.ID
				}
				snap := promptrepo.Snapshot{RepositoryID: p.ID, Revision: result.Revision, Digest: result.Catalog.Digest, FetchedAt: m.now().UTC(), Catalog: result.Catalog}
				state.Snapshots[p.ID] = snap
				state.Health[p.ID] = promptrepo.RepositoryHealth{State: "ready", CheckedAt: snap.FetchedAt, SnapshotAt: snap.FetchedAt}
				return nil
			})
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(p)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return firstErr
}

func (m *Manager) Discover(ctx context.Context, req promptrepo.DiscoveryRequest) (promptrepo.DiscoveryResult, error) {
	out := promptrepo.DiscoveryResult{Status: "success", Sources: []promptrepo.DiscoverySource{}, Solutions: []promptrepo.DiscoverySolution{}}
	if req.Limit < 0 || req.Offset < 0 || (req.Refresh && req.Offline) {
		return out, promptrepo.NewError(promptrepo.CodeInvalidRequest, "invalid pagination or refresh options", false, nil)
	}
	state, err := m.discoveryState(ctx, req, nil)
	if err != nil {
		return out, err
	}
	return m.projectDiscovery(state, req)
}

func (m *Manager) discoveryState(ctx context.Context, req promptrepo.DiscoveryRequest, selected []string) (*stateFile, error) {
	state, err := m.readState()
	if err != nil {
		return nil, err
	}
	if req.Repository != "" {
		if _, ok := state.Profiles[req.Repository]; !ok {
			return nil, promptrepo.NewError(promptrepo.CodeNotFound, "repository was not found", false, nil)
		}
	}
	var refresh []promptrepo.RepositoryProfile
	for id, p := range state.Profiles {
		if !p.Enabled || (req.Repository != "" && id != req.Repository) || (len(selected) > 0 && !has(selected, id)) {
			continue
		}
		snap, ok := state.Snapshots[id]
		if !req.Offline && (req.Refresh || !ok || m.now().Sub(snap.FetchedAt) >= discoveryTTL) {
			refresh = append(refresh, p)
		}
	}
	if len(refresh) > 0 {
		if err = m.refreshDiscovery(ctx, refresh, state); err != nil {
			return nil, err
		}
		state, err = m.readState()
		if err != nil {
			return nil, err
		}
	}
	if len(selected) > 0 {
		for id := range state.Profiles {
			if !has(selected, id) {
				delete(state.Profiles, id)
				delete(state.Snapshots, id)
				delete(state.Health, id)
			}
		}
	}
	return state, nil
}

func (m *Manager) projectDiscovery(state *stateFile, req promptrepo.DiscoveryRequest) (promptrepo.DiscoveryResult, error) {
	out := promptrepo.DiscoveryResult{Status: "success", Sources: []promptrepo.DiscoverySource{}, Solutions: []promptrepo.DiscoverySolution{}}

	ids := []string{}
	for id := range state.Profiles {
		if req.Repository == "" || req.Repository == id {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	categories, capabilities, consumers := map[string]int{}, map[string]int{}, map[string]int{}
	available, unavailable := 0, 0
	for _, id := range ids {
		p := state.Profiles[id]
		snap, ok := state.Snapshots[id]
		h := state.Health[id]
		src := promptrepo.DiscoverySource{ID: id, SourceKind: p.SourceKind, Enabled: p.Enabled, State: h.State, ErrorCode: h.Code, Reason: discoveryReason(h), PinnedRevision: p.Revision, Revision: snap.Revision, FetchedAt: snap.FetchedAt, Stale: !ok || m.now().Sub(snap.FetchedAt) >= discoveryTTL}
		src.Counts = countSolutions(snap.Catalog.Solutions)
		src.Counts.Repositories = 1
		if !p.Enabled {
			src.State = "disabled"
			out.Sources = append(out.Sources, src)
			continue
		}
		out.Counts = addCounts(out.Counts, src.Counts)
		if !ok {
			unavailable++
			if src.ErrorCode == "" {
				src.ErrorCode = "NOT_SYNCED"
				src.Reason = "not_synced"
			}
			out.Sources = append(out.Sources, src)
			continue
		}
		available++
		if src.Stale || h.State != "ready" {
			out.Status = "partial"
		}
		annotations := map[[4]string]promptrepo.TemplateDiscoveryAnnotation{}
		for _, a := range snap.Catalog.Discovery {
			annotations[[4]string{a.PackageID, a.SolutionID, a.Role, a.Locale}] = a
		}
		var matched []promptrepo.DiscoverySolution
		solutions := append([]promptrepo.Solution{}, snap.Catalog.Solutions...)
		sort.Slice(solutions, func(i, j int) bool {
			return solutions[i].PackageID+"/"+solutions[i].ID+"@"+solutions[i].Version < solutions[j].PackageID+"/"+solutions[j].ID+"@"+solutions[j].Version
		})
		for _, s := range solutions {
			if req.Category != "" && s.Category != req.Category {
				continue
			}
			if req.Capability != "" && !has(s.Capabilities, req.Capability) {
				continue
			}
			locale := req.Locale
			if locale == "" {
				locale = "en"
			}
			_, display := chooseLocale(s, locale, snap.Catalog.Repository.DefaultLocale)
			row := promptrepo.DiscoverySolution{RepositoryID: id, PackageID: s.PackageID, ID: s.ID, Version: s.Version, Title: display.Title, Summary: display.Summary, Category: s.Category, Capabilities: s.Capabilities, Templates: []promptrepo.DiscoveryTemplate{}}
			for _, t := range s.Templates {
				annotation := annotations[[4]string{s.PackageID, s.ID, t.Role, t.Locale}]
				media, owners := roleMetadata(s, annotation)
				if req.Media != "" && !has(media, req.Media) {
					continue
				}
				if req.Consumer != "" && !has(owners, req.Consumer) {
					continue
				}
				ref := promptrepo.FormatRef(promptrepo.Ref{RepositoryID: id, PackageID: s.PackageID, SolutionID: s.ID, Version: s.Version, Locale: t.Locale}) + "&kind=template&role=" + url.QueryEscape(t.Role)
				status := annotation.CompilerStatus
				if status == "" {
					status = "not_checked"
				}
				title := annotation.Title
				if title == "" {
					title = t.Role
				}
				summary := annotation.Summary
				if summary == "" {
					summary = display.Summary
				}
				row.Templates = append(row.Templates, promptrepo.DiscoveryTemplate{Role: t.Role, Locale: t.Locale, Title: title, Summary: summary, Ref: ref, OwnerRef: annotation.OwnerRef, Digest: t.Digest, Media: media, Consumers: owners, CompilerStatus: status})
				categories[s.Category]++
				for _, c := range unique(s.Capabilities) {
					capabilities[c]++
				}
				for _, c := range owners {
					consumers[c]++
				}
			}
			if len(row.Templates) > 0 {
				sort.Slice(row.Templates, func(i, j int) bool {
					return row.Templates[i].Role+row.Templates[i].Locale < row.Templates[j].Role+row.Templates[j].Locale
				})
				matched = append(matched, row)
			}
		}
		src.Matched = countRows(matched)
		if len(matched) > 0 {
			src.Matched.Repositories = 1
		}
		src.FilteredDocuments = src.Counts.Documents - src.Matched.Documents
		out.Matched = addCounts(out.Matched, src.Matched)
		out.Sources = append(out.Sources, src)
		out.Solutions = append(out.Solutions, matched...)
	}
	if unavailable > 0 {
		out.Status = "partial"
		if available == 0 {
			out.Status = "failed"
		}
	}
	// Duplicate detection is global before pagination, excludes empty digests, and
	// preserves every source/ref rather than choosing an arbitrary winner.

	// A representative link records duplication without quadratic output when
	// thousands of imported entries share the same content hash.
	digests := map[string][]string{}
	for _, s := range out.Solutions {
		for _, t := range s.Templates {
			if t.Digest != "" && len(digests[t.Digest]) < 2 {
				digests[t.Digest] = append(digests[t.Digest], t.Ref)
			}
		}
	}
	for i := range out.Solutions {
		for j := range out.Solutions[i].Templates {
			t := &out.Solutions[i].Templates[j]
			refs := digests[t.Digest]
			if len(refs) > 1 {
				ref := refs[0]
				if ref == t.Ref {
					ref = refs[1]
				}
				t.DuplicateOf = []string{ref}
			}
		}
	}

	out.Categories = facets(categories)
	out.Capabilities = facets(capabilities)
	out.Consumers = facets(consumers)
	out.Offset = req.Offset
	start := min(req.Offset, len(out.Solutions))
	end := len(out.Solutions)
	if req.Limit > 0 {
		end = start + min(req.Limit, end-start)
	}
	out.Remaining = len(out.Solutions) - end
	out.Solutions = out.Solutions[start:end]
	out.Returned = countRows(out.Solutions)
	return out, nil
}

func roleMetadata(s promptrepo.Solution, t promptrepo.TemplateDiscoveryAnnotation) ([]string, []string) {
	media := append([]string{}, t.Media...)
	owners := append([]string{}, t.Consumers...)
	if len(media) == 0 {
		for _, tag := range s.Tags {
			if strings.HasPrefix(tag, "modality:") {
				media = append(media, strings.TrimPrefix(tag, "modality:"))
			}
		}
	}
	if len(owners) == 0 {
		for _, tag := range s.Tags {
			if strings.HasPrefix(tag, "consumer:") {
				owners = append(owners, strings.TrimPrefix(tag, "consumer:"))
			}
		}
	}
	if len(media) == 0 {
		media = []string{"unknown"}
	}
	if len(owners) == 0 {
		owners = []string{"unknown"}
	}
	return unique(media), unique(owners)
}
func has(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func unique(a []string) []string {
	out := []string{}
	for _, s := range a {
		if !has(out, s) {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
func addCounts(a, b promptrepo.DiscoveryCounts) promptrepo.DiscoveryCounts {
	return promptrepo.DiscoveryCounts{Repositories: a.Repositories + b.Repositories, Solutions: a.Solutions + b.Solutions, Roles: a.Roles + b.Roles, Documents: a.Documents + b.Documents}
}
func countSolutions(ss []promptrepo.Solution) promptrepo.DiscoveryCounts {
	c := promptrepo.DiscoveryCounts{Solutions: len(ss)}
	for _, s := range ss {
		roles := map[string]bool{}
		for _, t := range s.Templates {
			roles[t.Role] = true
			c.Documents++
		}
		c.Roles += len(roles)
	}
	return c
}
func countRows(ss []promptrepo.DiscoverySolution) promptrepo.DiscoveryCounts {
	c := promptrepo.DiscoveryCounts{Solutions: len(ss)}
	repos := map[string]bool{}
	for _, s := range ss {
		repos[s.RepositoryID] = true
		roles := map[string]bool{}
		for _, t := range s.Templates {
			roles[t.Role] = true
			c.Documents++
		}
		c.Roles += len(roles)
	}
	c.Repositories = len(repos)
	return c
}
func facets(m map[string]int) []promptrepo.DiscoveryFacet {
	out := []promptrepo.DiscoveryFacet{}
	for k, v := range m {
		out = append(out, promptrepo.DiscoveryFacet{Value: k, Documents: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

func discoveryReason(h promptrepo.RepositoryHealth) string {
	switch h.Code {
	case promptrepo.CodeNotFound:
		if strings.Contains(h.Message, "catalog.json") {
			return "missing_catalog"
		}
		return "source_unavailable"
	case promptrepo.CodeAuthRequired:
		return "credentials_unavailable"
	case promptrepo.CodeAuthorizationFailed:
		return "access_denied"
	case promptrepo.CodeInvalidRequest:
		if strings.Contains(h.Message, "unsupported catalog schema") {
			return "unsupported_version"
		}
		return "invalid_metadata"
	case promptrepo.CodeStateSchemaTooNew:
		return "unsupported_version"
	case promptrepo.CodeDigestMismatch:
		return "integrity_failed"
	case promptrepo.CodeSourceFetchFailed:
		return "source_unreachable"
	case "SOURCE_TIMEOUT":
		return "source_timeout"
	case "EIKONA_CATALOG_UNSUPPORTED", "EIKONA_CLI_UNAVAILABLE_OR_UNSUPPORTED":
		return "owner_capability_unavailable"
	}
	return ""
}
