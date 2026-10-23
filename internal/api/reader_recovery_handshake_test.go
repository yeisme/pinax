package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/pkg/localsession"
)

func TestReaderSessionRecoveryHandshake(t *testing.T) {
	for _, mode := range []string{"approved", "wrong-binding", "eof", "cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			_, root, g := recoveryStoreFixture(t)
			before := snapshotFileTree(t, root)
			defer func() {
				if !reflect.DeepEqual(before, snapshotFileTree(t, root)) {
					t.Fatal("handshake wrote vault")
				}
			}()
			sessions, err := NewReaderSessionStore()
			if err != nil {
				t.Fatal(err)
			}
			if err := sessions.Create(ReaderSession{SessionID: "handshake", Nonce: g.SessionNonce, BindingID: g.BindingID}, "handshake-bearer"); err != nil {
				t.Fatal(err)
			}
			input, hostInput := io.Pipe()
			defer func() { _ = input.Close() }()
			defer func() { _ = hostInput.Close() }()
			hostOutput, output := io.Pipe()
			defer func() { _ = hostOutput.Close() }()
			defer func() { _ = output.Close() }()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "deadline" {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 100*time.Millisecond)
				defer stop()
			}
			done := make(chan error, 1)
			go func() {
				done <- prepareSessionRecovery(ctx, ServeSupervisedSessionOptions{Stdin: input, Stdout: output}, sessions, "handshake", root)
			}()
			var c localsession.RecoveryChallenge
			if err := json.NewDecoder(hostOutput).Decode(&c); err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(c)
			binding, err := localsession.DecodeRecoveryChallenge(encoded, g.SessionNonce, g.BindingID)
			if err != nil {
				t.Fatal(err)
			}
			g.VaultIdentityDigest = binding.VaultIdentityDigest
			switch mode {
			case "cancel":
				cancel()
			case "eof":
				_ = hostInput.Close()
			case "deadline": // Leave stdin open; the parent deadline must stop admission.
			default:
				if mode == "wrong-binding" {
					g.BindingID = "foreign"
				}
				raw, _ := json.Marshal(g)
				if _, err := hostInput.Write(append(raw, '\n')); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-done:
				if mode == "approved" {
					if err != nil {
						t.Fatal(err)
					}
					view, err := sessions.LookupRecovery(t.Context(), "handshake-bearer", g.OperationID)
					if err != nil || view.OperationID != g.OperationID {
						t.Fatal("installed grant unavailable", err)
					}
				} else if err == nil {
					t.Fatal("bad handshake accepted")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("handshake did not settle")
			}
		})
	}
}

func TestReaderSessionRecoveryFrameBound(t *testing.T) {
	for _, input := range []string{"", "{}", strings.Repeat("x", localsession.MaxRecoveryGrantBytes+1) + "\n"} {
		if _, err := readRecoveryLine(strings.NewReader(input)); err == nil {
			t.Fatal("unbounded or unterminated grant accepted")
		}
	}
	source := bytes.NewBufferString("{}\nnext-frame\n")
	got, err := readRecoveryLine(source)
	if err != nil || string(got) != "{}" || source.String() != "next-frame\n" {
		t.Fatal("consumed liveness bytes")
	}
}
