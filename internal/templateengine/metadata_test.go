package templateengine

import (
	"strings"
	"testing"
)

func TestTemplateMetadataParsesV2Frontmatter(t *testing.T) {
	t.Parallel()

	doc, err := ParseDocument("meeting", strings.Join([]string{
		"---",
		"schema_version: pinax.template.v2",
		"engine: go-template",
		"kind: note",
		"variables:",
		"  client:",
		"    required: true",
		"    description: 客户名称",
		"defaults:",
		"  owner: Pinax",
		"example:",
		"  title: 周会",
		"  vars:",
		"    client: Acme",
		"---",
		"# {{ .Title }}",
	}, "\n"))
	if err != nil {
		t.Fatalf("parse metadata: %v", err)
	}
	if doc.Engine != EngineGoTemplate || doc.Metadata.SchemaVersion != "pinax.template.v2" || doc.Metadata.Kind != "note" {
		t.Fatalf("metadata not parsed: %#v", doc)
	}
	client := doc.Metadata.Variables["client"]
	if !client.Required || client.Description != "客户名称" {
		t.Fatalf("client variable = %#v", client)
	}
	if doc.Metadata.Defaults["owner"] != "Pinax" || doc.Metadata.Example.Title != "周会" || doc.Metadata.Example.Vars["client"] != "Acme" {
		t.Fatalf("defaults/example not parsed: %#v", doc.Metadata)
	}
	if strings.TrimSpace(doc.Body) != "# {{ .Title }}" {
		t.Fatalf("body = %q", doc.Body)
	}
}

func TestTemplateSchemaInvalidReturnsStableCode(t *testing.T) {
	t.Parallel()

	_, err := ParseDocument("bad", strings.Join([]string{
		"---",
		"schema_version: pinax.template.v2",
		"engine: shell",
		"variables:",
		"  bad key:",
		"    required: true",
		"---",
		"body",
	}, "\n"))
	if err == nil {
		t.Fatal("expected schema error")
	}
	if code := ErrorCode(err); code != "template_schema_invalid" {
		t.Fatalf("error code = %q, want template_schema_invalid; err=%v", code, err)
	}
}

func TestTemplateDesignFrontmatterKeepsLegacyWarning(t *testing.T) {
	t.Parallel()

	doc, err := ParseDocument("design", strings.Join([]string{
		"---",
		"schema_version: pinax.template_design.v1",
		"kind: template_design",
		"title: 设计稿",
		"---",
		"# {{title}}",
	}, "\n"))
	if err != nil {
		t.Fatalf("parse design template: %v", err)
	}
	if doc.Engine != EngineSimple {
		t.Fatalf("engine = %q, want simple", doc.Engine)
	}
	if len(doc.Issues) != 1 || doc.Issues[0].Code != "template_design_legacy" {
		t.Fatalf("issues = %#v", doc.Issues)
	}
}

func TestStarterTemplateMetadataParsesCatalogFields(t *testing.T) {
	t.Parallel()

	doc, err := ParseDocument("note.quick", strings.Join([]string{
		"---",
		"schema_version: pinax.template.v2",
		"name: note.quick",
		"title: Quick Note",
		"kind: note_template",
		"use_cases: [quick capture, scratch]",
		"aliases: [quick, scratch]",
		"difficulty: starter",
		"starter: true",
		"output:",
		"  path_pattern: notes/{{ .Title }}.md",
		"defaults:",
		"  kind: note",
		"---",
		"# {{ .Title }}",
	}, "\n"))
	if err != nil {
		t.Fatalf("parse starter metadata: %v", err)
	}
	if doc.Metadata.Name != "note.quick" || doc.Metadata.Title != "Quick Note" || doc.Metadata.Kind != "note_template" {
		t.Fatalf("identity metadata = %#v", doc.Metadata)
	}
	if len(doc.Metadata.UseCases) != 2 || doc.Metadata.UseCases[0] != "quick capture" || len(doc.Metadata.Aliases) != 2 || doc.Metadata.Aliases[0] != "quick" {
		t.Fatalf("catalog metadata = %#v", doc.Metadata)
	}
	if doc.Metadata.Difficulty != "starter" || doc.Metadata.Starter == nil || !*doc.Metadata.Starter {
		t.Fatalf("starter metadata = %#v", doc.Metadata)
	}
}
