package store

import "github.com/yeisme/credentialctl/pkg/credentials"

// KeychainBackend targets the macOS Keychain through the `security` CLI so the
// build stays cgo-free. On non-darwin hosts (or where `security` is absent) it
// reports unavailable and the file backend serves as the safe fallback. The
// full `security add-generic-password` integration is left to the owning
// subproject's darwin hardening task; this stub keeps the selection contract
// honest on Linux without introducing cgo.
type KeychainBackend struct{}

func (KeychainBackend) Name() string { return "keychain" }

func (KeychainBackend) Available() bool {
	// 在 Get/Put/Delete 完整实现并通过 darwin 安全测试前，不能把 stub 选为可用 backend。
	return false
}

func (KeychainBackend) Get(string) ([]byte, error) { return nil, credentials.ErrBackendUnavailable }
func (KeychainBackend) Put(string, []byte) error   { return credentials.ErrBackendUnavailable }
func (KeychainBackend) Delete(string) error        { return nil }
