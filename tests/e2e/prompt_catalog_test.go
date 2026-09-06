package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/promptbridge/testfixture"
	promptrepo "github.com/yeisme/promptrepo"
)

// TestPromptCatalogRepositoryImportE2E drives the compiled pinax binary
// against real promptrepo file repositories: repository administration,
// provider-free catalog reads, and the rights-gated install into the local
// vault, with body/input/credential sentinels asserted on every output.
func TestPromptCatalogRepositoryImportE2E(t *testing.T) {
	t.Parallel()

	const (
		sentinelBody     = "SENTINEL-PROMPT-BODY-请生成旁白"
		sentinelInput    = "SENTINEL-INPUT-VALUE"
		sentinelCred     = "keychain://SENTINEL-CREDENTIAL-REF"
		importableTitle  = "可导入播客旁白"
		previewOnlyTitle = "仅预览旁白"
		blockedTitle     = "封禁旁白"
		noContractTitle  = "无合同旁白"
		mitLicenseTitle  = "公开许可旁白"
		conflictingTitle = "冲突旁白"
	)

	home := t.TempDir()
	vault := t.TempDir()
	importable := writeRepoFixture(t, "importable", testfixture.RepositoryOptions{
		RepositoryID: "importable", PackageID: "audio", SolutionID: "podcast", Title: importableTitle, Body: sentinelBody,
		WithContract: true, License: "internal", Permissions: []string{"inspect", "local_import"},
		Inputs: []promptrepo.InputDefinition{
			{Name: "subject", Type: promptrepo.InputTypeString, Required: true, Descriptions: map[string]string{"zh-CN": "旁白主题"}},
			{Name: "tone", Type: promptrepo.InputTypeEnum, Enum: []any{"calm", "bright"}, Default: "calm"},
		},
	})
	previewOnly := writeRepoFixture(t, "previewonly", testfixture.RepositoryOptions{
		RepositoryID: "previewonly", PackageID: "audio", SolutionID: "podcast", Title: previewOnlyTitle,
		WithContract: true, License: "internal", Permissions: []string{"inspect", "preview"},
	})
	blocked := writeRepoFixture(t, "blocked", testfixture.RepositoryOptions{
		RepositoryID: "blocked", PackageID: "audio", SolutionID: "podcast", Title: blockedTitle, Rights: "blocked",
		WithContract: true, Permissions: []string{"local_import"},
	})
	noContract := writeRepoFixture(t, "nocontract", testfixture.RepositoryOptions{
		RepositoryID: "nocontract", PackageID: "audio", SolutionID: "podcast", Title: noContractTitle,
		WithContract: false,
	})
	mitLicensed := writeRepoFixture(t, "mitlicensed", testfixture.RepositoryOptions{
		RepositoryID: "mitlicensed", PackageID: "audio", SolutionID: "podcast-public", Title: mitLicenseTitle,
		WithContract: true, License: "MIT", Permissions: []string{"copy"},
		Inputs: []promptrepo.InputDefinition{{Name: "subject", Type: promptrepo.InputTypeString, Required: true}},
	})
	conflicting := writeRepoFixture(t, "conflicting", testfixture.RepositoryOptions{
		RepositoryID: "conflicting", PackageID: "audio", SolutionID: "podcast", Title: conflictingTitle, Body: sentinelBody + "-v2",
		WithContract: true, License: "internal", Permissions: []string{"local_import"},
		Inputs: []promptrepo.InputDefinition{{Name: "subject", Type: promptrepo.InputTypeString, Required: true}},
	})

	pinaxBin := filepath.Join(sharedBinDir, "pinax")
	run := func(args ...string) map[string]any {
		t.Helper()
		out, err := runPinaxWithRepoEnv(t, pinaxBin, home, args...)
		if err != nil {
			t.Fatalf("pinax %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
		return parseCatalogEnvelope(t, out)
	}
	runFail := func(args ...string) (map[string]any, string) {
		t.Helper()
		out, _ := runPinaxWithRepoEnv(t, pinaxBin, home, args...)
		return parseCatalogEnvelope(t, out), out
	}

	// Repository administration through the shared store.
	for _, repo := range []string{importable, previewOnly, blocked, noContract, mitLicensed, conflicting} {
		envelope := run("prompt", "repository", "add", repoID(repo), "--source", repo, "--trust", "verified", "--credential-ref", sentinelCred, "--json")
		if envelope["status"] != "success" {
			t.Fatalf("repository add envelope = %#v", envelope)
		}
	}
	listEnvelope := run("prompt", "repository", "list", "--json")
	if listEnvelope["facts"].(map[string]any)["results"] != "6" {
		t.Fatalf("repository list facts = %#v", listEnvelope["facts"])
	}
	if strings.Contains(fmt.Sprint(listEnvelope), sentinelCred) {
		t.Fatal("repository list leaked credential ref")
	}
	if _, err := runPinaxWithRepoEnv(t, pinaxBin, home, "prompt", "repository", "sync", "--all", "--json"); err != nil {
		t.Fatalf("repository sync failed: %v", err)
	}
	doctorEnvelope := run("prompt", "repository", "doctor", "importable", "--json")
	if doctorEnvelope["facts"].(map[string]any)["state"] != "ready" {
		t.Fatalf("doctor facts = %#v", doctorEnvelope["facts"])
	}
	disableEnvelope := run("prompt", "repository", "disable", "nocontract", "--json")
	if disableEnvelope["facts"].(map[string]any)["enabled"] != "false" {
		t.Fatalf("disable facts = %#v", disableEnvelope["facts"])
	}
	if _, err := runPinaxWithRepoEnv(t, pinaxBin, home, "prompt", "repository", "enable", "nocontract", "--json"); err != nil {
		t.Fatalf("repository enable failed: %v", err)
	}

	// Catalog search stays federated; local `prompt search` stays vault-only.
	searchEnvelope := run("prompt", "catalog", "search", "旁白", "--json")
	if searchEnvelope["command"] != "prompt.catalog.search" {
		t.Fatalf("search command = %#v", searchEnvelope)
	}
	facts := searchEnvelope["facts"].(map[string]any)
	// pinax-en-prompt-template-default-v1 把 catalog 默认 locale 从 zh-CN 迁到 en；
	// 未显式传 --locale 的 federated search 报告请求 locale（en），卡片仍是仓库
	// 默认 locale（zh-CN）。
	if facts["results"] != "6" || facts["locale"] != "en" || facts["provider_calls"] != "0" || facts["durable_writes"] != "0" {
		t.Fatalf("search facts = %#v", facts)
	}
	if facts["operation_id"] != "promptrepo.catalog.search.v1" {
		t.Fatalf("search operation_id = %#v", facts["operation_id"])
	}
	localSearch, _ := runPinaxWithRepoEnv(t, pinaxBin, home, "prompt", "search", "旁白", "--vault", vault, "--json")
	localFacts := parseCatalogEnvelope(t, localSearch)["facts"].(map[string]any)
	if localFacts["results"] != "0" {
		t.Fatalf("local prompt search must stay vault-only, facts = %#v", localFacts)
	}

	// Session scope restriction: pins and deny-wins over the federated set.
	scopedOut, _ := runPinaxWithRepoEnv(t, pinaxBin, home, "prompt", "catalog", "search", "旁白", "--repository", "importable", "--deny", "importable", "--json")
	scoped := parseCatalogEnvelope(t, scopedOut)
	if scoped["facts"].(map[string]any)["results"] != "0" {
		t.Fatalf("deny-wins must exclude the pinned repository: %#v", scoped["facts"])
	}

	importableRef := "promptrepo://importable/audio/podcast@1.0.0?locale=zh-CN"

	// Show/resolve expose catalog metadata only.
	showEnvelope := run("prompt", "catalog", "show", importableRef, "--json")
	showFacts := showEnvelope["facts"].(map[string]any)
	if showFacts["title"] != importableTitle || showFacts["rights"] != "internal" || showFacts["snapshot_digest"] == "" {
		t.Fatalf("show facts = %#v", showFacts)
	}
	resolveAgent, _ := runPinaxWithRepoEnv(t, pinaxBin, home, "prompt", "catalog", "resolve", importableRef, "--agent")
	for _, want := range []string{"command=prompt.catalog.resolve", "fact.rights=internal", "fact.snapshot_digest=", "action.inspect="} {
		if !strings.Contains(resolveAgent, want) {
			t.Fatalf("resolve agent output missing %q:\n%s", want, resolveAgent)
		}
	}

	// Inspect is provider-free and contract-backed.
	inspectEnvelope := run("prompt", "catalog", "inspect", importableRef, "--json")
	inspectFacts := inspectEnvelope["facts"].(map[string]any)
	if inspectFacts["contract_verified"] != "true" || inspectFacts["provider_calls"] != "0" || inspectFacts["durable_writes"] != "0" {
		t.Fatalf("inspect facts = %#v", inspectFacts)
	}
	if inspectFacts["next_action"] != "supply_inputs" || inspectFacts["missing_inputs"] != "subject" {
		t.Fatalf("inspect readiness = %#v", inspectFacts)
	}

	// Validate reads values from a file; values never appear in output.
	valuesFile := filepath.Join(t.TempDir(), "values.json")
	if err := os.WriteFile(valuesFile, []byte(fmt.Sprintf(`{"subject": %q, "tone": "bright"}`, sentinelInput)), 0o600); err != nil {
		t.Fatal(err)
	}
	validateEnvelope := run("prompt", "catalog", "validate", importableRef, "--values", valuesFile, "--json")
	validateFacts := validateEnvelope["facts"].(map[string]any)
	if validateFacts["ready"] != "true" || validateFacts["issues"] != "0" {
		t.Fatalf("validate facts = %#v", validateFacts)
	}
	invalidValues := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(invalidValues, []byte(`{"subject": "ok", "tone": "nope"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	invalidEnvelope := run("prompt", "catalog", "validate", importableRef, "--values", invalidValues, "--json")
	if invalidEnvelope["facts"].(map[string]any)["ready"] != "false" {
		t.Fatalf("invalid validate facts = %#v", invalidEnvelope["facts"])
	}

	// Preview renders in memory: digest facts, zero provider calls, no body.
	previewEnvelope := run("prompt", "catalog", "preview", importableRef, "--values", valuesFile, "--json")
	previewFacts := previewEnvelope["facts"].(map[string]any)
	if previewFacts["ready"] != "true" || previewFacts["rendered_digest"] == "" || previewFacts["provider_calls"] != "0" {
		t.Fatalf("preview facts = %#v", previewFacts)
	}

	// Install plan without --yes performs no durable write.
	planEnvelope := run("prompt", "catalog", "install", importableRef, "--vault", vault, "--json")
	planFacts := planEnvelope["facts"].(map[string]any)
	if planFacts["prompt_asset_id"] != "catalog_audio_podcast" || planFacts["durable_writes"] != "0" || planFacts["lifecycle"] != "draft" {
		t.Fatalf("install plan facts = %#v", planFacts)
	}

	// Preview-only permissions fail closed with a rights reason.
	previewOnlyRef := "promptrepo://previewonly/audio/podcast@1.0.0?locale=zh-CN"
	rightsEnvelope, rightsOut := runFail("prompt", "catalog", "install", previewOnlyRef, "--vault", vault, "--yes", "--json")
	if rightsEnvelope["error"].(map[string]any)["code"] != "prompt_install_rights_blocked" {
		t.Fatalf("preview-only install envelope = %#v\n%s", rightsEnvelope, rightsOut)
	}
	blockedRef := "promptrepo://blocked/audio/podcast@1.0.0?locale=zh-CN"
	blockedEnvelope, _ := runFail("prompt", "catalog", "install", blockedRef, "--vault", vault, "--yes", "--json")
	if blockedEnvelope["error"].(map[string]any)["code"] != "prompt_install_rights_blocked" {
		t.Fatalf("blocked install envelope = %#v", blockedEnvelope)
	}
	noContractRef := "promptrepo://nocontract/audio/podcast@1.0.0?locale=zh-CN"
	noContractEnvelope, _ := runFail("prompt", "catalog", "install", noContractRef, "--vault", vault, "--yes", "--json")
	if noContractEnvelope["error"].(map[string]any)["code"] != "prompt_install_rights_blocked" {
		t.Fatalf("unverified contract install envelope = %#v", noContractEnvelope)
	}

	// Confirmed install creates a draft with provenance refs and no promotion.
	installEnvelope := run("prompt", "catalog", "install", importableRef, "--vault", vault, "--yes", "--json")
	installFacts := installEnvelope["facts"].(map[string]any)
	if installFacts["prompt_asset_id"] != "catalog_audio_podcast" || installFacts["lifecycle"] != "draft" || installFacts["durable_writes"] != "1" || installFacts["permission"] != "internal" {
		t.Fatalf("install facts = %#v", installFacts)
	}
	if installFacts["stage_receipt_id"] == "" {
		t.Fatalf("install facts missing stage receipt: %#v", installFacts)
	}
	data := installEnvelope["data"].(map[string]any)
	refs, _ := data["source_refs"].([]any)
	if len(refs) != 3 {
		t.Fatalf("install source refs = %#v", data["source_refs"])
	}

	// The installed draft resolves through the local vault URI and keeps the
	// body inside the vault; lifecycle stays draft.
	localShow, _ := runPinaxWithRepoEnv(t, pinaxBin, home, "prompt", "show", "catalog_audio_podcast", "--vault", vault, "--json")
	if parseCatalogEnvelope(t, localShow)["facts"].(map[string]any)["lifecycle"] != "draft" {
		t.Fatalf("installed asset must stay draft: %s", localShow)
	}

	// Idempotent install of the same digest reports already_installed.
	againEnvelope := run("prompt", "catalog", "install", importableRef, "--vault", vault, "--yes", "--json")
	if againEnvelope["facts"].(map[string]any)["already_installed"] != "true" {
		t.Fatalf("idempotent install facts = %#v", againEnvelope["facts"])
	}

	// Same local ID with different content conflicts without writing; --fork
	// installs side-by-side and leaves the existing version unchanged.
	conflictRef := "promptrepo://conflicting/audio/podcast@1.0.0?locale=zh-CN"
	conflictEnvelope, _ := runFail("prompt", "catalog", "install", conflictRef, "--vault", vault, "--yes", "--json")
	conflictFacts := conflictEnvelope["facts"].(map[string]any)
	if conflictEnvelope["error"].(map[string]any)["code"] != "prompt_install_conflict" || conflictFacts["conflict"] != "true" || conflictFacts["selected_plan"] != "reject" {
		t.Fatalf("conflict envelope = %#v", conflictEnvelope)
	}
	localShowAfterConflict, _ := runPinaxWithRepoEnv(t, pinaxBin, home, "prompt", "show", "catalog_audio_podcast", "--vault", vault, "--json")
	if parseCatalogEnvelope(t, localShowAfterConflict)["facts"].(map[string]any)["lifecycle"] != "draft" {
		t.Fatalf("conflict must keep the existing asset unchanged: %s", localShowAfterConflict)
	}
	forkEnvelope := run("prompt", "catalog", "install", conflictRef, "--vault", vault, "--yes", "--fork", "--json")
	forkFacts := forkEnvelope["facts"].(map[string]any)
	if !strings.HasPrefix(forkFacts["prompt_asset_id"].(string), "catalog_audio_podcast_fork_") {
		t.Fatalf("fork facts = %#v", forkFacts)
	}

	// MIT-licensed copy-granted template maps to a public permission.
	mitRef := "promptrepo://mitlicensed/audio/podcast-public@1.0.0?locale=zh-CN"
	mitEnvelope := run("prompt", "catalog", "install", mitRef, "--vault", vault, "--yes", "--json")
	if mitEnvelope["facts"].(map[string]any)["permission"] != "public" {
		t.Fatalf("MIT install facts = %#v", mitEnvelope["facts"])
	}

	// Repository removal keeps local drafts intact.
	removeEnvelope := run("prompt", "repository", "remove", "mitlicensed", "--json")
	if removeEnvelope["status"] != "success" {
		t.Fatalf("remove envelope = %#v", removeEnvelope)
	}
	localShowAfterRemove, _ := runPinaxWithRepoEnv(t, pinaxBin, home, "prompt", "show", "catalog_audio_podcast", "--vault", vault, "--json")
	if parseCatalogEnvelope(t, localShowAfterRemove)["status"] != "success" {
		t.Fatalf("local drafts must survive repository removal: %s", localShowAfterRemove)
	}

	// Sentinel scan across every raw output produced in this test: template
	// bodies, input values, and credential refs never appear.
	t.Run("sentinels", func(t *testing.T) {
		outputs, err := runPinaxWithRepoEnv(t, pinaxBin, home, "prompt", "catalog", "search", "旁白", "--agent")
		if err != nil {
			t.Fatalf("agent search failed: %v", err)
		}
		for _, sentinel := range []string{sentinelBody, sentinelInput, sentinelCred} {
			if strings.Contains(outputs, sentinel) {
				t.Fatalf("agent output leaked sentinel %q:\n%s", sentinel, outputs)
			}
		}
	})
}

func writeRepoFixture(t *testing.T, name string, options testfixture.RepositoryOptions) string {
	t.Helper()
	root := t.TempDir()
	source, err := testfixture.WriteRepository(filepath.Join(root, name), options)
	if err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
	return source
}

func repoID(source string) string {
	// file:///<tmp>/<name> → <name>
	trimmed := strings.TrimPrefix(source, "file://")
	return filepath.Base(trimmed)
}

func runPinaxWithRepoEnv(t *testing.T, bin, home string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(),
		"PROMPTREPO_HOME="+home,
		"PROMPTREPO_CACHE="+filepath.Join(home, "cache"),
		"NO_COLOR=1",
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func parseCatalogEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("output is not a JSON envelope: %v\n%s", err, out)
	}
	return envelope
}
