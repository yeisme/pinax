package agentprotocol

import (
	"strings"
	"unicode/utf8"
)

const NoteProposalSchemaVersion = "pinax.agent_note_proposal.v1alpha1"

// NoteChange is user content. It is never an audit or output payload.
type NoteChange struct {
	Kind             string `json:"kind"`
	TargetObjectID   string `json:"target_object_id,omitempty"`
	ExpectedRevision string `json:"expected_revision,omitempty"`
	Title            string `json:"title,omitempty"`
	Body             string `json:"body"`
	Date             string `json:"date,omitempty"`
	Rationale        string `json:"rationale"`
}

func (c NoteChange) Validate() error {
	switch c.Kind {
	case "inbox_capture", "journal_append", "note_draft", "note_edit":
	default:
		return NewStableError(ErrCodeValidationFailed, "Unsupported note proposal kind")
	}
	if strings.TrimSpace(c.Body) == "" || len(c.Body) > 64<<10 || !utf8.ValidString(c.Body) || strings.ContainsRune(c.Body, 0) {
		return NewStableError(ErrCodeValidationFailed, "Body must be nonempty UTF-8 text of at most 64 KiB")
	}
	if utf8.RuneCountInString(c.Rationale) > 500 || len(c.Title) > 512 || strings.ContainsAny(c.Title, "\r\n\x00") {
		return NewStableError(ErrCodeValidationFailed, "Rationale is limited to 500 characters; title must be a single line")
	}
	if c.Kind == "note_edit" {
		if c.TargetObjectID == "" || c.ExpectedRevision == "" || c.Title != "" || c.Date != "" {
			return NewStableError(ErrCodeValidationFailed, "Note edits require an object ID and expected revision, without title or date changes")
		}
	} else if c.Kind != "journal_append" && (c.TargetObjectID != "" || c.ExpectedRevision != "" || c.Date != "") {
		return NewStableError(ErrCodeValidationFailed, "New notes cannot target an existing object or revision")
	}
	if (c.Kind == "inbox_capture" || c.Kind == "note_draft") && strings.TrimSpace(c.Title) == "" {
		return NewStableError(ErrCodeValidationFailed, "A title is required for a new note")
	}
	if c.Kind == "journal_append" && (c.Title != "" || (c.TargetObjectID == "" && c.ExpectedRevision != "") || (c.TargetObjectID != "" && c.Date != "")) {
		return NewStableError(ErrCodeValidationFailed, "Journal append accepts either a date or an existing daily object ID; it cannot change a title")
	}
	return nil
}

func (c NoteChange) Risk() RiskLevel {
	if c.Kind == "inbox_capture" || c.Kind == "journal_append" || c.Kind == "note_draft" {
		return RiskLow
	}
	return RiskMedium
}
