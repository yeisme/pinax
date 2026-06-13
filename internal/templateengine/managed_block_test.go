package templateengine

import (
	"strings"
	"testing"
)

func TestPathPatternMetadataAndValidation(t *testing.T) {
	t.Parallel()

	doc, err := ParseDocument("journal.daily", strings.Join([]string{
		"---",
		"schema_version: pinax.template.v2",
		"kind: journal_template",
		"name: journal.daily",
		"title: 每日笔记",
		"engine: go-template",
		"output:",
		"  path_pattern: daily/{{ .Date }}.md",
		"---",
		"# {{ .Date }}",
	}, "\n"))
	if err != nil {
		t.Fatalf("parse path pattern metadata: %v", err)
	}
	if doc.Metadata.Kind != "journal_template" || doc.Metadata.Name != "journal.daily" || doc.Metadata.Title != "每日笔记" {
		t.Fatalf("metadata identity = %#v", doc.Metadata)
	}
	if doc.Metadata.Output.PathPattern != "daily/{{ .Date }}.md" {
		t.Fatalf("path pattern = %q", doc.Metadata.Output.PathPattern)
	}
}

func TestInvalidPathPatternReturnsStableCode(t *testing.T) {
	t.Parallel()

	for _, pattern := range []string{"/abs.md", "../outside.md", ".pinax/template.md", ".git/config", "attachments/file.md", "temp/file.md", "dist/file.md", "node_modules/pkg.md", "vendor/pkg.md"} {
		_, err := ParseDocument("bad", strings.Join([]string{
			"---",
			"schema_version: pinax.template.v2",
			"engine: go-template",
			"output:",
			"  path_pattern: " + pattern,
			"---",
			"body",
		}, "\n"))
		if err == nil {
			t.Fatalf("pattern %q accepted", pattern)
		}
		if code := ErrorCode(err); code != "template_output_path_invalid" {
			t.Fatalf("pattern %q error code = %q, want template_output_path_invalid; err=%v", pattern, code, err)
		}
	}
}

func TestManagedBlockInspectAndReplace(t *testing.T) {
	t.Parallel()

	body := strings.Join([]string{
		"# Home",
		"user text before",
		"<!-- pinax:managed name=recent -->",
		"old generated content",
		"<!-- /pinax:managed -->",
		"user text after",
	}, "\n")
	blocks, err := InspectManagedBlocks(body)
	if err != nil {
		t.Fatalf("inspect managed blocks: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Name != "recent" {
		t.Fatalf("blocks = %#v", blocks)
	}
	patched, err := ReplaceManagedBlock(body, "recent", "new generated content")
	if err != nil {
		t.Fatalf("replace managed block: %v", err)
	}
	for _, want := range []string{"user text before", "new generated content", "user text after"} {
		if !strings.Contains(patched, want) {
			t.Fatalf("patched body missing %q:\n%s", want, patched)
		}
	}
	if strings.Contains(patched, "old generated content") {
		t.Fatalf("old block content still present:\n%s", patched)
	}
}

func TestManagedBlockErrorsAreStable(t *testing.T) {
	t.Parallel()

	if _, err := ReplaceManagedBlock("# no block", "recent", "new"); ErrorCode(err) != "managed_block_missing" {
		t.Fatalf("missing block err = %v", err)
	}
	ambiguous := "<!-- pinax:managed name=recent -->\na\n<!-- /pinax:managed -->\n<!-- pinax:managed name=recent -->\nb\n<!-- /pinax:managed -->"
	if _, err := InspectManagedBlocks(ambiguous); ErrorCode(err) != "managed_block_ambiguous" {
		t.Fatalf("ambiguous block err = %v", err)
	}
	unclosed := "<!-- pinax:managed name=recent -->\na"
	if _, err := InspectManagedBlocks(unclosed); ErrorCode(err) != "managed_block_unclosed" {
		t.Fatalf("unclosed block err = %v", err)
	}
}
