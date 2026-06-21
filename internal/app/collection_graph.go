package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/contentbundle"
	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/promptasset"
)

type CollectionRequest struct {
	VaultPath string
	From      string
	To        string
	Format    string
	DryRun    bool
	Yes       bool
}

type GraphRequest struct {
	VaultPath string
	Kind      string
	Match     string
}

type collectionPlan struct {
	Bundle          contentbundle.Bundle `json:"-"`
	Stats           contentbundle.Stats  `json:"stats"`
	NewItems        int                  `json:"new_items"`
	ExistingItems   int                  `json:"existing_items"`
	NewPrompts      int                  `json:"new_prompts"`
	ExistingPrompts int                  `json:"existing_prompts"`
	Plans           []collectionItemPlan `json:"plans,omitempty"`
}

type collectionItemPlan struct {
	ItemID        string `json:"item_id"`
	Title         string `json:"title"`
	NotePath      string `json:"note_path"`
	PromptAssetID string `json:"prompt_asset_id,omitempty"`
	NoteStatus    string `json:"note_status"`
	PromptStatus  string `json:"prompt_status"`
}

type promptGraph struct {
	SchemaVersion string            `json:"schema_version"`
	Nodes         []promptGraphNode `json:"nodes"`
	Edges         []promptGraphEdge `json:"edges"`
}

type promptGraphNode struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Label         string `json:"label"`
	PromptAssetID string `json:"prompt_asset_id,omitempty"`
	Title         string `json:"title,omitempty"`
}

type promptGraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

func (s *Service) CollectionImport(ctx context.Context, req CollectionRequest) (domain.Projection, error) {
	root, plan, err := s.collectionPlan(ctx, req, "collection.import")
	if err != nil {
		return errorProjection("collection.import", err), err
	}
	projection := collectionProjection("collection.import", "Collection import plan generated.", plan)
	projection.Facts["dry_run"] = fmt.Sprint(req.DryRun)
	projection.Facts["local_write"] = "false"
	projection.Data = map[string]any{"plan": plan, "dry_run": req.DryRun}
	if req.DryRun {
		return projection, nil
	}
	if !req.Yes {
		err := &domain.CommandError{Code: "confirmation_required", Message: "collection import requires --yes", Hint: "Preview with --dry-run, then rerun with --yes"}
		return domain.NewErrorProjection("collection.import", err), err
	}
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return errorProjection("collection.import", err), err
	}
	importedNotes := 0
	importedPrompts := 0
	for _, item := range plan.Bundle.Items {
		itemPlan := collectionPlanForItem(plan.Bundle, item)
		if !fileExistsPath(root, itemPlan.NotePath) {
			if _, err := s.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: collectionItemTitle(item), Dir: filepath.ToSlash(filepath.Join("collections", collectionSafeID(plan.Bundle.ID))), Slug: contentbundle.StableID("item", item.ID), Kind: "reference", Status: "active", Tags: []string{"collection", "prompt", "source/" + collectionSafeID(plan.Bundle.ID)}, Body: collectionNoteBody(plan.Bundle, item)}); err != nil {
				return errorProjection("collection.import", err), err
			}
			importedNotes++
		}
		if strings.TrimSpace(item.Prompt) == "" {
			continue
		}
		if _, err := repo.Resolve(ctx, itemPlan.PromptAssetID); err == nil {
			continue
		} else if !promptasset.IsNotFound(err) {
			return errorProjection("collection.import", err), err
		}
		if _, err := repo.Create(ctx, collectionPromptAsset(plan.Bundle, item)); err != nil {
			return errorProjection("collection.import", err), err
		}
		importedPrompts++
	}
	if err := refreshIndex(root); err != nil {
		return errorProjection("collection.import", err), err
	}
	receiptRel, err := writeReceipt(root, "collection-import", map[string]any{"bundle_id": plan.Bundle.ID, "items": plan.Stats.Items, "imported_notes": importedNotes, "imported_prompts": importedPrompts, "missing_prompt_items": plan.Stats.MissingPromptItems})
	if err != nil {
		return errorProjection("collection.import", err), err
	}
	_ = appendEvent(root, "collection.import", "success", map[string]string{"bundle_id": plan.Bundle.ID, "imported_notes": fmt.Sprint(importedNotes), "imported_prompts": fmt.Sprint(importedPrompts), "receipt_path": receiptRel})
	projection.Summary = "Collection imported."
	projection.Facts["imported_notes"] = fmt.Sprint(importedNotes)
	projection.Facts["imported_prompts"] = fmt.Sprint(importedPrompts)
	projection.Facts["local_write"] = "true"
	projection.Facts["index_updated"] = "true"
	projection.Facts["receipt_path"] = receiptRel
	projection.Evidence = []string{receiptRel, filepath.ToSlash(filepath.Join(".pinax", "index.sqlite"))}
	projection.Data = map[string]any{"bundle_id": plan.Bundle.ID, "imported_notes": importedNotes, "imported_prompts": importedPrompts, "missing_prompt_items": plan.Stats.MissingPromptItems, "receipt_path": receiptRel}
	return projection, nil
}

func (s *Service) CollectionDiff(ctx context.Context, req CollectionRequest) (domain.Projection, error) {
	_, plan, err := s.collectionPlan(ctx, req, "collection.diff")
	if err != nil {
		return errorProjection("collection.diff", err), err
	}
	projection := collectionProjection("collection.diff", "Collection diff completed.", plan)
	projection.Data = map[string]any{"plan": plan}
	return projection, nil
}

func (s *Service) CollectionDoctor(ctx context.Context, req CollectionRequest) (domain.Projection, error) {
	_, plan, err := s.collectionPlan(ctx, req, "collection.doctor")
	if err != nil {
		return errorProjection("collection.doctor", err), err
	}
	projection := collectionProjection("collection.doctor", "Collection doctor completed.", plan)
	status := "ok"
	if plan.Stats.MissingPromptItems > 0 || len(plan.Stats.Issues) > 0 {
		status = "issues"
	}
	projection.Facts["status"] = status
	projection.Data = map[string]any{"stats": plan.Stats, "issues": plan.Stats.Issues}
	return projection, nil
}

func (s *Service) CollectionExport(ctx context.Context, req CollectionRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("collection.export", err), err
	}
	if strings.TrimSpace(req.To) == "" {
		err := &domain.CommandError{Code: "argument_required", Message: "collection export requires --to", Hint: "pinax collection export --to ./eikona-bundle.json --vault <vault> --json"}
		return domain.NewErrorProjection("collection.export", err), err
	}
	format := strings.TrimSpace(req.Format)
	if format == "" {
		format = "eikona.prompt_bundle.v1"
	}
	if format != "eikona.prompt_bundle.v1" {
		err := &domain.CommandError{Code: "collection_export_format_unsupported", Message: "unsupported collection export format", Hint: "Use --format eikona.prompt_bundle.v1"}
		return domain.NewErrorProjection("collection.export", err), err
	}
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return errorProjection("collection.export", err), err
	}
	assets, err := repo.Search(ctx, promptasset.SearchRequest{Domain: "visual_generation", Limit: 0})
	if err != nil {
		return errorProjection("collection.export", err), err
	}
	prompts := make([]map[string]any, 0, len(assets))
	for _, asset := range assets {
		details, err := repo.Details(ctx, asset.PromptAssetID)
		if err != nil {
			return errorProjection("collection.export", err), err
		}
		prompts = append(prompts, map[string]any{"id": asset.PromptAssetID, "title": asset.Title, "domain": asset.Domain, "tags": jsonStringList(asset.TagsJSON), "prompt": details.Version.PromptTemplate, "source_refs": details.SourceRefs})
	}
	sort.Slice(prompts, func(i, j int) bool { return fmt.Sprint(prompts[i]["id"]) < fmt.Sprint(prompts[j]["id"]) })
	body := map[string]any{"schema_version": "eikona.prompt_bundle.v1", "prompts": prompts}
	if err := writeJSONAsset(req.To, body); err != nil {
		return errorProjection("collection.export", err), err
	}
	projection := domain.NewProjection("collection.export", "Collection exported.")
	projection.Facts["format"] = format
	projection.Facts["exported_prompts"] = fmt.Sprint(len(prompts))
	projection.Facts["local_write"] = "true"
	projection.Evidence = []string{req.To}
	projection.Data = map[string]any{"format": format, "exported_prompts": len(prompts), "output": req.To}
	return projection, nil
}

func (s *Service) GraphRebuild(ctx context.Context, req GraphRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("graph.rebuild", err), err
	}
	graph, err := buildPromptGraph(ctx, root)
	if err != nil {
		return errorProjection("graph.rebuild", err), err
	}
	graphRel := filepath.ToSlash(filepath.Join(".pinax", "graph", "prompt_graph.json"))
	if err := writeJSONAsset(filepath.Join(root, filepath.FromSlash(graphRel)), graph); err != nil {
		return errorProjection("graph.rebuild", err), err
	}
	projection := domain.NewProjection("graph.rebuild", "Prompt graph projection rebuilt.")
	projection.Facts["graph_engine"] = "prompt_graph"
	projection.Facts["nodes"] = fmt.Sprint(len(graph.Nodes))
	projection.Facts["edges"] = fmt.Sprint(len(graph.Edges))
	projection.Facts["local_write"] = "true"
	projection.Evidence = []string{graphRel}
	projection.Data = map[string]any{"graph": graphGraphSummary(graph), "path": graphRel}
	return projection, nil
}

func (s *Service) GraphQuery(ctx context.Context, req GraphRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("graph.query", err), err
	}
	graph, err := loadPromptGraph(root)
	if err != nil {
		graph, err = buildPromptGraph(ctx, root)
		if err != nil {
			return errorProjection("graph.query", err), err
		}
	}
	results := queryPromptGraph(graph, req.Kind, req.Match)
	projection := domain.NewProjection("graph.query", "Prompt graph query completed.")
	projection.Facts["graph_engine"] = "prompt_graph"
	projection.Facts["results"] = fmt.Sprint(len(results))
	if strings.TrimSpace(req.Kind) != "" {
		projection.Facts["kind"] = strings.TrimSpace(req.Kind)
	}
	if strings.TrimSpace(req.Match) != "" {
		projection.Facts["match"] = strings.TrimSpace(req.Match)
	}
	projection.Evidence = []string{filepath.ToSlash(filepath.Join(".pinax", "graph", "prompt_graph.json"))}
	projection.Data = map[string]any{"results": results}
	return projection, nil
}

func (s *Service) collectionPlan(ctx context.Context, req CollectionRequest, command string) (string, collectionPlan, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return "", collectionPlan{}, err
	}
	bundle, err := loadCollectionBundle(req.From)
	if err != nil {
		return "", collectionPlan{}, err
	}
	stats := contentbundle.Analyze(bundle)
	if err := contentbundle.Validate(bundle); err != nil {
		return "", collectionPlan{}, &domain.CommandError{Code: "content_bundle_invalid", Message: err.Error(), Hint: "Provide a valid pinax.content_bundle.v1 file"}
	}
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return "", collectionPlan{}, err
	}
	plan := collectionPlan{Bundle: bundle, Stats: stats}
	for _, item := range bundle.Items {
		itemPlan := collectionPlanForItem(bundle, item)
		if fileExistsPath(root, itemPlan.NotePath) {
			plan.ExistingItems++
			itemPlan.NoteStatus = "existing"
		} else {
			plan.NewItems++
			itemPlan.NoteStatus = "new"
		}
		if strings.TrimSpace(item.Prompt) == "" {
			itemPlan.PromptStatus = "missing_prompt"
		} else if _, err := repo.Resolve(ctx, itemPlan.PromptAssetID); err == nil {
			plan.ExistingPrompts++
			itemPlan.PromptStatus = "existing"
		} else if promptasset.IsNotFound(err) {
			plan.NewPrompts++
			itemPlan.PromptStatus = "new"
		} else {
			return "", collectionPlan{}, err
		}
		plan.Plans = append(plan.Plans, itemPlan)
	}
	_ = command
	return root, plan, nil
}

func loadCollectionBundle(path string) (contentbundle.Bundle, error) {
	if strings.TrimSpace(path) == "" {
		return contentbundle.Bundle{}, &domain.CommandError{Code: "argument_required", Message: "collection command requires --from", Hint: "pinax collection import --from <bundle> --vault <vault> --dry-run --json"}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return contentbundle.Bundle{}, &domain.CommandError{Code: "content_bundle_unreadable", Message: "Content bundle cannot be read", Hint: "Check the --from file path and retry"}
	}
	bundle, err := contentbundle.Load(content)
	if err != nil {
		return contentbundle.Bundle{}, &domain.CommandError{Code: "content_bundle_invalid", Message: err.Error(), Hint: "Provide a valid pinax.content_bundle.v1 file"}
	}
	return bundle, nil
}

func collectionProjection(command, summary string, plan collectionPlan) domain.Projection {
	projection := domain.NewProjection(command, summary)
	projection.Facts["bundle_id"] = plan.Bundle.ID
	projection.Facts["items"] = fmt.Sprint(plan.Stats.Items)
	projection.Facts["complete_items"] = fmt.Sprint(plan.Stats.CompleteItems)
	projection.Facts["missing_prompt_items"] = fmt.Sprint(plan.Stats.MissingPromptItems)
	projection.Facts["new_items"] = fmt.Sprint(plan.NewItems)
	projection.Facts["existing_items"] = fmt.Sprint(plan.ExistingItems)
	projection.Facts["new_prompts"] = fmt.Sprint(plan.NewPrompts)
	projection.Facts["existing_prompts"] = fmt.Sprint(plan.ExistingPrompts)
	return projection
}

func collectionPlanForItem(bundle contentbundle.Bundle, item contentbundle.Item) collectionItemPlan {
	itemID := contentbundle.StableID("item", item.ID)
	return collectionItemPlan{ItemID: item.ID, Title: collectionItemTitle(item), NotePath: filepath.ToSlash(filepath.Join("notes", "collections", collectionSafeID(bundle.ID), itemID+".md")), PromptAssetID: collectionPromptID(bundle, item)}
}

func collectionPromptID(bundle contentbundle.Bundle, item contentbundle.Item) string {
	return collectionSafeID(bundle.ID) + "_" + contentbundle.StableID("prompt", item.ID)
}

func collectionSafeID(value string) string {
	if slug := contentbundle.SafeSlug(value); slug != "" {
		return slug
	}
	return contentbundle.StableID("collection", value)
}

func collectionItemTitle(item contentbundle.Item) string {
	if strings.TrimSpace(item.Title) != "" {
		return strings.TrimSpace(item.Title)
	}
	return strings.TrimSpace(item.ID)
}

func collectionNoteBody(bundle contentbundle.Bundle, item contentbundle.Item) string {
	var b strings.Builder
	b.WriteString("# " + collectionItemTitle(item) + "\n\n")
	b.WriteString("- Collection: " + bundle.ID + "\n")
	b.WriteString("- Prompt asset: pinax://prompt/" + collectionPromptID(bundle, item) + "\n")
	if item.SourceURL != "" {
		b.WriteString("- Source: " + item.SourceURL + "\n")
	} else if bundle.Source.URL != "" {
		b.WriteString("- Source: " + bundle.Source.URL + "\n")
	}
	if item.Category != "" {
		b.WriteString("- Category: " + item.Category + "\n")
	}
	if item.Language != "" {
		b.WriteString("- Language: " + item.Language + "\n")
	}
	b.WriteString("\n## Prompt\n\n")
	if strings.TrimSpace(item.Prompt) == "" {
		b.WriteString("Prompt unavailable in source payload.\n")
	} else {
		b.WriteString(strings.TrimSpace(item.Prompt) + "\n")
	}
	return b.String()
}

func collectionPromptAsset(bundle contentbundle.Bundle, item contentbundle.Item) promptasset.Asset {
	tags := collectionPromptTags(bundle, item)
	return promptasset.Asset{SchemaVersion: promptasset.SchemaVersion, ID: collectionPromptID(bundle, item), Title: collectionItemTitle(item), Domain: "visual_generation", Tags: tags, Lifecycle: "draft", Permission: "public", OwnerProject: bundle.ID, Variables: map[string]promptasset.Variable{"input": {Type: "string", Required: false, Description: "Optional adaptation input"}}, PromptTemplate: strings.TrimSpace(item.Prompt), SourceRefs: []promptasset.SourceRef{{URI: firstNonEmpty(item.SourceURL, bundle.Source.URL), Label: firstNonEmpty(bundle.Source.ID, bundle.ID), Evidence: "collection_import"}}}
}

func collectionPromptTags(bundle contentbundle.Bundle, item contentbundle.Item) []string {
	values := []string{"collection", "source/" + collectionSafeID(bundle.ID)}
	if item.Category != "" {
		values = append(values, "category/"+contentbundle.SafeSlug(item.Category))
	}
	if item.Language != "" {
		values = append(values, "language/"+contentbundle.SafeSlug(item.Language))
	}
	for _, value := range item.Techniques {
		values = append(values, "technique/"+contentbundle.SafeSlug(value))
	}
	for _, value := range item.Styles {
		values = append(values, "style/"+contentbundle.SafeSlug(value))
	}
	for _, value := range item.Subjects {
		values = append(values, "subject/"+contentbundle.SafeSlug(value))
	}
	values = append(values, item.Tags...)
	return contentbundle.Dedupe(values)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "unknown"
}

func buildPromptGraph(ctx context.Context, root string) (promptGraph, error) {
	repo, err := promptasset.OpenVaultRepository(root)
	if err != nil {
		return promptGraph{}, err
	}
	assets, err := repo.Search(ctx, promptasset.SearchRequest{Domain: "visual_generation", Limit: 0})
	if err != nil {
		return promptGraph{}, err
	}
	graph := promptGraph{SchemaVersion: "pinax.prompt_graph.v1"}
	nodes := map[string]promptGraphNode{}
	edges := map[string]promptGraphEdge{}
	addNode := func(node promptGraphNode) {
		if _, ok := nodes[node.ID]; !ok {
			nodes[node.ID] = node
		}
	}
	addEdge := func(edge promptGraphEdge) { edges[edge.From+"\x00"+edge.To+"\x00"+edge.Kind] = edge }
	for _, asset := range assets {
		promptNodeID := "prompt:" + asset.PromptAssetID
		addNode(promptGraphNode{ID: promptNodeID, Kind: "prompt", Label: asset.PromptAssetID, PromptAssetID: asset.PromptAssetID, Title: asset.Title})
		details, err := repo.Details(ctx, asset.PromptAssetID)
		if err != nil {
			return promptGraph{}, err
		}
		for _, source := range details.SourceRefs {
			label := firstNonEmpty(source.Label, source.URI)
			nodeID := "source:" + contentbundle.StableID("source", label)
			addNode(promptGraphNode{ID: nodeID, Kind: "source", Label: label})
			addEdge(promptGraphEdge{From: promptNodeID, To: nodeID, Kind: "derived_from"})
		}
		for _, tag := range jsonStringList(asset.TagsJSON) {
			kind, label, ok := graphDimensionFromTag(tag)
			if !ok {
				continue
			}
			nodeID := kind + ":" + label
			addNode(promptGraphNode{ID: nodeID, Kind: kind, Label: label})
			addEdge(promptGraphEdge{From: promptNodeID, To: nodeID, Kind: "has_" + kind})
		}
	}
	for _, node := range nodes {
		graph.Nodes = append(graph.Nodes, node)
	}
	for _, edge := range edges {
		graph.Edges = append(graph.Edges, edge)
	}
	sort.Slice(graph.Nodes, func(i, j int) bool { return graph.Nodes[i].ID < graph.Nodes[j].ID })
	sort.Slice(graph.Edges, func(i, j int) bool {
		return graph.Edges[i].From+graph.Edges[i].To+graph.Edges[i].Kind < graph.Edges[j].From+graph.Edges[j].To+graph.Edges[j].Kind
	})
	return graph, nil
}

func loadPromptGraph(root string) (promptGraph, error) {
	path := filepath.Join(root, ".pinax", "graph", "prompt_graph.json")
	content, err := os.ReadFile(path)
	if err != nil {
		return promptGraph{}, err
	}
	var graph promptGraph
	if err := json.Unmarshal(content, &graph); err != nil {
		return promptGraph{}, err
	}
	return graph, nil
}

func queryPromptGraph(graph promptGraph, kind, match string) []map[string]string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	match = strings.ToLower(strings.TrimSpace(match))
	nodeByID := map[string]promptGraphNode{}
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
	}
	matchedNodes := map[string]bool{}
	for _, node := range graph.Nodes {
		if kind != "" && node.Kind != kind {
			continue
		}
		if match != "" && !strings.Contains(strings.ToLower(node.Label), match) && !strings.Contains(strings.ToLower(node.ID), match) {
			continue
		}
		matchedNodes[node.ID] = true
	}
	seen := map[string]bool{}
	results := []map[string]string{}
	for _, edge := range graph.Edges {
		if !matchedNodes[edge.To] && !matchedNodes[edge.From] {
			continue
		}
		prompt := nodeByID[edge.From]
		if prompt.Kind != "prompt" {
			prompt = nodeByID[edge.To]
		}
		if prompt.Kind != "prompt" || seen[prompt.ID] {
			continue
		}
		seen[prompt.ID] = true
		results = append(results, map[string]string{"prompt_asset_id": prompt.PromptAssetID, "title": prompt.Title})
	}
	sort.Slice(results, func(i, j int) bool { return results[i]["prompt_asset_id"] < results[j]["prompt_asset_id"] })
	return results
}

func graphDimensionFromTag(tag string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(tag), "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		return "", "", false
	}
	switch parts[0] {
	case "category", "technique", "style", "subject":
		return parts[0], parts[1], true
	default:
		return "", "", false
	}
}

func jsonStringList(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}

func graphGraphSummary(graph promptGraph) map[string]any {
	return map[string]any{"schema_version": graph.SchemaVersion, "nodes": len(graph.Nodes), "edges": len(graph.Edges)}
}
