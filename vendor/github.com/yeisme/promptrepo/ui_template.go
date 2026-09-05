package promptrepo

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"sort"
	"strconv"
)

const (
	UITemplateSchemaVersionV1 = "promptrepo.ui-template.v1"

	MaxUITemplateHTMLBytesV1     = 256 << 10
	MaxUITemplateCSSBytesV1      = 256 << 10
	MaxUITemplateBodyBytesV1     = 512 << 10
	MaxUITemplateSlotsV1         = 64
	MaxUITemplateSlotNameBytesV1 = 64
)

type UITemplateSlotKindV1 string

const (
	UITemplateSlotKindText       UITemplateSlotKindV1 = "text"
	UITemplateSlotKindComponent  UITemplateSlotKindV1 = "component"
	UITemplateSlotKindCollection UITemplateSlotKindV1 = "collection"
)

type UITemplateSlotCardinalityV1 string

const (
	UITemplateSlotCardinalityOne  UITemplateSlotCardinalityV1 = "one"
	UITemplateSlotCardinalityMany UITemplateSlotCardinalityV1 = "many"
)

type UITemplateSecurityProfileV1 string

const UITemplateSecurityStaticReviewFragmentV1 UITemplateSecurityProfileV1 = "static-review-fragment-v1"

// UITemplateSlotV1 declares a consumer-owned injection point. It never carries
// callbacks, endpoints, methods, or mutation payloads.
type UITemplateSlotV1 struct {
	Name        string                      `json:"name" yaml:"name"`
	Kind        UITemplateSlotKindV1        `json:"kind" yaml:"kind"`
	Required    bool                        `json:"required" yaml:"required"`
	Cardinality UITemplateSlotCardinalityV1 `json:"cardinality" yaml:"cardinality"`
}

type UITemplateLimitsV1 struct {
	MaxHTMLBytes     int `json:"max_html_bytes" yaml:"max_html_bytes"`
	MaxCSSBytes      int `json:"max_css_bytes" yaml:"max_css_bytes"`
	MaxBodyBytes     int `json:"max_body_bytes" yaml:"max_body_bytes"`
	MaxSlots         int `json:"max_slots" yaml:"max_slots"`
	MaxSlotNameBytes int `json:"max_slot_name_bytes" yaml:"max_slot_name_bytes"`
}

func DefaultUITemplateLimitsV1() UITemplateLimitsV1 {
	return UITemplateLimitsV1{
		MaxHTMLBytes:     MaxUITemplateHTMLBytesV1,
		MaxCSSBytes:      MaxUITemplateCSSBytesV1,
		MaxBodyBytes:     MaxUITemplateBodyBytesV1,
		MaxSlots:         MaxUITemplateSlotsV1,
		MaxSlotNameBytes: MaxUITemplateSlotNameBytesV1,
	}
}

// UITemplateBundleV1 keeps body bytes in memory only. JSON/YAML projections
// intentionally omit HTML and CSS so machine output remains body-safe.
type UITemplateBundleV1 struct {
	SchemaVersion string                      `json:"schema_version" yaml:"schema_version"`
	Address       UITemplateAddress           `json:"address" yaml:"address"`
	HTMLFragment  []byte                      `json:"-" yaml:"-"`
	CSS           []byte                      `json:"-" yaml:"-"`
	Slots         []UITemplateSlotV1          `json:"slots,omitempty" yaml:"slots,omitempty"`
	Security      UITemplateSecurityProfileV1 `json:"security" yaml:"security"`
	Limits        UITemplateLimitsV1          `json:"limits" yaml:"limits"`
	ContentDigest string                      `json:"content_digest" yaml:"content_digest"`
	Snapshot      string                      `json:"snapshot" yaml:"snapshot"`
}

type UITemplateViolationV1 struct {
	Code    string `json:"code" yaml:"code"`
	Field   string `json:"field,omitempty" yaml:"field,omitempty"`
	Message string `json:"message" yaml:"message"`
}

type UITemplateValidationV1 struct {
	Valid      bool                    `json:"valid" yaml:"valid"`
	Violations []UITemplateViolationV1 `json:"violations,omitempty" yaml:"violations,omitempty"`
}

type InspectUITemplateRequest struct {
	Address UITemplateAddress `json:"address" yaml:"address"`
}

type LoadUITemplateRequest struct {
	Address UITemplateAddress `json:"address" yaml:"address"`
}

type UITemplateInspectionV1 struct {
	SchemaVersion string                      `json:"schema_version" yaml:"schema_version"`
	Address       UITemplateAddress           `json:"address" yaml:"address"`
	Slots         []UITemplateSlotV1          `json:"slots,omitempty" yaml:"slots,omitempty"`
	Security      UITemplateSecurityProfileV1 `json:"security" yaml:"security"`
	Limits        UITemplateLimitsV1          `json:"limits" yaml:"limits"`
	ContentDigest string                      `json:"content_digest" yaml:"content_digest"`
	Snapshot      string                      `json:"snapshot" yaml:"snapshot"`
	HTMLBytes     int                         `json:"html_bytes" yaml:"html_bytes"`
	CSSBytes      int                         `json:"css_bytes" yaml:"css_bytes"`
	BodyBytes     int                         `json:"body_bytes" yaml:"body_bytes"`
	Validation    UITemplateValidationV1      `json:"validation" yaml:"validation"`
}

type UITemplateInspector interface {
	InspectUITemplate(context.Context, InspectUITemplateRequest) (UITemplateInspectionV1, error)
}

type UITemplateLoader interface {
	LoadUITemplate(context.Context, LoadUITemplateRequest) (UITemplateBundleV1, error)
}

// CheckUITemplateBundle returns a body-free validation projection. The first
// deterministic violation is enough to fail closed and remains safe for logs.
func CheckUITemplateBundle(bundle UITemplateBundleV1) UITemplateValidationV1 {
	if err := ValidateUITemplateBundle(bundle); err != nil {
		return UITemplateValidationV1{
			Valid: false,
			Violations: []UITemplateViolationV1{{
				Code:    ErrorCode(err),
				Message: err.Error(),
			}},
		}
	}
	return UITemplateValidationV1{Valid: true}
}

func NewUITemplateInspection(bundle UITemplateBundleV1) UITemplateInspectionV1 {
	slots := canonicalUITemplateSlots(bundle.Slots)
	return UITemplateInspectionV1{
		SchemaVersion: bundle.SchemaVersion,
		Address:       bundle.Address,
		Slots:         slots,
		Security:      bundle.Security,
		Limits:        bundle.Limits,
		ContentDigest: bundle.ContentDigest,
		Snapshot:      bundle.Snapshot,
		HTMLBytes:     len(bundle.HTMLFragment),
		CSSBytes:      len(bundle.CSS),
		BodyBytes:     len(bundle.HTMLFragment) + len(bundle.CSS),
		Validation:    CheckUITemplateBundle(bundle),
	}
}

// CanonicalUITemplateDigest computes a length-delimited SHA-256 digest. The
// digest and snapshot bindings are deliberately excluded to avoid recursion
// and keep repository snapshot identity separate from content identity.
func CanonicalUITemplateDigest(bundle UITemplateBundleV1) (string, error) {
	if err := validateUITemplateBundleStructure(bundle); err != nil {
		return "", err
	}
	identity := bundle.Address
	identity.Digest = ""
	identity.Snapshot = ""
	hasher := sha256.New()
	writeUITemplateDigestField(hasher, "contract", []byte("promptrepo.ui-template.digest.v1"))
	writeUITemplateDigestField(hasher, "schema_version", []byte(bundle.SchemaVersion))
	writeUITemplateDigestField(hasher, "address", []byte(FormatUITemplateAddress(identity)))
	slots := canonicalUITemplateSlots(bundle.Slots)
	writeUITemplateDigestField(hasher, "slot_count", []byte(strconv.Itoa(len(slots))))
	for index, slot := range slots {
		prefix := "slot." + strconv.Itoa(index) + "."
		writeUITemplateDigestField(hasher, prefix+"name", []byte(slot.Name))
		writeUITemplateDigestField(hasher, prefix+"kind", []byte(slot.Kind))
		writeUITemplateDigestField(hasher, prefix+"required", []byte(strconv.FormatBool(slot.Required)))
		writeUITemplateDigestField(hasher, prefix+"cardinality", []byte(slot.Cardinality))
	}
	writeUITemplateDigestField(hasher, "security", []byte(bundle.Security))
	writeUITemplateDigestField(hasher, "limit.max_html_bytes", []byte(strconv.Itoa(bundle.Limits.MaxHTMLBytes)))
	writeUITemplateDigestField(hasher, "limit.max_css_bytes", []byte(strconv.Itoa(bundle.Limits.MaxCSSBytes)))
	writeUITemplateDigestField(hasher, "limit.max_body_bytes", []byte(strconv.Itoa(bundle.Limits.MaxBodyBytes)))
	writeUITemplateDigestField(hasher, "limit.max_slots", []byte(strconv.Itoa(bundle.Limits.MaxSlots)))
	writeUITemplateDigestField(hasher, "limit.max_slot_name_bytes", []byte(strconv.Itoa(bundle.Limits.MaxSlotNameBytes)))
	writeUITemplateDigestField(hasher, "html", bundle.HTMLFragment)
	writeUITemplateDigestField(hasher, "css", bundle.CSS)
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func canonicalUITemplateSlots(slots []UITemplateSlotV1) []UITemplateSlotV1 {
	clone := append([]UITemplateSlotV1(nil), slots...)
	sort.Slice(clone, func(i, j int) bool { return clone[i].Name < clone[j].Name })
	return clone
}

func writeUITemplateDigestField(destination hash.Hash, name string, value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(name)))
	_, _ = destination.Write(size[:])
	_, _ = destination.Write([]byte(name))
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = destination.Write(size[:])
	_, _ = destination.Write(value)
}
