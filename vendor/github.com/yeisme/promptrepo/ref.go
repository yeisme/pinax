package promptrepo

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var refPartPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

type Ref struct {
	RepositoryID string
	PackageID    string
	SolutionID   string
	Version      string
	Locale       string
}

func ParseRef(raw string) (Ref, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "promptrepo" {
		return Ref{}, NewError(CodeInvalidRequest, "prompt repository ref must use promptrepo://", false, err)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if parsed.Host == "" || len(parts) != 2 {
		return Ref{}, NewError(CodeInvalidRequest, "prompt repository ref must include repository, package, and solution", false, nil)
	}
	solutionParts := strings.Split(parts[1], "@")
	if len(solutionParts) != 2 || solutionParts[1] == "" {
		return Ref{}, NewError(CodeInvalidRequest, "prompt repository ref must include an exact version", false, nil)
	}
	ref := Ref{RepositoryID: parsed.Host, PackageID: parts[0], SolutionID: solutionParts[0], Version: solutionParts[1], Locale: parsed.Query().Get("locale")}
	for _, value := range []string{ref.RepositoryID, ref.PackageID, ref.SolutionID, ref.Version} {
		if !refPartPattern.MatchString(value) {
			return Ref{}, NewError(CodeInvalidRequest, fmt.Sprintf("invalid prompt repository ref component %q", value), false, nil)
		}
	}
	if ref.Locale != "" && !regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`).MatchString(ref.Locale) {
		return Ref{}, NewError(CodeInvalidRequest, "invalid BCP 47 locale", false, nil)
	}
	return ref, nil
}

func FormatRef(ref Ref) string {
	base := fmt.Sprintf("promptrepo://%s/%s/%s@%s", ref.RepositoryID, ref.PackageID, ref.SolutionID, ref.Version)
	if ref.Locale == "" {
		return base
	}
	return base + "?locale=" + url.QueryEscape(ref.Locale)
}
