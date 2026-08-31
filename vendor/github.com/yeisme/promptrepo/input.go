package promptrepo

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

type InputType string

const (
	InputTypeString  InputType = "string"
	InputTypeInteger InputType = "integer"
	InputTypeNumber  InputType = "number"
	InputTypeBoolean InputType = "boolean"
	InputTypeEnum    InputType = "enum"
)

type InputDefinition struct {
	Name         string            `json:"name" yaml:"name"`
	Type         InputType         `json:"type" yaml:"type"`
	Required     bool              `json:"required,omitempty" yaml:"required,omitempty"`
	Default      any               `json:"default,omitempty" yaml:"default,omitempty"`
	Examples     []any             `json:"examples,omitempty" yaml:"examples,omitempty"`
	Enum         []any             `json:"enum,omitempty" yaml:"enum,omitempty"`
	Regex        string            `json:"regex,omitempty" yaml:"regex,omitempty"`
	Min          *float64          `json:"min,omitempty" yaml:"min,omitempty"`
	Max          *float64          `json:"max,omitempty" yaml:"max,omitempty"`
	MinLength    *int              `json:"min_length,omitempty" yaml:"min_length,omitempty"`
	MaxLength    *int              `json:"max_length,omitempty" yaml:"max_length,omitempty"`
	Labels       map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Descriptions map[string]string `json:"descriptions,omitempty" yaml:"descriptions,omitempty"`
	Sensitivity  string            `json:"sensitivity,omitempty" yaml:"sensitivity,omitempty"`
}

// TemplateContract is an additive, caller-supplied template input and rights
// projection. It is intentionally separate from the released catalog/state DTOs.
type TemplateContract struct {
	Digest      string            `json:"digest,omitempty" yaml:"digest,omitempty"`
	Inputs      []InputDefinition `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	License     string            `json:"license,omitempty" yaml:"license,omitempty"`
	Permissions []string          `json:"permissions,omitempty" yaml:"permissions,omitempty"`
}

// ValidateTemplateContract validates the caller-supplied contract without
// storing it in catalog or state. An omitted digest is valid for draft input.
func ValidateTemplateContract(contract TemplateContract) error {
	if contract.Digest != "" && !sha256DigestPattern.MatchString(contract.Digest) {
		return NewError(CodeInvalidRequest, "template contract digest is invalid", false, nil)
	}
	if contract.License != "" && !validTemplateLicense(contract.License) {
		return NewError(CodeInvalidRequest, "template contract license is invalid", false, nil)
	}
	permissions := canonicalStrings(contract.Permissions)
	if !equalStringSlices(contract.Permissions, permissions) {
		return NewError(CodeInvalidRequest, "template contract permissions are not canonical", false, nil)
	}
	for _, permission := range permissions {
		if !companionIdentifierPattern.MatchString(permission) {
			return NewError(CodeInvalidRequest, "template contract permission is invalid", false, nil)
		}
	}
	return ValidateTemplateInputs(contract.Inputs)
}

type InputIssue struct {
	Code    string `json:"code" yaml:"code"`
	Field   string `json:"field,omitempty" yaml:"field,omitempty"`
	Message string `json:"message" yaml:"message"`
}

type InputStatus struct {
	Definition InputDefinition `json:"definition" yaml:"definition"`
	Status     string          `json:"status" yaml:"status"`
}

type InputValidation struct {
	Inputs []InputStatus `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Issues []InputIssue  `json:"issues,omitempty" yaml:"issues,omitempty"`
	Ready  bool          `json:"ready" yaml:"ready"`
	values map[string]any
}

func (validation InputValidation) Value(name string) (any, bool) {
	value, ok := validation.values[name]
	return value, ok
}

var inputNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
var templateLicensePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 .()+-]{0,255}$`)

func validTemplateLicense(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && templateLicensePattern.MatchString(value)
}

func ValidateTemplateInputs(inputs []InputDefinition) error {
	seen := map[string]struct{}{}
	for _, input := range inputs {
		if !inputNamePattern.MatchString(input.Name) {
			return invalidInputDefinition(input.Name, "name is invalid")
		}
		if _, ok := seen[input.Name]; ok {
			return invalidInputDefinition(input.Name, "name is duplicated")
		}
		seen[input.Name] = struct{}{}
		if !validInputType(input.Type) {
			return invalidInputDefinition(input.Name, "type is invalid")
		}
		if input.Sensitivity != "" && input.Sensitivity != "public" && input.Sensitivity != "sensitive" {
			return invalidInputDefinition(input.Name, "sensitivity is invalid")
		}
		if input.Sensitivity == "sensitive" && (input.Default != nil || len(input.Examples) != 0 || len(input.Enum) != 0) {
			return invalidInputDefinition(input.Name, "sensitive input cannot declare values")
		}
		if err := validateInputDefinitionConstraints(input); err != nil {
			return err
		}
		for _, value := range append(append([]any{}, input.Examples...), input.Enum...) {
			if issue := validateInputValue(input, value); issue != nil {
				return invalidInputDefinition(input.Name, issue.Message)
			}
		}
		if input.Default != nil {
			if issue := validateInputValue(input, input.Default); issue != nil {
				return invalidInputDefinition(input.Name, issue.Message)
			}
		}
		for locale, text := range input.Labels {
			if !validLocale(locale) || strings.TrimSpace(text) == "" {
				return invalidInputDefinition(input.Name, "localized label is invalid")
			}
		}
		for locale, text := range input.Descriptions {
			if !validLocale(locale) || strings.TrimSpace(text) == "" {
				return invalidInputDefinition(input.Name, "localized description is invalid")
			}
		}
	}
	return nil
}

func ValidateInputValues(inputs []InputDefinition, supplied map[string]any) InputValidation {
	validation := InputValidation{Ready: true, values: map[string]any{}}
	definitions := map[string]InputDefinition{}
	for _, input := range inputs {
		definitions[input.Name] = input
	}
	unknown := make([]string, 0)
	for name := range supplied {
		if _, ok := definitions[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	for _, name := range unknown {
		validation.addIssue(InputIssue{Code: CodeInputUnknown, Field: name, Message: "input is not declared"})
	}
	for _, input := range inputs {
		value, suppliedValue := supplied[input.Name]
		status := "supplied"
		if !suppliedValue {
			if input.Default != nil {
				value, suppliedValue, status = input.Default, true, "default"
			} else {
				status = "missing"
			}
		}
		validation.Inputs = append(validation.Inputs, InputStatus{Definition: input, Status: status})
		if !suppliedValue {
			if input.Required {
				validation.addIssue(InputIssue{Code: CodeInputRequired, Field: input.Name, Message: "required input is missing"})
			}
			continue
		}
		if issue := validateInputValue(input, value); issue != nil {
			issue.Field = input.Name
			validation.addIssue(*issue)
			continue
		}
		validation.values[input.Name] = value
	}
	return validation
}

func (validation *InputValidation) addIssue(issue InputIssue) {
	validation.Issues = append(validation.Issues, issue)
	validation.Ready = false
}

func validateInputDefinitionConstraints(input InputDefinition) error {
	if (input.Min != nil || input.Max != nil) && input.Type != InputTypeInteger && input.Type != InputTypeNumber {
		return invalidInputDefinition(input.Name, "min and max require a numeric type")
	}
	if input.Min != nil && input.Max != nil && *input.Min > *input.Max {
		return invalidInputDefinition(input.Name, "min cannot exceed max")
	}
	if (input.MinLength != nil || input.MaxLength != nil) && input.Type != InputTypeString && input.Type != InputTypeEnum {
		return invalidInputDefinition(input.Name, "length constraints require a text type")
	}
	if input.MinLength != nil && *input.MinLength < 0 || input.MaxLength != nil && *input.MaxLength < 0 || input.MinLength != nil && input.MaxLength != nil && *input.MinLength > *input.MaxLength {
		return invalidInputDefinition(input.Name, "length constraints are invalid")
	}
	if input.Regex != "" {
		if input.Type != InputTypeString && input.Type != InputTypeEnum {
			return invalidInputDefinition(input.Name, "regex requires a text type")
		}
		if _, err := regexp.Compile(input.Regex); err != nil {
			return invalidInputDefinition(input.Name, "regex is invalid")
		}
	}
	if input.Type == InputTypeEnum && len(input.Enum) == 0 {
		return invalidInputDefinition(input.Name, "enum type requires values")
	}
	if input.Type != InputTypeEnum && len(input.Enum) > 0 {
		return invalidInputDefinition(input.Name, "enum values require enum type")
	}
	return nil
}

func validateInputValue(input InputDefinition, value any) *InputIssue {
	validType := false
	switch input.Type {
	case InputTypeString, InputTypeEnum:
		_, validType = value.(string)
	case InputTypeBoolean:
		_, validType = value.(bool)
	case InputTypeInteger:
		_, validType = integerValue(value)
	case InputTypeNumber:
		_, validType = numberValue(value)
	}
	if !validType {
		return &InputIssue{Code: CodeInputType, Message: "input has an invalid type"}
	}
	if input.Type == InputTypeEnum && !containsValue(input.Enum, value) {
		return &InputIssue{Code: CodeInputEnum, Message: "input is not an allowed enum value"}
	}
	if input.Type == InputTypeInteger || input.Type == InputTypeNumber {
		number, _ := numberValue(value)
		if input.Min != nil && number < *input.Min || input.Max != nil && number > *input.Max {
			return &InputIssue{Code: CodeInputConstraint, Message: "input is outside the allowed range"}
		}
	}
	if input.Type == InputTypeString || input.Type == InputTypeEnum {
		text := value.(string)
		length := len([]rune(text))
		if input.MinLength != nil && length < *input.MinLength || input.MaxLength != nil && length > *input.MaxLength {
			return &InputIssue{Code: CodeInputConstraint, Message: "input has an invalid length"}
		}
		if input.Regex != "" {
			matched, _ := regexp.MatchString(input.Regex, text)
			if !matched {
				return &InputIssue{Code: CodeInputConstraint, Message: "input does not match the required pattern"}
			}
		}
	}
	return nil
}

func validInputType(inputType InputType) bool {
	return inputType == InputTypeString || inputType == InputTypeInteger || inputType == InputTypeNumber || inputType == InputTypeBoolean || inputType == InputTypeEnum
}

func integerValue(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		return int64(typed), uint64(typed) <= math.MaxInt64
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		return int64(typed), typed <= math.MaxInt64
	case float64:
		return int64(typed), math.Trunc(typed) == typed && typed >= math.MinInt64 && typed <= math.MaxInt64
	default:
		return 0, false
	}
}

func numberValue(value any) (float64, bool) {
	if integer, ok := integerValue(value); ok {
		return float64(integer), true
	}
	switch typed := value.(type) {
	case float32:
		return float64(typed), !math.IsNaN(float64(typed)) && !math.IsInf(float64(typed), 0)
	case float64:
		return typed, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	default:
		return 0, false
	}
}

func containsValue(values []any, want any) bool {
	for _, value := range values {
		if fmt.Sprintf("%T:%v", value, value) == fmt.Sprintf("%T:%v", want, want) {
			return true
		}
	}
	return false
}

func validLocale(value string) bool {
	return regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`).MatchString(value)
}

func invalidInputDefinition(name, message string) error {
	return NewError(CodeInvalidRequest, fmt.Sprintf("input definition %q %s", name, message), false, nil)
}
