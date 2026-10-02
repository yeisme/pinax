// Package render builds deterministic, bounded, renderer-independent note trees.
package render

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yeisme/pinax/internal/redaction"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

const SchemaVersion = "render.view.v1alpha1"

type Reference struct {
	Unsafe   bool   `json:"-"`
	URI      string `json:"uri,omitempty"`
	ObjectID string `json:"object_id,omitempty"`
	Title    string `json:"title,omitempty"`
	Version  uint64 `json:"version"`
	Revision string `json:"revision,omitempty"`
	Status   string `json:"status"`
	Mime     string `json:"mime,omitempty"`
}
type Badge struct {
	ObjectID   string `json:"object_id"`
	Version    uint64 `json:"version"`
	Revision   string `json:"revision"`
	RenderedAt string `json:"rendered_at"`
}
type Node struct {
	Type       string     `json:"type"`
	Anchor     string     `json:"anchor"`
	Text       string     `json:"text,omitempty"`
	Ref        *Reference `json:"ref,omitempty"`
	Fence      string     `json:"fence,omitempty"`
	Payload    any        `json:"payload,omitempty"`
	ErrorCode  string     `json:"error_code,omitempty"`
	NextAction string     `json:"next_action,omitempty"`
	Children   []Node     `json:"children,omitempty"`
	Truncated  bool       `json:"truncated"`
}
type Panel struct {
	Entities      []Reference `json:"entities"`
	Truncated     bool        `json:"truncated"`
	DroppedUnsafe int         `json:"dropped_unsafe"`
}
type View struct {
	SchemaVersion string           `json:"schema_version"`
	Ref           Reference        `json:"ref"`
	Display       string           `json:"display"`
	Tree          []Node           `json:"tree"`
	Panels        map[string]Panel `json:"panels"`
	Badge         Badge            `json:"badge"`
	Truncated     bool             `json:"truncated"`
	DroppedUnsafe int              `json:"dropped_unsafe"`
	Warnings      []string         `json:"warnings"`
}
type FenceResult struct {
	Payload    any
	Truncated  bool
	ErrorCode  string
	NextAction string
}
type Inputs struct {
	Body      string
	Ref       Reference
	Display   string
	Badge     Badge
	Backlinks []Reference
	Resolve   func(target, label string) Reference
	Fences    map[string]func(string) FenceResult
}

var (
	absPath         = regexp.MustCompile(`(?i)(^|[\s"'(])(/[A-Za-z0-9._~-]+(?:/[A-Za-z0-9._~-]+)*|[A-Z]:[\\/])`)
	unsafeID        = regexp.MustCompile(`(?i)token|authorization|cookie|secret|password|api_key|bearer`)
	safeID          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	wiki            = regexp.MustCompile(`!?\[\[([^\]]+)\]\]`)
	credentialQuery = regexp.MustCompile(`(?i)([?&]|\b)(x-amz-signature|x-amz-credential|x-amz-security-token|x-goog-signature|x-goog-credential|sig|signature|access_token|api-key|auth)=[^\s&]+`)
)

func SafeID(value string) bool { return safeID.MatchString(value) && !unsafeID.MatchString(value) }
func SafeText(value string) bool {
	return len(redaction.ScanSensitiveClasses(value)) == 0 && !absPath.MatchString(value) && !credentialQuery.MatchString(value) && !strings.Contains(value, "\x1b")
}

func NoteURI(id, revision string) string {
	u := url.URL{Scheme: "pinax", Host: "note", Path: "/" + id}
	if revision != "" {
		q := url.Values{}
		q.Set("v", revision)
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func Build(in Inputs) View {
	v := View{SchemaVersion: SchemaVersion, Ref: in.Ref, Display: in.Display, Badge: in.Badge, Tree: []Node{}, Panels: map[string]Panel{}, Warnings: []string{}}
	if in.Display == "" {
		v.Display = "card"
	}
	clean := func(value string) string {
		if !SafeText(value) {
			v.DroppedUnsafe++
			return "[REDACTED]"
		}
		return value
	}
	v.Ref.Title = clean(v.Ref.Title)
	if v.Ref.ObjectID != "" && !SafeID(v.Ref.ObjectID) {
		v.Ref.ObjectID = ""
		v.Ref.URI = ""
		v.Ref.Status = "unresolved"
		v.DroppedUnsafe++
	}
	if !SafeText(v.Ref.URI) || unsafeID.MatchString(v.Ref.URI) {
		v.Ref.URI = ""
		v.Ref.Status = "unresolved"
		v.DroppedUnsafe++
	}
	v.Ref.Revision = clean(v.Ref.Revision)
	if v.Badge.ObjectID != "" && !SafeID(v.Badge.ObjectID) {
		v.Badge.ObjectID = ""
		v.DroppedUnsafe++
	}
	v.Badge.Revision = clean(v.Badge.Revision)
	v.Badge.RenderedAt = clean(v.Badge.RenderedAt)
	if v.Ref.Unsafe {
		v.DroppedUnsafe++
	}
	panel := Panel{Entities: []Reference{}}
	// Scan unsafe entries before truncation so the evidence describes the full input.
	for _, ref := range in.Backlinks {
		if ref.Unsafe || !SafeID(ref.ObjectID) || !SafeText(ref.Title) {
			panel.DroppedUnsafe++
			continue
		}
		ref.Title = truncate(ref.Title, 160)
		if len(panel.Entities) == 50 {
			panel.Truncated = true
			continue
		}
		panel.Entities = append(panel.Entities, ref)
	}
	v.Panels["backlinks"] = panel
	v.DroppedUnsafe += panel.DroppedUnsafe
	v.Truncated = panel.Truncated
	budget, nodes, fences := 500, 0, 0
	headings := map[string]int{}
	source := []byte(in.Body)
	if len(source) > 64<<10 {
		source = source[:64<<10]
		for !utf8.Valid(source) {
			source = source[:len(source)-1]
		}
		v.Truncated = true
	}
	doc := goldmark.DefaultParser().Parse(text.NewReader(source))
	var blocks func(ast.Node) []Node
	blocks = func(parent ast.Node) []Node {
		out := []Node{}
		for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
			if nodes >= 512 {
				v.Truncated = true
				break
			}
			nodes++
			node := Node{Anchor: fmt.Sprintf("node-%d", nodes)}
			switch t := n.(type) {
			case *ast.Heading:
				node.Type, node.Text = "heading", clean(nodeText(t, source))
				slug := strings.Map(func(r rune) rune {
					if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' {
						return unicode.ToLower(r)
					}
					return '-'
				}, node.Text)
				slug = strings.Trim(slug, "-")
				if slug == "" {
					slug = "heading"
				}
				slug = truncate(slug, 80)
				if unsafeID.MatchString(slug) {
					slug = "redacted-heading"
					v.DroppedUnsafe++
				}
				headings[slug]++
				node.Anchor = fmt.Sprintf("heading-%s-%d", truncate(slug, 80), headings[slug])
			case *ast.FencedCodeBlock:
				fences++
				node.Type, node.Anchor = "code", fmt.Sprintf("fence-%d", fences)
				info := ""
				if t.Info != nil {
					info = string(t.Info.Value(source))
				}
				parts := strings.Fields(info)
				name, arg := "", ""
				if len(parts) > 0 {
					name = parts[0]
					arg = strings.TrimSpace(strings.TrimPrefix(info, name))
				}
				body := string(t.Lines().Value(source))
				if run, ok := in.Fences[name]; ok {
					if arg == "" {
						arg = strings.TrimSpace(body)
					}
					r := run(arg)
					node.Type, node.Fence, node.Truncated = "fence", name, r.Truncated
					node.Payload = BoundPayload(r.Payload, &v.DroppedUnsafe, &node.Truncated)
					if v.Display == "card" {
						node.Payload = fenceSummary(node.Payload)
					}
					if r.ErrorCode != "" {
						node.Type, node.ErrorCode, node.NextAction = "fence_failed", clean(r.ErrorCode), clean(r.NextAction)
						v.Warnings = append(v.Warnings, "fence_failed")
					}
				} else if v.Display != "card" {
					node.Text = clean(body)
				}
			case *ast.CodeBlock:
				node.Type = "code"
				if v.Display != "card" {
					node.Text = clean(string(t.Lines().Value(source)))
				}
			case *ast.List:
				node.Type = "list"
				node.Children = blocks(n)
			case *ast.ListItem, *ast.Blockquote:
				node.Type = "group"
				node.Children = blocks(n)
			default:
				node.Type = "paragraph"
				content := nodeText(n, source)
				if v.Display != "card" {
					node.Text = clean(content)
				}
			}
			if v.Display == "context" && node.Type != "heading" && node.Text != "" {
				r := []rune(node.Text)
				if len(r) > budget {
					node.Text = string(r[:budget])
					node.Truncated = true
				}
				budget -= len([]rune(node.Text))
			}
			// Inline references are resolved only for text blocks, never fenced code.
			if node.Type == "paragraph" || node.Type == "heading" {
				content := nodeText(n, source)
				addRef := func(target, label string) {
					if nodes >= 512 {
						v.Truncated = true
						node.Truncated = true
						return
					}
					if in.Resolve == nil {
						return
					}
					ref := in.Resolve(target, label)
					if ref.Unsafe || !SafeText(ref.URI) || unsafeID.MatchString(ref.URI) || !SafeText(ref.Title) || !SafeText(ref.Mime) || (ref.ObjectID != "" && !SafeID(ref.ObjectID)) {
						v.DroppedUnsafe++
						return
					}
					typ := "link"
					if strings.HasPrefix(ref.URI, "pinax-asset://") {
						typ = "asset"
					}
					if strings.HasPrefix(ref.URI, "drivebridge://") {
						typ = "external_ref"
					}
					node.Children = append(node.Children, Node{Type: typ, Anchor: fmt.Sprintf("%s-link-%d", node.Anchor, len(node.Children)+1), Ref: &ref})
					nodes++
				}
				for _, match := range wiki.FindAllStringSubmatch(content, -1) {
					target, label, _ := strings.Cut(match[1], "|")
					addRef(target, label)
				}
				_ = ast.Walk(n, func(inline ast.Node, entering bool) (ast.WalkStatus, error) {
					if entering {
						switch t := inline.(type) {
						case *ast.Link:
							addRef(string(t.Destination), nodeText(t, source))
						case *ast.Image:
							addRef(string(t.Destination), nodeText(t, source))
						}
					}
					return ast.WalkContinue, nil
				})
			}
			if node.Truncated {
				v.Truncated = true
			}
			out = append(out, node)
		}
		return out
	}
	v.Tree = blocks(doc)
	return v
}

func truncate(s string, count int) string {
	r := []rune(s)
	if len(r) > count {
		return string(r[:count])
	}
	return s
}

func nodeText(node ast.Node, source []byte) string {
	var b strings.Builder
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch t := n.(type) {
			case *ast.Text:
				b.Write(t.Value(source))
				if t.SoftLineBreak() || t.HardLineBreak() {
					b.WriteByte('\n')
				}
			case *ast.String:
				b.Write(t.Value)
			}
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// BoundPayload strips transport-private fields and bounds every nested collection.
func BoundPayload(payload any, dropped *int, truncated *bool) any {
	if payload == nil {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	var clean func(any, int) any
	clean = func(v any, depth int) any {
		if depth > 12 {
			*truncated = true
			return nil
		}
		switch t := v.(type) {
		case string:
			if !SafeText(t) {
				*dropped++
				return "[REDACTED]"
			}
			if len([]rune(t)) > 500 {
				*truncated = true
				return truncate(t, 500)
			}
			return t
		case map[string]any:
			out := map[string]any{}
			for k, val := range t {
				if strings.Contains(k, "path") || k == "body" || k == "excerpt" || strings.Contains(k, "evidence") || k == "agent_contexts" || k == "agent_context" || k == "notes_prefix" {
					continue
				}
				if !SafeText(k) {
					*dropped++
					continue
				}
				out[k] = clean(val, depth+1)
			}
			return out
		case []any:
			out := []any{}
			for i, val := range t {
				sanitized := clean(val, depth+1)
				if i < 100 {
					out = append(out, sanitized)
				} else {
					*truncated = true
				}
			}
			return out
		default:
			return v
		}
	}
	return clean(value, 0)
}

func Text(v View) string {
	var b strings.Builder
	var walk func([]Node, string)
	walk = func(nodes []Node, indent string) {
		for _, n := range nodes {
			fmt.Fprintf(&b, "%s[%s] %s", indent, n.Anchor, n.Type)
			if n.Text != "" {
				b.WriteString(" " + n.Text)
			}
			if n.Ref != nil {
				fmt.Fprintf(&b, " %s (%s) %s", n.Ref.Title, n.Ref.Status, n.Ref.URI)
			}
			if n.Fence != "" {
				b.WriteString(" " + n.Fence)
			}
			if n.Payload != nil {
				data, _ := json.Marshal(n.Payload)
				b.WriteString(" " + string(data))
			}
			if n.ErrorCode != "" {
				b.WriteString(" " + n.ErrorCode + "; " + n.NextAction)
			}
			if n.Truncated {
				b.WriteString(" … truncated")
			}
			b.WriteByte('\n')
			walk(n.Children, indent+"  ")
		}
	}
	walk(v.Tree, "")
	keys := []string{}
	for k := range v.Panels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := v.Panels[k]
		fmt.Fprintf(&b, "%s: %d (truncated=%t)\n", k, len(p.Entities), p.Truncated)
	}
	fmt.Fprintf(&b, "badge: %s · v%d · %s · %s\n", v.Badge.ObjectID, v.Badge.Version, v.Badge.Revision, v.Badge.RenderedAt)
	return b.String()
}

func HTML(v View) string {
	return "<!doctype html><meta charset=\"utf-8\"><title>Pinax rendered view</title><pre>" + html.EscapeString(Text(v)) + "</pre>\n"
}

func fenceSummary(payload any) any {
	switch p := payload.(type) {
	case []any:
		return map[string]any{"count": len(p)}
	case map[string]any:
		out := map[string]any{}
		for _, key := range []string{"display", "row_count", "hops"} {
			if value, ok := p[key]; ok {
				out[key] = value
			}
		}
		for _, key := range []string{"nodes", "edges"} {
			if value, ok := p[key].([]any); ok {
				out[key+"_count"] = len(value)
			}
		}
		if len(out) == 0 {
			out["section_count"] = len(p)
		}
		return out
	default:
		return payload
	}
}
