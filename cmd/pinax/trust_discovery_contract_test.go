package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// trustBodySentinel 标记 note 正文；信任/发现新面的有界投影在任何嵌套深度
// 都不得泄漏它（body 红线不变）。
const trustBodySentinel = "TRUST_BODY_SENTINEL_MUST_NOT_LEAK"

func trustContractVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "auth.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_auth\ntitle: Auth Contract\ntags: [auth]\nkind: reference\ndescription: bounded description only\nverified:\n  - by: human:ye\n    at: 2026-09-06T10:30:00+00:00\nstale_after: 2020-01-01T00:00:00+00:00\n---\n\n# Auth Contract\n\n"+trustBodySentinel+"\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "plain.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_plain\ntitle: Plain Contract\ntags: [auth]\nkind: runbook\n---\n\n# Plain Contract\n\n"+trustBodySentinel+"\n")
	runCLI(t, "index", "refresh", "--vault", root, "--json")
	return root
}

func TestTrustDiscoveryOutputContractCLI(t *testing.T) {
	t.Parallel()
	root := trustContractVault(t)

	surfaces := []struct {
		name string
		args []string
	}{
		{"search_facets", []string{"search", "auth", "--facets", "--vault", root, "--json"}},
		{"search_trust", []string{"search", "auth", "--trust", "human", "--vault", root, "--json"}},
		{"search_stale", []string{"search", "auth", "--stale", "only", "--vault", root, "--json"}},
		{"search_show", []string{"search", "show", "notes/auth.md", "--vault", root, "--json"}},
		{"browse", []string{"browse", "notes", "--lazy-index", "off", "--vault", root, "--json"}},
	}
	for _, surface := range surfaces {
		out := runCLI(t, surface.args...)
		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatalf("%s --json is not a single JSON envelope: %v\n%s", surface.name, err, out)
		}
		for _, key := range []string{"spec_version", "mode", "command", "status"} {
			if value, ok := envelope[key].(string); !ok || value == "" {
				t.Fatalf("%s --json envelope missing %q:\n%s", surface.name, key, out)
			}
		}
		assertNoRecursiveBodyLeak(t, surface.name+"/json", []byte(out))
		// search 的正文红线是"有界 snippet"：正文字段绝不出现，但允许有界摘录。
		if strings.HasPrefix(surface.name, "search") && surface.name != "search_show" {
			assertSentinelOnlyInSnippets(t, surface.name, out)
		} else if strings.Contains(out, trustBodySentinel) {
			t.Fatalf("%s --json leaked body sentinel:\n%s", surface.name, out)
		}
	}

	// agent 模式：稳定 key=value 且零 body 泄漏。
	agentSurfaces := []struct {
		name string
		args []string
		want []string
	}{
		{"search_facets", []string{"search", "auth", "--facets", "--vault", root, "--agent"}, []string{"command=note.search", "facets.tag.auth=2", "result.1.trust="}},
		{"search_show", []string{"search", "show", "notes/auth.md", "--vault", root, "--agent"}, []string{"command=search.show", "detail.trust_tier=human", "detail.fresh=stale"}},
		{"browse", []string{"browse", "notes", "--lazy-index", "off", "--vault", root, "--agent"}, []string{"command=browse", "note.1.trust=", "note.1.fresh="}},
	}
	for _, surface := range agentSurfaces {
		out := runCLI(t, surface.args...)
		for _, want := range surface.want {
			if !strings.Contains(out, want) {
				t.Fatalf("%s agent output missing %q:\n%s", surface.name, want, out)
			}
		}
		if surface.name == "search_facets" {
			assertAgentSentinelOnlyInSnippets(t, surface.name, out)
		} else if strings.Contains(out, trustBodySentinel) {
			t.Fatalf("%s agent output leaked body sentinel:\n%s", surface.name, out)
		}
		assertMachineOutputClean(t, out)
	}

	// events 模式：单阶段 NDJSON 事件流。
	for _, surface := range []struct {
		name string
		args []string
	}{
		{"search_facets", []string{"search", "auth", "--facets", "--vault", root, "--events"}},
		{"search_show", []string{"search", "show", "notes/auth.md", "--vault", root, "--events"}},
		{"browse", []string{"browse", "notes", "--vault", root, "--events"}},
	} {
		out := runCLI(t, surface.args...)
		events := parseNDJSONEvents(t, out)
		if len(events) == 0 {
			t.Fatalf("%s events output empty:\n%s", surface.name, out)
		}
		if strings.Contains(out, trustBodySentinel) {
			t.Fatalf("%s events output leaked body sentinel:\n%s", surface.name, out)
		}
	}
}

// assertSentinelOnlyInSnippets 保证正文 sentinel 只出现在有界 snippet 值内。
func assertSentinelOnlyInSnippets(t *testing.T, name, out string) {
	t.Helper()
	var envelope struct {
		Data struct {
			Results []map[string]any `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("%s: cannot parse envelope: %v\n%s", name, err, out)
	}
	for _, result := range envelope.Data.Results {
		for key, value := range result {
			if key == "snippet" {
				continue
			}
			if strings.Contains(fmt.Sprint(value), trustBodySentinel) {
				t.Fatalf("%s: body sentinel leaked outside snippet field (%s):\n%s", name, key, out)
			}
		}
	}
}

// assertAgentSentinelOnlyInSnippets 保证 agent 输出中 sentinel 只出现在 result.N.snippet 行。
func assertAgentSentinelOnlyInSnippets(t *testing.T, name, out string) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, trustBodySentinel) {
			continue
		}
		if !strings.HasPrefix(line, "result.") || !strings.Contains(line, ".snippet=") {
			t.Fatalf("%s: sentinel leaked outside result.N.snippet line: %q", name, line)
		}
	}
}

func TestNoteVerifyOutputModesContractCLI(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	created := runCLI(t, "note", "new", "Verify Contract", "--body", trustBodySentinel, "--vault", root, "--json")
	path := jsonParseFacts(t, created)["path"].(string)

	jsonOut := runCLI(t, "note", "verify", path, "--actor", "human:ye", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("verify json invalid: %v\n%s", err, jsonOut)
	}
	if envelope["command"] != "note.verify" || envelope["status"] != "success" {
		t.Fatalf("verify envelope = %#v", envelope)
	}
	assertNoRecursiveBodyLeak(t, "note.verify/json", []byte(jsonOut))

	agentOut := runCLI(t, "note", "verify", path, "--actor", "human:ye", "--vault", root, "--agent")
	for _, want := range []string{"command=note.verify", "fact.trust=human", "fact.idempotent=true"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("verify agent output missing %q:\n%s", want, agentOut)
		}
	}
	if strings.Contains(agentOut, trustBodySentinel) {
		t.Fatalf("verify agent output leaked body sentinel:\n%s", agentOut)
	}
	assertMachineOutputClean(t, agentOut)
}
