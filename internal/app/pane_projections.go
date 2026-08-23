package app

import (
	"errors"
	"sort"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
)

const (
	paneBacklinksLimit = 50
	paneGraphTopK      = 20
	paneHistoryLimit   = 100
	paneValueLimit     = 160
)

// PaneTimelineEvent is one bounded record-ledger revision event.
type PaneTimelineEvent struct {
	Op       string `json:"op"`
	Ref      string `json:"ref"`
	Revision string `json:"revision,omitempty"`
	Time     string `json:"time,omitempty"`
}

// PaneGraphTotals carries the bounded graph summary counts.
type PaneGraphTotals struct {
	Nodes      int  `json:"nodes"`
	Edges      int  `json:"edges"`
	Components int  `json:"components"`
	Truncated  bool `json:"truncated"`
}

func paneStatusForProjection(status string) string {
	switch status {
	case "failed", "error", "offline":
		return "offline"
	case "permission_denied":
		return "permission_denied"
	default:
		return "ready"
	}
}

// paneBoundString caps free-text values (titles) so a hostile or oversized
// vault cannot blow up the pane payload.
func paneBoundString(value string) string {
	if len(value) <= paneValueLimit {
		return value
	}
	return value[:paneValueLimit]
}

func panePayload(entities []PaneEntity, timeline []any, extra map[string]any) map[string]any {
	if entities == nil {
		entities = []PaneEntity{}
	}
	if timeline == nil {
		timeline = []any{}
	}
	payload := map[string]any{
		"entities": entities,
		"timeline": timeline,
		"receipts": []any{},
	}
	for key, value := range extra {
		payload[key] = value
	}
	return payload
}

// paneProjectionData extracts the projection's data map; projections without
// data (nil) are legal (e.g. failed runs), any other non-map shape fails closed.
func paneProjectionData(projection domain.Projection) (map[string]any, error) {
	if projection.Data == nil {
		return nil, nil
	}
	data, ok := projection.Data.(map[string]any)
	if !ok {
		return nil, errors.New("pane projection data has unsupported shape")
	}
	return data, nil
}

// AssemblePaneBacklinks maps a note.backlinks projection onto the pane
// envelope: bounded source entities, no paths, no note bodies.
func AssemblePaneBacklinks(projection domain.Projection, context PaneContext) (PaneSnapshotEnvelope, error) {
	if strings.TrimSpace(context.WorkspaceRef) == "" || strings.TrimSpace(context.Revision) == "" {
		return PaneSnapshotEnvelope{}, errors.New("pane context is required")
	}
	data, err := paneProjectionData(projection)
	if err != nil {
		return PaneSnapshotEnvelope{}, err
	}
	raw, ok := data["backlinks"]
	if !ok {
		raw = []domain.NoteLink(nil)
	}
	links, ok := raw.([]domain.NoteLink)
	if !ok {
		return PaneSnapshotEnvelope{}, errors.New("pane projection backlinks have unsupported shape; expected in-process note.backlinks links")
	}
	entities := make([]PaneEntity, 0, len(links))
	dropped := 0
	truncated := false
	for _, link := range links {
		ref := firstNonEmpty(link.SourceNoteID, link.SourceObjectID)
		if paneUnsafe(ref) {
			dropped++
			continue
		}
		if len(entities) >= paneBacklinksLimit {
			truncated = true
			continue
		}
		status := "ok"
		if link.Broken || link.Status == string(domain.LinkStatusBroken) {
			status = "broken"
		}
		entities = append(entities, PaneEntity{
			Ref:     ref,
			Version: 1,
			Value: map[string]any{
				"title":     paneBoundString(link.SourceTitle),
				"link_kind": paneBoundString(firstNonEmpty(link.Kind, "link")),
				"status":    status,
			},
		})
	}
	envelope := newPaneSnapshotEnvelope(context, paneStatusForProjection(projection.Status), "fresh", entities, nil)
	envelope.Payload = panePayload(entities, nil, map[string]any{"dropped_unsafe": dropped, "truncated": truncated})
	return envelope, nil
}

// AssemblePaneGraphSummary derives a path-free graph summary (node/edge/
// component counts plus bounded top-k degrees) from vault notes and links.
func AssemblePaneGraphSummary(notes []domain.Note, links []domain.NoteLink, context PaneContext) (PaneSnapshotEnvelope, error) {
	if strings.TrimSpace(context.WorkspaceRef) == "" || strings.TrimSpace(context.Revision) == "" {
		return PaneSnapshotEnvelope{}, errors.New("pane context is required")
	}
	pathToRef := make(map[string]string, len(notes))
	inDegree := map[string]int{}
	outDegree := map[string]int{}
	adjacent := map[string][]string{}
	nodes := 0
	dropped := 0
	for _, note := range notes {
		ref := paneNoteRef(note)
		if paneUnsafe(ref) {
			dropped++
			continue
		}
		pathToRef[note.Path] = ref
		nodes++
	}
	edges := 0
	for _, link := range links {
		source, okSource := pathToRef[link.SourcePath]
		if !okSource || source == "" {
			continue
		}
		target := firstNonEmpty(link.TargetNoteID, link.TargetObjectID)
		if target == "" {
			// Fall back to resolving the target through its path when the
			// link lacks object identity.
			target = pathToRef[link.TargetPath]
		}
		if target == "" || source == target {
			continue
		}
		edges++
		outDegree[source]++
		inDegree[target]++
		adjacent[source] = append(adjacent[source], target)
		adjacent[target] = append(adjacent[target], source)
	}
	components := paneConnectedComponents(pathToRef, adjacent)
	ranked := make([]PaneEntity, 0, len(inDegree)+len(outDegree))
	for ref, total := range paneTotalDegrees(inDegree, outDegree) {
		ranked = append(ranked, PaneEntity{Ref: ref, Version: 1, Value: map[string]any{
			"degree": total,
			"in":     inDegree[ref],
			"out":    outDegree[ref],
		}})
	}
	sort.Slice(ranked, func(i, j int) bool {
		left, right := ranked[i], ranked[j]
		leftTotal, rightTotal := left.Value["degree"].(int), right.Value["degree"].(int)
		if leftTotal != rightTotal {
			return leftTotal > rightTotal
		}
		return left.Ref < right.Ref
	})
	truncated := len(ranked) > paneGraphTopK
	if len(ranked) > paneGraphTopK {
		ranked = ranked[:paneGraphTopK]
	}
	envelope := newPaneSnapshotEnvelope(context, "ready", "fresh", ranked, nil)
	envelope.Payload = panePayload(ranked, nil, map[string]any{
		"totals":         PaneGraphTotals{Nodes: nodes, Edges: edges, Components: components, Truncated: truncated},
		"dropped_unsafe": dropped,
	})
	return envelope, nil
}

func paneTotalDegrees(inDegree, outDegree map[string]int) map[string]int {
	totals := make(map[string]int, len(inDegree)+len(outDegree))
	for ref, degree := range inDegree {
		totals[ref] += degree
	}
	for ref, degree := range outDegree {
		totals[ref] += degree
	}
	return totals
}

func paneConnectedComponents(pathToRef map[string]string, adjacent map[string][]string) int {
	parent := map[string]string{}
	var find func(string) string
	find = func(ref string) string {
		if parent[ref] == "" {
			parent[ref] = ref
			return ref
		}
		if parent[ref] == ref {
			return ref
		}
		root := find(parent[ref])
		parent[ref] = root
		return root
	}
	union := func(a, b string) {
		rootA, rootB := find(a), find(b)
		if rootA != rootB {
			parent[rootB] = rootA
		}
	}
	for _, ref := range pathToRef {
		find(ref)
	}
	for source, targets := range adjacent {
		for _, target := range targets {
			union(source, target)
		}
	}
	roots := map[string]bool{}
	for _, ref := range pathToRef {
		roots[find(ref)] = true
	}
	return len(roots)
}

// AssemblePaneHistory maps record-ledger revision events onto the pane
// timeline slot; events are bounded and carry no paths or bodies.
func AssemblePaneHistory(projection domain.Projection, context PaneContext) (PaneSnapshotEnvelope, error) {
	if strings.TrimSpace(context.WorkspaceRef) == "" || strings.TrimSpace(context.Revision) == "" {
		return PaneSnapshotEnvelope{}, errors.New("pane context is required")
	}
	data, err := paneProjectionData(projection)
	if err != nil {
		return PaneSnapshotEnvelope{}, err
	}
	raw, ok := data["events"]
	if !ok {
		raw = []domain.RecordEvent(nil)
	}
	events, ok := raw.([]domain.RecordEvent)
	if !ok {
		return PaneSnapshotEnvelope{}, errors.New("pane projection events have unsupported shape; expected in-process record events")
	}
	timeline := make([]any, 0, len(events))
	dropped := 0
	truncated := false
	for _, event := range events {
		ref := firstNonEmpty(event.ObjectID, event.NoteID)
		if paneUnsafe(ref) {
			dropped++
			continue
		}
		if len(timeline) >= paneHistoryLimit {
			truncated = true
			continue
		}
		timeline = append(timeline, PaneTimelineEvent{
			Op:       string(event.Kind),
			Ref:      ref,
			Revision: event.ContentRevision.Hash,
			Time:     event.CreatedAt,
		})
	}
	envelope := newPaneSnapshotEnvelope(context, paneStatusForProjection(projection.Status), "fresh", nil, timeline)
	envelope.Payload = panePayload(nil, timeline, map[string]any{"dropped_unsafe": dropped, "truncated": truncated})
	return envelope, nil
}
