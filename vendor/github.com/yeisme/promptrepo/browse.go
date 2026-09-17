package promptrepo

import (
	"fmt"
	"golang.org/x/text/unicode/norm"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// BrowseRequest extends discovery without retyping any released scalar filter.
type BrowseRequest struct {
	DiscoveryRequest
	Repositories    []string `json:"repositories,omitempty"`
	Categories      []string `json:"categories,omitempty"`
	MediaTypes      []string `json:"media_types,omitempty"`
	Consumers       []string `json:"consumers,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Roles           []string `json:"roles,omitempty"`
	TemplateLocales []string `json:"template_locales,omitempty"`
	Maturities      []string `json:"maturities,omitempty"`
	Rights          []string `json:"rights,omitempty"`
	CompilerStates  []string `json:"compiler_states,omitempty"`
	SourceStates    []string `json:"source_states,omitempty"`
	Query           string   `json:"query,omitempty"`
	GroupBy         []string `json:"group_by,omitempty"`
	Sort            string   `json:"sort,omitempty"`
	Order           string   `json:"order,omitempty"`
	Dedupe          string   `json:"dedupe,omitempty"`
	Metric          string   `json:"metric,omitempty"`
}

type BrowseRecord struct {
	SearchAliases []string `json:"search_aliases,omitempty"`
	DiscoveryTemplate
	Repository      string   `json:"repository"`
	Solution        string   `json:"solution"`
	SolutionTitle   string   `json:"solution_title"`
	SolutionSummary string   `json:"solution_summary"`
	Version         string   `json:"version"`
	Category        string   `json:"category"`
	Tags            []string `json:"tags"`
	Capabilities    []string `json:"capabilities"`
	Maturity        string   `json:"maturity"`
	Rights          string   `json:"rights"`
	SourceState     string   `json:"source_state"`
}
type BrowseTotals struct {
	Entries        int `json:"entries"`
	UniqueContents int `json:"unique_contents"`
}
type BrowseGroup struct {
	Keys []string `json:"keys"`
	BrowseTotals
}
type BrowseProjection struct {
	Records           []BrowseRecord `json:"records"`
	Totals            BrowseTotals   `json:"totals"`
	Returned          BrowseTotals   `json:"returned"`
	Groups            []BrowseGroup  `json:"groups"`
	GroupBy           []string       `json:"group_by"`
	Sort              string         `json:"sort"`
	Order             string         `json:"order"`
	Dedupe            string         `json:"dedupe"`
	Metric            string         `json:"metric"`
	OverlappingGroups bool           `json:"overlapping_groups"`
}
type BrowseResult struct {
	DiscoveryResult
	Browse BrowseProjection `json:"browse"`
}

var BrowseDimensions = []string{"repository", "solution", "category", "media", "consumer", "capability", "tag", "role", "template_locale", "maturity", "rights", "compiler_status", "source_state"}

func NormalizeBrowseRequest(q BrowseRequest) (BrowseRequest, error) {
	if q.Limit < 0 || q.Offset < 0 || (q.Refresh && q.Offline) {
		return q, NewError(CodeInvalidRequest, "invalid pagination or refresh options", false, nil)
	}
	q.Repositories = unionBrowse(q.Repositories, q.Repository)
	q.Categories = unionBrowse(q.Categories, q.Category)
	q.MediaTypes = unionBrowse(q.MediaTypes, q.Media)
	q.Consumers = unionBrowse(q.Consumers, q.Consumer)
	q.Capabilities = unionBrowse(q.Capabilities, q.Capability)
	if len(q.GroupBy) == 0 {
		q.GroupBy = []string{"repository", "solution"}
	}
	if len(q.GroupBy) == 1 && q.GroupBy[0] == "none" {
		q.GroupBy = []string{}
	}
	if len(q.GroupBy) > 2 {
		return q, NewError(CodeInvalidRequest, "at most two grouping dimensions are supported", false, nil)
	}
	seen := map[string]bool{}
	for _, key := range q.GroupBy {
		if !containsBrowse(BrowseDimensions, key) || seen[key] {
			return q, NewError(CodeInvalidRequest, "invalid or repeated grouping dimension", false, nil)
		}
		seen[key] = true
	}
	if q.Sort == "" {
		q.Sort = "id"
	}
	if !containsBrowse([]string{"id", "title", "repository", "category", "documents"}, q.Sort) {
		return q, NewError(CodeInvalidRequest, "unsupported sort field", false, nil)
	}
	if q.Order == "" {
		q.Order = "asc"
	}
	if q.Order != "asc" && q.Order != "desc" {
		return q, NewError(CodeInvalidRequest, "order must be asc or desc", false, nil)
	}
	if q.Dedupe == "" {
		q.Dedupe = "none"
	}
	if q.Dedupe != "none" && q.Dedupe != "content" {
		return q, NewError(CodeInvalidRequest, "dedupe must be none or content", false, nil)
	}
	if q.Metric == "" {
		q.Metric = "entries"
	}
	if q.Metric != "entries" && q.Metric != "unique-content" {
		return q, NewError(CodeInvalidRequest, "unsupported metric", false, nil)
	}
	return q, nil
}
func unionBrowse(values []string, value string) []string {
	out := append([]string{}, values...)
	if value != "" && !containsBrowse(out, value) {
		out = append(out, value)
	}
	return out
}
func containsBrowse(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func unknownBrowse(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
func BrowseValues(r BrowseRecord, key string) []string {
	var v []string
	switch key {
	case "repository":
		v = []string{r.Repository}
	case "solution":
		v = []string{r.Solution}
	case "category":
		v = []string{r.Category}
	case "media":
		v = r.Media
	case "consumer":
		v = r.Consumers
	case "capability":
		v = r.Capabilities
	case "tag":
		v = r.Tags
	case "role":
		v = []string{r.Role}
	case "template_locale":
		v = []string{r.Locale}
	case "maturity":
		v = []string{r.Maturity}
	case "rights":
		v = []string{r.Rights}
	case "compiler_status":
		v = []string{r.CompilerStatus}
	case "source_state":
		v = []string{r.SourceState}
	}
	out := []string{}
	for _, s := range v {
		s = unknownBrowse(s)
		if !containsBrowse(out, s) {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		out = []string{"unknown"}
	}
	sort.Strings(out)
	return out
}
func BrowseText(s string) string {
	s = norm.NFKC.String(strings.ToLower(strings.ToValidUTF8(s, "\uFFFD")))
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) }), " ")
}
func MatchBrowseRecord(r BrowseRecord, q BrowseRequest) bool {
	filters := map[string][]string{"repository": q.Repositories, "category": q.Categories, "media": q.MediaTypes, "consumer": q.Consumers, "capability": q.Capabilities, "tag": q.Tags, "role": q.Roles, "template_locale": q.TemplateLocales, "maturity": q.Maturities, "rights": q.Rights, "compiler_status": q.CompilerStates, "source_state": q.SourceStates}
	for key, wanted := range filters {
		if len(wanted) == 0 {
			continue
		}
		match := false
		for _, v := range BrowseValues(r, key) {
			if containsBrowse(wanted, v) {
				match = true
			}
		}
		if !match {
			return false
		}
	}
	text := BrowseText(strings.Join([]string{r.Solution, r.Title, r.SolutionTitle, r.Summary, r.SolutionSummary, r.Role, strings.Join(r.Tags, " "), strings.Join(r.SearchAliases, " ")}, " "))
	for _, token := range strings.Fields(BrowseText(q.Query)) {
		if !strings.Contains(text, token) {
			return false
		}
	}
	return true
}

var browseHash = regexp.MustCompile(`(?i)^sha256:[a-f0-9]{64}$`)

func BrowseContentKey(r BrowseRecord) string {
	if browseHash.MatchString(r.Digest) {
		return strings.ToLower(r.Digest)
	}
	return "ref:" + r.Ref
}
func CountBrowse(records []BrowseRecord) BrowseTotals {
	keys := map[string]bool{}
	for i, r := range records {
		key := BrowseContentKey(r)
		if r.Ref == "" && !browseHash.MatchString(r.Digest) {
			key = fmt.Sprintf("missing:%d", i)
		}
		keys[key] = true
	}
	return BrowseTotals{Entries: len(records), UniqueContents: len(keys)}
}
func GroupBrowse(records []BrowseRecord, dimensions []string) ([]BrowseGroup, bool) {
	groups := map[string][]BrowseRecord{}
	keys := map[string][]string{}
	overlap := false
	for _, r := range records {
		paths := [][]string{{}}
		for _, d := range dimensions {
			next := [][]string{}
			values := BrowseValues(r, d)
			if len(values) > 1 {
				overlap = true
			}
			for _, path := range paths {
				for _, v := range values {
					p := append(append([]string{}, path...), v)
					next = append(next, p)
				}
			}
			paths = next
		}
		for _, path := range paths {
			key := fmt.Sprintf("%q", path)
			groups[key] = append(groups[key], r)
			keys[key] = path
		}
	}
	ids := []string{}
	for k := range groups {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	out := []BrowseGroup{}
	for _, id := range ids {
		out = append(out, BrowseGroup{Keys: keys[id], BrowseTotals: CountBrowse(groups[id])})
	}
	return out, overlap
}
