package config

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

// validate is a singleton validator instance with custom validators registered.
var validate *validator.Validate

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

	if len(violations) > 0 {
		return &ValidationError{Violations: violations}
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
