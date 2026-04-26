// Package workflow provides the WorkflowRegistry for managing workflow
// definitions loaded from configuration.
package workflow

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"codemint.kanthorlabs.com/internal/config"
	"codemint.kanthorlabs.com/internal/domain"
)

// ErrWorkflowNotFound is returned when a workflow lookup fails.
var ErrWorkflowNotFound = errors.New("workflow: not found")

// ErrDuplicateWorkflow is returned when attempting to register a duplicate workflow type.
var ErrDuplicateWorkflow = errors.New("workflow: duplicate type")

// WorkflowRegistry manages workflow definitions and provides lookup capabilities.
type WorkflowRegistry struct {
	workflows map[domain.WorkflowType]domain.WorkflowDefinition
}

// NewWorkflowRegistry creates a new empty WorkflowRegistry.
func NewWorkflowRegistry() *WorkflowRegistry {
	return &WorkflowRegistry{
		workflows: make(map[domain.WorkflowType]domain.WorkflowDefinition),
	}
}

// Register adds a workflow definition to the registry.
// Returns ErrDuplicateWorkflow if the type is already registered.
func (r *WorkflowRegistry) Register(def domain.WorkflowDefinition) error {
	if _, exists := r.workflows[def.Type]; exists {
		return fmt.Errorf("%w: type %d already registered", ErrDuplicateWorkflow, def.Type)
	}
	r.workflows[def.Type] = def
	return nil
}

// Lookup retrieves a workflow definition by type.
// Returns ErrWorkflowNotFound if the type is not registered.
func (r *WorkflowRegistry) Lookup(t domain.WorkflowType) (domain.WorkflowDefinition, error) {
	def, ok := r.workflows[t]
	if !ok {
		return domain.WorkflowDefinition{}, fmt.Errorf("%w: type %d", ErrWorkflowNotFound, t)
	}
	return def, nil
}

// All returns all registered workflow definitions sorted by type.
func (r *WorkflowRegistry) All() []domain.WorkflowDefinition {
	defs := make([]domain.WorkflowDefinition, 0, len(r.workflows))
	for _, def := range r.workflows {
		defs = append(defs, def)
	}
	slices.SortFunc(defs, func(a, b domain.WorkflowDefinition) int {
		return int(a.Type) - int(b.Type)
	})
	return defs
}

// FindByTrigger searches for a workflow whose triggers match the given keyword.
// Matching is case-insensitive and checks if any trigger is contained in the input.
// Returns the matching definition and true, or an empty definition and false.
func (r *WorkflowRegistry) FindByTrigger(input string) (domain.WorkflowDefinition, bool) {
	inputLower := strings.ToLower(input)

	// Check all workflows, prioritized by type order.
	for _, def := range r.All() {
		for _, trigger := range def.Triggers {
			if strings.Contains(inputLower, strings.ToLower(trigger)) {
				return def, true
			}
		}
	}

	return domain.WorkflowDefinition{}, false
}

// Len returns the number of registered workflows.
func (r *WorkflowRegistry) Len() int {
	return len(r.workflows)
}

// LoadFromConfig creates a WorkflowRegistry from a validated configuration.
// It validates the config first, converts WorkflowConfig to WorkflowDefinition,
// and registers each workflow.
func LoadFromConfig(cfg *config.Config) (*WorkflowRegistry, error) {
	if err := config.Validate(cfg); err != nil {
		return nil, fmt.Errorf("workflow: %w", err)
	}

	reg := NewWorkflowRegistry()

	for _, wc := range cfg.Workflows {
		def := domain.WorkflowDefinition{
			Type:        domain.WorkflowType(wc.Type),
			Name:        wc.Name,
			Description: wc.Description,
			Triggers:    wc.Triggers,
		}

		if err := reg.Register(def); err != nil {
			return nil, fmt.Errorf("workflow: register %q (type=%d): %w", wc.Name, wc.Type, err)
		}
	}

	return reg, nil
}
