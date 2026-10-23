package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/notelinks"
	"github.com/yeisme/pinax/internal/render"
	schema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

type ReaderEmbedTarget struct {
	ObjectID string `json:"object_id"`
	Revision string `json:"revision"`
	Anchor   string `json:"anchor,omitempty"`
}
type ReaderEmbed struct {
	EmbedID       string             `json:"embed_id"`
	ParentEmbedID string             `json:"parent_embed_id"`
	SourceAnchor  string             `json:"source_anchor"`
	PageAnchors   []string           `json:"page_anchors"`
	Depth         int                `json:"depth"`
	Status        string             `json:"status"`
	Target        *ReaderEmbedTarget `json:"target,omitempty"`
}
type ReaderNodeOrigin struct {
	PageAnchor   string `json:"page_anchor"`
	EmbedID      string `json:"embed_id"`
	SourceAnchor string `json:"source_anchor"`
}
type ReaderComposedDocumentPage struct {
	SchemaVersion string                 `json:"schema_version"`
	RichDocument  ReaderRichDocumentPage `json:"rich_document"`
	Embeds        []ReaderEmbed          `json:"embeds"`
	Origins       []ReaderNodeOrigin     `json:"origins"`
}

func (s *Service) ReaderComposedDocument(ctx context.Context, req ReaderDocumentRequest) (domain.Projection, error) {
	return s.readerRichDocument(ctx, req, true)
}

type readerComposition struct {
	ctx                context.Context
	root               string
	notes              []domain.Note
	snapshot           notelinks.ResolverSnapshot
	embeds             []ReaderEmbed
	origins            map[string]ReaderNodeOrigin
	placeholders       map[string]string
	sourceBytes, nodes int
	headingRefs        readerHeadingRefs
}

func composedBudget() error { return &render.ReaderBudgetError{Code: "document_budget_exceeded"} }

func (c *readerComposition) build(note domain.Note, occurrence string, ancestors map[string]domain.Note, fragment string) (render.ReaderRichDocument, string, error) {
	if err := c.ctx.Err(); err != nil {
		return render.ReaderRichDocument{}, "", err
	}
	if note.OversizedSource || len(note.Body) > render.ReaderSourceMaxBytes-c.sourceBytes-c.headingRefs.sourceBytes {
		return render.ReaderRichDocument{}, "", composedBudget()
	}
	c.sourceBytes += len(note.Body)
	rich, err := render.BuildReaderRichDocument(note.Body)
	if err != nil {
		return rich, "", err
	}
	if err := resolveReaderAssetRefs(c.root, note, &rich.Document); err != nil {
		return rich, "", err
	}
	if err := resolveReaderNoteRefsWithBudget(note, c.notes, &rich.Document, &c.headingRefs); err != nil {
		return rich, "", err
	}
	c.nodes += rich.Document.NodeCount
	if c.nodes+c.headingRefs.nodes >= render.ReaderTotalMaxNodes || c.sourceBytes+c.headingRefs.sourceBytes > render.ReaderSourceMaxBytes {
		return rich, "", composedBudget()
	}
	selected, selection, found := composedSelection(rich.Document, fragment)
	if !found {
		return render.ReaderRichDocument{}, "", nil
	}
	// Keep the original source anchor map; namespacing happens only after
	// selecting the exact owner fragment and resolving its original references.
	allowed := map[string]bool{}
	var mark func([]render.ReaderDocumentNode)
	mark = func(ns []render.ReaderDocumentNode) {
		for _, n := range ns {
			allowed[n.Anchor] = true
			mark(n.Children)
		}
	}
	mark(selected)
	embeds := map[string]render.ReaderEmbedCandidate{}
	for _, e := range rich.Embeds {
		if allowed[e.Anchor] {
			embeds[e.Anchor] = e
		}
	}
	annotations := []render.ReaderRichAnnotation{}
	remap := func(anchor string) string {
		if occurrence == "" {
			return "root-" + anchor
		}
		return occurrence + "-" + anchor
	}
	var rename func(*render.ReaderDocumentNode)
	rename = func(n *render.ReaderDocumentNode) {
		original := n.Anchor
		n.Anchor = remap(original)
		if n.BlockAnchor != "" {
			n.BlockAnchor = remap(n.BlockAnchor)
		}
		c.origins[n.Anchor] = ReaderNodeOrigin{PageAnchor: n.Anchor, EmbedID: occurrence, SourceAnchor: original}
		for i := range n.Children {
			rename(&n.Children[i])
		}
	}
	for _, a := range rich.Annotations {
		if allowed[a.Anchor] {
			a.Anchor = remap(a.Anchor)
			annotations = append(annotations, a)
		}
	}
	blocks := []render.ReaderDocumentNode{}
	outline := []render.ReaderOutlineEntry{}
	for _, e := range rich.Document.Outline {
		if allowed[e.Anchor] {
			e.Anchor = remap(e.Anchor)
			outline = append(outline, e)
		}
	}
	for _, node := range selected {
		original := node.Anchor
		rename(&node)
		blocks = append(blocks, node)
		candidate, ok := embeds[original]
		if !ok {
			continue
		}
		// Assets already have an owner-issued ref; never reinterpret them as notes.
		isAsset := false
		for _, child := range node.Children {
			if strings.HasPrefix(child.Ref, "pinax-asset://") {
				isAsset = true
			}
		}
		rawTarget, alias, heading := notelinks.SplitWikiLinkParts(candidate.Raw[3 : len(candidate.Raw)-2])
		// A dotted title or alias is not an asset extension. Only an
		// owner-resolved asset ref takes precedence over note resolution.
		if isAsset {
			continue
		}
		if len(c.embeds) >= schema.ReaderComposedMaxEmbeds {
			return rich, "", composedBudget()
		}
		depth := len(ancestors)
		id := fmt.Sprintf("embed-%d", len(c.embeds)+1)
		edge := ReaderEmbed{EmbedID: id, ParentEmbedID: occurrence, SourceAnchor: original, Depth: depth, Status: "unavailable", PageAnchors: []string{}}
		index := len(c.embeds)
		c.embeds = append(c.embeds, edge)
		c.placeholders[node.Anchor] = id
		if depth > 2 {
			c.embeds[index].Status = "depth_limit"
			continue
		}
		target, ok := domain.Note{}, false
		if rawTarget == "" && heading != "" {
			target, ok = note, true
		} else {
			resolved := notelinks.ResolveLinkTarget(note, notelinks.RawLink{Kind: "wiki", Raw: candidate.Raw, Target: rawTarget, Alias: alias, Heading: heading}, c.snapshot)
			if resolved.Link.Status == string(domain.LinkStatusResolved) {
				target, ok = findNoteByObjectID(c.notes, resolved.Link.TargetObjectID)
				if target.ID == note.ID {
					target = note
				}
			}
		}
		if !ok {
			continue
		}
		if ancestor, cyclic := ancestors[target.ID]; cyclic {
			target = ancestor
			// Resolve a fragment without expanding or fetching new source.
			anchor := ""
			if heading != "" {
				uri, found, e := c.headingRefs.uri(target, heading)
				if e != nil {
					return rich, "", e
				}
				if !found {
					continue
				}
				if c.nodes+c.headingRefs.nodes >= render.ReaderTotalMaxNodes || c.sourceBytes+c.headingRefs.sourceBytes > render.ReaderSourceMaxBytes {
					return rich, "", composedBudget()
				}
				parsed, _ := url.Parse(uri)
				anchor = parsed.Query().Get("anchor")
			}
			c.embeds[index].Status = "cycle"
			c.embeds[index].Target = &ReaderEmbedTarget{ObjectID: target.ID, Revision: readerNoteRevision(target), Anchor: anchor}
			continue
		}
		chain := map[string]domain.Note{}
		for key, v := range ancestors {
			chain[key] = v
		}
		chain[target.ID] = target
		child, anchor, e := c.build(target, id, chain, heading)
		if e != nil {
			return rich, "", e
		}
		if heading != "" && anchor == "" {
			continue
		}
		c.embeds[index].Status = "expanded"
		c.embeds[index].Target = &ReaderEmbedTarget{ObjectID: target.ID, Revision: readerNoteRevision(target), Anchor: anchor}
		blocks = append(blocks, child.Document.Blocks...)
		outline = append(outline, child.Document.Outline...)
		annotations = append(annotations, child.Annotations...)
	}
	rich.Document.Blocks = blocks
	rich.Document.Outline = outline
	rich.Annotations = annotations
	rich.Document.AnchorIndex = map[string]int{}
	var indexNodes func([]render.ReaderDocumentNode, int)
	indexNodes = func(ns []render.ReaderDocumentNode, top int) {
		for _, n := range ns {
			rich.Document.AnchorIndex[n.Anchor] = top
			indexNodes(n.Children, top)
		}
	}
	for i, n := range blocks {
		indexNodes([]render.ReaderDocumentNode{n}, i)
	}
	sort.SliceStable(rich.Document.Outline, func(i, j int) bool {
		return rich.Document.AnchorIndex[rich.Document.Outline[i].Anchor] < rich.Document.AnchorIndex[rich.Document.Outline[j].Anchor]
	})
	return rich, selection, nil
}

// Select a heading section through the next same-or-higher heading, or an
// exact explicit block. Missing/ambiguous selectors never become whole-note.
func composedSelection(doc render.ReaderDocument, fragment string) ([]render.ReaderDocumentNode, string, bool) {
	if fragment == "" {
		return doc.Blocks, "", true
	}
	decoded, err := url.PathUnescape(fragment)
	if err != nil || len(decoded) > 512 {
		return nil, "", false
	}
	anchor := ""
	level := 0
	if strings.HasPrefix(decoded, "^") {
		anchor = doc.BlockRefs[strings.TrimPrefix(decoded, "^")]
	} else {
		count := 0
		for _, e := range doc.Outline {
			if e.Title == decoded {
				count++
				anchor = e.Anchor
				level = e.Level
			}
		}
		if count > 1 {
			return nil, "", false
		}
		if count == 0 {
			for _, e := range doc.Outline {
				if e.Anchor == decoded {
					anchor = e.Anchor
					level = e.Level
					break
				}
			}
		}
	}
	start, ok := doc.AnchorIndex[anchor]
	if anchor == "" || !ok {
		return nil, "", false
	}
	end := start + 1
	if level > 0 {
		end = len(doc.Blocks)
		for _, e := range doc.Outline {
			idx := doc.AnchorIndex[e.Anchor]
			if idx > start && e.Level <= level && idx < end {
				end = idx
			}
		}
	}
	return doc.Blocks[start:end], anchor, true
}

func (c *readerComposition) page(rich ReaderRichDocumentPage) (ReaderComposedDocumentPage, error) {
	result := ReaderComposedDocumentPage{SchemaVersion: schema.ReaderComposedDocumentV1Alpha1, RichDocument: rich, Embeds: []ReaderEmbed{}, Origins: []ReaderNodeOrigin{}}
	byID := map[string]ReaderEmbed{}
	for _, e := range c.embeds {
		byID[e.EmbedID] = e
	}
	anchors := map[string]map[string]bool{}
	add := func(id, anchor string) {
		for id != "" {
			e, ok := byID[id]
			if !ok {
				break
			}
			if anchors[id] == nil {
				anchors[id] = map[string]bool{}
			}
			anchors[id][anchor] = true
			id = e.ParentEmbedID
		}
	}
	var visit func([]render.ReaderDocumentNode) error
	visit = func(ns []render.ReaderDocumentNode) error {
		for _, n := range ns {
			original := n.Anchor
			if n.BlockAnchor != "" {
				original = n.BlockAnchor
			}
			origin, ok := c.origins[original]
			if !ok {
				return fmt.Errorf("missing node origin")
			}
			origin.PageAnchor = n.Anchor
			result.Origins = append(result.Origins, origin)
			add(origin.EmbedID, n.Anchor)
			if id := c.placeholders[original]; id != "" {
				add(id, n.Anchor)
			}
			if err := visit(n.Children); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(rich.Document.Nodes); err != nil {
		return result, err
	}
	for _, e := range c.embeds {
		if len(anchors[e.EmbedID]) == 0 {
			continue
		}
		for a := range anchors[e.EmbedID] {
			e.PageAnchors = append(e.PageAnchors, a)
		}
		sort.Strings(e.PageAnchors)
		result.Embeds = append(result.Embeds, e)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	var decoded any
	if err = json.Unmarshal(raw, &decoded); err != nil {
		return result, err
	}
	if err = schema.ValidatePayload(schema.Default(), schema.ReaderComposedDocumentV1Alpha1, decoded); err != nil {
		return result, err
	}
	if err := validateComposedOrigins(result); err != nil {
		return result, err
	}
	return result, nil
}
