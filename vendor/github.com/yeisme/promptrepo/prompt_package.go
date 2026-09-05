package promptrepo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/gowebpki/jcs"
	"golang.org/x/text/unicode/norm"
)

// These additive contracts are experimental and do not alter RenderTemplate.
const RecipeSchemaV1 = "promptrepo.recipe.v0.1"
const PromptPackageSchemaV1 = "promptrepo.prompt-package.v0.1"

var packageID = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,79}$`)

type RecipeV1 struct {
	SchemaVersion string                    `json:"schema_version"`
	ID            string                    `json:"id"`
	Steps         []RecipeStepV1            `json:"steps"`
	Presets       map[string]map[string]any `json:"presets,omitempty"`
}

type RecipeStepV1 struct {
	ID        string                    `json:"id"`
	Ref       string                    `json:"ref"`
	Role      string                    `json:"role,omitempty"`
	Locale    string                    `json:"locale,omitempty"`
	DependsOn []string                  `json:"depends_on,omitempty"`
	Bindings  map[string]InputBindingV1 `json:"bindings,omitempty"`
	Output    string                    `json:"output_requirement,omitempty"`
}

type InputBindingV1 struct {
	Kind string `json:"kind"` // field, source, or step_output
	Ref  string `json:"ref"`
}

// RecipeOrder checks the graph and returns a stable topological order.
func RecipeOrder(recipe RecipeV1) ([]string, error) {
	if recipe.SchemaVersion != RecipeSchemaV1 || !packageID.MatchString(recipe.ID) || len(recipe.Steps) == 0 || len(recipe.Steps) > 128 {
		return nil, fmt.Errorf("RECIPE_INVALID: invalid identity or step count")
	}
	steps := map[string]RecipeStepV1{}
	for _, step := range recipe.Steps {
		if !packageID.MatchString(step.ID) || steps[step.ID].ID != "" {
			return nil, fmt.Errorf("RECIPE_INVALID: invalid or duplicate step")
		}
		if _, err := ParseRef(step.Ref); err != nil {
			return nil, fmt.Errorf("RECIPE_INVALID: exact solution ref required")
		}
		for field, binding := range step.Bindings {
			if !inputNamePattern.MatchString(field) || binding.Ref == "" || len(binding.Ref) > 160 {
				return nil, fmt.Errorf("RECIPE_INVALID: invalid binding")
			}
			if binding.Kind != "field" && binding.Kind != "source" && binding.Kind != "step_output" {
				return nil, fmt.Errorf("RECIPE_INVALID: unsupported binding kind")
			}
		}
		steps[step.ID] = step
	}
	visited, active := map[string]bool{}, map[string]bool{}
	order := []string{}
	var visit func(string) error
	visit = func(id string) error {
		if active[id] {
			return fmt.Errorf("RECIPE_CYCLE: cyclic step dependency")
		}
		if visited[id] {
			return nil
		}
		step, exists := steps[id]
		if !exists {
			return fmt.Errorf("RECIPE_INVALID: missing dependency")
		}
		active[id] = true
		deps := append([]string{}, step.DependsOn...)
		for _, binding := range step.Bindings {
			if binding.Kind == "step_output" {
				deps = append(deps, binding.Ref)
			}
		}
		sort.Strings(deps)
		for _, dep := range deps {
			if err := visit(dep); err != nil {
				return err
			}
		}
		active[id], visited[id] = false, true
		order = append(order, id)
		return nil
	}
	ids := make([]string, 0, len(steps))
	for id := range steps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	for name, values := range recipe.Presets {
		if !packageID.MatchString(name) {
			return nil, fmt.Errorf("RECIPE_INVALID: invalid preset")
		}
		for key := range values {
			if len(key) > 160 || key == "" {
				return nil, fmt.Errorf("RECIPE_INVALID: invalid preset field")
			}
		}
	}
	return order, nil
}

type PackageFileV1 struct {
	Digest string `json:"digest"`
	Bytes  int64  `json:"bytes"`
	Kind   string `json:"kind"`
}

type PackageStepV1 struct {
	ID      string   `json:"id"`
	Status  string   `json:"status"` // ready or needs_step_output
	Path    string   `json:"path"`
	Missing []string `json:"missing,omitempty"`
}

type PromptPackageV1 struct {
	SchemaVersion string                   `json:"schema_version"`
	Compiler      string                   `json:"compiler"`
	Recipe        RecipeV1                 `json:"recipe"`
	Steps         []PackageStepV1          `json:"steps"`
	Files         map[string]PackageFileV1 `json:"files"`
	External      []string                 `json:"external_dependencies,omitempty"`
	Digest        string                   `json:"digest"`
}

func PromptBytesDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func CanonicalPromptJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(data)
}

// DecodePromptJSON rejects duplicate keys, deep input and unknown typed fields.
func DecodePromptJSON(body []byte, target any) error {
	if len(body) > 32<<20 {
		return fmt.Errorf("PROMPT_JSON_TOO_LARGE")
	}
	value, err := decodeStrictJSON(body, 64)
	if err != nil {
		return fmt.Errorf("PROMPT_JSON_INVALID")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("PROMPT_JSON_INVALID")
	}
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return fmt.Errorf("PROMPT_JSON_INVALID")
	}
	return nil
}

func PromptPackageDigest(manifest PromptPackageV1) (string, error) {
	manifest.Digest = ""
	body, err := CanonicalPromptJSON(manifest)
	if err != nil {
		return "", err
	}
	return PromptBytesDigest(body), nil
}

// SafePackagePath is intentionally portable across Windows, macOS and Linux.
func SafePackagePath(name string) bool {
	if name == "" || len(name) > 240 || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:\x00\r\n") || !norm.NFC.IsNormalString(name) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.ContainsAny(part, `<>"|?*`) {
			return false
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || regexp.MustCompile(`^(COM|LPT)[0-9]$`).MatchString(base) {
			return false
		}
	}
	return true
}

func ValidatePromptPackage(manifest PromptPackageV1) error {
	order, err := RecipeOrder(manifest.Recipe)
	if err != nil {
		return err
	}
	if manifest.SchemaVersion != PromptPackageSchemaV1 || manifest.Compiler == "" || len(manifest.Files) == 0 || len(manifest.Files) > 4096 || len(manifest.Steps) != len(order) {
		return fmt.Errorf("PACKAGE_INVALID: invalid package metadata")
	}
	seen := map[string]bool{}
	var total int64
	for name, file := range manifest.Files {
		key := strings.ToLower(name)
		if !SafePackagePath(name) || key == "manifest.json" || seen[key] || !sha256DigestPattern.MatchString(file.Digest) || file.Bytes < 0 || file.Bytes > 64<<20 {
			return fmt.Errorf("PACKAGE_INVALID: invalid file entry")
		}
		seen[key] = true
		switch file.Kind {
		case "source", "prompt", "deferred_prompt", "reproduction", "guide":
		default:
			return fmt.Errorf("PACKAGE_INVALID: unsupported file kind")
		}
		total += file.Bytes
	}
	if total > 512<<20 {
		return fmt.Errorf("PACKAGE_TOO_LARGE: package exceeds size limit")
	}
	for index, step := range manifest.Steps {
		if step.ID != order[index] || manifest.Files[step.Path].Digest == "" || (step.Status != "ready" && step.Status != "needs_step_output") || (step.Status == "ready" && len(step.Missing) > 0) || (step.Status == "needs_step_output" && len(step.Missing) == 0) {
			return fmt.Errorf("PACKAGE_INVALID: invalid step projection")
		}
		if (step.Status == "ready" && manifest.Files[step.Path].Kind != "prompt") || (step.Status == "needs_step_output" && manifest.Files[step.Path].Kind != "deferred_prompt") {
			return fmt.Errorf("PACKAGE_INVALID: invalid step file kind")
		}
		seenMissing := map[string]bool{}
		var definition RecipeStepV1
		for _, candidate := range manifest.Recipe.Steps {
			if candidate.ID == step.ID {
				definition = candidate
			}
		}
		for _, field := range step.Missing {
			if seenMissing[field] || definition.Bindings[field].Kind != "step_output" {
				return fmt.Errorf("PACKAGE_INVALID: invalid deferred input")
			}
			seenMissing[field] = true
		}
	}
	digest, err := PromptPackageDigest(manifest)
	if err != nil || digest != manifest.Digest {
		return fmt.Errorf("PACKAGE_DIGEST_MISMATCH: manifest digest does not match")
	}
	return nil
}

func VerifyPromptPackageFiles(manifest PromptPackageV1, files map[string][]byte) error {
	if err := ValidatePromptPackage(manifest); err != nil {
		return err
	}
	if len(files) != len(manifest.Files) {
		return fmt.Errorf("PACKAGE_FILES_MISMATCH: file inventory differs")
	}
	for name, file := range manifest.Files {
		body, ok := files[name]
		if !ok || int64(len(body)) != file.Bytes || PromptBytesDigest(body) != file.Digest {
			return fmt.Errorf("PACKAGE_DIGEST_MISMATCH: file bytes do not match")
		}
	}
	return nil
}
