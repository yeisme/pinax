// Package testfixture builds promptrepo file-source repository fixtures for
// Pinax tests. It computes canonical catalog and contract digests with the
// public SDK so fixtures stay valid without hand-maintained digests. It is
// test support only and never imported by production code paths.
package testfixture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	promptrepo "github.com/yeisme/promptrepo"
)

// RepositoryOptions describes one single-solution file repository fixture.
type RepositoryOptions struct {
	RepositoryID string
	PackageID    string
	SolutionID   string
	Version      string
	Category     string
	Rights       string
	Title        string
	Summary      string
	Body         string
	// WithContract writes the companion contract sidecar; without it the
	// fixture models a repository whose rights cannot be verified.
	WithContract bool
	License      string
	Permissions  []string
	Inputs       []promptrepo.InputDefinition
}

// Defaults fills the minimal fixture identity for any omitted field.
func (options RepositoryOptions) Defaults() RepositoryOptions {
	filled := options
	if filled.RepositoryID == "" {
		filled.RepositoryID = "official"
	}
	if filled.PackageID == "" {
		filled.PackageID = "audio"
	}
	if filled.SolutionID == "" {
		filled.SolutionID = "podcast-narration"
	}
	if filled.Version == "" {
		filled.Version = "1.0.0"
	}
	if filled.Category == "" {
		filled.Category = "audio"
	}
	if filled.Rights == "" {
		filled.Rights = "internal"
	}
	if filled.Title == "" {
		filled.Title = "中文播客旁白"
	}
	if filled.Summary == "" {
		filled.Summary = "生成自然、清晰的中文播客旁白"
	}
	if filled.Body == "" {
		filled.Body = "# 中文播客旁白\n\n请生成自然、清晰的旁白。\n"
	}
	if filled.License == "" {
		filled.License = "internal"
	}
	return filled
}

// Ref returns the exact promptrepo ref for the fixture.
func (options RepositoryOptions) Ref() string {
	filled := options.Defaults()
	return fmt.Sprintf("promptrepo://%s/%s/%s@%s?locale=zh-CN", filled.RepositoryID, filled.PackageID, filled.SolutionID, filled.Version)
}

// WriteRepository writes catalog.json, the template body, and (optionally)
// the contract companion under root, returning the file:// source URI.
func WriteRepository(root string, options RepositoryOptions) (string, error) {
	filled := options.Defaults()
	if err := os.MkdirAll(filepath.Join(root, "prompts"), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(root, "prompts", "main.zh-CN.md"), []byte(filled.Body), 0o600); err != nil {
		return "", err
	}
	templateDigest := sha256.Sum256([]byte(filled.Body))
	template := promptrepo.TemplateRole{Role: "main", Locale: "zh-CN", Path: "prompts/main.zh-CN.md", Digest: "sha256:" + hex.EncodeToString(templateDigest[:])}
	catalog := promptrepo.Catalog{
		SchemaVersion: promptrepo.CatalogSchemaVersion,
		Repository:    promptrepo.RepositoryMetadata{ID: filled.RepositoryID, Name: "Fixture " + filled.RepositoryID, DefaultLocale: "zh-CN", TaxonomyVersion: "v1"},
		GeneratedAt:   time.Unix(1, 0).UTC(),
		Solutions: []promptrepo.Solution{{
			PackageID: filled.PackageID, ID: filled.SolutionID, Version: filled.Version, Category: filled.Category,
			Rights: filled.Rights, Maturity: "first-support",
			Locales:   map[string]promptrepo.LocalizedText{"zh-CN": {Title: filled.Title, Summary: filled.Summary}},
			Templates: []promptrepo.TemplateRole{template},
		}},
	}
	digest, err := promptrepo.CanonicalCatalogDigest(catalog)
	if err != nil {
		return "", err
	}
	catalog.Digest = digest
	payload, err := json.Marshal(catalog)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(root, "catalog.json"), payload, 0o600); err != nil {
		return "", err
	}
	if filled.WithContract {
		if err := writeContractCompanion(root, filled, template); err != nil {
			return "", err
		}
	}
	return "file://" + root, nil
}

func writeContractCompanion(root string, filled RepositoryOptions, template promptrepo.TemplateRole) error {
	inputs := append([]promptrepo.InputDefinition(nil), filled.Inputs...)
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Name < inputs[j].Name })
	document := promptrepo.TemplateContractDocument{
		SchemaVersion:  promptrepo.TemplateContractSchemaVersion,
		PackageID:      filled.PackageID,
		SolutionID:     filled.SolutionID,
		Version:        filled.Version,
		Role:           "main",
		Locale:         "zh-CN",
		TemplatePath:   template.Path,
		TemplateDigest: template.Digest,
		License:        filled.License,
		Permissions:    filled.Permissions,
		Inputs:         inputs,
	}
	digest, err := promptrepo.CanonicalTemplateContractDigest(document)
	if err != nil {
		return err
	}
	document.Digest = digest
	payload, err := json.Marshal(document)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "contracts"), 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "contracts", "main.zh-CN.json"), payload, 0o600)
}
