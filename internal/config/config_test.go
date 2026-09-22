package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigOmitsVectorRuntime(t *testing.T) {
	cfg := DefaultConfig()
	if _, ok := Value(cfg, "kb.sidecar.executable"); ok {
		t.Fatal("removed KB sidecar setting should not be readable")
	}
	if cfg.Output.Style != "table" {
		t.Fatalf("output style = %q, want table", cfg.Output.Style)
	}
}

func TestOutputStyleLoadsFromEnvAndExplicitFlag(t *testing.T) {
	root := t.TempDir()
	result, err := Load(LoadOptions{VaultPath: root, Env: mapEnv(map[string]string{"PINAX_OUTPUT_STYLE": "compact"}), ExplicitFlags: map[string]string{}})
	if err != nil {
		t.Fatalf("load compact output style: %v", err)
	}
	if result.Config.Output.Style != "compact" || !result.Sources.Contains("PINAX_OUTPUT_STYLE") {
		t.Fatalf("env output style = %#v sources=%#v", result.Config.Output, result.Sources)
	}
	result, err = Load(LoadOptions{VaultPath: root, Env: mapEnv(map[string]string{}), ExplicitFlags: map[string]string{"output.style": "table"}})
	if err != nil {
		t.Fatalf("load table output style: %v", err)
	}
	if result.Config.Output.Style != "table" || !result.Sources.Contains("output.style") {
		t.Fatalf("flag output style = %#v sources=%#v", result.Config.Output, result.Sources)
	}
}

func TestLoadMergesDefaultsUserProjectEnvAndExplicitFlags(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "user.yaml")
	project := filepath.Join(root, "vault", ".pinax", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(project), 0o755); err != nil {
		t.Fatalf("mkdir project config: %v", err)
	}
	writeConfigFixture(t, user, "output:\n  theme: mono\n  color: never\nsearch:\n  limit: 33\n")
	writeConfigFixture(t, project, "output:\n  width: 120\n  markdown:\n    enabled: false\neditor:\n  command: code --wait\n")

	result, err := Load(LoadOptions{
		VaultPath:         filepath.Join(root, "vault"),
		UserConfigPath:    user,
		ProjectConfigPath: project,
		Env: mapEnv(map[string]string{
			"PINAX_OUTPUT_COLOR": "auto",
			"PINAX_SEARCH_LIMIT": "25",
			"EDITOR":             "vim",
		}),
		ExplicitFlags: map[string]string{"output.color": "always"},
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg := result.Config
	if cfg.Output.Color != "always" || cfg.Output.Theme != "mono" || cfg.Output.Width != 120 || cfg.Output.Markdown.Enabled != false || cfg.Search.Limit != 25 || cfg.Editor.Command != "code --wait" {
		t.Fatalf("merged config = %#v", cfg)
	}
	for _, want := range []string{user, project, "PINAX_OUTPUT_COLOR", "PINAX_SEARCH_LIMIT", "output.color"} {
		if !result.Sources.Contains(want) {
			t.Fatalf("sources missing %q: %#v", want, result.Sources)
		}
	}
}
func TestLoadMergesRemoteAPIURLFromConfigEnvAndFlags(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "user.yaml")
	project := filepath.Join(root, "vault", ".pinax", "config.yaml")
	writeConfigFixture(t, user, "remote:\n  api_url: http://user.example.test:8787\n")
	writeConfigFixture(t, project, "remote:\n  api_url: http://project.example.test:8787\n")

	result, err := Load(LoadOptions{
		VaultPath:         filepath.Join(root, "vault"),
		UserConfigPath:    user,
		ProjectConfigPath: project,
		Env:               mapEnv(map[string]string{"PINAX_API_URL": "http://env.example.test:8787"}),
		ExplicitFlags:     map[string]string{"remote.api_url": "http://flag.example.test:8787"},
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if result.Config.Remote.APIURL != "http://flag.example.test:8787" {
		t.Fatalf("remote api url = %q", result.Config.Remote.APIURL)
	}
	for _, want := range []string{user, project, "PINAX_API_URL", "remote.api_url"} {
		if !result.Sources.Contains(want) {
			t.Fatalf("sources missing %q: %#v", want, result.Sources)
		}
	}
}

func TestConnectionModePrecedenceIsFlagEnvProjectUser(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "user.yaml")
	project := filepath.Join(root, "vault", ".pinax", "config.yaml")
	writeConfigFixture(t, user, "remote:\n  api_url: https://user.example.test\n  mode: remote-service\n")
	writeConfigFixture(t, project, "remote:\n  api_url: https://project.example.test\n  mode: self-hosted-service\n")

	result, err := Load(LoadOptions{
		VaultPath:         filepath.Join(root, "vault"),
		UserConfigPath:    user,
		ProjectConfigPath: project,
		Env: mapEnv(map[string]string{
			"PINAX_API_URL":         "https://env.example.test",
			"PINAX_CONNECTION_MODE": "remote-service",
		}),
		ExplicitFlags: map[string]string{
			"remote.api_url": "https://flag.example.test",
			"remote.mode":    "self-hosted-service",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Config.Remote.APIURL != "https://flag.example.test" || result.Config.Remote.Mode != "self-hosted-service" {
		t.Fatalf("remote config = %#v", result.Config.Remote)
	}
	modeSource := ""
	for _, setting := range result.Settings {
		if setting.Key == "remote.mode" {
			modeSource = setting.Source
		}
	}
	if modeSource != "flag" {
		t.Fatalf("remote.mode source = %q", modeSource)
	}
}

func TestConnectionModeRejectsUnknownValueAndRemoteWithoutEndpoint(t *testing.T) {
	for _, body := range []string{
		"remote:\n  mode: guessed-service\n",
		"remote:\n  mode: remote-service\n",
	} {
		root := t.TempDir()
		user := filepath.Join(root, "config.yaml")
		writeConfigFixture(t, user, body)
		if _, err := Load(LoadOptions{VaultPath: root, UserConfigPath: user, Env: mapEnv(map[string]string{})}); err == nil || ErrorCode(err) != "config_invalid" {
			t.Fatalf("body=%q error=%v", body, err)
		}
	}
}

func TestValidateRejectsInvalidRemoteAPIURL(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "config.yaml")
	writeConfigFixture(t, user, "remote:\n  api_url: ftp://example.test\n")

	_, err := Load(LoadOptions{VaultPath: root, UserConfigPath: user, Env: mapEnv(map[string]string{})})
	if err == nil || ErrorCode(err) != "config_invalid" {
		t.Fatalf("remote api url error = %q, err = %v", ErrorCode(err), err)
	}
}

func TestFlagDefaultsDoNotOverrideConfigFiles(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "config.yaml")
	writeConfigFixture(t, user, "output:\n  color: never\n  width: 110\n")
	result, err := Load(LoadOptions{VaultPath: root, UserConfigPath: user, ExplicitFlags: map[string]string{"output.theme": ""}})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if result.Config.Output.Color != "never" || result.Config.Output.Width != 110 || result.Config.Output.Theme != "pinax" {
		t.Fatalf("config defaults overrode files: %#v", result.Config.Output)
	}
}

func TestMissingMarkdownEnabledDoesNotDisableDefault(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "config.yaml")
	writeConfigFixture(t, user, "output:\n  theme: mono\n")

	result, err := Load(LoadOptions{VaultPath: root, UserConfigPath: user, Env: mapEnv(map[string]string{})})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !result.Config.Output.Markdown.Enabled {
		t.Fatalf("missing output.markdown.enabled disabled markdown: %#v", result.Config.Output.Markdown)
	}
}

func TestValidateRejectsSecretLikeConfig(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "config.yaml")
	writeConfigFixture(t, user, "storage:\n  token: raw-secret\n")
	_, err := Load(LoadOptions{VaultPath: root, UserConfigPath: user, Env: mapEnv(map[string]string{})})
	if err == nil || ErrorCode(err) != "config_secret_rejected" {
		t.Fatalf("secret config err = %v", err)
	}
}

func TestValidateRejectsInvalidEnumsColorsAndS3(t *testing.T) {
	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "invalid color mode", body: "output:\n  color: sometimes\n", code: "config_invalid"},
		{name: "invalid custom color", body: "output:\n  theme: custom\nthemes:\n  custom:\n    success: greenish\n", code: "config_invalid"},
		{name: "secret like key", body: "provider:\n  authorization: Bearer abc\n", code: "config_secret_rejected"},
		{name: "secret like value", body: "storage:\n  backend: s3\n  bucket: notes\n  region: us-east-1\n  endpoint: https://example.com/webhook/abc\n", code: "config_secret_rejected"},
		{name: "s3 missing bucket", body: "storage:\n  backend: s3\n  region: us-east-1\n", code: "config_invalid"},
		{name: "s3 missing region", body: "storage:\n  backend: s3\n  bucket: notes\n", code: "config_invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			user := filepath.Join(root, "config.yaml")
			writeConfigFixture(t, user, tt.body)
			_, err := Load(LoadOptions{VaultPath: root, UserConfigPath: user, Env: mapEnv(map[string]string{})})
			if err == nil || ErrorCode(err) != tt.code {
				t.Fatalf("error code = %q, err = %v", ErrorCode(err), err)
			}
		})
	}
}

func TestValidateAcceptsS3AndCustomThemeSafeFields(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "config.yaml")
	writeConfigFixture(t, user, "output:\n  theme: custom\nthemes:\n  custom:\n    success: '#00ff66'\n    danger: bright-red\n    rule: '8'\nstorage:\n  backend: s3\n  bucket: notes\n  region: us-east-1\n  prefix: pinax/\n  endpoint: https://s3.example.com\n  profile: work\n")

	result, err := Load(LoadOptions{VaultPath: root, UserConfigPath: user, Env: mapEnv(map[string]string{})})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if result.Config.Themes.Custom["success"] != "#00ff66" || result.Config.Storage.Profile != "work" {
		t.Fatalf("config = %#v", result.Config)
	}
}

func TestLoadRejectsProjectConfigOutsideVault(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	outside := filepath.Join(root, "outside", "config.yaml")
	writeConfigFixture(t, outside, "output:\n  theme: mono\n")

	_, err := Load(LoadOptions{VaultPath: vault, ProjectConfigPath: outside, Env: mapEnv(map[string]string{})})
	if err == nil || ErrorCode(err) != "config_path_outside_vault" {
		t.Fatalf("project path error = %q, err = %v", ErrorCode(err), err)
	}
}

func TestConfigPathsUseXDGAndProjectVault(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	xdg := filepath.Join(t.TempDir(), "xdg")
	paths := ResolvePaths(PathOptions{HomeDir: home, XDGConfigHome: xdg, VaultPath: filepath.Join(home, "vault")})
	if paths.User != filepath.Join(xdg, "pinax", "config.yaml") || paths.Project != filepath.Join(home, "vault", ".pinax", "config.yaml") {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestJudgmentSwitchDefaultsDormant(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Judgment.Enabled || cfg.Judgment.Mode != JudgmentModeOff {
		t.Fatalf("judgment default must stay dormant: %#v", cfg.Judgment)
	}
	if value, ok := Value(cfg, "judgment.enabled"); !ok || value != "false" {
		t.Fatalf("judgment.enabled value = %q ok=%v", value, ok)
	}
	if value, ok := Value(cfg, "judgment.mode"); !ok || value != "off" {
		t.Fatalf("judgment.mode value = %q ok=%v", value, ok)
	}

	root := t.TempDir()
	result, err := Load(LoadOptions{VaultPath: root, Env: mapEnv(map[string]string{})})
	if err != nil {
		t.Fatalf("load default: %v", err)
	}
	if result.Config.Judgment.Enabled || JudgmentEffectiveMode(result.Config.Judgment.Mode) != JudgmentModeOff {
		t.Fatalf("loaded judgment config must stay dormant: %#v", result.Config.Judgment)
	}
	sources := map[string]SettingProjection{}
	for _, setting := range result.Settings {
		sources[setting.Key] = setting
	}
	for _, key := range []string{"judgment.enabled", "judgment.mode"} {
		setting, ok := sources[key]
		if !ok {
			t.Fatalf("settings projection missing %q", key)
		}
		if setting.Source != "default" || !setting.Writable {
			t.Fatalf("%s setting = %#v", key, setting)
		}
	}
}

func TestJudgmentSwitchMergesFilesEnvAndFlags(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "user.yaml")
	project := filepath.Join(root, "vault", ".pinax", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(project), 0o755); err != nil {
		t.Fatalf("mkdir project config: %v", err)
	}
	writeConfigFixture(t, user, "judgment:\n  enabled: true\n  mode: shadow\n")
	writeConfigFixture(t, project, "judgment:\n  mode: assist\n")

	result, err := Load(LoadOptions{
		VaultPath:         filepath.Join(root, "vault"),
		UserConfigPath:    user,
		ProjectConfigPath: project,
		Env:               mapEnv(map[string]string{"PINAX_JUDGMENT_ENABLED": "true"}),
		ExplicitFlags:     map[string]string{"judgment.mode": "shadow"},
	})
	if err != nil {
		t.Fatalf("load judgment config: %v", err)
	}
	if !result.Config.Judgment.Enabled || result.Config.Judgment.Mode != JudgmentModeShadow {
		t.Fatalf("merged judgment config = %#v", result.Config.Judgment)
	}
	for _, want := range []string{user, project, "PINAX_JUDGMENT_ENABLED", "judgment.mode"} {
		if !result.Sources.Contains(want) {
			t.Fatalf("sources missing %q: %#v", want, result.Sources)
		}
	}
	modeSource := ""
	for _, setting := range result.Settings {
		if setting.Key == "judgment.mode" {
			modeSource = setting.Source
		}
	}
	if modeSource != "flag" {
		t.Fatalf("judgment.mode source = %q, want flag", modeSource)
	}
}

func TestJudgmentSwitchEnvModeOffDisablesEnabledConfig(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "user.yaml")
	writeConfigFixture(t, user, "judgment:\n  enabled: true\n  mode: assist\n")
	result, err := Load(LoadOptions{
		VaultPath:      root,
		UserConfigPath: user,
		Env:            mapEnv(map[string]string{"PINAX_JUDGMENT_MODE": "off"}),
	})
	if err != nil {
		t.Fatalf("load disabled judgment config: %v", err)
	}
	if JudgmentEffectiveMode(result.Config.Judgment.Mode) != JudgmentModeOff {
		t.Fatalf("env off must disable judgment, mode = %q", result.Config.Judgment.Mode)
	}
}

func TestJudgmentSwitchRejectsInvalidCombinations(t *testing.T) {
	tests := []struct {
		name string
		body string
		env  map[string]string
		code string
	}{
		{name: "mode shadow without enabled", body: "judgment:\n  mode: shadow\n", code: "config_invalid"},
		{name: "mode assist without enabled", body: "judgment:\n  mode: assist\n", code: "config_invalid"},
		{name: "unknown mode", body: "judgment:\n  enabled: true\n  mode: auto\n", code: "config_invalid"},
		{name: "env mode without enabled", body: "", env: map[string]string{"PINAX_JUDGMENT_MODE": "shadow"}, code: "config_invalid"},
		{name: "env disable with enabled mode", body: "judgment:\n  enabled: true\n  mode: assist\n", env: map[string]string{"PINAX_JUDGMENT_ENABLED": "false"}, code: "config_invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			user := filepath.Join(root, "config.yaml")
			if tt.body != "" {
				writeConfigFixture(t, user, tt.body)
			}
			env := map[string]string{}
			for k, v := range tt.env {
				env[k] = v
			}
			_, err := Load(LoadOptions{VaultPath: root, UserConfigPath: user, Env: mapEnv(env)})
			if err == nil || ErrorCode(err) != tt.code {
				t.Fatalf("error code = %q, err = %v", ErrorCode(err), err)
			}
		})
	}
}

func TestJudgmentSwitchSetValueParsesTypes(t *testing.T) {
	if parsed, err := parseConfigValue("judgment.enabled", "true"); err != nil || parsed != true {
		t.Fatalf("judgment.enabled true = %#v, %v", parsed, err)
	}
	if parsed, err := parseConfigValue("judgment.enabled", "off"); err != nil || parsed != false {
		t.Fatalf("judgment.enabled off = %#v, %v", parsed, err)
	}
	if _, err := parseConfigValue("judgment.enabled", "maybe"); err == nil || ErrorCode(err) != "config_invalid" {
		t.Fatalf("judgment.enabled maybe must be rejected, got %v", err)
	}
	if parsed, err := parseConfigValue("judgment.mode", JudgmentModeShadow); err != nil || parsed != JudgmentModeShadow {
		t.Fatalf("judgment.mode shadow = %#v, %v", parsed, err)
	}
}

func writeConfigFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mapEnv(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
