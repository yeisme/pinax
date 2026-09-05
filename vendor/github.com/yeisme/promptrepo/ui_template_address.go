package promptrepo

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var uiTemplatePathPartPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// UITemplateAddress identifies one declarative UI template bundle. It is an
// additive address family and deliberately does not widen TemplateAddress.
type UITemplateAddress struct {
	RepositoryID string `json:"repository_id" yaml:"repository_id"`
	PackageID    string `json:"package_id" yaml:"package_id"`
	SolutionID   string `json:"solution_id" yaml:"solution_id"`
	Version      string `json:"version" yaml:"version"`
	Locale       string `json:"locale" yaml:"locale"`
	Role         string `json:"role,omitempty" yaml:"role,omitempty"`
	Path         string `json:"path,omitempty" yaml:"path,omitempty"`
	Digest       string `json:"digest,omitempty" yaml:"digest,omitempty"`
	Snapshot     string `json:"snapshot,omitempty" yaml:"snapshot,omitempty"`
}

// Ref projects the solution identity without changing the existing Ref
// grammar or its locale semantics.
func (address UITemplateAddress) Ref() Ref {
	return Ref{
		RepositoryID: address.RepositoryID,
		PackageID:    address.PackageID,
		SolutionID:   address.SolutionID,
		Version:      address.Version,
		Locale:       address.Locale,
	}
}

// ParseUITemplateAddress parses the independent kind=ui-template address
// family. Unknown and duplicate fields fail closed.
func ParseUITemplateAddress(raw string) (UITemplateAddress, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "promptrepo" || parsed.Opaque != "" {
		return UITemplateAddress{}, uiTemplateAddressError("UI template address must use promptrepo://", err)
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return UITemplateAddress{}, uiTemplateAddressError("UI template address must not contain user info or a fragment", nil)
	}
	if parsed.Host == "" || !strings.HasPrefix(parsed.Path, "/") || strings.HasSuffix(parsed.Path, "/") {
		return UITemplateAddress{}, uiTemplateAddressError("UI template address identity is incomplete", nil)
	}
	parts := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	if len(parts) != 2 {
		return UITemplateAddress{}, uiTemplateAddressError("UI template address must include repository, package, and solution", nil)
	}
	solutionParts := strings.Split(parts[1], "@")
	if len(solutionParts) != 2 || solutionParts[0] == "" || solutionParts[1] == "" {
		return UITemplateAddress{}, uiTemplateAddressError("UI template address must include an exact version", nil)
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return UITemplateAddress{}, uiTemplateAddressError("UI template address query is invalid", err)
	}
	allowed := map[string]struct{}{
		"kind": {}, "locale": {}, "role": {}, "path": {}, "digest": {}, "snapshot": {},
	}
	for key, entries := range values {
		if _, ok := allowed[key]; !ok || len(entries) != 1 {
			return UITemplateAddress{}, uiTemplateAddressError("UI template address has an unsupported or duplicate query field", nil)
		}
	}
	if values.Get("kind") != "ui-template" || values.Get("locale") == "" {
		return UITemplateAddress{}, uiTemplateAddressError("UI template address requires kind=ui-template and locale", nil)
	}
	address := UITemplateAddress{
		RepositoryID: parsed.Host,
		PackageID:    parts[0],
		SolutionID:   solutionParts[0],
		Version:      solutionParts[1],
		Locale:       values.Get("locale"),
		Role:         values.Get("role"),
		Path:         values.Get("path"),
		Digest:       values.Get("digest"),
		Snapshot:     values.Get("snapshot"),
	}
	if err := ValidateUITemplateAddress(address); err != nil {
		return UITemplateAddress{}, err
	}
	return address, nil
}

// FormatUITemplateAddress emits the canonical query order:
// kind, locale, role, path, digest, snapshot.
func FormatUITemplateAddress(address UITemplateAddress) string {
	base := fmt.Sprintf("promptrepo://%s/%s/%s@%s", address.RepositoryID, address.PackageID, address.SolutionID, address.Version)
	parts := []string{"kind=" + url.QueryEscape("ui-template")}
	if address.Locale != "" {
		parts = append(parts, "locale="+url.QueryEscape(address.Locale))
	}
	for _, item := range []struct{ key, value string }{
		{"role", address.Role},
		{"path", address.Path},
		{"digest", address.Digest},
		{"snapshot", address.Snapshot},
	} {
		if item.value != "" {
			parts = append(parts, item.key+"="+url.QueryEscape(item.value))
		}
	}
	return base + "?" + strings.Join(parts, "&")
}

// ValidateUITemplateAddress validates the structured address without requiring
// digest and snapshot. Exact loaders add those requirements separately.
func ValidateUITemplateAddress(address UITemplateAddress) error {
	for _, value := range []string{address.RepositoryID, address.PackageID, address.SolutionID, address.Version} {
		if !refPartPattern.MatchString(value) {
			return uiTemplateAddressError("UI template address identity is invalid", nil)
		}
	}
	if !validLocale(address.Locale) {
		return uiTemplateAddressError("UI template address locale is invalid", nil)
	}
	if address.Role != "" && !refPartPattern.MatchString(address.Role) {
		return uiTemplateAddressError("UI template address role is invalid", nil)
	}
	if address.Path != "" && !safeUITemplatePath(address.Path) {
		return uiTemplateAddressError("UI template address path is invalid", nil)
	}
	for _, digest := range []string{address.Digest, address.Snapshot} {
		if digest != "" && !sha256DigestPattern.MatchString(digest) {
			return uiTemplateAddressError("UI template address digest is invalid", nil)
		}
	}
	return nil
}

func safeUITemplatePath(value string) bool {
	if !safeTemplateAddressPath(value) || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\t ") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if !uiTemplatePathPartPattern.MatchString(part) {
			return false
		}
	}
	return true
}

func uiTemplateAddressError(message string, cause error) error {
	return NewError(CodeUITemplateAddressInvalid, message, false, cause)
}
