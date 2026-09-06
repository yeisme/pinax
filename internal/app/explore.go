package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/output"
)

// ExploreBundleSchemaVersion 是 vault explore 数据投影的冻结 schema。
const ExploreBundleSchemaVersion = "pinax.explore_bundle.v1"

// ExploreBundle 容量上限（fail-safe 截断并置 truncated=true）。
const (
	ExploreMaxNodes    = 5000
	ExploreMaxEdges    = 20000
	ExploreMaxTitle    = 160
	ExploreMaxSummary  = 240
	ExploreMaxTags     = 8
	exploreMaxTargetID = 160
	// exploreNotePreviewBytes 限制 /explore/note/<id> 有界预览长度（不嵌全文）。
	exploreNotePreviewBytes = 2048
)

// ExploreBundle 是有界 vault explore 投影：无正文、无绝对路径、无凭据。
type ExploreBundle struct {
	SchemaVersion string              `json:"schema_version"`
	GeneratedAt   string              `json:"generated_at"`
	Counts        ExploreBundleCounts `json:"counts"`
	Nodes         []ExploreNode       `json:"nodes"`
	Edges         []ExploreEdge       `json:"edges"`
}

// ExploreBundleCounts 报告投影规模与截断状态。
type ExploreBundleCounts struct {
	Nodes     int  `json:"nodes"`
	Edges     int  `json:"edges"`
	Truncated bool `json:"truncated"`
}

// ExploreNode 是单个 note 的有界卡片（summary 为有界摘要，绝不携带正文）。
type ExploreNode struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Kind      string   `json:"kind"`
	Tags      []string `json:"tags"`
	Trust     string   `json:"trust"`
	Fresh     string   `json:"fresh"`
	Summary   string   `json:"summary"`
	UpdatedAt string   `json:"updated_at"`
}

// ExploreEdge 是 note 间链接边；broken 链接保留为虚线候选（OKF 容忍 broken）。
type ExploreEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Broken bool   `json:"broken"`
}

// exploreBundleLimits 允许测试注入更小的容量上限验证截断语义。
type exploreBundleLimits struct {
	maxNodes   int
	maxEdges   int
	maxTitle   int
	maxSummary int
	maxTags    int
}

func defaultExploreBundleLimits() exploreBundleLimits {
	return exploreBundleLimits{
		maxNodes:   ExploreMaxNodes,
		maxEdges:   ExploreMaxEdges,
		maxTitle:   ExploreMaxTitle,
		maxSummary: ExploreMaxSummary,
		maxTags:    ExploreMaxTags,
	}
}

// BuildExploreBundle 从 vault 扫描合成 pinax.explore_bundle.v1（默认上限）。
// 组装完全在内存完成，不写 vault 或 .pinax/**。
func BuildExploreBundle(vaultPath string) (ExploreBundle, error) {
	return buildExploreBundle(vaultPath, defaultExploreBundleLimits(), time.Now().UTC())
}

func buildExploreBundle(vaultPath string, limits exploreBundleLimits, now time.Time) (ExploreBundle, error) {
	notes, err := scanNotes(vaultPath)
	if err != nil {
		return ExploreBundle{}, err
	}
	truncated := false
	kept := notes
	if len(kept) > limits.maxNodes {
		kept = kept[:limits.maxNodes]
		truncated = true
	}

	nodeIDByPath := map[string]string{}
	nodes := make([]ExploreNode, 0, len(kept))
	for _, note := range kept {
		id := exploreNodeID(note)
		nodeIDByPath[note.Path] = id
		nodes = append(nodes, exploreNode(note, limits, now))
	}

	outgoing, _ := BuildEnhancedLinkGraph(notes)
	edgeSeen := map[ExploreEdge]bool{}
	edges := make([]ExploreEdge, 0)
	for _, note := range kept {
		from, ok := nodeIDByPath[note.Path]
		if !ok {
			continue
		}
		for _, link := range outgoing[note.Path] {
			if link.Status == string(domain.LinkStatusExternal) || link.Status == string(domain.LinkStatusIgnored) {
				continue
			}
			if link.Status == string(domain.LinkStatusResolved) && link.TargetPath != "" {
				to, ok := nodeIDByPath[link.TargetPath]
				if !ok {
					// 目标被节点截断丢弃：fail-safe 丢弃该边。
					truncated = true
					continue
				}
				edge := ExploreEdge{From: from, To: to, Broken: false}
				if edgeSeen[edge] {
					continue
				}
				edgeSeen[edge] = true
				edges = append(edges, edge)
				continue
			}
			// broken/ambiguous/legacy-broken：保留为 broken 边，目标用有界 raw target。
			if link.Status != string(domain.LinkStatusBroken) && link.Status != string(domain.LinkStatusAmbiguous) && !link.Broken {
				continue
			}
			edge := ExploreEdge{From: from, To: exploreBoundText(link.Target, exploreMaxTargetID), Broken: true}
			if edgeSeen[edge] {
				continue
			}
			edgeSeen[edge] = true
			edges = append(edges, edge)
		}
	}
	sortExploreEdges(edges)
	if len(edges) > limits.maxEdges {
		edges = edges[:limits.maxEdges]
		truncated = true
	}

	return ExploreBundle{
		SchemaVersion: ExploreBundleSchemaVersion,
		GeneratedAt:   now.Format(time.RFC3339),
		Counts:        ExploreBundleCounts{Nodes: len(nodes), Edges: len(edges), Truncated: truncated},
		Nodes:         nodes,
		Edges:         edges,
	}, nil
}

// exploreNodeID 返回 URL 安全的稳定节点 id：note_id 优先，缺失时退回路径摘要。
func exploreNodeID(note domain.Note) string {
	if id := strings.TrimSpace(note.ID); id != "" {
		return id
	}
	digest := sha256.Sum256([]byte(note.Path))
	return "n" + hex.EncodeToString(digest[:8])
}

func exploreNode(note domain.Note, limits exploreBundleLimits, now time.Time) ExploreNode {
	signals := parseExploreOKFSignals(note)
	tags := make([]string, 0, len(note.Tags))
	for _, tag := range note.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
		if len(tags) >= limits.maxTags {
			break
		}
	}
	kind := strings.TrimSpace(note.Kind)
	if kind == "" {
		kind = "note"
	}
	return ExploreNode{
		ID:        exploreNodeID(note),
		Title:     exploreBoundText(note.Title, limits.maxTitle),
		Kind:      kind,
		Tags:      tags,
		Trust:     signals.TrustTier(),
		Fresh:     signals.Freshness(now),
		Summary:   exploreBoundText(exploreNoteSummary(note), limits.maxSummary),
		UpdatedAt: strings.TrimSpace(note.UpdatedAt),
	}
}

// exploreNoteSummary 只从作者维护的 frontmatter summary/description 派生有界摘要，
// 绝不从正文截取片段（正文红线）。
func exploreNoteSummary(note domain.Note) string {
	for _, key := range []string{"summary", "description"} {
		if value := strings.TrimSpace(note.Frontmatter[key]); value != "" {
			return value
		}
	}
	return ""
}

func exploreBoundText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

// --- OKF trust/fresh 本地派生 ---
//
// NOTE(share-explore): 这里的 OKF 信任信号解析是 pinax-share-explore-v1 的本地派生
// helper，直接消费 domain.Note.Frontmatter（map[string]string，嵌套 YAML 已被
// markdownnote.ParseFrontmatter 扁平化为 "by:x,at:y" 形态）。配套 change
// pinax-okf-trust-discovery-v1 落地 typed domain helper 后，两者待合并去重。

type exploreOKFSignals struct {
	GeneratedBy    string
	GeneratedAt    string
	VerifiedActors []string
	StaleAfter     string
}

// parseExploreOKFSignals 解析 frontmatter 的 generated/verified/stale_after。
// verified 兼容 OKF bare mapping（单元素列表扁平化后形态一致）。
func parseExploreOKFSignals(note domain.Note) exploreOKFSignals {
	signals := exploreOKFSignals{}
	generated := strings.TrimSpace(note.Frontmatter["generated"])
	if generated != "" {
		signals.GeneratedBy = exploreFlatMappingValue(generated, "by")
		signals.GeneratedAt = exploreFlatMappingValue(generated, "at")
	}
	for _, part := range strings.Split(strings.TrimSpace(note.Frontmatter["verified"]), ",") {
		part = strings.Trim(strings.TrimSpace(part), `"'`)
		if part == "" {
			continue
		}
		if actor, ok := strings.CutPrefix(part, "by:"); ok {
			signals.VerifiedActors = append(signals.VerifiedActors, strings.TrimSpace(actor))
			continue
		}
		if strings.HasPrefix(part, "at:") {
			continue
		}
		// 兼容裸标量形态（verified: human:ye）。
		signals.VerifiedActors = append(signals.VerifiedActors, part)
	}
	signals.StaleAfter = strings.TrimSpace(note.Frontmatter["stale_after"])
	return signals
}

// exploreFlatMappingValue 从扁平化 mapping（"by:actor:pinax/1.0,at:2026-…"）取子键值。
func exploreFlatMappingValue(flat, key string) string {
	prefix := key + ":"
	for _, part := range strings.Split(flat, ",") {
		part = strings.TrimSpace(part)
		if value, ok := strings.CutPrefix(part, prefix); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// TrustTier 返回 unverified|machine|human（只看 verified 列表，绝不存储回 frontmatter）。
func (s exploreOKFSignals) TrustTier() string {
	if len(s.VerifiedActors) == 0 {
		return "unverified"
	}
	for _, actor := range s.VerifiedActors {
		if strings.HasPrefix(actor, "human:") {
			return "human"
		}
	}
	return "machine"
}

// Freshness 返回 fresh|stale（now >= stale_after 即 stale；缺字段/非法时间戳按 fresh，
// 未知格式不崩溃，消费端容忍精神）。
func (s exploreOKFSignals) Freshness(now time.Time) string {
	if s.StaleAfter == "" {
		return "fresh"
	}
	staleAfter, err := time.Parse(time.RFC3339, s.StaleAfter)
	if err != nil {
		if staleAfter, err = time.Parse("2006-01-02", s.StaleAfter); err != nil {
			return "fresh"
		}
	}
	if !now.Before(staleAfter) {
		return "stale"
	}
	return "fresh"
}

// exploreNotePreview 返回 /explore/note/<id> 的有界预览（脱敏 + 字节上限，不嵌全文）。
func exploreNotePreview(note domain.Note) string {
	preview := normalizeExploreWhitespace(note.Body)
	if preview == "" {
		return ""
	}
	projection := domain.NewProjection("share.explore.note", "")
	projection.Data = map[string]any{"preview": preview}
	output.ApplyProjectionRedaction(&projection)
	data, _ := projection.Data.(map[string]any)
	bounded, _ := data["preview"].(string)
	return exploreBoundText(bounded, exploreNotePreviewBytes)
}

// normalizeExploreWhitespace 折叠空白为单空格，避免预览携带多行正文结构。
func normalizeExploreWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// exploreResolveNote 在 bundle 节点集内按 id 定位 note；id 必须通过 paneUnsafe 同源
// denylist，找不到返回错误（404 fail-closed，不区分原因避免探测）。
func exploreResolveNote(notes []domain.Note, id string) (domain.Note, error) {
	id = strings.TrimSpace(id)
	if id == "" || paneUnsafe(id) {
		return domain.Note{}, fmt.Errorf("explore note %q not found", id)
	}
	for _, note := range notes {
		if exploreNodeID(note) == id {
			return note, nil
		}
	}
	return domain.Note{}, fmt.Errorf("explore note %q not found", id)
}

// sortExploreEdges 按 from/to/broken 稳定排序（确定性输出便于 golden 比对）。
func sortExploreEdges(edges []ExploreEdge) {
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return !edges[i].Broken && edges[j].Broken
	})
}
