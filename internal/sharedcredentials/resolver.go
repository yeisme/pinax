package sharedcredentials

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const scheme = "yeisme-credential"

type Ref struct {
	Provider string
	Account  string
}

func (ref Ref) String() string {
	return fmt.Sprintf("%s://%s/%s", scheme, ref.Provider, ref.Account)
}

func (ref Ref) key() string {
	return filepath.Join(ref.Provider, ref.Account)
}

func ParseRef(value string) (Ref, error) {
	prefix := scheme + "://"
	if !strings.HasPrefix(value, prefix) {
		return Ref{}, fmt.Errorf("invalid credential ref %q", value)
	}
	parts := strings.SplitN(strings.TrimPrefix(value, prefix), "/", 2)
	if len(parts) != 2 || !validComponent(parts[0]) || !validComponent(parts[1]) {
		return Ref{}, fmt.Errorf("invalid credential ref %q", value)
	}
	return Ref{Provider: parts[0], Account: parts[1]}, nil
}

func validComponent(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\ \t\n")
}

type Resolution struct {
	Ref     Ref
	Backend string
	Secret  []byte
}

type Resolver interface {
	Resolve(consumer, capability string, ref Ref) (Resolution, error)
}

type fileResolver struct {
	base string
}

func NewResolver() (Resolver, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return fileResolver{base: filepath.Join(configDir, "yeisme", "credentialctl", "secrets")}, nil
}

func (resolver fileResolver) Resolve(consumer, capability string, ref Ref) (Resolution, error) {
	if consumer != "pinax" || capability != "embedding" {
		return Resolution{}, fmt.Errorf("consumer/capability not allowed for shared credential")
	}
	secret, err := os.ReadFile(filepath.Join(resolver.base, ref.key()))
	if err != nil {
		return Resolution{}, err
	}
	if len(secret) == 0 {
		return Resolution{}, fmt.Errorf("shared credential is empty")
	}
	return Resolution{Ref: ref, Backend: "file", Secret: secret}, nil
}
