package app

import (
	"fmt"

	"github.com/yeisme/pinax/internal/render"
)

// Cross-field checks complement the closed schema. Every delivered node has
// exactly one origin, and every embed's ancestry is present on the same page.
func validateComposedOrigins(page ReaderComposedDocumentPage) error {
	nodes := map[string]bool{}
	var walk func([]render.ReaderDocumentNode)
	walk = func(ns []render.ReaderDocumentNode) {
		for _, n := range ns {
			nodes[n.Anchor] = true
			walk(n.Children)
		}
	}
	walk(page.RichDocument.Document.Nodes)
	edges := map[string]ReaderEmbed{}
	for _, e := range page.Embeds {
		if _, ok := edges[e.EmbedID]; ok {
			return fmt.Errorf("duplicate embed")
		}
		edges[e.EmbedID] = e
	}
	for _, e := range page.Embeds {
		if e.ParentEmbedID == "" {
			if e.Depth != 1 {
				return fmt.Errorf("invalid root depth")
			}
		} else {
			parent, ok := edges[e.ParentEmbedID]
			if !ok || parent.Status != "expanded" || e.Depth != parent.Depth+1 {
				return fmt.Errorf("invalid embed ancestry")
			}
		}
		for _, a := range e.PageAnchors {
			if !nodes[a] {
				return fmt.Errorf("embed points outside page")
			}
		}
	}
	seen := map[string]bool{}
	for _, o := range page.Origins {
		if !nodes[o.PageAnchor] || seen[o.PageAnchor] {
			return fmt.Errorf("invalid origin coverage")
		}
		seen[o.PageAnchor] = true
		if o.EmbedID != "" {
			edge, ok := edges[o.EmbedID]
			if !ok || edge.Status != "expanded" {
				return fmt.Errorf("origin not expanded")
			}
		}
	}
	if len(seen) != len(nodes) {
		return fmt.Errorf("missing origin")
	}
	return nil
}
