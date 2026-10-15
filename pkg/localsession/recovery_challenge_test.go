package localsession_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yeisme/pinax/pkg/localsession"
)

func TestReaderSessionRecoveryChallengeClosed(t *testing.T) {
	c := localsession.RecoveryChallenge{SchemaVersion: localsession.RecoveryChallengeVersion, Type: "recovery_challenge", SessionNonce: strings.Repeat("n", 32), BindingID: "binding", VaultIdentityDigest: "sha256:" + strings.Repeat("a", 64)}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	valid := string(b)
	if _, err := localsession.DecodeRecoveryChallenge(b, c.SessionNonce, c.BindingID); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "null", "[]", valid + " {}", strings.Repeat("x", 1025),
		strings.Replace(valid, `"type":"recovery_challenge"`, `"type":"ready"`, 1),
		strings.Replace(valid, `"type":"recovery_challenge"`, `"type":null`, 1),
		strings.Replace(valid, `"type":"recovery_challenge"`, `"type":"recovery_challenge","type":"recovery_challenge"`, 1),
		strings.Replace(valid, `"type":"recovery_challenge"`, `"type":"recovery_challenge","Type":"recovery_challenge"`, 1),
		strings.Replace(valid, `"type":"recovery_challenge"`, `"type":"recovery_challenge","bearer":"secret"`, 1),
		strings.Replace(valid, `"binding_id":"binding"`, `"binding_id":"\ud800"`, 1),
		strings.Replace(valid, `"binding_id":"binding"`, `"binding_id":"other"`, 1),
		strings.Replace(valid, `"binding_id":"binding"`, `"binding_id":"`+string([]byte{0xff})+`"`, 1),
		strings.Replace(valid, `"binding_id":"binding",`, "", 1),
	} {
		if _, err := localsession.DecodeRecoveryChallenge([]byte(bad), c.SessionNonce, c.BindingID); err == nil || err.Error() != "recovery_challenge_invalid" {
			t.Fatal("bad challenge accepted or raw diagnostic leaked")
		}
	}
	if _, err := localsession.DecodeRecoveryChallenge(b, strings.Repeat("x", 32), c.BindingID); err == nil {
		t.Fatal("wrong process nonce")
	}
}
