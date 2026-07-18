package app

import (
	"fmt"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

func noteAgentContext(note domain.Note, display domain.NoteDisplayKind, sourceKind string, snippets []domain.AgentContextSnippet, actions []domain.Action) domain.AgentContext {
	if sourceKind == "" {
		sourceKind = "note"
	}
	title := strings.TrimSpace(note.Title)
	if title == "" {
		title = note.Path
	}
	contextID := note.ID
	if contextID == "" {
		contextID = note.Path
	}
	if len(snippets) == 0 {
		if excerpt := noteExcerpt(note.Body); excerpt != "" && display != domain.NoteDisplayBody {
			snippets = []domain.AgentContextSnippet{{Kind: "excerpt", Text: excerpt, Source: note.Path}}
		}
	}
	if len(actions) == 0 {
		actions = []domain.Action{{Name: "read", Command: fmt.Sprintf("pinax note read %s --display card --vault <vault> --json", shellQuote(title))}}
	}
	return domain.AgentContext{
		SchemaVersion: domain.AgentContextSchemaVersion,
		ContextID:     sourceKind + ":" + contextID,
		SourceKind:    sourceKind,
		DisplayTitle:  title,
		Refs:          []domain.AgentContextRef{{Kind: "note", ID: note.ID, Path: note.Path, Title: title}},
		Snippets:      snippets,
		Evidence:      compactEvidence(note),
		BodyExposure:  string(display),
		Actions:       actions,
	}
}

func boardItemAgentContext(item domain.BoardItem, vaultRoot string) domain.AgentContext {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = item.ItemID
	}
	refs := []domain.AgentContextRef{{Kind: "project_item", ID: item.ItemID, Path: item.Path, Title: title}}
	if item.NoteID != "" || item.Path != "" {
		refs = append(refs, domain.AgentContextRef{Kind: "note", ID: item.NoteID, Path: item.Path, Title: title})
	}
	evidence := append([]string{}, item.EvidenceRefs...)
	if item.Path != "" {
		evidence = append(evidence, item.Path)
	}
	if len(evidence) == 0 && item.ItemID != "" {
		evidence = append(evidence, item.ItemID)
	}
	return domain.AgentContext{SchemaVersion: domain.AgentContextSchemaVersion, ContextID: "project_board_item:" + item.ItemID, SourceKind: "project_board_item", DisplayTitle: title, Refs: refs, Snippets: []domain.AgentContextSnippet{}, Evidence: evidence, BodyExposure: "card", Actions: []domain.Action{{Name: "show", Command: fmt.Sprintf("pinax project item show %s --vault %s --json", shellQuote(item.ItemID), shellQuote(vaultRoot))}}}
}

func graphAgentContext(note domain.Note, links []domain.NoteLink) domain.AgentContext {
	snippets := make([]domain.AgentContextSnippet, 0, len(links))
	evidence := compactEvidence(note)
	for _, link := range links {
		if strings.TrimSpace(link.Evidence) != "" {
			snippets = append(snippets, domain.AgentContextSnippet{Kind: "link_evidence", Text: link.Evidence, Source: link.SourcePath})
			evidence = append(evidence, link.Evidence)
		}
		if len(snippets) >= 3 {
			break
		}
	}
	title := strings.TrimSpace(note.Title)
	if title == "" {
		title = note.Path
	}
	return domain.AgentContext{
		SchemaVersion: domain.AgentContextSchemaVersion,
		ContextID:     "graph_entity:" + note.ID,
		SourceKind:    "graph_entity",
		DisplayTitle:  title,
		Refs:          []domain.AgentContextRef{{Kind: "note", ID: note.ID, Path: note.Path, Title: title}},
		Snippets:      snippets,
		Evidence:      evidence,
		BodyExposure:  "context",
		Actions:       []domain.Action{{Name: "links", Command: fmt.Sprintf("pinax note links %s --vault <vault> --json", shellQuote(title))}},
	}
}

type AgentBrainContextBundleRequest struct {
	Task      string
	Contexts  []domain.AgentContext
	Receipts  []domain.AgentBrainReceiptRef
	Freshness domain.AgentBrainFreshness
	Actions   []domain.Action
}

func BuildAgentBrainContextBundle(req AgentBrainContextBundleRequest) domain.AgentBrainContextBundle {
	bundle := domain.AgentBrainContextBundle{
		SchemaVersion: domain.AgentBrainContextBundleSchemaVersion,
		Task:          strings.TrimSpace(req.Task),
		Receipts:      append([]domain.AgentBrainReceiptRef{}, req.Receipts...),
		Freshness:     req.Freshness,
		BodyExposure:  "bounded_projection",
	}
	if bundle.Freshness.GeneratedFrom == "" {
		bundle.Freshness.GeneratedFrom = "existing_projections"
	}

	seenEntities := map[string]bool{}
	seenRefs := map[string]bool{}
	seenActions := map[string]bool{}
	addEntity := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seenEntities[value] {
			return
		}
		seenEntities[value] = true
		bundle.Entities = append(bundle.Entities, value)
	}
	addAction := func(action domain.Action) {
		if strings.TrimSpace(action.Name) == "" || strings.TrimSpace(action.Command) == "" {
			return
		}
		key := action.Name + "\x00" + action.Command
		if seenActions[key] {
			return
		}
		seenActions[key] = true
		bundle.NextActions = append(bundle.NextActions, action)
	}
	addRef := func(bucket string, ref domain.AgentContextRef) {
		if strings.TrimSpace(ref.Kind) == "" {
			return
		}
		key := bucket + "\x00" + ref.Kind + "\x00" + ref.ID + "\x00" + ref.Path + "\x00" + ref.Title
		if seenRefs[key] {
			return
		}
		seenRefs[key] = true
		safeRef := domain.AgentContextRef{Kind: ref.Kind, ID: ref.ID, Path: ref.Path, Title: ref.Title}
		switch bucket {
		case "memory":
			bundle.MemoryRefs = append(bundle.MemoryRefs, safeRef)
		case "semantic":
			bundle.SemanticRefs = append(bundle.SemanticRefs, safeRef)
		case "graph":
			bundle.GraphRefs = append(bundle.GraphRefs, safeRef)
		case "query":
			bundle.QueryRefs = append(bundle.QueryRefs, safeRef)
		}
		addEntity(ref.Title)
	}

	for _, action := range req.Actions {
		addAction(action)
	}
	for _, ctx := range req.Contexts {
		addEntity(ctx.DisplayTitle)
		bucket := agentBrainBucketForSource(ctx.SourceKind)
		for _, ref := range ctx.Refs {
			addRef(bucket, ref)
		}
		for _, action := range ctx.Actions {
			addAction(action)
		}
	}
	return bundle
}

func agentBrainBucketForSource(sourceKind string) string {
	lower := strings.ToLower(strings.TrimSpace(sourceKind))
	switch {
	case strings.Contains(lower, "memory"):
		return "memory"
	case strings.Contains(lower, "kb") || strings.Contains(lower, "semantic") || strings.Contains(lower, "search"):
		return "semantic"
	case strings.Contains(lower, "graph") || strings.Contains(lower, "link") || strings.Contains(lower, "backlink"):
		return "graph"
	case strings.Contains(lower, "query") || strings.Contains(lower, "database") || strings.Contains(lower, "dataview"):
		return "query"
	default:
		return "semantic"
	}
}

func compactEvidence(note domain.Note) []string {
	evidence := []string{}
	if note.ID != "" {
		evidence = append(evidence, "note_id:"+note.ID)
	}
	if note.Path != "" {
		evidence = append(evidence, note.Path)
	}
	return evidence
}
