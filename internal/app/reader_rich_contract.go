package app

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/render"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// Schema bounds shape; the owner also proves UTF-8 byte budgets and that
// every page target is a node actually delivered in this exact response.
func validateRichDocumentPage(page ReaderRichDocumentPage) error {
	raw, err := json.Marshal(page)
	if err != nil {
		return err
	}
	var value any
	if err = json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if err = catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.ReaderRichDocumentV1Alpha1, value); err != nil {
		return err
	}
	nodes := map[string]bool{}
	count := 0
	var walk func([]render.ReaderDocumentNode) error
	walk = func(items []render.ReaderDocumentNode) error {
		for _, node := range items {
			count++
			if count > catalogschema.ReaderDocumentPageMaxNodes || nodes[node.Anchor] {
				return fmt.Errorf("invalid page node identity or budget")
			}
			nodes[node.Anchor] = true
			if err := walk(node.Children); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(page.Document.Nodes); err != nil {
		return err
	}
	seen := map[string]bool{}
	total := 0
	for _, annotation := range page.Annotations {
		if seen[annotation.Anchor] {
			return fmt.Errorf("duplicate rich annotation identity")
		}
		seen[annotation.Anchor] = true
		if !utf8.ValidString(annotation.Source) || !utf8.ValidString(annotation.Title) {
			return fmt.Errorf("invalid annotation UTF-8")
		}
		size := len(annotation.Source) + len(annotation.Title)
		if size > catalogschema.ReaderRichMaxPayloadBytes || size > catalogschema.ReaderRichTotalPayloadBytes-total || len(annotation.Title) > 512 {
			return fmt.Errorf("annotation byte budget exceeded")
		}
		total += size
		for _, anchor := range annotation.PageAnchors {
			if !nodes[anchor] {
				return fmt.Errorf("annotation target is not on the page")
			}
		}
	}
	return nil
}
