package briefing

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const FeedbackStateSchemaVersion = "pinax.briefing.feedback.v1"

type FeedbackAction string

const (
	FeedbackAccept       FeedbackAction = "accept"
	FeedbackArchive      FeedbackAction = "archive"
	FeedbackDismiss      FeedbackAction = "dismiss"
	FeedbackFollowUp     FeedbackAction = "follow_up"
	FeedbackMoreLikeThis FeedbackAction = "more_like_this"
	FeedbackLessLikeThis FeedbackAction = "less_like_this"
)

type FeedbackRequest struct {
	CandidateID string
	EvidenceID  string
	Action      FeedbackAction
	Reason      string
}

type FeedbackState struct {
	SchemaVersion string          `json:"schema_version"`
	Events        []FeedbackEvent `json:"events"`
}

type FeedbackEvent struct {
	FeedbackID  string         `json:"feedback_id"`
	CandidateID string         `json:"candidate_id"`
	EvidenceID  string         `json:"evidence_id"`
	Action      FeedbackAction `json:"action"`
	Reason      string         `json:"reason,omitempty"`
	WeightDelta float64        `json:"weight_delta"`
	CreatedAt   string         `json:"created_at"`
}

func ApplyFeedback(root string, req FeedbackRequest) (FeedbackState, error) {
	now := time.Now().UTC()
	event := FeedbackEvent{FeedbackID: feedbackID(req.CandidateID, string(req.Action), now), CandidateID: req.CandidateID, EvidenceID: req.EvidenceID, Action: req.Action, Reason: req.Reason, WeightDelta: PreferenceWeight(req.Action), CreatedAt: now.Format(time.RFC3339)}
	if err := appendFeedbackJSONL(filepath.Join(root, ".pinax", "briefing", "feedback.jsonl"), event); err != nil {
		return FeedbackState{}, err
	}
	if err := appendFeedbackJSONL(filepath.Join(root, ".pinax", "events.jsonl"), map[string]any{"type": "briefing.feedback", "status": "success", "candidate_id": req.CandidateID, "action": req.Action, "created_at": event.CreatedAt}); err != nil {
		return FeedbackState{}, err
	}
	return FeedbackState{SchemaVersion: FeedbackStateSchemaVersion, Events: []FeedbackEvent{event}}, nil
}

func PreferenceWeight(action FeedbackAction) float64 {
	switch action {
	case FeedbackAccept:
		return 1
	case FeedbackMoreLikeThis:
		return 0.5
	case FeedbackFollowUp:
		return 0.25
	case FeedbackArchive:
		return -0.2
	case FeedbackDismiss:
		return -0.5
	case FeedbackLessLikeThis:
		return -0.7
	default:
		return 0
	}
}

func appendFeedbackJSONL(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	b, err := json.Marshal(value)
	if err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(append(b, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func feedbackID(candidateID, action string, t time.Time) string {
	h := sha1.Sum([]byte(candidateID + "\x00" + action + "\x00" + t.Format(time.RFC3339Nano)))
	return "fb_" + hex.EncodeToString(h[:])[:16]
}
