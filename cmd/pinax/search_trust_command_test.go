package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTrustSearchVault 建一个带信任字段的 vault：
// - auth-design：human verified、stale_after 已过（stale）
// - auth-runbook：machine verified、fresh
// - auth-legacy：无信任字段（unverified、fresh）
func setupTrustSearchVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	fixtures := map[string]string{
		filepath.Join(root, "notes", "architecture", "auth-design.md"):  "---\nschema_version: pinax.note.v1\nnote_id: note_auth_design\ntitle: Auth Design\ntags: [auth, security]\nkind: reference\nstatus: stable\ndescription: token rotation must invalidate refresh grants\nverified:\n  - by: human:ye\n    at: 2026-09-06T10:30:00+00:00\nstale_after: 2020-01-01T00:00:00+00:00\n---\n\n# Auth Design\n\n[[auth-runbook]] links out for rotation detail.\n",
		filepath.Join(root, "notes", "architecture", "auth-runbook.md"): "---\nschema_version: pinax.note.v1\nnote_id: note_auth_runbook\ntitle: Auth Runbook\ntags: [auth, ops]\nkind: runbook\nstatus: stable\nverified:\n  - by: agent:pinax/0.9.0\n    at: 2026-09-05T08:00:00+00:00\n---\n\n# Auth Runbook\n\nrunbook rotation body\n",
		filepath.Join(root, "notes", "architecture", "auth-legacy.md"):  "---\nschema_version: pinax.note.v1\nnote_id: note_auth_legacy\ntitle: Auth Legacy\ntags: [auth]\nkind: decision\nstatus: draft\n---\n\n# Auth Legacy\n\nlegacy auth decision body\n",
	}
	for path, body := range fixtures {
		writeCLIFixture(t, path, body)
	}
	runCLI(t, "index", "refresh", "--vault", root, "--json")
	return root
}

func trustSearchFacts(t *testing.T, out string) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json invalid: %v\n%s", err, out)
	}
	return envelope["facts"].(map[string]any)
}

func TestSearchTrustFacetsAndFiltersCLI(t *testing.T) {
	t.Parallel()
	root := setupTrustSearchVault(t)

	// --facets：json envelope 携带 facets，计数覆盖全匹配集（3 条 auth 命中）。
	out := runCLI(t, "search", "auth", "--facets", "--vault", root, "--json")
	var envelope struct {
		Data struct {
			Facets map[string][]struct {
				Value string `json:"value"`
				Count int    `json:"count"`
			} `json:"facets"`
			Results []struct {
				Note struct {
					Path string `json:"path"`
				} `json:"note"`
				Trust string `json:"trust"`
				Fresh string `json:"fresh"`
			} `json:"results"`
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("facets json invalid: %v\n%s", err, out)
	}
	if envelope.Data.Total != 3 {
		t.Fatalf("total = %d, want 3; out=%s", envelope.Data.Total, out)
	}
	facets := envelope.Data.Facets
	if len(facets["trust"]) != 3 || len(facets["fresh"]) != 2 {
		t.Fatalf("trust/fresh facets = %#v / %#v", facets["trust"], facets["fresh"])
	}
	var humanCount, staleCount int
	for _, item := range facets["trust"] {
		if item.Value == "human" {
			humanCount = item.Count
		}
	}
	for _, item := range facets["fresh"] {
		if item.Value == "stale" {
			staleCount = item.Count
		}
	}
	if humanCount != 1 || staleCount != 1 {
		t.Fatalf("human=%d stale=%d facets=%#v", humanCount, staleCount, facets)
	}
	// tag facet 覆盖全匹配集：auth=3。
	var authTag int
	for _, item := range facets["tag"] {
		if item.Value == "auth" {
			authTag = item.Count
		}
	}
	if authTag != 3 {
		t.Fatalf("tag auth count = %d, want 3", authTag)
	}

	// 追加 --tag auth --trust human 后结果是子集。
	narrowed := runCLI(t, "search", "auth", "--tag", "auth", "--trust", "human", "--vault", root, "--json")
	var narrowEnvelope struct {
		Data struct {
			Results []struct {
				Note struct {
					Path string `json:"path"`
				} `json:"note"`
			} `json:"results"`
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(narrowed), &narrowEnvelope); err != nil {
		t.Fatalf("narrowed json invalid: %v\n%s", err, narrowed)
	}
	if narrowEnvelope.Data.Total != 1 || narrowEnvelope.Data.Results[0].Note.Path != "notes/architecture/auth-design.md" {
		t.Fatalf("trust human narrowed results = %#v", narrowEnvelope.Data.Results)
	}

	// --stale only 只含过期 note。
	staleOnly := trustSearchFacts(t, runCLI(t, "search", "auth", "--stale", "only", "--vault", root, "--json"))
	if staleOnly["total"] != "1" || staleOnly["filter.stale"] != "only" {
		t.Fatalf("stale only facts = %#v", staleOnly)
	}

	// agent 输出带 trust=/fresh= 与 facets.* 行。
	agent := runCLI(t, "search", "auth", "--facets", "--vault", root, "--agent")
	for _, want := range []string{
		"result.1.trust=", "result.1.fresh=", "facets.tag.auth=3", "facets.trust.human=1", "facets.fresh.stale=1",
	} {
		if !strings.Contains(agent, want) {
			t.Fatalf("agent output missing %q:\n%s", want, agent)
		}
	}
	assertMachineOutputClean(t, agent)

	// human 输出信任列与 Facets 块。
	human := runCLI(t, "search", "auth", "--facets", "--vault", root)
	for _, want := range []string{"Trust", "Fresh", "Facets", "human ✓", "stale"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output missing %q:\n%s", want, human)
		}
	}

	// 默认行为零变更：不带新 flag 时无 Trust/Fresh 列、无 facets、agent 无 trust=。
	plainJSON := runCLI(t, "search", "auth", "--vault", root, "--json")
	if strings.Contains(plainJSON, "facets") || strings.Contains(plainJSON, "\"trust\"") {
		t.Fatalf("default search json should not carry trust surface:\n%s", plainJSON)
	}
	plainAgent := runCLI(t, "search", "auth", "--vault", root, "--agent")
	if strings.Contains(plainAgent, "trust=") || strings.Contains(plainAgent, "fresh=") {
		t.Fatalf("default agent output should not carry trust fields:\n%s", plainAgent)
	}
	plainHuman := runCLI(t, "search", "auth", "--vault", root)
	if strings.Contains(plainHuman, "Trust") || strings.Contains(plainHuman, "Facets") {
		t.Fatalf("default human output should not carry trust columns:\n%s", plainHuman)
	}

	// 非法过滤器 fail-closed。
	if invalid, err := runCLIExpectError("search", "auth", "--trust", "super", "--vault", root, "--json"); err == nil || !strings.Contains(invalid, "invalid_trust_filter") {
		t.Fatalf("invalid trust filter must fail closed: err=%v out=%s", err, invalid)
	}
	if invalid, err := runCLIExpectError("search", "auth", "--stale", "maybe", "--vault", root, "--json"); err == nil || !strings.Contains(invalid, "invalid_stale_filter") {
		t.Fatalf("invalid stale filter must fail closed: err=%v out=%s", err, invalid)
	}

	// 无匹配：facet 空集表示，不报错。
	none := runCLI(t, "search", "nonexistent-token-xyz", "--facets", "--vault", root, "--json")
	if !strings.Contains(none, `"total":0`) {
		t.Fatalf("no-match facets output should still succeed:\n%s", none)
	}
}

func TestSearchShowCardCLI(t *testing.T) {
	t.Parallel()
	root := setupTrustSearchVault(t)

	out := runCLI(t, "search", "show", "notes/architecture/auth-design.md", "--vault", root, "--json")
	var envelope struct {
		Command string `json:"command"`
		Status  string `json:"status"`
		Data    struct {
			Title string `json:"title"`
			Trust struct {
				Tier          string `json:"tier"`
				LatestHumanBy string `json:"latest_human_by"`
				StaleAfter    string `json:"stale_after"`
				VerifiedCount int    `json:"verified_count"`
			} `json:"trust"`
			Fresh   string `json:"fresh"`
			Snippet string `json:"snippet"`
			Links   struct {
				Outgoing int `json:"outgoing"`
				Incoming int `json:"incoming"`
			} `json:"links"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("show json invalid: %v\n%s", err, out)
	}
	if envelope.Command != "search.show" || envelope.Data.Trust.Tier != "human" || envelope.Data.Fresh != "stale" {
		t.Fatalf("show card trust panel = %#v", envelope.Data)
	}
	if envelope.Data.Trust.LatestHumanBy != "human:ye" || envelope.Data.Trust.VerifiedCount != 1 {
		t.Fatalf("show card human event = %#v", envelope.Data.Trust)
	}
	if envelope.Data.Links.Outgoing != 1 || envelope.Data.Links.Incoming != 0 {
		t.Fatalf("show card links = %#v", envelope.Data.Links)
	}
	if !strings.Contains(envelope.Data.Snippet, "token rotation") {
		t.Fatalf("show card snippet = %q", envelope.Data.Snippet)
	}
	// 正文红线：除有界 snippet 外不出现正文标题外的正文内容。
	if strings.Count(out, "rotation") > 2 {
		t.Fatalf("show card must not output full body:\n%s", out)
	}

	agent := runCLI(t, "search", "show", "notes/architecture/auth-design.md", "--vault", root, "--agent")
	for _, want := range []string{"command=search.show", "detail.trust_tier=human", "detail.fresh=stale", "detail.links_out=1"} {
		if !strings.Contains(agent, want) {
			t.Fatalf("agent show missing %q:\n%s", want, agent)
		}
	}
	assertMachineOutputClean(t, agent)

	human := runCLI(t, "search", "show", "notes/architecture/auth-design.md", "--vault", root)
	for _, want := range []string{"Trust", "human ✓", "Neighbors"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human show missing %q:\n%s", want, human)
		}
	}

	// 歧义 fail-closed：stem/title 均唯一这里用 path 前缀构造歧义。
	writeCLIFixture(t, filepath.Join(root, "notes", "other", "auth-design.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_auth_design2\ntitle: Auth Design Two\n---\n\n# Duplicate\n")
	ambiguous, err := runCLIExpectError("search", "show", "auth-design", "--vault", root, "--json")
	if err == nil || !strings.Contains(ambiguous, "note_ref_ambiguous") {
		t.Fatalf("ambiguous show must fail closed: err=%v out=%s", err, ambiguous)
	}
}

func TestBrowseReadOnlyDirectoryViewCLI(t *testing.T) {
	t.Parallel()
	root := setupTrustSearchVault(t)

	out := runCLI(t, "browse", "notes/architecture", "--lazy-index", "off", "--vault", root, "--json")
	var envelope struct {
		Command string `json:"command"`
		Data    struct {
			Path       string `json:"path"`
			Notes      int    `json:"notes"`
			Subfolders []any  `json:"subfolders"`
			Items      []struct {
				Title string `json:"title"`
				Trust string `json:"trust"`
				Fresh string `json:"fresh"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("browse json invalid: %v\n%s", err, out)
	}
	if envelope.Command != "browse" || envelope.Data.Path != "notes/architecture" || envelope.Data.Notes != 3 {
		t.Fatalf("browse data = %#v", envelope.Data)
	}
	if len(envelope.Data.Items) == 0 {
		t.Fatalf("browse items empty")
	}

	// 只读断言：--lazy-index off 不写 .pinax/index.sqlite（删除后 browse 不得重建）。
	indexSQL := filepath.Join(root, ".pinax", "index.sqlite")
	if err := os.Remove(indexSQL); err != nil {
		t.Fatalf("remove index for read-only assertion: %v", err)
	}
	runCLI(t, "browse", "notes/architecture", "--lazy-index", "off", "--vault", root, "--json")
	if fileExists(indexSQL) {
		t.Fatalf("browse --lazy-index off must not write .pinax/index.sqlite")
	}

	agent := runCLI(t, "browse", "notes/architecture", "--lazy-index", "off", "--vault", root, "--agent")
	for _, want := range []string{"command=browse", "detail.path=notes/architecture", "note.1.trust=", "note.1.fresh="} {
		if !strings.Contains(agent, want) {
			t.Fatalf("browse agent missing %q:\n%s", want, agent)
		}
	}
	assertMachineOutputClean(t, agent)

	human := runCLI(t, "browse", "notes/architecture", "--vault", root)
	for _, want := range []string{"Notes (by updated)", "human ✓", "stale"} {
		if !strings.Contains(human, want) {
			t.Fatalf("browse human missing %q:\n%s", want, human)
		}
	}

	// 不存在的路径：稳定错误并列可用目录，不猜测就近目录。
	missing, err := runCLIExpectError("browse", "notes/nonexistent", "--vault", root, "--json")
	if err == nil || !strings.Contains(missing, "browse_path_not_found") || !strings.Contains(missing, "notes/architecture") {
		t.Fatalf("browse missing path must fail with candidates: err=%v out=%s", err, missing)
	}

	// 根目录浏览 + 无 index 写入。
	runCLI(t, "browse", "--vault", root, "--json")
	if fileExists(indexSQL) {
		t.Fatalf("browse must never write .pinax/index.sqlite")
	}
}
