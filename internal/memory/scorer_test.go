package memory

import (
	"strings"
	"testing"
	"time"
)

func TestScorerRanksFieldSignalsAboveBodyFallback(t *testing.T) {
	now := time.Date(2026, 6, 24, 8, 0, 0, 0, time.UTC)
	scorer := Scorer{Now: now}
	hits := scorer.Score(RecallFilter{Query: "release"}, []Candidate{
		{Record: scorerRecord("body", TypeFact, "pinax", "notes", "plain", "release appears only in the body", now.Add(-time.Hour))},
		{Record: scorerRecord("predicate", TypeFact, "pinax", "release_workflow", "tag push", "", now.Add(-2*time.Hour))},
	}, nil, 10)
	if len(hits) != 2 {
		t.Fatalf("hits = %#v", hits)
	}
	if hits[0].Record.ID != "predicate" {
		t.Fatalf("predicate hit should outrank body fallback: %#v", hits)
	}
	if !strings.Contains(hits[0].RecallReason, "field:predicate") || !strings.Contains(hits[1].RecallReason, "field:body") {
		t.Fatalf("field reasons missing: %#v", hits)
	}
	if hits[0].Signals.KeywordField != "predicate" || hits[1].Signals.KeywordField != "body" {
		t.Fatalf("keyword signals missing: %#v", hits)
	}
}

func TestScorerSourceConfidenceFreshnessAndTaskFitness(t *testing.T) {
	now := time.Date(2026, 6, 24, 8, 0, 0, 0, time.UTC)
	scorer := Scorer{Now: now}
	hits := scorer.Score(RecallFilter{Query: "release test"}, []Candidate{
		{Record: scorerRecord("file", TypeDecision, "pinax", "release_file", "workflow", "", now.Add(-2*time.Hour))},
		{Record: func() Record {
			r := scorerRecord("openspec", TypeDecision, "pinax", "release_decision", "workflow", "", now.Add(-24*time.Hour))
			r.SourceURI = "openspec/changes/release/design.md"
			r.Confidence = "high"
			return r
		}()},
		{Record: func() Record {
			r := scorerRecord("event", TypeEvent, "pinax", "release_event", "test run passed", "", now.Add(-time.Hour))
			r.SourceURI = "docs/releases.md"
			return r
		}()},
	}, nil, 10)
	if len(hits) != 3 {
		t.Fatalf("hits = %#v", hits)
	}
	if hits[0].Record.ID != "openspec" && hits[0].Record.ID != "event" {
		t.Fatalf("expected authority or freshness to lead: %#v", hits)
	}
	var openspecHit RecallHit
	for _, hit := range hits {
		if hit.Record.ID == "openspec" {
			openspecHit = hit
		}
	}
	if openspecHit.Signals.SourceKind != "openspec" || openspecHit.Signals.SourceAuthority <= 0 || openspecHit.Signals.Confidence <= 0 || !strings.Contains(openspecHit.RecallReason, "source:openspec") {
		t.Fatalf("source/confidence signals missing: %#v", openspecHit)
	}
}

func TestScorerTieBreakAndCollapse(t *testing.T) {
	now := time.Date(2026, 6, 24, 8, 0, 0, 0, time.UTC)
	scorer := Scorer{Now: now}
	old := scorerRecord("old", TypeFact, "pinax", "release", "old", "", now.Add(-3*time.Hour))
	newer := scorerRecord("newer", TypeFact, "pinax", "release", "new", "", now.Add(-2*time.Hour))
	superseding := scorerRecord("superseding", TypeFact, "pinax", "release", "newer", "", now.Add(-time.Hour))
	superseding.SupersedesID = "old"
	hits := scorer.Score(RecallFilter{Query: "release"}, []Candidate{{Record: old}, {Record: newer}, {Record: superseding}}, nil, 10)
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.Record.ID)
	}
	if strings.Join(ids, ",") != "superseding" {
		t.Fatalf("collapse should hide superseded and duplicate subject+predicate records, got %v", ids)
	}

	a := scorerRecord("a", TypeFact, "pinax", "cloud", "same", "", now.Add(-time.Hour))
	b := scorerRecord("b", TypeFact, "pinax", "kb", "same", "", now.Add(-time.Hour))
	hits = scorer.Score(RecallFilter{Query: "same"}, []Candidate{{Record: b}, {Record: a}}, nil, 10)
	if len(hits) != 2 || hits[0].Record.ID != "a" || hits[1].Record.ID != "b" {
		t.Fatalf("tie-break should fall back to id asc: %#v", hits)
	}
}

func scorerRecord(id, typ, subject, predicate, object, body string, created time.Time) Record {
	return Record{ID: id, Type: typ, Subject: subject, Predicate: predicate, Object: object, Body: body, Status: StatusConfirmed, Confidence: "confirmed", SourceURI: "file://memory.md", CreatedAt: created, UpdatedAt: created}
}
