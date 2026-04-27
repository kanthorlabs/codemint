package config

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

// validate is a singleton validator instance with custom validators registered.
var validate *validator.Validate

// BuiltinProviderNames is set during initialization to provide access to the
// builtin provider catalog. This avoids an import cycle between config and agent.
var BuiltinProviderNames func() []string

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())
}

// ValidationError aggregates multiple validation violations.
type ValidationError struct {
	Violations []string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("config validation failed: %s", strings.Join(e.Violations, "; "))
}

// Validate checks the configuration for consistency and completeness.
// It returns a ValidationError containing all violations (not just the first).
func Validate(c *Config) error {
	if c == nil {
		return &ValidationError{Violations: []string{"config is nil"}}
	}

	var violations []string

	// Run struct validation.
	if err := validate.Struct(c); err != nil {
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			for _, ve := range validationErrors {
				violations = append(violations, formatValidationError(ve))
			}
		} else {
			violations = append(violations, err.Error())
		}
	}

	// Custom validation: check for duplicate workflow types.
	seenTypes := make(map[int]int) // type -> index in config
	for i, w := range c.Workflows {
		if prevIdx, exists := seenTypes[w.Type]; exists {
			violations = append(violations,
				fmt.Sprintf("Workflows[%d].Type: duplicate type %d (first seen at Workflows[%d])", i, w.Type, prevIdx))
		}
		seenTypes[w.Type] = i
	}

	// Custom validation: check for duplicate provider names.
	seenProviders := make(map[string]int) // name -> index in config
	for i, p := range c.Providers {
		if prevIdx, exists := seenProviders[p.Name]; exists {
			violations = append(violations,
				fmt.Sprintf("Providers[%d].Name: duplicate provider name %q (first seen at Providers[%d])", i, p.Name, prevIdx))
		}
		seenProviders[p.Name] = i
	}

	// Build set of known provider names (builtin + configured).
	knownProviders := make(map[string]bool)
	if BuiltinProviderNames != nil {
		for _, name := range BuiltinProviderNames() {
			knownProviders[name] = true
		}
	}
	for _, p := range c.Providers {
		knownProviders[p.Name] = true
	}

	// Custom validation: check assistant bindings reference known providers.
	violations = append(violations, validateAssistantBinding(c.Assistants.System, "assistants.system", knownProviders)...)
	violations = append(violations, validateAssistantBinding(c.Assistants.Brainstormer, "assistants.brainstormer", knownProviders)...)
	violations = append(violations, validateAssistantBinding(c.Assistants.Clarifier, "assistants.clarifier", knownProviders)...)
	violations = append(violations, validateAssistantBinding(c.Assistants.Archivist, "assistants.archivist", knownProviders)...)

	if len(violations) > 0 {
		return &ValidationError{Violations: violations}
	}

	return nil
}

// validateAssistantBinding checks that an assistant binding references a known provider.
func validateAssistantBinding(binding AssistantBindingConfig, field string, knownProviders map[string]bool) []string {
	// Empty provider is valid (defaults to opencode).
	if binding.Provider == "" {
		return nil
	}

	if !knownProviders[binding.Provider] {
		var known []string
		for name := range knownProviders {
			known = append(known, name)
		}
		return []string{fmt.Sprintf("%s.provider: unknown provider %q (known: %s; or declare under providers:)",
			field, binding.Provider, strings.Join(known, ", "))}
	}

	return nil
}

// formatValidationError converts a validator.FieldError to a human-readable string.
func formatValidationError(fe validator.FieldError) string {
	field := fe.Namespace()
	tag := fe.Tag()
	param := fe.Param()

	switch tag {
	case "required":
		return fmt.Sprintf("%s: is required", field)
	case "min":
		return fmt.Sprintf("%s: must be at least %s", field, param)
	case "max":
		return fmt.Sprintf("%s: must be at most %s", field, param)
	default:
		return fmt.Sprintf("%s: failed '%s' validation", field, tag)
	}
}
