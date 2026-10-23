package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/yeisme/pinax/pkg/localsession"
)

const recoveryHandshakeDeadline = 10 * time.Second

// prepareSessionRecovery consumes exactly one additional private stdin line.
// No listener or ready event exists until the host instruction is installed.
func prepareSessionRecovery(ctx context.Context, options ServeSupervisedSessionOptions, sessions *ReaderSessionStore, sessionID, root string) error {
	binding, err := sessions.PrepareRecovery(sessionID, root)
	if err != nil {
		return err
	}
	challenge := localsession.RecoveryChallenge{SchemaVersion: localsession.RecoveryChallengeVersion, Type: "recovery_challenge", SessionNonce: binding.SessionNonce, BindingID: binding.BindingID, VaultIdentityDigest: binding.VaultIdentityDigest}
	if err := json.NewEncoder(options.Stdout).Encode(challenge); err != nil {
		return err
	}
	deadline, cancel := context.WithTimeout(ctx, recoveryHandshakeDeadline)
	defer cancel()
	type result struct {
		frame []byte
		err   error
	}
	done := make(chan result, 1)
	go func() { frame, err := readRecoveryLine(options.Stdin); done <- result{frame, err} }()
	select {
	case <-deadline.Done():
		return errors.New("recovery_handshake_failed")
	case r := <-done:
		if r.err != nil || deadline.Err() != nil {
			return errors.New("recovery_handshake_failed")
		}
		return sessions.AuthorizeRecovery(sessionID, r.frame)
	}
}

// Do not buffer past the newline: the remaining stream is the existing
// liveness channel, whose non-whitespace input is still forbidden.
func readRecoveryLine(reader io.Reader) ([]byte, error) {
	frame := make([]byte, 0, 1024)
	one := make([]byte, 1)
	for len(frame) <= localsession.MaxRecoveryGrantBytes {
		n, err := reader.Read(one)
		if n > 0 {
			if one[0] == '\n' {
				return frame, nil
			}
			frame = append(frame, one[0])
		}
		if err != nil {
			return nil, errors.New("recovery_handshake_failed")
		}
	}
	return nil, errors.New("recovery_handshake_failed")
}
