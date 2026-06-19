package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetLinkCommandCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	notePath := filepath.Join(root, "notes", "alpha.md")
	writeCLIFixture(t, notePath, "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\nbody before link\n")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")
	before, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("read note before link: %v", err)
	}

	help := runCLI(t, "asset", "--help")
	if !strings.Contains(help, "link") {
		t.Fatalf("asset help missing link command:\n%s", help)
	}

	out := runCLI(t, "asset", "link", "diagram", "--note", "Alpha", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("asset link json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "asset.link" || envelope["status"] != "partial" || facts["writes"] != "false" || facts["asset_path"] != "assets/diagram.png" || facts["note_path"] != "notes/alpha.md" {
		t.Fatalf("asset link envelope = %#v\n%s", envelope, out)
	}
	if !strings.Contains(out, "asset_link") || !strings.Contains(out, "requires_snapshot") {
		t.Fatalf("asset link output missing plan details:\n%s", out)
	}
	after, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("read note after link: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("asset link command modified note body:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestAssetRelationshipCommandsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	writeCLIFixture(t, filepath.Join(root, "assets", "orphan.pdf"), "pdf")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\n![Diagram](../assets/diagram.png)\n![Missing](../assets/missing.png)\n")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")

	backlinksOut := runCLI(t, "asset", "backlinks", "diagram", "--vault", root, "--json")
	var backlinks map[string]any
	if err := json.Unmarshal([]byte(backlinksOut), &backlinks); err != nil {
		t.Fatalf("asset backlinks json invalid: %v\n%s", err, backlinksOut)
	}
	backlinkFacts := backlinks["facts"].(map[string]any)
	if backlinks["command"] != "asset.backlinks" || backlinkFacts["linked_notes"] != "1" || !strings.Contains(backlinksOut, "notes/alpha.md") || !strings.Contains(backlinksOut, "![Diagram](../assets/diagram.png)") {
		t.Fatalf("asset backlinks output = %#v\n%s", backlinks, backlinksOut)
	}

	orphansOut := runCLI(t, "asset", "orphans", "--vault", root, "--json")
	var orphans map[string]any
	if err := json.Unmarshal([]byte(orphansOut), &orphans); err != nil {
		t.Fatalf("asset orphans json invalid: %v\n%s", err, orphansOut)
	}
	orphanFacts := orphans["facts"].(map[string]any)
	if orphans["command"] != "asset.orphans" || orphanFacts["orphan"] != "1" || !strings.Contains(orphansOut, "assets/orphan.pdf") {
		t.Fatalf("asset orphans output = %#v\n%s", orphans, orphansOut)
	}

	missingOut := runCLI(t, "asset", "missing", "--vault", root, "--json")
	var missing map[string]any
	if err := json.Unmarshal([]byte(missingOut), &missing); err != nil {
		t.Fatalf("asset missing json invalid: %v\n%s", err, missingOut)
	}
	missingFacts := missing["facts"].(map[string]any)
	if missing["command"] != "asset.missing" || missingFacts["missing"] != "1" || !strings.Contains(missingOut, "assets/missing.png") {
		t.Fatalf("asset missing output = %#v\n%s", missing, missingOut)
	}

	repairOut := runCLI(t, "asset", "repair", "--plan", "--vault", root, "--json")
	var repair map[string]any
	if err := json.Unmarshal([]byte(repairOut), &repair); err != nil {
		t.Fatalf("asset repair json invalid: %v\n%s", err, repairOut)
	}
	repairFacts := repair["facts"].(map[string]any)
	if repair["command"] != "asset.repair" || repairFacts["writes"] != "false" || repairFacts["missing"] != "1" || repairFacts["orphan"] != "1" || !strings.Contains(repairOut, "asset_missing") || !strings.Contains(repairOut, "asset_orphan") {
		t.Fatalf("asset repair output = %#v\n%s", repair, repairOut)
	}
}

func TestAssetMoveRemovePlanCommandsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\n![Diagram](../assets/diagram.png)\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "beta.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\n---\n\n# Beta\n\n![[../assets/diagram.png]]\n")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")

	moveOut := runCLI(t, "asset", "move", "diagram", "attachments/archive/diagram.png", "--plan", "--vault", root, "--json")
	var move map[string]any
	if err := json.Unmarshal([]byte(moveOut), &move); err != nil {
		t.Fatalf("asset move json invalid: %v\n%s", err, moveOut)
	}
	moveFacts := move["facts"].(map[string]any)
	if move["command"] != "asset.move" || moveFacts["writes"] != "false" || moveFacts["linked_notes"] != "2" || moveFacts["requires_snapshot"] != "true" || !strings.Contains(moveOut, "asset_move") || !strings.Contains(moveOut, "asset_reference_rewrite") || !strings.Contains(moveOut, "![Diagram](../assets/diagram.png)") || !strings.Contains(moveOut, "![[../assets/diagram.png]]") || !strings.Contains(moveOut, "pinax asset move diagram attachments/archive/diagram.png --vault") {
		t.Fatalf("asset move plan output = %#v\n%s", move, moveOut)
	}
	if _, err := os.Stat(filepath.Join(root, "assets", "diagram.png")); err != nil {
		t.Fatalf("asset move --plan should not move source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", "archive", "diagram.png")); !os.IsNotExist(err) {
		t.Fatalf("asset move --plan unexpectedly created target: %v", err)
	}

	removeOut := runCLI(t, "asset", "remove", "diagram", "--plan", "--vault", root, "--json")
	var remove map[string]any
	if err := json.Unmarshal([]byte(removeOut), &remove); err != nil {
		t.Fatalf("asset remove json invalid: %v\n%s", err, removeOut)
	}
	removeFacts := remove["facts"].(map[string]any)
	if remove["command"] != "asset.remove" || removeFacts["writes"] != "false" || removeFacts["shared"] != "true" || removeFacts["delete_allowed"] != "false" || removeFacts["requires_snapshot"] != "true" || strings.Contains(removeOut, "asset_delete") || !strings.Contains(removeOut, "asset_reference_review") || !strings.Contains(removeOut, "pinax asset remove diagram --vault") {
		t.Fatalf("asset remove plan output = %#v\n%s", remove, removeOut)
	}
	if _, err := os.Stat(filepath.Join(root, "assets", "diagram.png")); err != nil {
		t.Fatalf("asset remove --plan should not delete source: %v", err)
	}
}

func TestAssetApplyLikeCommandsRequirePlanCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	assetPath := filepath.Join(root, "assets", "diagram.png")
	writeCLIFixture(t, assetPath, "png")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\n![Diagram](../assets/diagram.png)\n")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")
	before := readCLIFile(t, assetPath)

	checks := [][]string{
		{"asset", "move", "diagram", "attachments/archive/diagram.png", "--vault", root, "--json"},
		{"asset", "remove", "diagram", "--vault", root, "--json"},
		{"asset", "repair", "--vault", root, "--json"},
	}
	for _, args := range checks {
		out, err := runCLIExpectError(args...)
		if err == nil || !strings.Contains(out, "approval_required") || !strings.Contains(out, "--plan") {
			t.Fatalf("%v error = %v output = %s", args, err, out)
		}
	}
	if got := readCLIFile(t, assetPath); got != before {
		t.Fatalf("asset command without plan modified asset: %q", got)
	}
	if fileExists(filepath.Join(root, "attachments", "archive", "diagram.png")) {
		t.Fatalf("asset move without plan created target")
	}
}

func TestAssetPreviewOutputContractCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	specSource := filepath.Join(root, "spec-source.md")
	writeCLIFixture(t, specSource, "# Spec\n\nAsset preview body")
	textSource := filepath.Join(root, "transcript-source.txt")
	writeCLIFixture(t, textSource, "abcdef")
	imageSource := filepath.Join(root, "diagram-source.png")
	writeCLIFixture(t, imageSource, "png-bytes")
	runCLI(t, "asset", "add", specSource, "--vault", root, "--json")
	runCLI(t, "asset", "add", textSource, "--vault", root, "--json")
	runCLI(t, "asset", "add", imageSource, "--vault", root, "--json")

	markdownOut := runCLI(t, "asset", "preview", "spec-source", "--as", "markdown", "--vault", root, "--json")
	if !strings.Contains(markdownOut, "\"command\":\"asset.preview\"") || !strings.Contains(markdownOut, "Asset preview body") || !strings.Contains(markdownOut, "embedded_asset") {
		t.Fatalf("asset markdown preview:\n%s", markdownOut)
	}
	textOut := runCLI(t, "asset", "preview", "transcript-source", "--as", "text", "--max-preview-bytes", "3", "--vault", root, "--json")
	if !strings.Contains(textOut, "abc") || strings.Contains(textOut, "def") || !strings.Contains(textOut, "\"truncated\":true") {
		t.Fatalf("asset text preview:\n%s", textOut)
	}
	imageOut := runCLI(t, "asset", "preview", "diagram-source", "--as", "markdown", "--vault", root, "--json")
	if !strings.Contains(imageOut, "image/png") || !strings.Contains(imageOut, "placeholder") || strings.Contains(imageOut, "png-bytes") {
		t.Fatalf("asset image preview:\n%s", imageOut)
	}
}

func TestRenderedNoteAttachmentPreviewCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "spec.md"), "# Spec\n\nInline spec body")
	writeCLIFixture(t, filepath.Join(root, "notes", "transcript.txt"), "abcdef")
	writeCLIFixture(t, filepath.Join(root, "notes", "diagram.png"), "png-bytes")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\n![[spec.md]]\n![[transcript.txt]]\n![[diagram.png]]\n")

	renderedOut := runCLI(t, "note", "show", "Alpha", "--view", "rendered", "--embed-attachments", "markdown", "--vault", root, "--json")
	if !strings.Contains(renderedOut, "Inline spec body") || !strings.Contains(renderedOut, "pinax asset show diagram.png") || !strings.Contains(renderedOut, "embedded_assets") {
		t.Fatalf("rendered attachment preview:\n%s", renderedOut)
	}
	sourceOut := runCLI(t, "note", "show", "Alpha", "--view", "source", "--embed-attachments", "markdown", "--vault", root, "--json")
	if strings.Contains(sourceOut, "Inline spec body") || strings.Contains(sourceOut, "embedded_assets") {
		t.Fatalf("source view should not inline attachments:\n%s", sourceOut)
	}
	textOut := runCLI(t, "note", "show", "Alpha", "--view", "rendered", "--embed-attachments", "text", "--max-embed-bytes", "3", "--vault", root, "--json")
	if !strings.Contains(textOut, "truncated") {
		t.Fatalf("text attachment bounded preview:\n%s", textOut)
	}
	previewOut := runCLI(t, "note", "preview", "Alpha", "--embed-attachments", "markdown", "--vault", root, "--json")
	if !strings.Contains(previewOut, "\"command\":\"note.preview\"") || !strings.Contains(previewOut, "Inline spec body") {
		t.Fatalf("note preview output:\n%s", previewOut)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "render-runs")); !os.IsNotExist(err) {
		t.Fatalf("note preview should not write render run receipts: %v", err)
	}
}

func TestAttachmentPathStyleCommandsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\n![Diagram](../assets/diagram.png)\n")
	writeCLIFixture(t, filepath.Join(root, "attachments", "other", "diagram.png"), "other")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")

	attachmentsOut := runCLI(t, "note", "attachments", "Alpha", "--path-style", "note-relative", "--vault", root, "--json")
	if !strings.Contains(attachmentsOut, "\"path\":\"assets/diagram.png\"") || !strings.Contains(attachmentsOut, "\"display_path\":\"../assets/diagram.png\"") {
		t.Fatalf("note attachments path style output:\n%s", attachmentsOut)
	}

	missingContext, _, err := runCLISeparate("asset", "show", "diagram", "--path-style", "note-relative", "--vault", root, "--json")
	if err == nil || !strings.Contains(missingContext, "path_context_required") || !strings.Contains(missingContext, "--context-note") {
		t.Fatalf("missing context stdout=%s err=%v", missingContext, err)
	}
	markdownOut := runCLI(t, "asset", "show", "diagram", "--path-style", "markdown", "--context-note", "Alpha", "--vault", root, "--json")
	if !strings.Contains(markdownOut, "\"display_path\":\"![diagram.png](../assets/diagram.png)\"") {
		t.Fatalf("markdown path style output:\n%s", markdownOut)
	}
	wikiOut := runCLI(t, "asset", "show", "diagram", "--path-style", "wiki", "--vault", root, "--json")
	if !strings.Contains(wikiOut, "\"display_path\":\"![[assets/diagram.png]]\"") || strings.Contains(wikiOut, "![[diagram.png]]") {
		t.Fatalf("wiki path style output:\n%s", wikiOut)
	}
	absoluteOut := runCLI(t, "asset", "show", "diagram", "--path-style", "absolute", "--vault", root, "--json")
	if !strings.Contains(absoluteOut, filepath.ToSlash(filepath.Join(root, "assets", "diagram.png"))) || strings.Contains(absoluteOut, filepath.ToSlash(filepath.Join(root, "notes", "alpha.md"))) {
		t.Fatalf("absolute path style output:\n%s", absoluteOut)
	}
}

func TestAssetCompletionUsesIndexedAttachmentCandidatesCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\n![Diagram](../assets/diagram.png)\n")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")

	showCompletion := runCLI(t, "__complete", "asset", "show", "--vault", root, "")
	for _, want := range []string{"diagram.png\timage/png linked_notes=1", "assets/diagram.png\timage/png linked_notes=1", "diagram\timage/png linked_notes=1", "ShellCompDirectiveNoFileComp"} {
		if !strings.Contains(showCompletion, want) {
			t.Fatalf("asset show completion missing %q:\n%s", want, showCompletion)
		}
	}

	backlinksCompletion := runCLI(t, "__complete", "asset", "backlinks", "--vault", root, "dia")
	if !strings.Contains(backlinksCompletion, "diagram.png\timage/png linked_notes=1") || !strings.Contains(backlinksCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("asset backlinks completion = %s", backlinksCompletion)
	}
	moveCompletion := runCLI(t, "__complete", "asset", "move", "--vault", root, "dia")
	if !strings.Contains(moveCompletion, "diagram.png\timage/png linked_notes=1") || !strings.Contains(moveCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("asset move completion = %s", moveCompletion)
	}
	removeCompletion := runCLI(t, "__complete", "asset", "remove", "--vault", root, "dia")
	if !strings.Contains(removeCompletion, "diagram.png\timage/png linked_notes=1") || !strings.Contains(removeCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("asset remove completion = %s", removeCompletion)
	}

	attachSourceCompletion := runCLI(t, "__complete", "note", "attach", "Alpha", "--vault", root, "")
	if strings.Contains(attachSourceCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("note attach source file completion should remain enabled:\n%s", attachSourceCompletion)
	}
}

func TestAssetCommandContractsCLI(t *testing.T) {

	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	source := filepath.Join(root, "diagram-source.png")
	payload := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 'p', 'i', 'n', 'a', 'x', '-', 'b', 'i', 'n', 'a', 'r', 'y'}
	if err := os.WriteFile(source, payload, 0o644); err != nil {
		t.Fatalf("write asset source: %v", err)
	}

	addOut, addErr, err := runCLISeparate("asset", "add", source, "--vault", root, "--json")
	if err != nil || addErr != "" {
		t.Fatalf("asset add err=%v stderr=%q stdout=%s", err, addErr, addOut)
	}
	if strings.Contains(addOut, string(payload)) || strings.Contains(addOut, "pinax-binary") {
		t.Fatalf("asset add leaked payload:\n%s", addOut)
	}
	var addEnvelope map[string]any
	if err := json.Unmarshal([]byte(addOut), &addEnvelope); err != nil {
		t.Fatalf("asset add json invalid: %v\n%s", err, addOut)
	}
	addFacts := addEnvelope["facts"].(map[string]any)
	assetPath, _ := addFacts["asset_path"].(string)
	if addEnvelope["command"] != "asset.add" || addFacts["sha256"] == "" || addFacts["media_type"] != "image/png" || assetPath == "" {
		t.Fatalf("asset add envelope = %#v", addEnvelope)
	}

	listAgent, listErr, err := runCLISeparate("asset", "list", "--vault", root, "--agent")
	if err != nil || listErr != "" {
		t.Fatalf("asset list err=%v stderr=%q stdout=%s", err, listErr, listAgent)
	}
	for _, want := range []string{"command=asset.list", "fact.assets=1", "fact.asset.1.path="} {
		if !strings.Contains(listAgent, want) {
			t.Fatalf("asset list agent missing %q:\n%s", want, listAgent)
		}
	}

	showOut := runCLI(t, "asset", "show", filepath.Base(assetPath), "--vault", root, "--json")
	var showEnvelope map[string]any
	if err := json.Unmarshal([]byte(showOut), &showEnvelope); err != nil {
		t.Fatalf("asset show json invalid: %v\n%s", err, showOut)
	}
	showFacts := showEnvelope["facts"].(map[string]any)
	if showEnvelope["command"] != "asset.show" || showFacts["asset_path"] != assetPath || strings.Contains(showOut, "pinax-binary") {
		t.Fatalf("asset show envelope = %#v\n%s", showEnvelope, showOut)
	}

	verifyOut := runCLI(t, "asset", "verify", "--vault", root, "--json")
	var verifyEnvelope map[string]any
	if err := json.Unmarshal([]byte(verifyOut), &verifyEnvelope); err != nil {
		t.Fatalf("asset verify json invalid: %v\n%s", err, verifyOut)
	}
	verifyFacts := verifyEnvelope["facts"].(map[string]any)
	if verifyEnvelope["command"] != "asset.verify" || verifyFacts["verified"] != "1" || verifyFacts["missing"] != "0" || verifyFacts["changed"] != "0" {
		t.Fatalf("asset verify envelope = %#v", verifyEnvelope)
	}
}

func TestNoteAttachmentCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nkind: reference\n---\n\n# Alpha\n\nBody.\n")
	sourceDir := t.TempDir()
	source := filepath.Join(sourceDir, "diagram.png")
	writeCLIFixture(t, source, "png-bytes")

	attachOut := runCLI(t, "note", "attach", "Alpha", source, "--vault", root, "--json")
	var attachEnvelope map[string]any
	if err := json.Unmarshal([]byte(attachOut), &attachEnvelope); err != nil {
		t.Fatalf("note attach json invalid: %v\n%s", err, attachOut)
	}
	attachFacts := attachEnvelope["facts"].(map[string]any)
	attachmentPath, _ := attachFacts["attachment_path"].(string)
	if attachEnvelope["command"] != "note.attach" || attachFacts["path"] != "notes/alpha.md" || attachmentPath == "" || !strings.HasPrefix(attachmentPath, "attachments/note_alpha/") {
		t.Fatalf("note attach envelope = %#v", attachEnvelope)
	}
	if !fileExists(filepath.Join(root, attachmentPath)) {
		t.Fatalf("attachment file missing at %s", attachmentPath)
	}
	alphaContent := readCLIFile(t, filepath.Join(root, "notes", "alpha.md"))
	if !strings.Contains(alphaContent, "![diagram.png](/"+attachmentPath+")") && !strings.Contains(alphaContent, "![diagram.png](../../"+attachmentPath+")") && !strings.Contains(alphaContent, "![diagram.png](../"+attachmentPath+")") {
		t.Fatalf("note body missing attachment reference:\n%s", alphaContent)
	}

	attachmentsOut := runCLI(t, "note", "attachments", "Alpha", "--vault", root, "--json")
	var attachmentsEnvelope map[string]any
	if err := json.Unmarshal([]byte(attachmentsOut), &attachmentsEnvelope); err != nil {
		t.Fatalf("note attachments json invalid: %v\n%s", err, attachmentsOut)
	}
	attachmentsFacts := attachmentsEnvelope["facts"].(map[string]any)
	if attachmentsEnvelope["command"] != "note.attachments" || attachmentsFacts["attachments"] != "1" || attachmentsFacts["missing"] != "0" || !strings.Contains(attachmentsOut, attachmentPath) {
		t.Fatalf("note attachments envelope = %#v out=%s", attachmentsEnvelope, attachmentsOut)
	}

	before := readCLIFile(t, filepath.Join(root, "notes", "alpha.md"))
	failed, err := runCLIExpectError("note", "attach", "Alpha", filepath.Join(sourceDir, "missing.png"), "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "attachment_source_missing") {
		t.Fatalf("missing attachment err=%v out=%s", err, failed)
	}
	if after := readCLIFile(t, filepath.Join(root, "notes", "alpha.md")); after != before {
		t.Fatalf("missing source modified note:\nbefore=%s\nafter=%s", before, after)
	}
}
