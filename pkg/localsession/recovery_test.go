package localsession_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/api"
	"github.com/yeisme/pinax/internal/transportcatalog/schema"
	"github.com/yeisme/pinax/pkg/localsession"
)

func recoveryFixture() (localsession.RecoveryLookupGrant, localsession.RecoveryLookupContext, time.Time) {
	now := time.Date(2026, 10, 6, 16, 0, 0, 0, time.UTC)
	digest := "sha256:" + strings.Repeat("a", 64)
	grant := localsession.RecoveryLookupGrant{SchemaVersion: localsession.RecoveryGrantVersion, SessionNonce: strings.Repeat("n", 32), BindingID: "binding-test", VaultIdentityDigest: digest, OperationID: "op-test", CapabilityID: "writer.daily.edit", OperationBindingID: "rest.writer.daily.edit", PrincipalDigest: digest, ScopeDigest: digest, RequestDigest: digest, Action: "lookup", IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano)}
	return grant, localsession.RecoveryLookupContext{SessionNonce: grant.SessionNonce, BindingID: grant.BindingID, VaultIdentityDigest: digest}, now
}

func TestReaderSessionRecoveryLookupContract(t *testing.T) {
	g, expected, now := recoveryFixture()
	encode := func(v localsession.RecoveryLookupGrant) []byte {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, capability := range []string{"writer.apply", "writer.daily.apply", "writer.daily.edit", "writer.tags.apply"} {
		v := g
		v.CapabilityID = capability
		v.OperationBindingID = "rest." + capability
		got, err := localsession.DecodeRecoveryLookupGrant(encode(v), expected, now)
		if err != nil || got != v {
			t.Fatal("valid grant rejected", err)
		}
	}
	cases := map[string]func(*localsession.RecoveryLookupGrant){
		"version":           func(g *localsession.RecoveryLookupGrant) { g.SchemaVersion = "future" },
		"nonce":             func(g *localsession.RecoveryLookupGrant) { g.SessionNonce = strings.Repeat("x", 32) },
		"binding":           func(g *localsession.RecoveryLookupGrant) { g.BindingID = "other" },
		"vault":             func(g *localsession.RecoveryLookupGrant) { g.VaultIdentityDigest = "sha256:" + strings.Repeat("b", 64) },
		"operation":         func(g *localsession.RecoveryLookupGrant) { g.OperationID = "../op" },
		"empty-operation":   func(g *localsession.RecoveryLookupGrant) { g.OperationID = "" },
		"principal":         func(g *localsession.RecoveryLookupGrant) { g.PrincipalDigest = "invalid" },
		"scope":             func(g *localsession.RecoveryLookupGrant) { g.ScopeDigest = "sha256:" + strings.Repeat("A", 64) },
		"request":           func(g *localsession.RecoveryLookupGrant) { g.RequestDigest = "" },
		"capability":        func(g *localsession.RecoveryLookupGrant) { g.CapabilityID = "inbox.capture" },
		"child":             func(g *localsession.RecoveryLookupGrant) { g.CapabilityID = "writer.tags.item" },
		"operation-binding": func(g *localsession.RecoveryLookupGrant) { g.OperationBindingID = "rest.writer.apply" },
		"apply":             func(g *localsession.RecoveryLookupGrant) { g.Action = "apply" },
		"reconcile":         func(g *localsession.RecoveryLookupGrant) { g.Action = "reconcile" },
		"future":            func(g *localsession.RecoveryLookupGrant) { g.IssuedAt = now.Add(time.Second).Format(time.RFC3339Nano) },
		"expired":           func(g *localsession.RecoveryLookupGrant) { g.ExpiresAt = now.Format(time.RFC3339Nano) },
		"lifetime": func(g *localsession.RecoveryLookupGrant) {
			g.ExpiresAt = now.Add(5*time.Minute + time.Nanosecond).Format(time.RFC3339Nano)
		},
		"bad-time":    func(g *localsession.RecoveryLookupGrant) { g.IssuedAt = "today" },
		"offset-time": func(g *localsession.RecoveryLookupGrant) { g.IssuedAt = "2026-10-06T16:00:00+00:00" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			v := g
			mutate(&v)
			if _, err := localsession.DecodeRecoveryLookupGrant(encode(v), expected, now); err == nil || err.Error() != "recovery_grant_invalid" {
				t.Fatal("invalid grant accepted or diagnostics leaked")
			}
		})
	}
	if g.ValidFor(expected, time.Time{}) || g.ValidFor(expected, now.Add(time.Minute)) || g.ValidFor(expected, now.Add(-time.Nanosecond)) {
		t.Fatal("temporal boundary ignored")
	}
	g.ExpiresAt = now.Add(5 * time.Minute).Format(time.RFC3339Nano)
	if !g.ValidFor(expected, now) {
		t.Fatal("maximum lifetime rejected")
	}
	for _, context := range []localsession.RecoveryLookupContext{{}, {SessionNonce: expected.SessionNonce, BindingID: expected.BindingID, VaultIdentityDigest: "sha256:" + strings.Repeat("c", 64)}} {
		if g.ValidFor(context, now) {
			t.Fatal("untrusted or missing context accepted")
		}
	}
}

func TestReaderSessionRecoveryLookupClosedFrame(t *testing.T) {
	g, expected, now := recoveryFixture()
	encoded, _ := json.Marshal(g)
	valid := string(encoded)
	cases := []string{
		"", "null", "[]", valid + " {}", strings.Repeat("x", localsession.MaxRecoveryGrantBytes+1),
		strings.Replace(valid, `"action":"lookup"`, `"action":"lookup","action":"lookup"`, 1),
		strings.Replace(valid, `"action":"lookup"`, `"action":"lookup","Action":"lookup"`, 1),
		strings.Replace(valid, `"action":"lookup"`, `"action":null`, 1),
		strings.Replace(valid, `"action":"lookup"`, `"action":{}`, 1),
		strings.Replace(valid, `"action":"lookup"`, `"action":true`, 1),
		strings.Replace(valid, `"action":"lookup",`, "", 1),
		strings.Replace(valid, `"action":"lookup"`, `"action":"lookup","bearer":"test-secret"`, 1),
		strings.Replace(valid, `"operation_id":"op-test"`, `"operation_id":"\ud800"`, 1),
		strings.Replace(valid, `"operation_id":"op-test"`, `"operation_id":"a\u0000b"`, 1),
		strings.Replace(valid, `"operation_id":"op-test"`, `"operation_id":"`+string([]byte{0xff})+`"`, 1),
	}
	for i, frame := range cases {
		if _, err := localsession.DecodeRecoveryLookupGrant([]byte(frame), expected, now); err == nil || err.Error() != "recovery_grant_invalid" {
			t.Fatalf("frame %d accepted or diagnostic leaked", i)
		}
	}
	// Independent contract only: adding it to legacy bootstrap remains forbidden.
	bootstrap := localsession.Bootstrap{SchemaVersion: localsession.BootstrapVersion, SessionNonce: g.SessionNonce, Bearer: strings.Repeat("b", 64), BindingID: g.BindingID, VaultRoot: "/fixture/vault", Scopes: []string{"reader.metadata"}, Readonly: true}
	body, err := json.Marshal(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.ReadSessionBootstrap(strings.NewReader(string(body) + "\n")); err != nil {
		t.Fatal("legacy positive control failed", err)
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	object["recovery_grant"] = g
	body, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.ValidatePayload(schema.Default(), schema.LocalSessionBootstrapV1Alpha1, object); err == nil {
		t.Fatal("legacy schema accepts recovery authority")
	}
	if _, err := api.ReadSessionBootstrap(strings.NewReader(string(body) + "\n")); err == nil {
		t.Fatal("legacy bootstrap accepts recovery authority")
	}
}
