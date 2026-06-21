package contentbundle

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const SchemaVersion = "pinax.content_bundle.v1"

type Bundle struct {
	SchemaVersion string `json:"schema_version" yaml:"schema_version"`
	ID            string `json:"id" yaml:"id"`
	Title         string `json:"title,omitempty" yaml:"title,omitempty"`
	Source        Source `json:"source,omitempty" yaml:"source,omitempty"`
	Items         []Item `json:"items" yaml:"items"`
}

type Source struct {
	ID  string `json:"id,omitempty" yaml:"id,omitempty"`
	URL string `json:"url,omitempty" yaml:"url,omitempty"`
}

type Item struct {
	ID         string   `json:"id" yaml:"id"`
	Title      string   `json:"title,omitempty" yaml:"title,omitempty"`
	Category   string   `json:"category,omitempty" yaml:"category,omitempty"`
	Language   string   `json:"language,omitempty" yaml:"language,omitempty"`
	Prompt     string   `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	SourceURL  string   `json:"source_url,omitempty" yaml:"source_url,omitempty"`
	Featured   bool     `json:"featured,omitempty" yaml:"featured,omitempty"`
	Tags       []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Techniques []string `json:"techniques,omitempty" yaml:"techniques,omitempty"`
	Styles     []string `json:"styles,omitempty" yaml:"styles,omitempty"`
	Subjects   []string `json:"subjects,omitempty" yaml:"subjects,omitempty"`
}

type Issue struct {
	ItemID  string `json:"item_id,omitempty"`
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type Stats struct {
	Items              int            `json:"items"`
	CompleteItems      int            `json:"complete_items"`
	MissingPromptItems int            `json:"missing_prompt_items"`
	FeaturedItems      int            `json:"featured_items"`
	Languages          map[string]int `json:"languages,omitempty"`
	Categories         map[string]int `json:"categories,omitempty"`
	Issues             []Issue        `json:"issues,omitempty"`
}

func Load(content []byte) (Bundle, error) {
	var bundle Bundle
	if err := yaml.Unmarshal(content, &bundle); err != nil {
		return Bundle{}, fmt.Errorf("load content bundle: %w", err)
	}
	return bundle, nil
}

func Validate(bundle Bundle) error {
	stats := Analyze(bundle)
	if len(stats.Issues) > 0 {
		messages := make([]string, 0, len(stats.Issues))
		for _, issue := range stats.Issues {
			messages = append(messages, issue.Code+": "+issue.Message)
		}
		return errors.New(strings.Join(messages, "; "))
	}
	return nil
}

func Analyze(bundle Bundle) Stats {
	stats := Stats{Items: len(bundle.Items), Languages: map[string]int{}, Categories: map[string]int{}}
	if strings.TrimSpace(bundle.SchemaVersion) != SchemaVersion {
		stats.Issues = append(stats.Issues, Issue{Code: "content_bundle_schema_invalid", Field: "schema_version", Message: "schema_version must be " + SchemaVersion})
	}
	if strings.TrimSpace(bundle.ID) == "" {
		stats.Issues = append(stats.Issues, Issue{Code: "content_bundle_id_required", Field: "id", Message: "bundle id is required"})
	}
	seen := map[string]bool{}
	for _, item := range bundle.Items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			stats.Issues = append(stats.Issues, Issue{Code: "content_item_id_required", Field: "items.id", Message: "item id is required"})
		} else if seen[id] {
			stats.Issues = append(stats.Issues, Issue{ItemID: id, Code: "content_item_duplicate", Field: "items.id", Message: "item id must be unique"})
		}
		seen[id] = true
		if strings.TrimSpace(item.Prompt) == "" {
			stats.MissingPromptItems++
		} else {
			stats.CompleteItems++
		}
		if item.Featured {
			stats.FeaturedItems++
		}
		if lang := strings.TrimSpace(item.Language); lang != "" {
			stats.Languages[lang]++
		}
		if category := strings.TrimSpace(item.Category); category != "" {
			stats.Categories[category]++
		}
	}
	return stats
}

func StableID(prefix, raw string) string {
	base := SafeSlug(raw)
	if base != "" {
		return base
	}
	sum := sha1.Sum([]byte(prefix + "\x00" + raw))
	return prefix + "-" + hex.EncodeToString(sum[:])[:12]
}

func SafeSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func Dedupe(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
