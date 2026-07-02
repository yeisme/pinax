package vaultignore

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	PinaxIgnoreName = ".pinaxignore"
	beginGitBlock   = "# BEGIN PINAX METADATA-ONLY"
	endGitBlock     = "# END PINAX METADATA-ONLY"
)

type Matcher struct {
	rules []rule
}

type rule struct {
	pattern  string
	negate   bool
	dirOnly  bool
	anchored bool
	hasSlash bool
}

func Load(root string) (Matcher, error) {
	payload, err := os.ReadFile(filepath.Join(root, PinaxIgnoreName))
	if err != nil {
		if os.IsNotExist(err) {
			return Matcher{}, nil
		}
		return Matcher{}, err
	}
	return Parse(string(payload)), nil
}

func Parse(body string) Matcher {
	rules := make([]rule, 0)
	for _, line := range splitLines(body) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r := rule{}
		if strings.HasPrefix(line, "!") {
			r.negate = true
			line = strings.TrimSpace(strings.TrimPrefix(line, "!"))
		}
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			r.anchored = true
			line = strings.TrimPrefix(line, "/")
		}
		if strings.HasSuffix(line, "/") {
			r.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		line = path.Clean(filepath.ToSlash(line))
		if line == "." {
			continue
		}
		r.pattern = line
		r.hasSlash = strings.Contains(line, "/")
		rules = append(rules, r)
	}
	return Matcher{rules: rules}
}

func (m Matcher) Ignored(rel string, isDir bool) bool {
	rel = cleanRel(rel)
	if hardDenied(rel) {
		return true
	}
	ignored := false
	for _, r := range m.rules {
		if r.matches(rel, isDir) {
			ignored = !r.negate
		}
	}
	return ignored
}

func hardDenied(rel string) bool {
	rel = cleanRel(rel)
	return rel == ".git" || strings.HasPrefix(rel, ".git/") || rel == ".pinax" || strings.HasPrefix(rel, ".pinax/")
}

func (r rule) matches(rel string, isDir bool) bool {
	if r.dirOnly && !isDir && rel != r.pattern && !strings.HasPrefix(rel, r.pattern+"/") {
		return false
	}
	if r.dirOnly {
		return rel == r.pattern || strings.HasPrefix(rel, r.pattern+"/")
	}
	if r.anchored || r.hasSlash {
		return globMatch(r.pattern, rel)
	}
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if globMatch(r.pattern, part) {
			return true
		}
	}
	return false
}

func globMatch(pattern, value string) bool {
	if !strings.Contains(pattern, "**") {
		matched, err := path.Match(pattern, value)
		return err == nil && matched
	}
	re := regexp.QuoteMeta(pattern)
	re = strings.ReplaceAll(re, `\*\*`, `.*`)
	re = strings.ReplaceAll(re, `\*`, `[^/]*`)
	re = strings.ReplaceAll(re, `\?`, `[^/]`)
	matched, err := regexp.MatchString("^"+re+"$", value)
	return err == nil && matched
}

func cleanRel(rel string) string {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	rel = strings.TrimPrefix(rel, "./")
	clean := path.Clean(rel)
	if clean == "." {
		return ""
	}
	return clean
}

func DefaultPinaxIgnore() string {
	return strings.Join([]string{
		"# Pinax content ignore rules. This controls Pinax sync/content manifest, not Git.",
		".pinax/",
		".git/",
		".obsidian/",
		".env*",
		"*.pem",
		"*.key",
		"dist/",
		"temp/",
		"tmp/",
		"node_modules/",
		"vendor/",
		"",
	}, "\n")
}

func MetadataOnlyGitignoreBlock() string {
	return strings.Join([]string{
		beginGitBlock,
		"# Git tracks only safe Pinax project metadata. Pinax sync manages vault content.",
		"*",
		"!.gitignore",
		"!.pinaxignore",
		"!.pinax/",
		"!.pinax/config.yaml",
		"!.pinax/projects.json",
		"!.pinax/records/",
		"!.pinax/records/**",
		".pinax/records/version.json",
		".pinax/version/",
		".pinax/last_snapshot",
		"!.pinax/assets/",
		"!.pinax/assets/manifest.json",
		"!.pinax/templates/",
		"!.pinax/templates/**",
		"!.pinax/publish/",
		"!.pinax/publish/profiles/",
		"!.pinax/publish/profiles/**",
		"!.pinax/plugins/",
		"!.pinax/plugins/registry.json",
		"!.pinax/plugins/plugin-lock.json",
		endGitBlock,
		"",
	}, "\n")
}

func ApplyMetadataOnlyGitignore(existing string) string {
	block := MetadataOnlyGitignoreBlock()
	start := strings.Index(existing, beginGitBlock)
	end := strings.Index(existing, endGitBlock)
	if start >= 0 && end >= start {
		end += len(endGitBlock)
		updated := strings.TrimRight(existing[:start], "\n")
		if updated != "" {
			updated += "\n\n"
		}
		updated += strings.TrimRight(block, "\n")
		rest := strings.TrimLeft(existing[end:], "\n")
		if rest != "" {
			updated += "\n\n" + rest
		} else {
			updated += "\n"
		}
		return updated
	}
	trimmed := strings.TrimRight(existing, "\n")
	if trimmed == "" {
		return block
	}
	return trimmed + "\n\n" + block
}

func splitLines(body string) []string {
	return strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
}
