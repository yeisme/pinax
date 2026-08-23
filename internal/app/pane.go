package app

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

const (
	PaneEventSchema    = "pane.event.v1alpha1"
	PaneArtifactSchema = "pane.artifact.v1alpha1"
)

// PaneContext is the DSH Pane context carried on every envelope.
type PaneContext struct {
	WorkspaceRef string `json:"workspaceRef"`
	SessionRef   string `json:"sessionRef,omitempty"`
	PrincipalRef string `json:"principalRef,omitempty"`
	Revision     string `json:"revision"`
}

// PaneEntity is one redacted note item.
type PaneEntity struct {
	Ref     string         `json:"ref"`
	Version int            `json:"version"`
	Value   map[string]any `json:"value"`
}

// PaneSnapshotEnvelope is a PaneEventEnvelopeV1 snapshot payload.
type PaneSnapshotEnvelope struct {
	Schema     string         `json:"schema"`
	Stream     string         `json:"stream"`
	Cursor     string         `json:"cursor"`
	Sequence   int            `json:"sequence"`
	Context    PaneContext    `json:"context"`
	OccurredAt string         `json:"occurredAt"`
	ObservedAt string         `json:"observedAt"`
	Freshness  string         `json:"freshness"`
	Status     string         `json:"status"`
	Op         string         `json:"op"`
	Payload    map[string]any `json:"payload"`
}

// PaneArtifactRef is ArtifactRefV1 for a Pinax note.
type PaneArtifactRef struct {
	Schema       string   `json:"schema"`
	Owner        string   `json:"owner"`
	Kind         string   `json:"kind"`
	Ref          string   `json:"ref"`
	Version      string   `json:"version"`
	MediaType    string   `json:"mediaType"`
	Title        string   `json:"title"`
	Summary      string   `json:"summary,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs"`
	Capabilities []string `json:"capabilities"`
}

// PaneGatedAction is an owner-authored mutation the Pane may submit.
type PaneGatedAction struct {
	ID                  string `json:"id"`
	Gated               bool   `json:"gated"`
	ExpectedRevision    string `json:"expected_revision"`
	IdempotencyRequired bool   `json:"idempotency_required"`
	ReceiptRequired     bool   `json:"receipt_required"`
	OwnerCommand        string `json:"owner_command,omitempty"`
}

func paneTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// newPaneSnapshotEnvelope builds the shared PaneEventEnvelopeV1 snapshot
// skeleton; callers only decide status, freshness, entities, and timeline.
func newPaneSnapshotEnvelope(context PaneContext, status, freshness string, entities []PaneEntity, timeline []any) PaneSnapshotEnvelope {
	now := paneTimestamp()
	if entities == nil {
		entities = []PaneEntity{}
	}
	if timeline == nil {
		timeline = []any{}
	}
	return PaneSnapshotEnvelope{
		Schema:     PaneEventSchema,
		Stream:     "domain.pinax",
		Cursor:     "c-1",
		Sequence:   -1,
		Context:    context,
		OccurredAt: now,
		ObservedAt: now,
		Freshness:  freshness,
		Status:     status,
		Op:         "snapshot",
		Payload: map[string]any{
			"entities": entities,
			"timeline": timeline,
			"receipts": []any{},
		},
	}
}

// paneRefPattern is the closed allowlist for pane refs: note IDs are UUIDs or
// legacy hex/dotted ids, never paths or URLs. No "/" or ":" so filesystem
// paths and URL schemes are structurally rejected; length-capped like the
// harness SAFE_REF contract.
var paneRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// paneRefDenySubstrings rejects credential-shaped ids even when they match the
// allowlist shape.
var paneRefDenySubstrings = []string{"token", "authorization", "cookie", "secret", "password", "api_key", "bearer"}

func paneUnsafe(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	if !paneRefPattern.MatchString(trimmed) {
		return true
	}
	lower := strings.ToLower(trimmed)
	for _, banned := range paneRefDenySubstrings {
		if strings.Contains(lower, banned) {
			return true
		}
	}
	return false
}

func paneNoteRef(note domain.Note) string {
	if strings.TrimSpace(note.ID) != "" {
		return note.ID
	}
	return ""
}

// AssemblePaneSnapshot maps a note.list projection onto PaneEventEnvelopeV1.
func AssemblePaneSnapshot(projection domain.Projection, context PaneContext) (PaneSnapshotEnvelope, error) {
	if strings.TrimSpace(context.WorkspaceRef) == "" || strings.TrimSpace(context.Revision) == "" {
		return PaneSnapshotEnvelope{}, errors.New("pane context is required")
	}
	notes, err := paneNotes(projection)
	if err != nil {
		return PaneSnapshotEnvelope{}, err
	}
	entities := make([]PaneEntity, 0, len(notes))
	for _, note := range notes {
		ref := paneNoteRef(note)
		if paneUnsafe(ref) {
			continue
		}
		entities = append(entities, PaneEntity{
			Ref:     ref,
			Version: 1,
			Value: map[string]any{
				"title":  note.Title,
				"kind":   firstNonEmpty(note.Kind, "note"),
				"status": firstNonEmpty(note.Status, "active"),
				"tags":   note.Tags,
			},
		})
	}
	// NewErrorProjection reports Status "failed"; never present a failed
	// projection as a ready pane with empty entities.
	return newPaneSnapshotEnvelope(context, paneStatusForProjection(projection.Status), "fresh", entities, nil), nil
}

// paneNotes extracts in-process note entities from a note.list projection.
// A projection whose data carries notes in any other shape (e.g. after a JSON
// round-trip) fails closed instead of silently yielding an empty snapshot.
func paneNotes(projection domain.Projection) ([]domain.Note, error) {
	data, ok := projection.Data.(map[string]any)
	if !ok {
		if projection.Data == nil {
			return nil, nil
		}
		return nil, errors.New("pane projection data has unsupported shape")
	}
	raw, ok := data["notes"]
	if !ok {
		return nil, nil
	}
	notes, ok := raw.([]domain.Note)
	if !ok {
		return nil, errors.New("pane projection notes have unsupported shape; expected in-process note.list notes")
	}
	return notes, nil
}

// PaneArtifactFromNote maps a note to ArtifactRefV1 without emitting a filesystem path.
func PaneArtifactFromNote(note domain.Note) (PaneArtifactRef, error) {
	ref := paneNoteRef(note)
	if paneUnsafe(ref) {
		return PaneArtifactRef{}, errors.New("artifact ref is unsafe")
	}
	return PaneArtifactRef{
		Schema:       PaneArtifactSchema,
		Owner:        "pinax",
		Kind:         firstNonEmpty(note.Kind, "note"),
		Ref:          ref,
		Version:      "1",
		MediaType:    "text/markdown",
		Title:        note.Title,
		EvidenceRefs: []string{},
		Capabilities: []string{"open", "link", "attach_context"},
	}, nil
}

// PaneGatedActions returns capture/sync that must run through Pinax CLI/service.
func PaneGatedActions(revision string) []PaneGatedAction {
	return []PaneGatedAction{
		{ID: "inbox.capture", Gated: true, ExpectedRevision: revision, IdempotencyRequired: true, ReceiptRequired: true, OwnerCommand: "pinax inbox capture"},
		{ID: "sync.run", Gated: true, ExpectedRevision: revision, IdempotencyRequired: true, ReceiptRequired: true, OwnerCommand: "pinax sync run"},
	}
}

// RejectHandwrittenMetadata fails closed when a client submits a metadata blob.
func RejectHandwrittenMetadata(blob string) error {
	if strings.TrimSpace(blob) == "" {
		return errors.New("empty metadata blob")
	}
	return errors.New("pinax pane rejects handwritten metadata; use pinax CLI or service")
}

// AssemblePaneNegative maps owner recovery states. Clients must not poll.
func AssemblePaneNegative(kind string, context PaneContext) (PaneSnapshotEnvelope, error) {
	if strings.TrimSpace(context.WorkspaceRef) == "" || strings.TrimSpace(context.Revision) == "" {
		return PaneSnapshotEnvelope{}, errors.New("pane context is required")
	}
	var status string
	switch kind {
	case "offline":
		status = "offline"
	case "permission_denied":
		status = "permission_denied"
	case "handwritten_metadata":
		return PaneSnapshotEnvelope{}, RejectHandwrittenMetadata("schema_version: pinax.note.v1")
	default:
		return PaneSnapshotEnvelope{}, errors.New("unknown pane negative kind")
	}
	return newPaneSnapshotEnvelope(context, status, "unknown", nil, nil), nil
}
