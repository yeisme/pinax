package app

import (
	"errors"
	"fmt"

	"github.com/yeisme/pinax/internal/domain"
	pinaxremote "github.com/yeisme/pinax/internal/remote"
)

// SupportedSyncCapabilities is the closed set of repository sync capabilities
// this binary understands (pinax-passphrase-s3-bootstrap task 6.8). A declaration
// that requires a capability outside this set fails closed so a partial/old
// binary cannot silently produce an incomplete backup.
var SupportedSyncCapabilities = map[string]bool{
	"repository-encrypted-s3-v1": true,
	"capsa-remote-commit-v1":     true,
	"pull-only-bootstrap-v1":     true,
}

// syncCapabilityGate validates that this binary supports every capability the
// repository declaration requires before any remote access. A missing
// declaration (device-profile runtime without a portable declaration) passes —
// the gate only constrains declarations that opt into capability requirements.
// A binary missing a required capability returns sync_capability_unsupported
// with remote_write=false and a concrete upgrade action.
func syncCapabilityGate(root string) error {
	cfg, err := pinaxremote.LoadSyncConfig(root)
	if err != nil {
		if errors.Is(err, pinaxremote.ErrSyncDeclarationMissing) {
			return nil
		}
		return err
	}
	for _, capability := range cfg.Requires.Capabilities {
		if !SupportedSyncCapabilities[capability] {
			return &domain.CommandError{
				Code:    "sync_capability_unsupported",
				Message: fmt.Sprintf("this pinax binary does not support required capability %q", capability),
				Hint:    "Upgrade pinax to a version that declares support for this repository sync capability before remote access.",
			}
		}
	}
	return nil
}
