package domain

import (
	"strings"
	"testing"
	"time"
)

func trustTestNote(frontmatter string) []byte {
	if frontmatter == "" {
		return []byte("# Body only\n\nplain markdown\n")
	}
	return []byte("---\n" + frontmatter + "\n---\n\n# Body\n\nplain body\n")
}

func TestParseTrustSignalsZeroWhenAbsent(t *testing.T) {
	signals, err := ParseTrustSignals(trustTestNote("schema_version: pinax.note.v1\ntitle: Legacy\n"))
	if err != nil {
		t.Fatalf("legacy note should parse: %v", err)
	}
	if !signals.IsZero() {
		t.Fatalf("legacy note signals = %#v, want zero", signals)
	}
	if signals, err := ParseTrustSignals(trustTestNote("")); err != nil || !signals.IsZero() {
		t.Fatalf("body-only note should parse to zero signals: %#v err=%v", signals, err)
	}
}

func TestParseTrustSignalsFullShape(t *testing.T) {
	content := trustTestNote(`schema_version: pinax.note.v1
title: Auth Design
generated:
  by: agent:pinax/0.9.0
  at: 2026-09-06T08:00:00+00:00
verified:
  - by: human:ye
    at: 2026-09-06T10:30:00+00:00
  - by: agent:pinax/0.9.0
    at: 2026-09-06T09:00:00+00:00
stale_after: 2026-12-01T00:00:00+00:00`)
	signals, err := ParseTrustSignals(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !signals.HasGenerated || signals.Generated.By != "agent:pinax/0.9.0" {
		t.Fatalf("generated = %#v", signals.Generated)
	}
	if len(signals.Verified) != 2 {
		t.Fatalf("verified len = %d, want 2", len(signals.Verified))
	}
	if signals.StaleAfter != "2026-12-01T00:00:00+00:00" {
		t.Fatalf("stale_after = %q", signals.StaleAfter)
	}
}

func TestParseTrustSignalsBareMappingVerified(t *testing.T) {
	content := trustTestNote(`verified:
  by: human:ye
  at: 2026-09-06T10:30:00+00:00`)
	signals, err := ParseTrustSignals(content)
	if err != nil {
		t.Fatalf("parse bare mapping: %v", err)
	}
	if len(signals.Verified) != 1 || signals.Verified[0].By != "human:ye" {
		t.Fatalf("bare mapping verified = %#v", signals.Verified)
	}
	if TrustTierOf(&signals) != TrustTierHuman {
		t.Fatalf("bare mapping tier = %q", TrustTierOf(&signals))
	}
}

func TestParseTrustSignalsInvalidTimestampsFailClosed(t *testing.T) {
	cases := map[string]string{
		"verified[].at": "verified:\n  - by: human:ye\n    at: 2026-13-99T00:00:00+00:00",
		"stale_after":   "stale_after: 2026-12-01",
		"date-only":     "stale_after: not-a-time",
	}
	for name, frontmatter := range cases {
		signals, err := ParseTrustSignals(trustTestNote(frontmatter))
		if err == nil {
			t.Fatalf("%s: expected fail-closed error, got %#v", name, signals)
		}
		fieldErr, ok := err.(*TrustFieldError)
		if !ok {
			t.Fatalf("%s: error type = %T", name, err)
		}
		if !strings.Contains(fieldErr.Error(), fieldErr.Field) {
			t.Fatalf("%s: error should reference field %q: %v", name, fieldErr.Field, err)
		}
	}
}

func TestTrustTierDerivation(t *testing.T) {
	if tier := TrustTierOf(nil); tier != TrustTierUnverified {
		t.Fatalf("nil signals tier = %q", tier)
	}
	empty := TrustSignals{}
	if tier := TrustTierOf(&empty); tier != TrustTierUnverified {
		t.Fatalf("empty verified tier = %q", tier)
	}
	machine := TrustSignals{Verified: []TrustActorEvent{{By: "agent:pinax/0.9.0", At: "2026-09-06T08:00:00+00:00"}, {By: "svc:ci", At: "2026-09-06T08:00:00+00:00"}}}
	if tier := TrustTierOf(&machine); tier != TrustTierMachine {
		t.Fatalf("unknown actor prefix should be machine, got %q", tier)
	}
	human := TrustSignals{Verified: []TrustActorEvent{{By: "agent:pinax/0.9.0", At: "2026-09-06T08:00:00+00:00"}, {By: "human:ye", At: "2026-09-06T09:00:00+00:00"}}}
	if tier := TrustTierOf(&human); tier != TrustTierHuman {
		t.Fatalf("any human event tier = %q", tier)
	}
}

func TestFreshnessDerivation(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	if fresh := FreshnessOf(nil, now); fresh != FreshnessFresh {
		t.Fatalf("nil signals freshness = %q", fresh)
	}
	pending := TrustSignals{StaleAfter: "2026-12-01T00:00:00+00:00"}
	if fresh := FreshnessOf(&pending, now); fresh != FreshnessFresh {
		t.Fatalf("future stale_after freshness = %q", fresh)
	}
	expired := TrustSignals{StaleAfter: "2026-09-06T00:00:00+00:00"}
	if fresh := FreshnessOf(&expired, now); fresh != FreshnessStale {
		t.Fatalf("now >= stale_after freshness = %q", fresh)
	}
	boundary := TrustSignals{StaleAfter: "2026-09-06T12:00:00+00:00"}
	if fresh := FreshnessOf(&boundary, now); fresh != FreshnessStale {
		t.Fatalf("now == stale_after must be stale, got %q", fresh)
	}
}

func TestVerifiedEventSameDayIdempotency(t *testing.T) {
	signals := TrustSignals{Verified: []TrustActorEvent{{By: "human:ye", At: "2026-09-06T23:30:00+00:00"}}}
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	if event, ok := signals.FindVerifiedEventSameDay("human:ye", now); !ok || event.At != "2026-09-06T23:30:00+00:00" {
		t.Fatalf("same-day event = %#v ok=%v", event, ok)
	}
	if _, ok := signals.FindVerifiedEventSameDay("human:other", now); ok {
		t.Fatalf("different actor must not match")
	}
	nextDay := time.Date(2026, 9, 7, 0, 30, 0, 0, time.UTC)
	if _, ok := signals.FindVerifiedEventSameDay("human:ye", nextDay); ok {
		t.Fatalf("different UTC day must not match")
	}
}

func TestAppendTrustVerifiedEventRoundTrip(t *testing.T) {
	base := trustTestNote(`schema_version: pinax.note.v1
note_id: note_abc
title: Keep Me
tags: [auth, security]
# provider comment stays
`)
	updated, err := AppendTrustVerifiedEvent(base, TrustActorEvent{By: "human:ye", At: "2026-09-06T10:30:00+00:00"})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	text := string(updated)
	for _, want := range []string{
		"title: Keep Me",
		"# provider comment stays",
		"verified:",
		"- by: human:ye",
		"at: \"2026-09-06T10:30:00+00:00\"",
		"# Body",
		"plain body",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated content missing %q:\n%s", want, text)
		}
	}
	signals, err := ParseTrustSignals(updated)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(signals.Verified) != 1 || signals.Verified[0].By != "human:ye" {
		t.Fatalf("re-parsed verified = %#v", signals.Verified)
	}
	// 幂等追加第二条（不同 actor）后仍能 round-trip。
	updated2, err := AppendTrustVerifiedEvent(updated, TrustActorEvent{By: "agent:pinax/0.9.0", At: "2026-09-06T11:00:00+00:00"})
	if err != nil {
		t.Fatalf("append second: %v", err)
	}
	signals2, err := ParseTrustSignals(updated2)
	if err != nil {
		t.Fatalf("re-parse second: %v", err)
	}
	if len(signals2.Verified) != 2 {
		t.Fatalf("verified len = %d, want 2", len(signals2.Verified))
	}
	if TrustTierOf(&signals2) != TrustTierHuman {
		t.Fatalf("tier after second append = %q", TrustTierOf(&signals2))
	}
}

func TestAppendTrustVerifiedEventBareMappingBecomesList(t *testing.T) {
	base := trustTestNote("verified:\n  by: human:ye\n  at: 2026-09-06T10:30:00+00:00\n")
	updated, err := AppendTrustVerifiedEvent(base, TrustActorEvent{By: "human:ye", At: "2026-09-06T11:30:00+00:00"})
	if err != nil {
		t.Fatalf("append onto bare mapping: %v", err)
	}
	signals, err := ParseTrustSignals(updated)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(signals.Verified) != 2 {
		t.Fatalf("bare mapping should become two-entry list, got %#v", signals.Verified)
	}
}

func TestAppendTrustVerifiedEventValidation(t *testing.T) {
	if _, err := AppendTrustVerifiedEvent(trustTestNote(""), TrustActorEvent{By: "", At: "2026-09-06T10:30:00+00:00"}); err == nil {
		t.Fatalf("empty actor must fail")
	}
	if _, err := AppendTrustVerifiedEvent(trustTestNote(""), TrustActorEvent{By: "human:ye", At: "not-a-time"}); err == nil {
		t.Fatalf("invalid at must fail")
	}
}

func TestEnsureTrustGeneratedFillsOnlyMissing(t *testing.T) {
	base := trustTestNote("schema_version: pinax.note.v1\n")
	updated, changed, err := EnsureTrustGenerated(base, "agent:pinax/0.9.0", "2026-09-06T08:00:00+00:00", "2026-12-01T00:00:00+00:00")
	if err != nil || !changed {
		t.Fatalf("fill generated: changed=%v err=%v", changed, err)
	}
	signals, err := ParseTrustSignals(updated)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if !signals.HasGenerated || signals.Generated.By != "agent:pinax/0.9.0" || signals.StaleAfter == "" {
		t.Fatalf("signals = %#v", signals)
	}
	// 已存在时不改写。
	again, changed, err := EnsureTrustGenerated(updated, "agent:other/1.0.0", "2027-01-01T00:00:00+00:00", "2027-06-01T00:00:00+00:00")
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if changed || string(again) != string(updated) {
		t.Fatalf("existing fields must stay unchanged:\n%s", again)
	}
	if updated, changed, err := EnsureTrustGenerated(base, "", "", ""); err != nil || changed || string(updated) != string(base) {
		t.Fatalf("no-op ensure changed=%v err=%v", changed, err)
	}
}

func TestLatestVerifiedHelpers(t *testing.T) {
	signals := TrustSignals{Verified: []TrustActorEvent{
		{By: "human:ye", At: "2026-09-06T10:30:00+00:00"},
		{By: "human:ye", At: "2026-09-01T08:00:00+00:00"},
		{By: "agent:pinax/0.9.0", At: "2026-09-08T08:00:00+00:00"},
	}}
	if got := signals.LatestVerifiedAt(); got != "2026-09-08T08:00:00+00:00" {
		t.Fatalf("LatestVerifiedAt = %q", got)
	}
	event, ok := signals.LatestHumanVerified()
	if !ok || event.At != "2026-09-06T10:30:00+00:00" {
		t.Fatalf("LatestHumanVerified = %#v ok=%v", event, ok)
	}
}
