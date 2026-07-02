package markdownnote

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

type Document struct {
	Path         string
	Frontmatter  map[string]string
	Body         string
	Note         domain.Note
	Headings     []Heading
	Links        []Link
	Tasks        []Task
	Properties   []Property
	FencedBlocks []FencedBlock
	Diagnostics  []Diagnostic
}

type Heading struct {
	Level int
	Text  string
}

type Link struct {
	Target string
	Label  string
	Style  string
}

type Task struct {
	Text string
	Done bool
}

type Property struct {
	Name  string
	Value string
}

type FencedBlock struct {
	Language string
	Info     string
	Body     string
}

type Diagnostic struct {
	Code    string
	Message string
}

var (
	wikiLinkPattern       = regexp.MustCompile(`\[\[([^\]|#]+)(?:#[^\]|]+)?(?:\|([^\]]+))?\]\]`)
	markdownLinkPattern   = regexp.MustCompile(`!?\[([^\]]*)\]\(([^)]+)\)`)
	taskPattern           = regexp.MustCompile(`(?m)^\s*[-*]\s+\[([ xX])\]\s+(.+?)\s*$`)
	propertyPattern       = regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_-]*)::\s*(.+?)\s*$`)
	fencedBlockOpen       = regexp.MustCompile("^```\\s*([^\\s`]*)\\s*(.*)$")
	frontmatterDelimBytes = []byte("---")
)

func ParseFull(path string, content []byte) (Document, error) {
	meta, body, diagnostics := ParseFrontmatter(content)
	doc := Document{Path: filepath.ToSlash(path), Frontmatter: meta, Body: strings.TrimSpace(body), Diagnostics: diagnostics}
	doc.Headings = parseHeadings([]byte(body))
	doc.Links = parseLinks(body)
	doc.Tasks = parseTasks(body)
	doc.Properties = parseProperties(body)
	doc.FencedBlocks = parseFencedBlocks(body)
	doc.Note = noteFromDocument(doc)
	return doc, nil
}

func ParseFrontmatter(content []byte) (map[string]string, string, []Diagnostic) {
	meta := map[string]string{}
	if !bytes.HasPrefix(content, []byte("---\n")) && !bytes.HasPrefix(content, []byte("---\r\n")) {
		return meta, string(content), nil
	}
	lines := bytes.SplitAfter(content, []byte("\n"))
	rawStart := len(lines[0])
	rawEnd := -1
	bodyStart := -1
	offset := rawStart
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if bytes.Equal(bytes.TrimSpace(line), frontmatterDelimBytes) {
			rawEnd = offset
			bodyStart = offset + len(line)
			break
		}
		offset += len(line)
	}
	if bodyStart == -1 {
		return meta, string(content), []Diagnostic{{Code: "frontmatter_unclosed", Message: "YAML frontmatter is not closed"}}
	}
	raw := content[rawStart:rawEnd]
	var parsed yaml.Node
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return meta, string(content[bodyStart:]), []Diagnostic{{Code: "frontmatter_invalid", Message: err.Error()}}
	}
	root := &parsed
	if parsed.Kind == yaml.DocumentNode && len(parsed.Content) > 0 {
		root = parsed.Content[0]
	}
	if root.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(root.Content); i += 2 {
			meta[root.Content[i].Value] = flattenYAMLNode(root.Content[i+1])
		}
	}
	return meta, string(content[bodyStart:]), nil
}

func flattenYAMLNode(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	switch node.Kind {
	case yaml.ScalarNode:
		return strings.TrimSpace(node.Value)
	case yaml.SequenceNode:
		parts := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			value := strings.TrimSpace(flattenYAMLNode(item))
			if value != "" {
				parts = append(parts, value)
			}
		}
		return strings.Join(parts, ",")
	case yaml.MappingNode:
		parts := make([]string, 0, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := strings.TrimSpace(node.Content[i].Value)
			value := strings.TrimSpace(flattenYAMLNode(node.Content[i+1]))
			if key != "" && value != "" {
				parts = append(parts, key+":"+value)
			}
		}
		return strings.Join(parts, ",")
	default:
		return ""
	}
}

func parseHeadings(body []byte) []Heading {
	parser := goldmark.DefaultParser()
	root := parser.Parse(text.NewReader(body))
	headings := []Heading{}
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := node.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		headings = append(headings, Heading{Level: heading.Level, Text: strings.TrimSpace(nodeText(heading, body))})
		return ast.WalkContinue, nil
	})
	return headings
}

func nodeText(node ast.Node, source []byte) string {
	var b strings.Builder
	var collect func(ast.Node)
	collect = func(current ast.Node) {
		for child := current.FirstChild(); child != nil; child = child.NextSibling() {
			switch typed := child.(type) {
			case *ast.Text:
				b.Write(typed.Segment.Value(source))
			case *ast.CodeSpan:
				for segment := typed.FirstChild(); segment != nil; segment = segment.NextSibling() {
					if text, ok := segment.(*ast.Text); ok {
						b.Write(text.Segment.Value(source))
					}
				}
			default:
				collect(child)
			}
		}
	}
	collect(node)
	return b.String()
}

func parseLinks(body string) []Link {
	links := []Link{}
	for _, match := range wikiLinkPattern.FindAllStringSubmatch(body, -1) {
		label := match[1]
		if len(match) > 2 && strings.TrimSpace(match[2]) != "" {
			label = strings.TrimSpace(match[2])
		}
		links = append(links, Link{Target: strings.TrimSpace(match[1]), Label: label, Style: "wiki"})
	}
	for _, match := range markdownLinkPattern.FindAllStringSubmatch(body, -1) {
		links = append(links, Link{Target: strings.TrimSpace(match[2]), Label: strings.TrimSpace(match[1]), Style: "markdown"})
	}
	return links
}

func parseTasks(body string) []Task {
	tasks := []Task{}
	for _, match := range taskPattern.FindAllStringSubmatch(body, -1) {
		done := strings.EqualFold(strings.TrimSpace(match[1]), "x")
		tasks = append(tasks, Task{Text: strings.TrimSpace(match[2]), Done: done})
	}
	return tasks
}

func parseProperties(body string) []Property {
	properties := []Property{}
	for _, match := range propertyPattern.FindAllStringSubmatch(body, -1) {
		properties = append(properties, Property{Name: strings.TrimSpace(match[1]), Value: strings.TrimSpace(match[2])})
	}
	return properties
}

func parseFencedBlocks(body string) []FencedBlock {
	blocks := []FencedBlock{}
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		match := fencedBlockOpen.FindStringSubmatch(lines[i])
		if match == nil {
			continue
		}
		start := i + 1
		end := start
		for ; end < len(lines); end++ {
			if strings.HasPrefix(strings.TrimSpace(lines[end]), "```") {
				break
			}
		}
		blocks = append(blocks, FencedBlock{Language: strings.TrimSpace(match[1]), Info: strings.TrimSpace(match[2]), Body: strings.Join(lines[start:end], "\n")})
		i = end
	}
	return blocks
}

func noteFromDocument(doc Document) domain.Note {
	title := strings.TrimSpace(doc.Frontmatter["title"])
	if title == "" && len(doc.Headings) > 0 {
		title = doc.Headings[0].Text
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(doc.Path), filepath.Ext(doc.Path))
	}
	return domain.Note{ID: doc.Frontmatter["note_id"], Title: title, Path: doc.Path, Tags: splitCSV(doc.Frontmatter["tags"]), Labels: splitCSV(doc.Frontmatter["labels"]), Body: doc.Body, Frontmatter: doc.Frontmatter, Project: doc.Frontmatter["project"], Subproject: doc.Frontmatter["subproject"], Folder: doc.Frontmatter["folder"], Kind: doc.Frontmatter["kind"], Status: doc.Frontmatter["status"], BoardColumn: doc.Frontmatter["board_column"], Milestone: doc.Frontmatter["milestone"], Priority: doc.Frontmatter["priority"], Due: doc.Frontmatter["due"], DueAt: doc.Frontmatter["due_at"], BlockedBy: splitCSV(doc.Frontmatter["blocked_by"]), CreatedAt: doc.Frontmatter["created_at"], UpdatedAt: doc.Frontmatter["updated_at"]}
}

func splitCSV(value string) []string {
	value = strings.TrimSpace(strings.Trim(value, "[]"))
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.Trim(part, `"'`))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
