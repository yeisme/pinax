package promptrepo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func ValidateCatalog(catalog Catalog) error {
	if catalog.SchemaVersion != CatalogSchemaVersion {
		return NewError(CodeInvalidRequest, fmt.Sprintf("unsupported catalog schema %q", catalog.SchemaVersion), false, nil)
	}
	if strings.TrimSpace(catalog.Repository.ID) == "" || strings.TrimSpace(catalog.Repository.DefaultLocale) == "" {
		return NewError(CodeInvalidRequest, "catalog repository id and default locale are required", false, nil)
	}
	seen := map[string]struct{}{}
	for index := range catalog.Solutions {
		solution := &catalog.Solutions[index]
		if solution.PackageID == "" || solution.ID == "" || solution.Version == "" || solution.Category == "" {
			return NewError(CodeInvalidRequest, "solution identity, version, and category are required", false, nil)
		}
		key := solution.PackageID + "/" + solution.ID + "@" + solution.Version
		if _, ok := seen[key]; ok {
			return NewError(CodeInvalidRequest, "duplicate solution identity "+key, false, nil)
		}
		seen[key] = struct{}{}
		if len(solution.Locales) == 0 {
			return NewError(CodeInvalidRequest, "solution locales are required", false, nil)
		}
		for locale, display := range solution.Locales {
			if strings.TrimSpace(locale) == "" || strings.TrimSpace(display.Title) == "" || strings.TrimSpace(display.Summary) == "" {
				return NewError(CodeInvalidRequest, "localized title and summary are required", false, nil)
			}
		}
		if solution.Rights == "" || solution.Maturity == "" {
			return NewError(CodeInvalidRequest, "solution rights and maturity are required", false, nil)
		}
	}
	return nil
}

func CanonicalCatalogDigest(catalog Catalog) (string, error) {
	clone := catalog
	clone.Digest = ""
	// Build timestamps are operational metadata. Excluding them keeps the
	// content digest stable when the same repository is rebuilt later.
	clone.GeneratedAt = time.Time{}
	sort.Slice(clone.Solutions, func(i, j int) bool {
		left := clone.Solutions[i].PackageID + "/" + clone.Solutions[i].ID + "@" + clone.Solutions[i].Version
		right := clone.Solutions[j].PackageID + "/" + clone.Solutions[j].ID + "@" + clone.Solutions[j].Version
		return left < right
	})
	payload, err := json.Marshal(clone)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
