package app

import (
	"fmt"
	"strings"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	pinaxremote "github.com/yeisme/pinax/internal/remote"
)

// DaemonCredentialReadiness reports whether the daemon may perform remote writes
// (push/pull that can commit a revision) given the S3 credential mode, the
// configured unlock source and whether the repository-encrypted envelope is
// present. The daemon must never open an interactive TTY prompt; in
// repository-encrypted mode it may only use a non-interactive source
// (keychain/file/env), and when that source is missing it must enter a degraded
// state and forbid remote writes (pinax-passphrase-s3-bootstrap task 4.4).
type DaemonCredentialReadiness struct {
	Allowed bool
	Reason  string // non-sensitive, redacted
	Mode    string // device-profile | repository-encrypted
	Source  string // unlock source descriptor when applicable
}

// AssessDaemonCredentialReadiness is the gate function. credentialMode is the
// normalized S3 credential_mode; unlockSource is the source the daemon would use
// (nil means none configured); envelopePresent reports whether the project-
// secrets envelope exists at the vault root.
func AssessDaemonCredentialReadiness(credentialMode string, unlockSource projectsecrets.UnlockSource, envelopePresent bool) DaemonCredentialReadiness {
	if credentialMode == "" {
		credentialMode = pinaxremote.CredentialModeDeviceProfile
	}
	if credentialMode == pinaxremote.CredentialModeDeviceProfile {
		// Legacy path: credentials resolve from the device-local profile / AWS
		// default chain; the daemon does not need a project unlock source.
		return DaemonCredentialReadiness{Allowed: true, Reason: "device-profile credentials resolve without a project unlock source", Mode: credentialMode}
	}
	if credentialMode != pinaxremote.CredentialModeRepositoryEncrypted {
		return DaemonCredentialReadiness{Allowed: false, Reason: fmt.Sprintf("unsupported credential_mode %q", credentialMode), Mode: credentialMode}
	}
	if !envelopePresent {
		return DaemonCredentialReadiness{Allowed: false, Reason: "repository-encrypted envelope missing; run `pinax sync repo credential init`", Mode: credentialMode}
	}
	if unlockSource == nil {
		return DaemonCredentialReadiness{Allowed: false, Reason: "repository-encrypted mode requires a non-interactive unlock source (keychain/file/env); prompt is rejected for the daemon", Mode: credentialMode}
	}
	desc := unlockSource.Descriptor()
	if !isDaemonSafeUnlockDescriptor(desc) {
		return DaemonCredentialReadiness{Allowed: false, Reason: fmt.Sprintf("unlock source %q is interactive; the daemon may not prompt", desc), Mode: credentialMode, Source: desc}
	}
	return DaemonCredentialReadiness{Allowed: true, Reason: "repository-encrypted envelope present with non-interactive unlock source", Mode: credentialMode, Source: desc}
}

// DaemonCredentialGate adapts AssessDaemonCredentialReadiness to the
// syncdaemon.CredentialGate interface so the daemon loop can forbid remote
// writes when the repository-encrypted credential is unavailable or would
// require an interactive prompt.
type DaemonCredentialGate struct {
	Mode            string
	UnlockSource    projectsecrets.UnlockSource
	EnvelopePresent bool
}

// RemoteWriteAllowed implements syncdaemon.CredentialGate.
func (g DaemonCredentialGate) RemoteWriteAllowed() (bool, string) {
	r := AssessDaemonCredentialReadiness(g.Mode, g.UnlockSource, g.EnvelopePresent)
	return r.Allowed, r.Reason
}

// isDaemonSafeUnlockDescriptor returns true for non-interactive unlock sources.
// "prompt" reads from the controlling TTY and would block a daemon; "static" is
// a test/CI source and is treated as daemon-safe only because it carries no
// interactive dependency (production daemons use keychain/file/env).
func isDaemonSafeUnlockDescriptor(desc string) bool {
	desc = strings.TrimSpace(desc)
	switch desc {
	case "keychain", "file", "env", "static":
		return true
	default:
		return false
	}
}
