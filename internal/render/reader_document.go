// Package render: human-reading document projection (pinax-human-reading-api-v1
// P03). Unlike the Agent view above, this builder keeps user-written code,
// paths and terminology intact; untrusted HTML stays inert text. Pages are
// bounded by UTF-8 text bytes and node counts, oversized blocks continue via
// stable fragments, and budgets are explicit instead of silent truncation.
package render

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	goldmarkast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Reader budget defaults mirror the frozen schema constants.
const (
	ReaderPageMaxTextBytes = 256 * 1024
	ReaderPageMaxNodes     = 512
	ReaderSourceMaxBytes   = 8 * 1024 * 1024
	ReaderTotalMaxNodes    = 65536
	ReaderOutlinePageSize  = 512
)

// ReaderDocumentNode is the human-reading node shape; JSON tags match the
// pinax.reader.document_node.v1alpha1 machine schema.
type ReaderDocumentNode struct {
	Type          string               `json:"type"`
	Anchor        string               `json:"anchor"`
	Text          string               `json:"text,omitempty"`
	Children      []ReaderDocumentNode `json:"children,omitempty"`
	Ref           string               `json:"ref,omitempty"`
	Language      string               `json:"language,omitempty"`
	Alt           string               `json:"alt,omitempty"`
	BlockAnchor   string               `json:"block_anchor,omitempty"`
	FragmentIndex int                  `json:"fragment_index,omitempty"`
	LastFragment  bool                 `json:"last_fragment,omitempty"`
	OriginalType  string               `json:"original_type,omitempty"`
	Header        []string             `json:"header,omitempty"`
	// Tokens records [start, end) byte ranges of indivisible inline tokens
	// (code spans) inside Text so pagination never splits them. Transport
	// only; never serialized.
	Tokens [][2]int `json:"-"`
	// Fence semantics (owner-registered read-only handlers): fence name,
	// bounded payload and failure diagnostics. Mirror the legacy Agent
	// contract; payloads are bounded via BoundPayload, never executed.
	Fence      string `json:"fence,omitempty"`
	Payload    any    `json:"payload,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	NextAction string `json:"next_action,omitempty"`
	// payloadBytes carries the bounded payload's marshaled size for page
	// budget accounting. Transport only; never serialized.
	payloadBytes int `json:"-"`
}

// ReaderOutlineEntry is one outline row.
type ReaderOutlineEntry struct {
	Anchor string `json:"anchor"`
	Title  string `json:"title"`
	Level  int    `json:"level"`
}

// ReaderDocument is the parsed, budget-checked full document.
type ReaderDocument struct {
	Blocks      []ReaderDocumentNode
	Outline     []ReaderOutlineEntry
	AnchorIndex map[string]int // anchor -> top-level block index
	NodeCount   int
	TextBytes   int
	Warnings    []string
}

// ReaderPagePosition identifies the next page start: the top-level block
// index, and for mid-fragment blocks the exact byte offset plus the number
// of fragments already emitted for that block.
type ReaderPagePosition struct {
	BlockIndex int
	// Path is the opaque intra-block resume position (nested child indices,
	// byte offset, ref index, fragment counter) produced by the previous
	// page; empty means "start of block".
	Path string
	// DroppedBlocks carries prior pages' explicit budget drops so the final
	// page can never claim completeness after content was dropped.
	DroppedBlocks int
}

// resumeState is the parsed intra-block pagination state.
type resumeState struct {
	children []int // nested child indices (structural descent)
	byteOff  int   // byte offset into a fragmenting leaf's text
	refIdx   int   // next inline ref index (leaf ref batching)
	frag     int   // fragments emitted so far for the current leaf
}

func (s resumeState) serialize() string {
	parts := make([]string, len(s.children))
	for i, c := range s.children {
		parts[i] = strconv.Itoa(c)
	}
	return fmt.Sprintf("%s:%d:%d:%d", strings.Join(parts, "."), s.byteOff, s.refIdx, s.frag)
}

func parseResume(path string) resumeState {
	if path == "" {
		return resumeState{}
	}
	first := strings.LastIndex(path, ":")
	if first < 0 {
		return resumeState{}
	}
	second := strings.LastIndex(path[:first], ":")
	if second < 0 {
		return resumeState{}
	}
	third := strings.LastIndex(path[:second], ":")
	if third < 0 {
		return resumeState{}
	}
	state := resumeState{}
	if head := path[:third]; head != "" {
		for _, part := range strings.Split(head, ".") {
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 {
				return resumeState{}
			}
			state.children = append(state.children, index)
		}
	}
	state.byteOff, _ = strconv.Atoi(path[third+1 : second])
	state.refIdx, _ = strconv.Atoi(path[second+1 : first])
	state.frag, _ = strconv.Atoi(path[first+1:])
	if state.byteOff < 0 || state.refIdx < 0 || state.frag < 0 {
		return resumeState{}
	}
	return state
}

// ReaderPage is one bounded page of the document.
type ReaderPage struct {
	Nodes        []ReaderDocumentNode
	Next         *ReaderPagePosition
	Complete     bool
	ContentBytes int
	NodeCount    int
	Warnings     []string
}

// ReaderBudgetError distinguishes the two explicit failure modes.
type ReaderBudgetError struct {
	Code string // document_too_large | document_budget_exceeded
}

func (e *ReaderBudgetError) Error() string {
	if e.Code == "document_too_large" {
		return "source exceeds the 8 MiB interactive document budget"
	}
	return "parsed document exceeds the 65536 node budget"
}

var readerMarkdown = goldmark.New(
	goldmark.WithExtensions(extension.Table, extension.TaskList),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

// BuildReaderDocument parses body into bounded human-reading blocks with a
// stable outline. It never mutates or redacts user text: paths, code and
// terminology survive verbatim while consumers treat text as inert.
// BuildReaderDocument parses a note body with no fence handlers; fenced
// blocks without an owner handler stay inert code.
func BuildReaderDocument(body string) (ReaderDocument, error) {
	return BuildReaderDocumentWithFences(body, nil)
}

// BuildReaderDocumentWithFences parses with owner-registered read-only
// fence handlers. A fenced block whose language matches a handler produces
// a fence (or fence_failed) node with an independently bounded payload and
// explicit error fields; registered handlers never execute note code.
func BuildReaderDocumentWithFences(body string, fences map[string]func(string) FenceResult) (ReaderDocument, error) {
	if len(body) > ReaderSourceMaxBytes {
		return ReaderDocument{}, &ReaderBudgetError{Code: "document_too_large"}
	}
	document := ReaderDocument{AnchorIndex: map[string]int{}, Warnings: []string{}}
	source := []byte(body)
	parsed := readerMarkdown.Parser().Parse(text.NewReader(source))

	var build func(parent ast.Node, depth int) []ReaderDocumentNode
	build = func(parent ast.Node, depth int) []ReaderDocumentNode {
		nodes := make([]ReaderDocumentNode, 0)
		for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
			if document.NodeCount >= ReaderTotalMaxNodes {
				return nodes
			}
			node := ReaderDocumentNode{}
			switch t := child.(type) {
			case *ast.Heading:
				node.Type = "heading"
				node.Text, node.Tokens = readerNodeTextTokens(t, source)
				node.Anchor = headingAnchor(t, source, &document)
				if t.Level >= 1 && t.Level <= 6 && node.Text != "" {
					document.Outline = append(document.Outline, ReaderOutlineEntry{Anchor: node.Anchor, Title: node.Text, Level: t.Level})
				}
			case *ast.FencedCodeBlock:
				node.Type = "code"
				fenceName := ""
				if t.Info != nil {
					fenceName = firstField(string(t.Info.Value(source)))
					node.Language = fenceName
				}
				node.Anchor = document.reserveAnchor("code")
				node.Text = string(t.Lines().Value(source))
				if run, registered := fences[fenceName]; registered && fenceName != "" {
					arg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(t.Info.Value(source))), fenceName))
					if arg == "" {
						arg = strings.TrimSpace(node.Text)
					}
					result := run(arg)
					dropped := 0
					truncated := false
					node.Fence = fenceName
					if result.ErrorCode != "" {
						node.Type, node.ErrorCode, node.NextAction = "fence_failed", result.ErrorCode, result.NextAction
					} else {
						node.Type = "fence"
						node.Payload = BoundPayload(result.Payload, &dropped, &truncated)
						if encoded, err := json.Marshal(node.Payload); err == nil {
							node.payloadBytes = len(encoded)
						}
						if truncated || result.Truncated {
							document.Warnings = append(document.Warnings, "fence_payload_truncated")
						}
					}
					// Handler output replaces the raw source in the reading
					// surface; the payload keeps the contract's independent
					// budget, and fence payloads never fragment.
					node.Text = ""
					node.Tokens = nil
				}
			case *ast.CodeBlock:
				node.Type = "code"
				node.Anchor = document.reserveAnchor("code")
				node.Text = string(t.Lines().Value(source))
			case *ast.List:
				node.Type = "list"
				node.Anchor = document.reserveAnchor("list")
				node.Children = build(t, depth+1)
			case *ast.ListItem:
				node.Type = "paragraph"
				node.Anchor = document.reserveAnchor("item")
				// Children carry the item content; a flattened Text here
				// would duplicate every child byte in output while slices
				// budget only the children.
				node.Children = build(t, depth+1)
				if prefix, _ := taskCheckBoxPrefix(t); prefix != "" && len(node.Children) > 0 {
					// GFM task state lives on TaskCheckBox nodes; surface it
					// on the first inner block so done and todo items differ.
					// The prefix shifts every token range by its length.
					node.Children[0].Text = prefix + node.Children[0].Text
					shift := len(prefix)
					shifted := make([][2]int, len(node.Children[0].Tokens))
					for i, tok := range node.Children[0].Tokens {
						shifted[i] = [2]int{tok[0] + shift, tok[1] + shift}
					}
					node.Children[0].Tokens = shifted
				}
			case *ast.Blockquote:
				node.Type = "quote"
				node.Anchor = document.reserveAnchor("quote")
				node.Children = build(t, depth+1)
			case *goldmarkast.Table:
				node.Type = "table"
				node.Anchor = document.reserveAnchor("table")
				node.Header, node.Text, node.Children, node.Tokens = tableContent(t, source, node.Anchor)
				// Cell ref children never pass the build loop; count them
				// toward the total node budget here.
				document.NodeCount += len(node.Children)
			case *ast.HTMLBlock:
				// Raw HTML stays inert escaped text; nothing executes.
				node.Type = "unsupported"
				node.Anchor = document.reserveAnchor("html")
				node.Text = htmlBlockText(t, source)
			case *ast.ThematicBreak:
				node.Type = "unsupported"
				node.Anchor = document.reserveAnchor("break")
			default:
				node.Type = "paragraph"
				node.Anchor = document.reserveAnchor("block")
				node.Text, node.Tokens = readerNodeTextTokens(child, source)
			}
			// Inline links and images become typed child refs (no fetching).
			// Only nodes whose children were NOT built recursively collect
			// refs here: ListItem/List/Quote subtrees already collected
			// theirs per inner block, so walking the whole item again would
			// duplicate every ref. Text-less paragraphs — e.g. a standalone
			// ![](image) with no alt — still carry their refs, and refs are
			// collected at ANY nesting depth (no silent depth cap).
			if node.Children == nil && (node.Type == "paragraph" || node.Type == "heading") {
				before := len(node.Children)
				node.Children = append(node.Children, inlineRefs(child, source, node.Anchor, &document)...)
				// Generated ref nodes bypass the build loop; count the delta.
				document.NodeCount += len(node.Children) - before
			}
			// Every node passes this loop exactly once (children recurse
			// through the same build), so counting 1 per visit yields the
			// true total; adding the subtree here would double-count.
			document.NodeCount++
			document.TextBytes += readerNodeTextBytes(node)
			// Every top-level block registers its anchor for page jumps.
			if depth == 0 {
				document.AnchorIndex[node.Anchor] = len(nodes)
			}
			nodes = append(nodes, node)
		}
		return nodes
	}

	document.Blocks = build(parsed, 0)
	if len(body) <= ReaderSourceMaxBytes && document.NodeCount >= ReaderTotalMaxNodes {
		return document, &ReaderBudgetError{Code: "document_budget_exceeded"}
	}
	normalizeChildAnchors(&document)
	return document, nil
}

// normalizeChildAnchors routes every inline ref anchor (paragraph links and
// images, table cell refs, refs inside structural children) through the
// shared registry: collisions get a deterministic -2/-3 variant and every
// anchor maps to its owning top-level block so page jumps resolve.
func normalizeChildAnchors(document *ReaderDocument) {
	claim := func(anchor string, topIndex int) string {
		if _, exists := document.AnchorIndex[anchor]; !exists {
			document.AnchorIndex[anchor] = topIndex
			return anchor
		}
		for suffix := 2; ; suffix++ {
			candidate := fmt.Sprintf("%s-%d", anchor, suffix)
			if _, taken := document.AnchorIndex[candidate]; !taken {
				document.AnchorIndex[candidate] = topIndex
				return candidate
			}
		}
	}
	var walkRefs func(nodes []ReaderDocumentNode, topIndex int)
	walkRefs = func(nodes []ReaderDocumentNode, topIndex int) {
		for index := range nodes {
			node := &nodes[index]
			if node.Type == "link" || node.Type == "image" {
				node.Anchor = claim(node.Anchor, topIndex)
			} else if node.Anchor != "" {
				// Structural children (nested headings, list items, quote
				// bodies) were registered with a build-time placeholder
				// index; remap them to their owning top-level block so
				// anchor jumps land on the right page.
				document.AnchorIndex[node.Anchor] = topIndex
			}
			if len(node.Children) > 0 {
				walkRefs(node.Children, topIndex)
			}
		}
	}
	for topIndex := range document.Blocks {
		// The top-level block's own anchor maps to itself already.
		walkRefs(document.Blocks[topIndex].Children, topIndex)
	}
}

// uniqueFragmentAnchor decorates a continuation anchor when it collides
// with a registered (build-time) anchor. The registry is fixed after the
// build, so the decoration is deterministic across paginations; fragments
// are never registered.
func (d *ReaderDocument) uniqueFragmentAnchor(anchor string) string {
	if _, exists := d.AnchorIndex[anchor]; !exists {
		return anchor
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-x%d", anchor, suffix)
		if _, exists := d.AnchorIndex[candidate]; !exists {
			return candidate
		}
	}
}

// claimAnchor registers a slug-derived anchor, appending -2, -3, ... until
// the name is free in the shared registry.
func (d *ReaderDocument) claimAnchor(base string) string {
	anchor := base
	for index := 2; ; index++ {
		if _, exists := d.AnchorIndex[anchor]; !exists {
			break
		}
		anchor = fmt.Sprintf("%s-%d", base, index)
	}
	d.AnchorIndex[anchor] = len(d.AnchorIndex)
	return anchor
}

func (d *ReaderDocument) reserveAnchor(prefix string) string {
	index := len(d.AnchorIndex) + 1
	anchor := fmt.Sprintf("%s-%d", prefix, index)
	for _, exists := d.AnchorIndex[anchor]; exists; _, exists = d.AnchorIndex[anchor] {
		index++
		anchor = fmt.Sprintf("%s-%d", prefix, index)
	}
	d.AnchorIndex[anchor] = len(d.Blocks)
	return anchor
}

func countReaderNodes(node ReaderDocumentNode) int {
	count := 1
	for _, child := range node.Children {
		count += countReaderNodes(child)
	}
	return count
}

// readerNodeTextBytes counts UTF-8 text bytes of a block including all
// nested children and table headers, so the 256 KiB page budget covers the
// whole subtree.
func readerNodeTextBytes(node ReaderDocumentNode) int {
	total := len(node.Text) + len(node.Alt) + node.payloadBytes
	for _, header := range node.Header {
		total += len(header)
	}
	for _, child := range node.Children {
		total += readerNodeTextBytes(child)
	}
	return total
}

func fragmentable(block ReaderDocumentNode) bool {
	// Children that are pure inline refs (links/images) do not block
	// fragmentation: a paragraph carrying links continues via fragments,
	// and a header-only table (empty text, ref children) batches its refs
	// with its header through the same machinery. Structural children
	// (nested list or quote blocks) stay non-fragmentable.
	if len(block.Children) == 0 && block.Text == "" {
		return false
	}
	for _, child := range block.Children {
		if (child.Type != "link" && child.Type != "image") || len(child.Children) > 0 {
			return false
		}
	}
	return true
}

// ReaderOutlinePage slices the outline with a stable cursor.
func ReaderOutlinePage(document ReaderDocument, offset int) (entries []ReaderOutlineEntry, next int) {
	end := offset + ReaderOutlinePageSize
	if end > len(document.Outline) {
		end = len(document.Outline)
	}
	if offset > len(document.Outline) {
		offset = len(document.Outline)
	}
	entries = document.Outline[offset:end]
	if end < len(document.Outline) {
		next = end
	}
	return entries, next
}

// LocateReaderAnchor resolves an anchor to its top-level block index; -1
// when unknown. The renderer never scans the whole document on its own.
func LocateReaderAnchor(document ReaderDocument, anchor string) int {
	if index, ok := document.AnchorIndex[anchor]; ok {
		return index
	}
	return -1
}

// PaginateReaderDocument assembles one bounded page starting at position.
// Budgets count UTF-8 bytes (headers, nested subtrees and inline refs
// included) and nodes. Content paginates CONTINUOUSLY: leaf blocks batch
// their inline refs across pages and then fragment their text; structural
// blocks (lists, quotes) slice their children, recursing into oversized
// children so in-budget content is never dropped. Drops happen only for a
// single ref exceeding a whole page budget, always with an explicit warning.
func PaginateReaderDocument(document ReaderDocument, position ReaderPagePosition) ReaderPage {
	page := ReaderPage{Nodes: []ReaderDocumentNode{}, Warnings: []string{}}
	emitter := &pageEmitter{
		document:   &document,
		page:       &page,
		textBudget: ReaderPageMaxTextBytes,
		nodeBudget: ReaderPageMaxNodes,
	}
	blockIndex := position.BlockIndex
	if blockIndex < 0 {
		blockIndex = 0
	}
	for blockIndex < len(document.Blocks) && emitter.nodeBudget > 0 {
		state := resumeState{}
		if blockIndex == position.BlockIndex {
			state = parseResume(position.Path)
		}
		block := document.Blocks[blockIndex]
		emitter.resetBlockState()
		done := emitter.emitNode(block, state)
		if emitter.aborted {
			// Page is full mid-block: resume here next page.
			page.Next = &ReaderPagePosition{BlockIndex: blockIndex, Path: emitter.resume.serialize(), DroppedBlocks: position.DroppedBlocks + emitter.dropped}
			return page
		}
		if !done {
			page.Next = &ReaderPagePosition{BlockIndex: blockIndex, Path: emitter.resume.serialize(), DroppedBlocks: position.DroppedBlocks + emitter.dropped}
			return page
		}
		blockIndex++
	}
	if blockIndex < len(document.Blocks) {
		page.Next = &ReaderPagePosition{BlockIndex: blockIndex, DroppedBlocks: position.DroppedBlocks + emitter.dropped}
		return page
	}
	page.Complete = emitter.dropped == 0 && position.DroppedBlocks == 0
	if len(document.Warnings) > 0 {
		// Build-time warnings (e.g. fence payload truncation) must be
		// explicit: they ride the terminating page and force incomplete so
		// consumers never read a silently truncated document as complete.
		page.Warnings = append(page.Warnings, document.Warnings...)
		page.Complete = false
	}
	return page
}

// pageEmitter carries the mutable budget/state of one page assembly.
type pageEmitter struct {
	document   *ReaderDocument
	page       *ReaderPage
	textBudget int
	nodeBudget int
	dropped    int
	resume     resumeState // resume point when the current block is not done
	aborted    bool        // true when the page ran out of budget mid-node
	blockOpen  bool
}

// resetBlockState clears per-block bookkeeping; page budgets persist
// across blocks so one page is one shared budget.
func (e *pageEmitter) resetBlockState() {
	e.blockOpen = true
	e.resume = resumeState{}
	e.aborted = false
}

// emitNode emits as much of node (from state) as the page budget allows.
// Returns true when the node is fully consumed. When false, e.resume holds
// the exact resume point and e.aborted distinguishes budget exhaustion.
func (e *pageEmitter) emitNode(node ReaderDocumentNode, state resumeState) bool {
	total := readerNodeTextBytes(node)
	nodes := countReaderNodes(node)
	if len(state.children) == 0 && state.byteOff == 0 && state.refIdx == 0 && total <= e.textBudget && nodes <= e.nodeBudget {
		e.page.Nodes = append(e.page.Nodes, node)
		e.page.ContentBytes += total
		e.page.NodeCount += nodes
		e.textBudget -= total
		e.nodeBudget -= nodes
		return true
	}
	if fragmentable(node) {
		return e.emitFragmentingLeaf(node, state)
	}
	if len(node.Children) == 0 {
		// Childless block (e.g. a header-only table with no data rows):
		// emit whole when it fits, page-break when the page has content,
		// and warn-drop when no page can ever hold it — never a silent
		// skip that still reports completeness.
		total := readerNodeTextBytes(node)
		nodes := countReaderNodes(node)
		if total > ReaderPageMaxTextBytes || nodes > ReaderPageMaxNodes {
			e.page.Warnings = append(e.page.Warnings, "block_exceeds_page_budget")
			e.dropped++
			return true
		}
		if total > e.textBudget || nodes > e.nodeBudget {
			e.resume = state
			e.aborted = true
			return false
		}
		e.page.Nodes = append(e.page.Nodes, node)
		e.page.ContentBytes += total
		e.page.NodeCount += nodes
		e.textBudget -= total
		e.nodeBudget -= nodes
		return true
	}
	return e.emitStructural(node, state)
}

// emitFragmentingLeaf paginates a text leaf with (optional) inline ref
// children: refs batch first (text start riding the first fragment), then
// the text fragments by exact byte offsets. One fragment per page.
func (e *pageEmitter) emitFragmentingLeaf(node ReaderDocumentNode, state resumeState) bool {
	// Whole-block-first: a block that fits a FRESH page but not the current
	// page's remaining budget moves to the next page intact — only blocks
	// exceeding a whole page paginate via continuation (contract: page
	// boundaries split at complete blocks).
	if len(state.children) == 0 && state.byteOff == 0 && state.refIdx == 0 && state.frag == 0 && e.pageHasBlockContent() {
		if readerNodeTextBytes(node) <= ReaderPageMaxTextBytes && countReaderNodes(node) <= ReaderPageMaxNodes {
			e.resume = state
			e.aborted = true
			return false
		}
	}
	headerBytes := 0
	for _, header := range node.Header {
		headerBytes += len(header)
	}
	text := node.Text
	refs := node.Children
	// Phase A: refs.
	if state.refIdx < len(refs) {
		budget := e.textBudget - headerBytes
		nodesAvail := e.nodeBudget - 1 // continuation wrapper
		batch := make([]ReaderDocumentNode, 0, len(refs)-state.refIdx)
		refBytes := 0
		i := state.refIdx
		for i < len(refs) {
			size := readerNodeTextBytes(refs[i])
			if len(batch) == 0 && size > ReaderPageMaxTextBytes {
				// A single ref bigger than a whole page: explicit drop of
				// that ref only; progress is guaranteed.
				e.page.Warnings = append(e.page.Warnings, "block_exceeds_page_budget")
				e.dropped++
				i++
				continue
			}
			if len(batch) > 0 && (refBytes+size > budget || len(batch)+1 > nodesAvail) {
				break
			}
			if len(batch) == 0 && (refBytes+size > budget || nodesAvail < 1) {
				if headerBytes+size > ReaderPageMaxTextBytes {
					// Even a fresh page cannot hold this header plus the
					// ref: the block is unpageable — explicit drop with
					// warning instead of an infinite page-break loop.
					e.page.Warnings = append(e.page.Warnings, "block_exceeds_page_budget")
					e.dropped++
					return true
				}
				// The first ref does not fit the remaining budget: break for
				// a fresh page instead of dropping in-budget content.
				e.resume = resumeState{children: state.children, byteOff: state.byteOff, refIdx: i, frag: state.frag}
				e.aborted = true
				return false
			}
			batch = append(batch, refs[i])
			refBytes += size
			i++
		}
		if len(batch) == 0 {
			// Nothing fit even on this budget: if the page is fresh and a
			// full page can hold one ref, break for a fresh page; else the
			// drop path above already advanced.
			e.resume = resumeState{children: state.children, byteOff: state.byteOff, refIdx: i, frag: state.frag}
			e.aborted = true
			return false
		}
		// First fragment also carries the text start (contract: refs ride
		// the first fragment together with the text beginning).
		textPart := ""
		if state.byteOff == 0 {
			remaining := budget - refBytes
			if remaining > 0 && len(text) > 0 {
				end := state.byteOff + remaining
				if end >= len(text) {
					end = len(text)
				} else {
					end = safeSplitPointBytes(text, state.byteOff, end, node.Tokens)
				}
				if end > state.byteOff {
					textPart = text[state.byteOff:end]
				}
			}
		}
		newByte := state.byteOff + len(textPart)
		doneRefs := i >= len(refs)
		doneText := newByte >= len(text)
		e.emitFragmentNode(node, textPart, batch, state, headerBytes, refBytes, doneRefs && doneText)
		if doneRefs && doneText {
			return true
		}
		e.resume = resumeState{children: state.children, byteOff: newByte, refIdx: i, frag: state.frag + 1}
		e.aborted = true
		return false
	}
	// Phase B: text fragments (refs already emitted). Progress requires the
	// page to hold the header plus the full indivisible unit at the resume
	// position — one rune, or a whole escaped entity when one starts here.
	unit := indivisibleUnitSize(text, state.byteOff, node.Tokens)
	budget := e.textBudget - headerBytes
	if budget < unit || e.nodeBudget < 1 {
		if headerBytes+unit > ReaderPageMaxTextBytes {
			// No page can ever carry this header plus the unit: explicit
			// drop with warning instead of an unpageable stall.
			e.page.Warnings = append(e.page.Warnings, "block_exceeds_page_budget")
			e.dropped++
			return true
		}
		e.resume = state
		e.aborted = true
		return false
	}
	if state.byteOff >= len(text) {
		return true
	}
	fragment, next, done := fragmentReaderBlock(node, state.byteOff, state.frag, budget)
	fragment.LastFragment = done
	if fragment.Text == "" && !done {
		// The remaining budget cannot hold the indivisible unit starting
		// here. Break for a fresh page; if a fresh page also cannot hold
		// the header plus the unit, drop explicitly.
		if e.pageHasBlockContent() || len(e.page.Warnings) > 0 || headerBytes+unit <= ReaderPageMaxTextBytes {
			e.resume = state
			e.aborted = true
			return false
		}
		e.page.Warnings = append(e.page.Warnings, "block_exceeds_page_budget")
		e.dropped++
		return true
	}
	fragment.Anchor = e.document.uniqueFragmentAnchor(fragment.Anchor)
	fragment.Children = nil
	e.page.Nodes = append(e.page.Nodes, fragment)
	e.page.ContentBytes += len(fragment.Text) + headerBytes
	e.page.NodeCount++
	e.textBudget -= len(fragment.Text) + headerBytes
	e.nodeBudget--
	if done {
		return true
	}
	e.resume = resumeState{children: state.children, byteOff: next, refIdx: state.refIdx, frag: state.frag + 1}
	e.aborted = true
	return false
}

func (e *pageEmitter) emitFragmentNode(node ReaderDocumentNode, textPart string, batch []ReaderDocumentNode, state resumeState, headerBytes, refBytes int, lastFragment bool) {
	fragment := ReaderDocumentNode{
		Type:          "continuation",
		Anchor:        e.document.uniqueFragmentAnchor(fmt.Sprintf("%s-f%d", node.Anchor, state.frag+1)),
		BlockAnchor:   node.Anchor,
		OriginalType:  node.Type,
		Language:      node.Language,
		Header:        node.Header,
		FragmentIndex: state.frag + 1,
		LastFragment:  lastFragment,
		Text:          textPart,
		Children:      batch,
	}
	e.page.Nodes = append(e.page.Nodes, fragment)
	e.page.ContentBytes += len(textPart) + headerBytes + refBytes
	e.page.NodeCount += len(batch) + 1
	e.textBudget -= len(textPart) + headerBytes + refBytes
	e.nodeBudget -= len(batch) + 1
}

func (e *pageEmitter) pageHasBlockContent() bool {
	return len(e.page.Nodes) > 0
}

// emitStructural slices a structural block (list/quote) by children. An
// oversized child recurses into its own fragmentation, so in-budget
// children are never dropped.
func (e *pageEmitter) emitStructural(node ReaderDocumentNode, state resumeState) bool {
	// Whole-block-first for structural blocks too: a list/quote that fits a
	// FRESH page but not the remaining budget moves to the next page intact
	// instead of being sliced across pages.
	if len(state.children) == 0 && e.pageHasBlockContent() {
		if readerNodeTextBytes(node) <= ReaderPageMaxTextBytes && countReaderNodes(node) <= ReaderPageMaxNodes {
			e.resume = state
			e.aborted = true
			return false
		}
	}
	start := 0
	sub := resumeState{}
	if len(state.children) > 0 {
		start = state.children[0]
		sub = resumeState{children: state.children[1:], byteOff: state.byteOff, refIdx: state.refIdx, frag: state.frag}
	}
	if start > len(node.Children) {
		start = len(node.Children)
	}
	taken := make([]ReaderDocumentNode, 0, len(node.Children)-start)
	takenBytes, takenNodes := 0, 0
	i := start
	blockEmitted := false
	for i < len(node.Children) {
		child := node.Children[i]
		cb := readerNodeTextBytes(child)
		cn := countReaderNodes(child)
		resumingInto := len(state.children) > 0 && i == state.children[0]
		if resumingInto && !resumeStateIsFresh(sub) {
			// Resuming INTO a mid-fragment child: dispatch with its nested
			// state so byte/ref positions continue exactly where they
			// stopped. A fresh resume state falls through to the normal
			// take-or-fragment logic so the child keeps its list context.
			childDone := e.emitNode(child, sub)
			if !childDone {
				full := append([]int{i}, e.resume.children...)
				e.resume = resumeState{children: full, byteOff: e.resume.byteOff, refIdx: e.resume.refIdx, frag: e.resume.frag}
				return false
			}
			i++
			blockEmitted = true
			// Following siblings start fresh.
			state.children = nil
			sub = resumeState{}
			continue
		}
		if len(taken) > 0 && (takenBytes+cb > e.textBudget || takenNodes+cn+1 > e.nodeBudget) {
			break
		}
		if len(taken) == 0 && (cb > e.textBudget || cn+1 > e.nodeBudget) {
			// Child does not fit the remaining budget on its own.
			if e.pageHasBlockContent() {
				e.resume = resumeState{children: []int{i}}
				e.aborted = true
				return false
			}
			// Fresh page: recurse into the child's own fragmentation.
			childDone := e.emitNode(child, resumeState{})
			if !childDone {
				full := append([]int{i}, e.resume.children...)
				e.resume = resumeState{children: full, byteOff: e.resume.byteOff, refIdx: e.resume.refIdx, frag: e.resume.frag}
				return false
			}
			i++
			blockEmitted = true
			continue
		}
		taken = append(taken, child)
		takenBytes += cb
		takenNodes += cn
		if resumingInto {
			// A fresh-state resumed child was taken into the slice: the
			// following siblings are no longer resumed positions.
			state.children = nil
			sub = resumeState{}
		}
		i++
	}
	structHeaderBytes := 0
	for _, header := range node.Header {
		structHeaderBytes += len(header)
	}
	if len(taken) > 0 {
		slice := node
		if start > 0 || i < len(node.Children) || blockEmitted {
			slice.Anchor = e.document.uniqueFragmentAnchor(fmt.Sprintf("%s-p%d", node.Anchor, start+1))
		}
		// Structural slices carry only their taken children; any parent
		// Text would double the emitted bytes against the budget. The
		// re-carried header consumes budget exactly like fragment headers.
		slice.Text = ""
		slice.Children = taken
		e.page.Nodes = append(e.page.Nodes, slice)
		e.page.ContentBytes += takenBytes + structHeaderBytes
		e.page.NodeCount += takenNodes + 1
		e.textBudget -= takenBytes + structHeaderBytes
		e.nodeBudget -= takenNodes + 1
	}
	if i < len(node.Children) {
		e.resume = resumeState{children: []int{i}}
		e.aborted = len(taken) > 0 || blockEmitted
		return false
	}
	return true
}

// readerNodeText extracts inline text for the human reading surface. Unlike
// the legacy Agent nodeText, it also carries raw inline HTML (as inert
// text) and autolink URLs, so nothing in the body silently disappears.
func readerNodeText(node ast.Node, source []byte) string {
	var builder strings.Builder
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.Text:
			builder.Write(t.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				builder.WriteByte('\n')
			}
		case *ast.String:
			builder.Write(t.Value)
		case *ast.RawHTML:
			builder.Write(t.Segments.Value(source))
		case *ast.AutoLink:
			builder.Write(t.URL(source))
		}
		return ast.WalkContinue, nil
	})
	return builder.String()
}

// readerNodeTextTokens mirrors readerNodeText but also records the byte
// ranges of inline code spans in the produced text.
func readerNodeTextTokens(node ast.Node, source []byte) (string, [][2]int) {
	var builder strings.Builder
	tokens := [][2]int{}
	var spanStack []int
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if span, ok := n.(*ast.CodeSpan); ok {
			if entering {
				spanStack = append(spanStack, builder.Len())
			} else if len(spanStack) > 0 {
				start := spanStack[len(spanStack)-1]
				spanStack = spanStack[:len(spanStack)-1]
				tokens = append(tokens, [2]int{start, builder.Len()})
			}
			_ = span
			return ast.WalkContinue, nil
		}
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.Text:
			builder.Write(t.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				builder.WriteByte('\n')
			}
		case *ast.String:
			builder.Write(t.Value)
		case *ast.RawHTML:
			builder.Write(t.Segments.Value(source))
		case *ast.AutoLink:
			builder.Write(t.URL(source))
		}
		return ast.WalkContinue, nil
	})
	return builder.String(), tokens
}

// taskCheckBoxPrefix reports the GFM task state of a list item: "[x] " for
// checked, "[ ] " for unchecked, "" when the item is not a task. Only the
// item's OWN first text block is inspected — a checkbox inside a NESTED
// child list belongs to that child, never to the parent item.
func taskCheckBoxPrefix(item ast.Node) (string, bool) {
	first := item.FirstChild()
	if first == nil {
		return "", false
	}
	prefix := ""
	checked := false
	_ = ast.Walk(first, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, isList := n.(*ast.List); isList {
			return ast.WalkSkipChildren, nil
		}
		if box, ok := n.(*goldmarkast.TaskCheckBox); ok {
			checked = box.IsChecked
			if box.IsChecked {
				prefix = "[x] "
			} else {
				prefix = "[ ] "
			}
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return prefix, checked
}

func fragmentReaderBlock(block ReaderDocumentNode, startByte, fragmentIndex, budget int) (ReaderDocumentNode, int, bool) {
	return fragmentReaderBlockTokens(block, startByte, fragmentIndex, budget, block.Tokens)
}

func fragmentReaderBlockTokens(block ReaderDocumentNode, startByte, fragmentIndex, budget int, tokens [][2]int) (ReaderDocumentNode, int, bool) {
	text := block.Text
	if budget <= 0 {
		// Callers must page-break before this point; guard anyway.
		budget = 1
	}
	end := startByte + budget
	if end >= len(text) {
		end = len(text)
	} else {
		end = safeSplitPointBytes(text, startByte, end, tokens)
	}
	if end <= startByte {
		end = startByte // degenerate guard: never emit an empty fragment
	}
	original := "paragraph"
	switch block.Type {
	case "heading", "paragraph", "code", "table":
		original = block.Type
	}
	fragment := ReaderDocumentNode{
		Type:          "continuation",
		Anchor:        fmt.Sprintf("%s-f%d", block.Anchor, fragmentIndex+1),
		BlockAnchor:   block.Anchor,
		OriginalType:  original,
		Language:      block.Language,
		Header:        block.Header,
		FragmentIndex: fragmentIndex + 1,
		LastFragment:  end >= len(text),
		Text:          text[startByte:end],
	}
	return fragment, end, end >= len(text)
}

// safeSplitPointBytes backs end up to a line boundary, an entity-safe seam
// or a UTF-8 rune start; a rune is never severed mid-byte.
func safeSplitPointBytes(text string, start, end int, tokens [][2]int) int {
	point := safeSplitPointInner(text, start, end)
	// Never split inside an indivisible inline token: back off to its start.
	for _, token := range tokens {
		if token[0] < point && point < token[1] {
			if token[0] > start {
				return token[0]
			}
			// The token starts at/before the fragment start and still
			// straddles the boundary: the fragment cannot hold it whole.
			return start
		}
	}
	return point
}

func safeSplitPointInner(text string, start, end int) int {
	if boundary := strings.LastIndexByte(text[start:end], '\n'); boundary >= 0 {
		return start + boundary + 1
	}
	// Never leave a partial entity tail at the seam: if an opening '&' sits
	// within the last bytes without a closing ';', split before the '&'.
	// The lookback covers the longest HTML5 named entities (~32 chars, e.g.
	// &CounterClockwiseContourIntegral;) AND numeric character references
	// with arbitrarily long leading zeros (bounded by entityScanMax).
	// The scan includes offset == start: when the FRAGMENT ITSELF begins
	// with '&' and the budget cannot hold the whole entity, returning start
	// yields an empty fragment so the caller page-breaks instead of cutting
	// the entity open.
	for offset := end - 1; offset >= start; offset-- {
		switch text[offset] {
		case ';':
			return backToRuneStart(text, start, end)
		case '&':
			if entityCompletedWithin(text, offset) {
				return backToRuneStart(text, start, offset)
			}
			// A bare '&' without a terminator is plain text, not an entity:
			// splitting after it is safe, so keep scanning.
		}
	}
	return backToRuneStart(text, start, end)
}

// entityCompletedWithin reports whether the '&' at offset starts a real
// escaped entity with valid syntax (see entityLengthAt).
func entityCompletedWithin(text string, offset int) bool {
	return entityLengthAt(text, offset) > 0
}

// entityLengthAt returns the total byte length of the entity starting at
// the '&' on offset, or 0 when this is not an entity. Valid forms are named
// references (&Name; — ASCII letters/digits only), decimal references
// (&#123;) and hex references (&#x1F;). Spaces, punctuation or a bare '&'
// make the text plain prose, splittable anywhere after the '&'.
func entityLengthAt(text string, offset int) int {
	i := offset + 1
	if i >= len(text) {
		return 0
	}
	namedLimit := offset + 40 // longest real named references are ~32 chars
	isAlnum := func(c byte) bool {
		return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
	}
	isHex := func(c byte) bool {
		return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
	}
	if text[i] == '#' {
		i++
		hex := false
		if i < len(text) && (text[i] == 'x' || text[i] == 'X') {
			hex = true
			i++
		}
		digits := 0
		for i < len(text) {
			c := text[i]
			if hex {
				if !isHex(c) {
					break
				}
			} else if c < '0' || c > '9' {
				break
			}
			// Numeric references may carry arbitrarily long leading
			// zeros: the digit run is bounded by the text itself, not a
			// fixed window.
			digits++
			i++
		}
		// At least one digit is required; &#; and &#x; are plain text.
		if digits == 0 {
			return 0
		}
	} else {
		for i < len(text) && i <= namedLimit && isAlnum(text[i]) {
			i++
		}
	}
	if i > offset+1 && i < len(text) && text[i] == ';' {
		return i - offset + 1
	}
	return 0
}

func backToRuneStart(text string, start, end int) int {
	for end > start && !utf8.RuneStart(text[end]) {
		end--
	}
	return end
}

func inlineRefs(parent ast.Node, source []byte, blockAnchor string, document *ReaderDocument) []ReaderDocumentNode {
	children := make([]ReaderDocumentNode, 0)
	_ = ast.Walk(parent, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := node.(type) {
		case *ast.AutoLink:
			url := string(t.URL(source))
			children = append(children, ReaderDocumentNode{
				Type:   "link",
				Anchor: fmt.Sprintf("%s-link-%d", blockAnchor, len(children)+1),
				Text:   url,
				Ref:    url,
			})
		case *ast.Link:
			children = append(children, ReaderDocumentNode{
				Type:   "link",
				Anchor: fmt.Sprintf("%s-link-%d", blockAnchor, len(children)+1),
				Text:   readerNodeText(t, source),
				Ref:    string(t.Destination),
			})
		case *ast.Image:
			children = append(children, ReaderDocumentNode{
				Type:   "image",
				Anchor: fmt.Sprintf("%s-image-%d", blockAnchor, len(children)+1),
				Alt:    readerNodeText(t, source),
				Ref:    string(t.Destination),
			})
		}
		return ast.WalkContinue, nil
	})
	return children
}

func tableContent(table *goldmarkast.Table, source []byte, blockAnchor string) (header []string, body string, children []ReaderDocumentNode, tokens [][2]int) {
	header = []string{}
	rows := make([]string, 0)
	rowTokens := make([][][2]int, 0)
	children = []ReaderDocumentNode{}
	// One running sequence across every cell (header included) so child
	// anchors stay unique no matter how many cells carry refs.
	appendCellRefs := func(cell ast.Node) {
		for _, ref := range inlineRefs(cell, source, blockAnchor, nil) {
			ref.Anchor = fmt.Sprintf("%s-cell-%d", blockAnchor, len(children)+1)
			children = append(children, ref)
		}
	}
	for child := table.FirstChild(); child != nil; child = child.NextSibling() {
		if head, isHead := child.(*goldmarkast.TableHeader); isHead {
			for cell := head.FirstChild(); cell != nil; cell = cell.NextSibling() {
				header = append(header, readerNodeText(cell, source))
				// Header cells carry refs too: an attachment referenced from
				// the header row must not silently disappear.
				appendCellRefs(cell)
			}
			continue
		}
		row, ok := child.(*goldmarkast.TableRow)
		if !ok {
			continue
		}
		cells := make([]string, 0)
		cellsTokens := [][2]int{}
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			if len(cells) > 0 {
				// Cell separator contributes one byte to later offsets.
				cells = append(cells, "\t")
			}
			offset := 0
			for _, c := range cells {
				offset += len(c)
			}
			cellText, cellTokens := readerNodeTextTokens(cell, source)
			for _, tok := range cellTokens {
				cellsTokens = append(cellsTokens, [2]int{tok[0] + offset, tok[1] + offset})
			}
			cells = append(cells, cellText)
			// Images and links inside cells keep their refs as children;
			// table text alone would silently drop them.
			appendCellRefs(cell)
		}
		rows = append(rows, strings.Join(cells, ""))
		rowTokens = append(rowTokens, cellsTokens)
	}
	var builder strings.Builder
	tokens = [][2]int{}
	for index, row := range rows {
		if index > 0 {
			builder.WriteByte('\n')
		}
		offset := builder.Len()
		for _, tok := range rowTokens[index] {
			tokens = append(tokens, [2]int{tok[0] + offset, tok[1] + offset})
		}
		builder.WriteString(row)
	}
	return header, builder.String(), children, tokens
}

func htmlBlockText(block *ast.HTMLBlock, source []byte) string {
	// Goldmark stores the line carrying the closing tag in ClosureLine,
	// separate from Lines(); both belong to the block body and dropping
	// either would silently lose content.
	var builder strings.Builder
	if lines := block.Lines(); lines != nil {
		builder.Write(lines.Value(source))
	}
	// ClosureLine only holds a valid segment when the block actually has a
	// closure line; reading the zero segment would panic.
	if block.HasClosure() {
		builder.Write(block.ClosureLine.Value(source))
	}
	return builder.String()
}

func headingAnchor(heading *ast.Heading, source []byte, document *ReaderDocument) string {
	title := readerNodeText(heading, source)
	slug := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' {
			return unicode.ToLower(r)
		}
		return '-'
	}, title)
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "heading"
	}
	// Truncate on rune boundaries: byte slicing a long CJK title would emit
	// invalid UTF-8 that JSON round-trips into a different string, breaking
	// anchor lookups against the internal index.
	if runes := []rune(slug); len(runes) > 80 {
		slug = string(runes[:80])
	}
	// Claim a unique name in the shared anchor registry so headings cannot
	// collide with each other (foo, foo, foo-2) nor with block anchors.
	return document.claimAnchor(slug)
}

func firstField(value string) string {
	if fields := strings.Fields(value); len(fields) > 0 {
		return fields[0]
	}
	return ""
}

// resumeStateIsFresh reports whether a resume state points at the very
// start of a node (no nested descent, no byte/ref/fragment progress).
func resumeStateIsFresh(s resumeState) bool {
	return len(s.children) == 0 && s.byteOff == 0 && s.refIdx == 0 && s.frag == 0
}

// indivisibleUnitSize returns the byte size of the smallest unit that must
// stay together at the resume position: a whole escaped entity when one
// starts there (the '&'..';' extent, bounded like safeSplitPointBytes), or
// a single UTF-8 rune otherwise.
func indivisibleUnitSize(text string, offset int, tokens [][2]int) int {
	if offset >= len(text) {
		return 1
	}
	for _, token := range tokens {
		if token[0] <= offset && offset < token[1] {
			return token[1] - offset
		}
	}
	if text[offset] == '&' {
		if n := entityLengthAt(text, offset); n > 0 {
			return n
		}
	}
	for _, r := range text[offset:] {
		return utf8.RuneLen(r)
	}
	return 1
}
