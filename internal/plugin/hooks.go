package plugin

type HookRegistration struct {
	PluginID     string `json:"plugin_id"`
	CapabilityID string `json:"capability_id"`
	Kind         string `json:"kind"`
	Target       string `json:"target"`
	DirectWrite  bool   `json:"direct_write,omitempty"`
}

type HookRegistry struct {
	hooks []HookRegistration
}

func NewHookRegistry() *HookRegistry { return &HookRegistry{hooks: []HookRegistration{}} }

func (r *HookRegistry) Register(plugin RegistryPlugin, hook HookRegistration) error {
	if !plugin.Enabled {
		return &RunnerError{Code: "plugin_disabled", Message: "Plugin hook registration requires an enabled plugin"}
	}
	if hook.PluginID == "" {
		hook.PluginID = plugin.ID
	}
	if hook.PluginID != plugin.ID {
		return &RunnerError{Code: "plugin_capability_conflict", Message: "Plugin hook id does not match installed plugin"}
	}
	if err := validateHookKind(hook.Kind); err != nil {
		return err
	}
	if hook.DirectWrite {
		return &RunnerError{Code: "plugin_direct_write_denied", Message: "Plugin hooks may only return action plans for writes"}
	}
	if builtinQuerySource(hook.Kind, hook.Target) {
		return &RunnerError{Code: "plugin_capability_conflict", Message: "Plugin hook cannot replace a built-in query source"}
	}
	for _, existing := range r.hooks {
		if existing.Kind == hook.Kind && existing.Target == hook.Target {
			return &RunnerError{Code: "plugin_capability_conflict", Message: "Plugin hook target is already registered"}
		}
	}
	r.hooks = append(r.hooks, hook)
	return nil
}

func (r *HookRegistry) QuerySources() []HookRegistration {
	return r.byKind("query.source.read")
}

func (r *HookRegistry) TemplateFunctions() []HookRegistration {
	return r.byKind("template.function")
}

func (r *HookRegistry) Diagnostics() []HookRegistration {
	return r.byKind("diagnostic.rule")
}

func (r *HookRegistry) byKind(kind string) []HookRegistration {
	out := []HookRegistration{}
	for _, hook := range r.hooks {
		if hook.Kind == kind {
			out = append(out, hook)
		}
	}
	return out
}

func validateHookKind(kind string) error {
	switch kind {
	case "query.source.read", "template.function", "export.render", "diagnostic.rule", "note.action_plan":
		return nil
	default:
		return &RunnerError{Code: "plugin_capability_invalid", Message: "Plugin hook kind is not supported"}
	}
}

func builtinQuerySource(kind, target string) bool {
	if kind != "query.source.read" {
		return false
	}
	switch target {
	case "notes", "tasks", "links", "backlinks", "assets":
		return true
	default:
		return false
	}
}
