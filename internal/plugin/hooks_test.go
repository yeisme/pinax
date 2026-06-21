package plugin

import "testing"

func TestPluginHookRegistryAcceptsReadOnlyExtensionPoints(t *testing.T) {
	registry := NewHookRegistry()
	plugin := RegistryPlugin{ID: "reader", Enabled: true}
	for _, hook := range []HookRegistration{
		{PluginID: plugin.ID, CapabilityID: "csv_rows", Kind: "query.source.read", Target: "csv.rows"},
		{PluginID: plugin.ID, CapabilityID: "slugify", Kind: "template.function", Target: "slugify"},
		{PluginID: plugin.ID, CapabilityID: "vault_rule", Kind: "diagnostic.rule", Target: "vault.rule"},
	} {
		if err := registry.Register(plugin, hook); err != nil {
			t.Fatalf("register hook %#v: %v", hook, err)
		}
	}
	if len(registry.QuerySources()) != 1 || len(registry.TemplateFunctions()) != 1 || len(registry.Diagnostics()) != 1 {
		t.Fatalf("registry = %#v", registry)
	}
}

func TestPluginHookRegistryRejectsBuiltinSourceConflict(t *testing.T) {
	registry := NewHookRegistry()
	err := registry.Register(RegistryPlugin{ID: "bad", Enabled: true}, HookRegistration{PluginID: "bad", CapabilityID: "notes", Kind: "query.source.read", Target: "notes"})
	if RunnerErrorCode(err) != "plugin_capability_conflict" {
		t.Fatalf("conflict err = %v", err)
	}
}

func TestPluginHookRegistryWriteHooksAreActionPlansOnly(t *testing.T) {
	registry := NewHookRegistry()
	err := registry.Register(RegistryPlugin{ID: "writer", Enabled: true}, HookRegistration{PluginID: "writer", CapabilityID: "note_plan", Kind: "note.action_plan", Target: "note.update", DirectWrite: true})
	if RunnerErrorCode(err) != "plugin_direct_write_denied" {
		t.Fatalf("direct write err = %v", err)
	}
	if err := registry.Register(RegistryPlugin{ID: "writer", Enabled: true}, HookRegistration{PluginID: "writer", CapabilityID: "note_plan", Kind: "note.action_plan", Target: "note.update"}); err != nil {
		t.Fatalf("action plan hook should register: %v", err)
	}
}
