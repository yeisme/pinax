package credentials

// Allowlist maps a consumer to the capabilities it is permitted to use from the
// shared credential store. It prevents accidental cross-consumer use; it does
// not claim to isolate a malicious process sharing the same OS user.
type Allowlist map[string]map[string]bool

// Preset returns the named allowlist preset, or nil if the name is unknown.
// The first-party "local-ai" preset allows:
//
//	eikona → image
//	pinax  → embedding
//	aigora → text
func Preset(name string) Allowlist {
	if name == DefaultPreset || name == "local-ai" {
		return Allowlist{
			"eikona": {"image": true},
			"pinax":  {"embedding": true},
			"aigora": {"text": true},
		}
	}
	return nil
}

// Allows reports whether consumer is permitted to use capability. An empty or
// unknown consumer is never allowed.
func (a Allowlist) Allows(consumer, capability string) bool {
	if a == nil {
		return false
	}
	caps, ok := a[consumer]
	if !ok {
		return false
	}
	return caps[capability]
}

// Consumers returns the sorted-in-definition-order consumers in the preset.
func (a Allowlist) Consumers() []string {
	out := make([]string, 0, len(a))
	for c := range a {
		out = append(out, c)
	}
	return out
}
