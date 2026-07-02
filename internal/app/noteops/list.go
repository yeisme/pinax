package noteops

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yeisme/pinax/internal/domain"
)

type ListRequest struct {
	Project       string
	Group         string
	Folder        string
	Kind          string
	Status        string
	CreatedAfter  string
	UpdatedAfter  string
	UpdatedBefore string
	PathPrefix    string
	Tags          []string
}

var inlineTagPattern = regexp.MustCompile(`(^|\s)#([\pL\pN_/-]+)`)

func MatchesList(note domain.Note, req ListRequest) bool {
	if req.Project != "" && note.Project != req.Project {
		return false
	}
	if req.Group != "" && note.Project != req.Group {
		return false
	}
	if req.Folder != "" && note.Folder != req.Folder {
		return false
	}
	if req.Kind != "" && note.Kind != req.Kind {
		return false
	}
	if req.Status != "" && note.Status != req.Status {
		return false
	}
	if req.CreatedAfter != "" && !timestampAfterOrEqual(note.CreatedAt, req.CreatedAfter) {
		return false
	}
	if req.UpdatedAfter != "" && !timestampAfterOrEqual(note.UpdatedAt, req.UpdatedAfter) {
		return false
	}
	if req.UpdatedBefore != "" && !timestampBeforeOrEqual(note.UpdatedAt, req.UpdatedBefore) {
		return false
	}
	if req.PathPrefix != "" && !strings.HasPrefix(note.Path, filepath.ToSlash(req.PathPrefix)) {
		return false
	}
	for _, tag := range req.Tags {
		if tag != "" && !containsString(AllTags(note), strings.TrimPrefix(tag, "#")) {
			return false
		}
	}
	return true
}

func AllTags(note domain.Note) []string {
	seen := map[string]bool{}
	for _, tag := range note.Tags {
		tag = strings.TrimPrefix(strings.TrimSpace(tag), "#")
		if tag != "" {
			seen[tag] = true
		}
	}
	for _, match := range inlineTagPattern.FindAllStringSubmatch(note.Body, -1) {
		if len(match) > 2 && match[2] != "" {
			seen[match[2]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func timestampAfterOrEqual(value, boundary string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	valueTime, err := parseUserDate(value)
	if err != nil {
		return false
	}
	boundaryTime, err := parseUserDate(boundary)
	if err != nil {
		return false
	}
	return valueTime.Equal(boundaryTime) || valueTime.After(boundaryTime)
}

func timestampBeforeOrEqual(value, boundary string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	valueTime, err := parseUserDate(value)
	if err != nil {
		return false
	}
	boundaryTime, err := parseUserDate(boundary)
	if err != nil {
		return false
	}
	return valueTime.Equal(boundaryTime) || valueTime.Before(boundaryTime)
}

func parseUserDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", value)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
