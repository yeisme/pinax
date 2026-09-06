package searchops

import (
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	noteindex "github.com/yeisme/pinax/internal/index"
)

// SearchFacet 是一个 facet 维度下的单值计数。
type SearchFacet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// SearchFacets 是消费时合成的 facet 计数（基于过滤前的全匹配集，不落盘）。
// 每组按计数降序、同数按字典序稳定排序。
type SearchFacets struct {
	Tag    []SearchFacet `json:"tag,omitempty"`
	Kind   []SearchFacet `json:"kind,omitempty"`
	Status []SearchFacet `json:"status,omitempty"`
	Folder []SearchFacet `json:"folder,omitempty"`
	Trust  []SearchFacet `json:"trust,omitempty"`
	Fresh  []SearchFacet `json:"fresh,omitempty"`
}

// FacetDimensionOrder 是 human/agent 输出中的维度稳定顺序。
var FacetDimensionOrder = []string{"tag", "kind", "status", "folder", "trust", "fresh"}

// ComputeFacets 从结果项集合合成 facet 计数（调用方保证传入过滤前全匹配集）。
// 未请求信任标注的结果项按 unverified/fresh 兜底（索引引擎总是带列）。
func ComputeFacets(items []noteindex.ResultItem) *SearchFacets {
	facets := &SearchFacets{}
	counts := map[string]map[string]int{
		"tag":    {},
		"kind":   {},
		"status": {},
		"folder": {},
		"trust":  {},
		"fresh":  {},
	}
	for _, item := range items {
		for _, tag := range CleanTags(item.Note.Tags) {
			counts["tag"][tag]++
		}
		counts["kind"][dimensionValue(item.Note.Kind)]++
		counts["status"][dimensionValue(item.Note.Status)]++
		counts["folder"][dimensionValue(item.Note.Folder)]++
		trust := strings.TrimSpace(item.Trust)
		if trust == "" {
			trust = domain.TrustTierOf(nil)
		}
		counts["trust"][trust]++
		fresh := strings.TrimSpace(item.Fresh)
		if fresh == "" {
			fresh = domain.FreshnessFresh
		}
		counts["fresh"][fresh]++
	}
	facets.Tag = sortedFacets(counts["tag"])
	facets.Kind = sortedFacets(counts["kind"])
	facets.Status = sortedFacets(counts["status"])
	facets.Folder = sortedFacets(counts["folder"])
	facets.Trust = sortedFacets(counts["trust"])
	facets.Fresh = sortedFacets(counts["fresh"])
	return facets
}

// Group 按 dimension 名取 facet 组（未知维度返回 nil）。
func (f *SearchFacets) Group(dimension string) []SearchFacet {
	if f == nil {
		return nil
	}
	switch dimension {
	case "tag":
		return f.Tag
	case "kind":
		return f.Kind
	case "status":
		return f.Status
	case "folder":
		return f.Folder
	case "trust":
		return f.Trust
	case "fresh":
		return f.Fresh
	default:
		return nil
	}
}

func sortedFacets(counts map[string]int) []SearchFacet {
	facets := make([]SearchFacet, 0, len(counts))
	for value, count := range counts {
		facets = append(facets, SearchFacet{Value: value, Count: count})
	}
	sort.SliceStable(facets, func(i, j int) bool {
		if facets[i].Count != facets[j].Count {
			return facets[i].Count > facets[j].Count
		}
		return facets[i].Value < facets[j].Value
	})
	return facets
}

func dimensionValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

// itemMatchesTrustFilters 报告结果项是否通过 --trust/--stale 过滤。
// 项未携带信任标注时按 unverified/fresh 兜底（与存量 note 语义一致）。
func itemMatchesTrustFilters(item noteindex.ResultItem, req Request) bool {
	trustFilter := NormalizedTrustFilter(req.Trust)
	if trustFilter != "" {
		trust := strings.TrimSpace(item.Trust)
		if trust == "" {
			trust = domain.TrustTierOf(nil)
		}
		if trust != trustFilter {
			return false
		}
	}
	switch NormalizedStaleFilter(req.Stale) {
	case "only":
		return strings.TrimSpace(item.Fresh) == domain.FreshnessStale
	case "exclude":
		return strings.TrimSpace(item.Fresh) != domain.FreshnessStale
	}
	return true
}

// applyTrustFiltersAndLimit 应用信任/新鲜度过滤并统一裁剪 limit。
// 返回过滤后（未裁剪）的项集，供调用方设置 Total。
func applyTrustFiltersAndLimit(req Request, items []noteindex.ResultItem) ([]noteindex.ResultItem, int) {
	if searchTrustAware(req) {
		filtered := make([]noteindex.ResultItem, 0, len(items))
		for _, item := range items {
			if itemMatchesTrustFilters(item, req) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	total := len(items)
	limit := req.Limit
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, total
}

// annotateFallbackItems 为 fallback（scan）引擎的结果项补派生 trust/fresh 标注。
// 仅在信任感知模式下调用，保证默认输出零变更。
func annotateFallbackItems(items []noteindex.ResultItem) {
	for i := range items {
		items[i].Trust = domain.TrustTierOf(items[i].Note.Trust)
		items[i].Fresh = domain.FreshnessOf(items[i].Note.Trust, time.Now().UTC())
	}
}
