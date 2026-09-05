package promptrepo

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var sha256DigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// TemplateAddress identifies a template projection inside a resolved solution.
// It deliberately uses a different grammar from a repository source URI.
type TemplateAddress struct {
	RepositoryID string `json:"repository_id" yaml:"repository_id"`
	PackageID    string `json:"package_id" yaml:"package_id"`
	SolutionID   string `json:"solution_id" yaml:"solution_id"`
	Version      string `json:"version" yaml:"version"`
	Kind         string `json:"kind" yaml:"kind"`
	Locale       string `json:"locale" yaml:"locale"`
	Role         string `json:"role,omitempty" yaml:"role,omitempty"`
	Path         string `json:"path,omitempty" yaml:"path,omitempty"`
	Selector     string `json:"selector,omitempty" yaml:"selector,omitempty"`
	Digest       string `json:"digest,omitempty" yaml:"digest,omitempty"`
	Snapshot     string `json:"snapshot,omitempty" yaml:"snapshot,omitempty"`
}

func (address TemplateAddress) Ref() Ref {
	return Ref{RepositoryID: address.RepositoryID, PackageID: address.PackageID, SolutionID: address.SolutionID, Version: address.Version, Locale: address.Locale}
}

func ParseTemplateAddress(raw string) (TemplateAddress, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return TemplateAddress{}, NewError(CodeInvalidRequest, "template address is invalid", false, err)
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return TemplateAddress{}, NewError(CodeInvalidRequest, "template address must not contain user info or a fragment", false, nil)
	}
	ref, err := ParseRef(raw)
	if err != nil {
		return TemplateAddress{}, err
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return TemplateAddress{}, NewError(CodeInvalidRequest, "template address query is invalid", false, err)
	}
	allowed := map[string]struct{}{"kind": {}, "locale": {}, "role": {}, "path": {}, "selector": {}, "digest": {}, "snapshot": {}}
	for key, entries := range values {
		if _, ok := allowed[key]; !ok || len(entries) != 1 {
			return TemplateAddress{}, NewError(CodeInvalidRequest, "template address has an unsupported or duplicate query field", false, nil)
		}
	}
	address := TemplateAddress{RepositoryID: ref.RepositoryID, PackageID: ref.PackageID, SolutionID: ref.SolutionID, Version: ref.Version, Kind: values.Get("kind"), Locale: ref.Locale, Role: values.Get("role"), Path: values.Get("path"), Selector: values.Get("selector"), Digest: values.Get("digest"), Snapshot: values.Get("snapshot")}
	if address.Kind != "template" || address.Locale == "" {
		return TemplateAddress{}, NewError(CodeInvalidRequest, "template address requires kind=template and locale", false, nil)
	}
	if address.Role != "" && !refPartPattern.MatchString(address.Role) {
		return TemplateAddress{}, NewError(CodeInvalidRequest, "template address role is invalid", false, nil)
	}
	if address.Path != "" && !safeTemplateAddressPath(address.Path) {
		return TemplateAddress{}, NewError(CodeInvalidRequest, "template address path is invalid", false, nil)
	}
	if address.Selector != "" && !validTemplateSelector(address.Selector) {
		return TemplateAddress{}, NewError(CodeInvalidRequest, "template address selector is invalid", false, nil)
	}
	for _, digest := range []string{address.Digest, address.Snapshot} {
		if digest != "" && !sha256DigestPattern.MatchString(digest) {
			return TemplateAddress{}, NewError(CodeInvalidRequest, "template address digest is invalid", false, nil)
		}
	}
	return address, nil
}

func FormatTemplateAddress(address TemplateAddress) string {
	base := fmt.Sprintf("promptrepo://%s/%s/%s@%s", address.RepositoryID, address.PackageID, address.SolutionID, address.Version)
	parts := []string{"kind=" + url.QueryEscape("template")}
	if address.Locale != "" {
		parts = append(parts, "locale="+url.QueryEscape(address.Locale))
	}
	for _, item := range []struct{ key, value string }{{"role", address.Role}, {"path", address.Path}, {"selector", address.Selector}, {"digest", address.Digest}, {"snapshot", address.Snapshot}} {
		if item.value != "" {
			parts = append(parts, item.key+"="+url.QueryEscape(item.value))
		}
	}
	return base + "?" + strings.Join(parts, "&")
}

func safeTemplateAddressPath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") || strings.ContainsAny(value, "\x00\n\r") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validTemplateSelector(value string) bool {
	for _, prefix := range []string{"heading:", "json-pointer:", "yaml-pointer:", "jsonl-id:"} {
		if strings.HasPrefix(value, prefix) {
			part := strings.TrimPrefix(value, prefix)
			if part == "" || strings.ContainsAny(part, "\x00\n\r") {
				return false
			}
			if (prefix == "json-pointer:" || prefix == "yaml-pointer:") && !strings.HasPrefix(part, "/") {
				return false
			}
			return true
		}
	}
	return false
}
