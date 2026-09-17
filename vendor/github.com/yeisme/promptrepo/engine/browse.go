package engine

import (
	"context"
	"github.com/yeisme/promptrepo"
	"sort"
	"strings"
)

func (m *Manager) Browse(ctx context.Context, request promptrepo.BrowseRequest) (promptrepo.BrowseResult, error) {
	q, err := promptrepo.NormalizeBrowseRequest(request)
	if err != nil {
		return promptrepo.BrowseResult{}, err
	}
	base := promptrepo.DiscoveryRequest{Locale: q.Locale, Refresh: q.Refresh, Offline: q.Offline}
	state, err := m.discoveryState(ctx, base, q.Repositories)
	if err != nil {
		return promptrepo.BrowseResult{}, err
	}
	// Unknown explicitly selected sources are errors rather than empty success.
	for _, id := range q.Repositories {
		if _, ok := state.Profiles[id]; !ok {
			return promptrepo.BrowseResult{}, promptrepo.NewError(promptrepo.CodeNotFound, "selected repository was not found", false, nil)
		}
	}
	d, err := m.projectDiscovery(state, base)
	if err != nil {
		return promptrepo.BrowseResult{}, err
	}
	meta := map[[3]string]promptrepo.Solution{}
	for id, snapshot := range state.Snapshots {
		for _, s := range snapshot.Catalog.Solutions {
			meta[[3]string{id, s.PackageID + "/" + s.ID, s.Version}] = s
		}
	}
	records := map[string][]promptrepo.BrowseRecord{}
	solutions := []promptrepo.DiscoverySolution{}
	for _, s := range d.Solutions {
		source := meta[[3]string{s.RepositoryID, s.PackageID + "/" + s.ID, s.Version}]
		aliases := []string{}
		locales := []string{}
		for locale := range source.Locales {
			locales = append(locales, locale)
		}
		sort.Strings(locales)
		for _, locale := range locales {
			v := source.Locales[locale]
			aliases = append(aliases, v.Title, v.Summary)
			aliases = append(aliases, v.Aliases...)
		}
		matched := []promptrepo.DiscoveryTemplate{}
		for _, t := range s.Templates {
			row := promptrepo.BrowseRecord{SearchAliases: aliases, DiscoveryTemplate: t, Repository: s.RepositoryID, Solution: s.PackageID + "/" + s.ID, SolutionTitle: s.Title, SolutionSummary: s.Summary, Version: s.Version, Category: s.Category, Tags: source.Tags, Capabilities: s.Capabilities, Maturity: source.Maturity, Rights: source.Rights, SourceState: state.Health[s.RepositoryID].State}
			if promptrepo.MatchBrowseRecord(row, q) {
				matched = append(matched, t)
				key := browseSolutionKey(s)
				records[key] = append(records[key], row)
			}
		}
		if len(matched) > 0 {
			s.Templates = matched
			solutions = append(solutions, s)
		}
	}
	sort.SliceStable(solutions, func(i, j int) bool {
		a, b := solutions[i], solutions[j]
		var less, equal bool
		switch q.Sort {
		case "documents":
			less = len(a.Templates) < len(b.Templates)
			equal = len(a.Templates) == len(b.Templates)
		default:
			av, bv := browseSortValue(a, q.Sort), browseSortValue(b, q.Sort)
			less = av < bv
			equal = av == bv
		}
		if equal {
			return browseSolutionKey(a) < browseSolutionKey(b)
		}
		if q.Order == "desc" {
			return !less
		}
		return less
	})
	all := []promptrepo.BrowseRecord{}
	for _, s := range solutions {
		all = append(all, records[browseSolutionKey(s)]...)
	}
	d.Matched = countRows(solutions)
	cats, caps, cons := map[string]int{}, map[string]int{}, map[string]int{}
	for _, r := range all {
		cats[r.Category]++
		for _, c := range promptrepo.BrowseValues(r, "capability") {
			caps[c]++
		}
		for _, c := range promptrepo.BrowseValues(r, "consumer") {
			cons[c]++
		}
	}
	d.Categories = facets(cats)
	d.Capabilities = facets(caps)
	d.Consumers = facets(cons)
	for i := range d.Sources {
		ss := []promptrepo.DiscoverySolution{}
		for _, s := range solutions {
			if s.RepositoryID == d.Sources[i].ID {
				ss = append(ss, s)
			}
		}
		d.Sources[i].Matched = countRows(ss)
		d.Sources[i].FilteredDocuments = d.Sources[i].Counts.Documents - d.Sources[i].Matched.Documents
	}
	start := min(q.Offset, len(solutions))
	end := len(solutions)
	if q.Limit > 0 {
		end = start + min(q.Limit, end-start)
	}
	d.Offset = q.Offset
	d.Remaining = len(solutions) - end
	d.Solutions = solutions[start:end]
	d.Returned = countRows(d.Solutions)
	page := []promptrepo.BrowseRecord{}
	for _, s := range d.Solutions {
		page = append(page, records[browseSolutionKey(s)]...)
	}
	groups, overlap := promptrepo.GroupBrowse(all, q.GroupBy)
	return promptrepo.BrowseResult{DiscoveryResult: d, Browse: promptrepo.BrowseProjection{Records: page, Totals: promptrepo.CountBrowse(all), Returned: promptrepo.CountBrowse(page), Groups: groups, GroupBy: q.GroupBy, Sort: q.Sort, Order: q.Order, Dedupe: q.Dedupe, Metric: q.Metric, OverlappingGroups: overlap}}, nil
}
func browseSolutionKey(s promptrepo.DiscoverySolution) string {
	return s.RepositoryID + "/" + s.PackageID + "/" + s.ID + "@" + s.Version
}
func browseSortValue(s promptrepo.DiscoverySolution, key string) string {
	switch key {
	case "title":
		return strings.ToLower(s.Title)
	case "category":
		return s.Category
	case "repository":
		return s.RepositoryID
	}
	return s.PackageID + "/" + s.ID
}
