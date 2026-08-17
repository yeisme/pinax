// Package projectcrypto provides the versioned authenticated encryption
// primitives for repository-scoped project secrets.
//
// The envelope uses a random 256-bit data key (DEK) per envelope. The DEK is
// wrapped by a key-encryption key (KEK) derived from the unlock passphrase
// with Argon2id. Entries are authenticated-encrypted per-name with
// XChaCha20-Poly1305, binding schema/project/repository/entry/identity/kind/
// format/version as associated data so a ciphertext copied to a different
// project, repository, name, kind, format or version fails closed.
//
// All security-relevant failures (wrong passphrase, tamper, swap, cross-repo
// copy, replay-attempt via tag mismatch) return the unified sentinel
// ErrUnlockFailed. Detailed reasons exist only for non-sensitive internal test
// assertions and are never returned to callers, so a brute-force attacker
// cannot distinguish "wrong passphrase" from "tampered ciphertext".
//
// The package depends only on golang.org/x/crypto pure-Go implementations;
// the build remains CGO-free (CGO_ENABLED=0).
package projectcrypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	keyLen    = 32                          // 256-bit DEK and KEK
	nonceSize = chacha20poly1305.NonceSizeX // 24 bytes for XChaCha20-Poly1305
	saltLen   = 16
	tagLen    = chacha20poly1305.Overhead // 16 bytes
)

// ErrUnlockFailed is the unified fail-closed error for every authentication
// failure. Callers MUST treat any non-nil error from Wrap/Unwrap/Encrypt/
// Decrypt as "unlock failed" and must not branch on the message.
var ErrUnlockFailed = errors.New("project secret unlock failed")

// Params are the Argon2id parameters used to derive the KEK from a passphrase.
// MinimumParams returns the library-fixed floor; envelope-stored parameters may
// only be at or above this floor (raise, never lower) so a repository cannot
// weaken the KDF below the safe default.
type Params struct {
	Time    uint32 // iterations
	Memory  uint32 // KiB
	Threads uint8  // parallelism
	KeyLen  uint32 // derived key length (always keyLen)
}

// MinimumParams returns the minimum acceptable Argon2id parameters. These
// match the OWASP-recommended floor (t=3, m=64 MiB, p=4) for passphrase KDF.
func MinimumParams() Params {
	return Params{Time: 3, Memory: 64 * 1024, Threads: 4, KeyLen: keyLen}
}

// Validate returns an error if the parameters are below the library minimum.
func (p Params) Validate() error {
	min := MinimumParams()
	if p.Time < min.Time {
		return fmt.Errorf("projectcrypto: time %d below minimum %d", p.Time, min.Time)
	}
	if p.Memory < min.Memory {
		return fmt.Errorf("projectcrypto: memory %d below minimum %d", p.Memory, min.Memory)
	}
	if p.Threads < 1 {
		return fmt.Errorf("projectcrypto: threads %d below minimum 1", p.Threads)
	}
	if p.KeyLen != keyLen {
		return fmt.Errorf("projectcrypto: key length must be %d", keyLen)
	}
	return nil
}

// AtOrAbove returns nil only when p meets or exceeds floor in every dimension
// that affects cost (Time, Memory). Threads and KeyLen are structural.
func (p Params) AtOrAbove(floor Params) bool {
	return p.Time >= floor.Time && p.Memory >= floor.Memory && p.KeyLen == keyLen && p.Threads >= 1
}

// Binding is the associated data bound to every encryption operation. Every
// field participates in the AAD so a ciphertext cannot be replayed against a
// different project, repository, entry name, identity, kind, format or version.
type Binding struct {
	Schema     string
	Project    string
	Repository string
	EntryName  string
	Identity   string
	Kind       string
	Format     string
	Version    string
}

// associatedData builds the deterministic AAD byte string. The length-prefixed
// encoding prevents ambiguous concatenation across fields with shared content.
// The schema marker byte prefixes the record so future versions can evolve the
// layout without ambiguity.
func (b Binding) associatedData() []byte {
	fields := []string{b.Schema, b.Project, b.Repository, b.EntryName, b.Identity, b.Kind, b.Format, b.Version}
	var buf []byte
	buf = append(buf, 0x01) // AAD layout version
	var sz [4]byte
	for _, f := range fields {
		binary.BigEndian.PutUint32(sz[:], uint32(len(f)))
		buf = append(buf, sz[:]...)
		buf = append(buf, f...)
	}
	return buf
}

// Ciphertext is one authenticated entry blob.
type Ciphertext struct {
	Nonce      []byte `json:"nonce" yaml:"nonce"`           // hex/base64 in storage layer
	Ciphertext []byte `json:"ciphertext" yaml:"ciphertext"` // encrypted payload + 16-byte tag
}

// MustRandomKey returns a fresh 256-bit random key. It panics if the system
// RNG fails (a healthy host never reaches that path). Production callers should
// use RandomKey and surface the error.
func MustRandomKey() []byte {
	k, err := RandomKey()
	if err != nil {
		panic(fmt.Sprintf("projectcrypto: read random key: %v", err))
	}
	return k
}

// RandomKey returns a fresh 256-bit random key from crypto/rand.
func RandomKey() ([]byte, error) {
	k := make([]byte, keyLen)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	return k, nil
}

// MustRandomSalt returns a fresh 128-bit salt for the KDF.
func MustRandomSalt() []byte {
	s := make([]byte, saltLen)
	if _, err := rand.Read(s); err != nil {
		panic(fmt.Sprintf("projectcrypto: read random salt: %v", err))
	}
	return s
}

// deriveKEK derives the 256-bit KEK from a passphrase and salt using Argon2id.
func deriveKEK(passphrase, salt []byte, p Params) []byte {
	return argon2.IDKey(passphrase, salt, p.Time, p.Memory, p.Threads, p.KeyLen)
}

// WrapKey generates a fresh random DEK and wraps it under the passphrase-derived
// KEK, returning only the wrapped ciphertext. The plaintext DEK is never exposed
// by this function; callers recover it via UnwrapKey with the passphrase. The
// binding is part of the AAD so a wrapped key copied to another project or
// repository fails closed on unwrap.
func WrapKey(passphrase, salt []byte, p Params, b Binding) (Ciphertext, error) {
	if err := p.Validate(); err != nil {
		return Ciphertext{}, err
	}
	dek, err := RandomKey()
	if err != nil {
		return Ciphertext{}, err
	}
	defer wipe(dek)
	return RewrapKey(passphrase, salt, p, b, dek)
}

// RewrapKey wraps an existing plaintext DEK under the passphrase-derived KEK.
// It is used by rekey to rewrap the unchanged DEK with a new passphrase: entry
// plaintexts and domain key identities remain the same because the DEK does not
// change.
func RewrapKey(passphrase, salt []byte, p Params, b Binding, dek []byte) (Ciphertext, error) {
	if err := p.Validate(); err != nil {
		return Ciphertext{}, err
	}
	if len(dek) != keyLen {
		return Ciphertext{}, fmt.Errorf("projectcrypto: dek length must be %d", keyLen)
	}
	kek := deriveKEK(passphrase, salt, p)
	defer wipe(kek)
	aead, err := chacha20poly1305.NewX(kek)
	if err != nil {
		return Ciphertext{}, err
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return Ciphertext{}, err
	}
	// Bind envelope-level identity (schema/project/repository) into the wrapped
	// DEK so the DEK itself cannot be transplanted to another envelope.
	ct := aead.Seal(nil, nonce, dek, b.associatedData())
	return Ciphertext{Nonce: nonce, Ciphertext: ct}, nil
}

// UnwrapKey decrypts the wrapped DEK with the passphrase-derived KEK.
func UnwrapKey(passphrase, salt []byte, p Params, b Binding, wrapped Ciphertext) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if len(wrapped.Nonce) != nonceSize || len(wrapped.Ciphertext) < tagLen {
		return nil, ErrUnlockFailed
	}
	kek := deriveKEK(passphrase, salt, p)
	defer wipe(kek)
	aead, err := chacha20poly1305.NewX(kek)
	if err != nil {
		return nil, ErrUnlockFailed
	}
	dek, err := aead.Open(nil, wrapped.Nonce, wrapped.Ciphertext, b.associatedData())
	if err != nil {
		return nil, ErrUnlockFailed
	}
	return dek, nil
}

// EncryptEntry authenticates and encrypts a single entry payload under dek.
func EncryptEntry(dek []byte, b Binding, plaintext []byte) (Ciphertext, error) {
	if len(dek) != keyLen {
		return Ciphertext{}, fmt.Errorf("projectcrypto: dek length must be %d", keyLen)
	}
	aead, err := chacha20poly1305.NewX(dek)
	if err != nil {
		return Ciphertext{}, err
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return Ciphertext{}, err
	}
	ct := aead.Seal(nil, nonce, plaintext, b.associatedData())
	return Ciphertext{Nonce: nonce, Ciphertext: ct}, nil
}

// DecryptEntry verifies and decrypts a single entry payload under dek.
func DecryptEntry(dek []byte, b Binding, ct Ciphertext) ([]byte, error) {
	if len(dek) != keyLen {
		return nil, ErrUnlockFailed
	}
	if len(ct.Nonce) != nonceSize || len(ct.Ciphertext) < tagLen {
		return nil, ErrUnlockFailed
	}
	aead, err := chacha20poly1305.NewX(dek)
	if err != nil {
		return nil, ErrUnlockFailed
	}
	pt, err := aead.Open(nil, ct.Nonce, ct.Ciphertext, b.associatedData())
	if err != nil {
		return nil, ErrUnlockFailed
	}
	return pt, nil
}

// ConstantTimeEqual compares two byte slices in constant time.
func ConstantTimeEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// Wipe best-effort zeroes a key slice. Go does not guarantee the compiler keeps
// this write, but it raises the bar for scavenging freed key material.
func Wipe(b []byte) { wipe(b) }

// wipe best-effort zeroes a key slice. Go does not guarantee the compiler keeps
// this write, but it raises the bar for scavenging freed key material.
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
