package app

import (
	"strings"
	"testing"
)

func TestExploreExternalRefScannerCatchesMarkupButNotScriptData(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		page    string
		leaks   []string
	}{
		{
			name: "clean self-contained page",
			page: "<!doctype html><html><head><style>body{background:#fff}</style></head><body><svg></svg></body></html>",
		},
		{
			name: "script string mentioning external url is not an attribute",
			page: `<script>window.__D__ = JSON.parse("{\"title\":\"x href='https://evil.example/y'\"}");</script>`,
		},
		{
			name: "img src external caught",
			page: `<html><body><img src="https://cdn.example/logo.png"></body></html>`,
			leaks: []string{"src=https://cdn.example/logo.png"},
		},
		{
			name: "link href external caught",
			page: `<html><head><link rel="stylesheet" href='http://fonts.example/x.css'></head></html>`,
			leaks: []string{"href=http://fonts.example/x.css"},
		},
		{
			name: "protocol relative caught",
			page: `<html><script src="//cdn.example/lib.js"></script></html>`,
			leaks: []string{"src=//cdn.example/lib.js"},
		},
		{
			name: "relative and hash refs pass",
			page: `<html><a href="#q=auth">f</a><img src="pinax-data/x.png"></html>`,
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got := ScanExploreExternalRefs([]byte(testCase.page))
			if strings.Join(got, "|") != strings.Join(testCase.leaks, "|") {
				t.Fatalf("leaks = %#v want %#v", got, testCase.leaks)
			}
		})
	}
}

func TestExplorePageRendersEmbedAndNoEmbed(t *testing.T) {
	t.Parallel()
	bundle := ExploreBundle{
		SchemaVersion: ExploreBundleSchemaVersion,
		GeneratedAt:   "2026-09-06T00:00:00Z",
		Counts:        ExploreBundleCounts{Nodes: 1, Edges: 0},
		Nodes:         []ExploreNode{{ID: "note_a", Title: "Alpha", Kind: "reference", Trust: "human", Fresh: "fresh"}},
		Edges:         []ExploreEdge{},
	}
	embedded, err := RenderExplorePage(bundle, true)
	if err != nil {
		t.Fatalf("render embedded page: %v", err)
	}
	if !strings.Contains(string(embedded), `data-embed="on"`) || !strings.Contains(string(embedded), "note_a") {
		t.Fatalf("embedded page must inline bundle data")
	}
	standalone, err := RenderExplorePage(bundle, false)
	if err != nil {
		t.Fatalf("render no-embed page: %v", err)
	}
	if !strings.Contains(string(standalone), `data-embed="off"`) || strings.Contains(string(standalone), "note_a") {
		t.Fatalf("no-embed page must not inline bundle data")
	}
	for _, page := range [][]byte{embedded, standalone} {
		if refs := ScanExploreExternalRefs(page); len(refs) != 0 {
			t.Fatalf("page leaked external refs: %v", refs)
		}
	}
}
