package main

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func buildExplorePageFixtureBundle(t *testing.T) app.ExploreBundle {
	t.Helper()
	root := t.TempDir()
	notes := map[string]string{
		"notes/auth-design.md":    "---\nschema_version: pinax.note.v1\nnote_id: note_auth_design\ntitle: Auth Design\nkind: reference\ntags: [auth, security]\nsummary: Token rotation must invalidate refresh grants.\nupdated_at: 2026-09-01T10:00:00+00:00\ngenerated: {by: agent:pinax/0.9.0, at: 2026-09-01T08:00:00+00:00}\nverified: [{by: human:ye, at: 2026-09-01T10:30:00+00:00}]\nstale_after: 2030-01-01T00:00:00+00:00\n---\n\n# Auth Design\n\nVAULT_BODY_SENTINEL see [[Auth Runbook]] and [[Missing Page]].",
		"notes/auth-runbook.md":   "---\nschema_version: pinax.note.v1\nnote_id: note_auth_runbook\ntitle: Auth Runbook\nkind: runbook\ntags: [auth, ops]\nupdated_at: 2026-09-02T10:00:00+00:00\nverified: {by: machine:pinax/0.9.0, at: 2026-09-02T10:00:00+00:00}\n---\n\n# Auth Runbook\n\nVAULT_BODY_SENTINEL",
		"notes/gateway-design.md": "---\nschema_version: pinax.note.v1\nnote_id: note_gateway_design\ntitle: Gateway Design\nkind: decision\ntags: [gateway]\nupdated_at: 2026-08-30T09:00:00+00:00\nstale_after: 2026-01-01T00:00:00+00:00\n---\n\n# Gateway\n\nVAULT_BODY_SENTINEL links to [[Auth Design]]",
	}
	for rel, content := range notes {
		writeCLIFixture(t, filepath.Join(root, filepath.FromSlash(rel)), content)
	}
	bundle, err := app.BuildExploreBundle(root)
	if err != nil {
		t.Fatalf("build explore bundle: %v", err)
	}
	return bundle
}

func TestExplorePageExternalRefScanZeroHits(t *testing.T) {
	t.Parallel()
	bundle := buildExplorePageFixtureBundle(t)
	// 注入对抗性标题：证明标题内容无法借 JSON 内嵌伪造 src=/href= 外链属性。
	bundle.Nodes = append(bundle.Nodes, app.ExploreNode{
		ID: "note_evil", Title: `Evil" src="http://evil.example/x` + " href='https://cdn.example/lib.js'",
		Kind: "reference", Trust: "human", Fresh: "fresh", UpdatedAt: "2026-09-01T00:00:00+00:00",
	})
	page, err := app.RenderExplorePage(bundle, true)
	if err != nil {
		t.Fatalf("render explore page: %v", err)
	}
	if refs := app.ScanExploreExternalRefs(page); len(refs) != 0 {
		t.Fatalf("explore page leaked external refs: %v\npage tail:\n%s", refs, clipForLog(string(page)))
	}
	if strings.Contains(string(page), "VAULT_BODY_SENTINEL") {
		t.Fatalf("explore page leaked note body sentinel")
	}
}

func TestExplorePageContract(t *testing.T) {
	t.Parallel()
	page, err := app.RenderExplorePage(buildExplorePageFixtureBundle(t), true)
	if err != nil {
		t.Fatalf("render explore page: %v", err)
	}
	body := string(page)
	for _, want := range []string{
		`<style>`,                               // CSS 全内联
		`<script>`,                              // JS 全内联
		`type="search"`,                         // 客户端搜索
		`id="kind"`, `id="trust"`, `id="fresh"`, // kind/trust/fresh 过滤
		`id="view-graph"`, `id="view-list"`, // 图/列表双布局
		"prefers-reduced-motion", // reduced-motion 禁动画
		"/explore/note/",         // Load preview 有界预览端点
		"/explore/data.json",     // --no-embed 模式数据端点
		"history.replaceState",   // URL hash 记录过滤态
		"ArrowDown", "ArrowUp",   // 键盘导航
		"Backlinks", // backlinks 面板
		"pinax.explore_bundle.v1",
		"__PINAX_EXPLORE_DATA__",
		"computeLayout", // 手写力导向布局（无外部库）
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("explore page missing %q", want)
		}
	}
	for _, banned := range []string{"d3.", "cytoscape", "unpkg", "cdn.", "googleapis", "fonts.googleapis"} {
		if strings.Contains(body, banned) {
			t.Fatalf("explore page references external library %q", banned)
		}
	}

	noEmbedPage, err := app.RenderExplorePage(buildExplorePageFixtureBundle(t), false)
	if err != nil {
		t.Fatalf("render no-embed page: %v", err)
	}
	if !strings.Contains(string(noEmbedPage), `data-embed="off"`) || strings.Contains(string(noEmbedPage), "note_auth_design") {
		t.Fatalf("no-embed page must not inline bundle data")
	}
}

// explorePageTimestampPattern 归一化时间戳字段后 golden 哈希才稳定。
var explorePageTimestampPattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})`)

func TestExplorePageGoldenDigest(t *testing.T) {
	t.Parallel()
	page, err := app.RenderExplorePage(buildExplorePageFixtureBundle(t), true)
	if err != nil {
		t.Fatalf("render explore page: %v", err)
	}
	normalized := explorePageTimestampPattern.ReplaceAllString(string(page), "<TS>")
	digest := sha256.Sum256([]byte(normalized))
	got := "sha256:" + hex.EncodeToString(digest[:])
	const want = "sha256:df6a4a4478fad6ee30f9ad98e8423845fb7b0c379e3a49459f0bc99d829409f2"
	if got != want {
		t.Fatalf("explore page golden digest = %q, want %q", got, want)
	}
}

func clipForLog(value string) string {
	if len(value) > 4000 {
		return value[:4000] + "…"
	}
	return value
}
