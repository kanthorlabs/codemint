// Package agent defines agent interfaces and implementations for CodeMint.
// This file implements the ProviderRegistry for resolving ACP providers.
package agent

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"sync"

	"codemint.kanthorlabs.com/internal/config"
)

// ErrProviderNotFound is returned when a provider cannot be resolved.
var ErrProviderNotFound = errors.New("provider not found")

// ErrProviderDisabled is returned when attempting to resolve a disabled provider.
var ErrProviderDisabled = errors.New("provider is disabled")

// BinaryNotFoundError is returned when a provider's binary cannot be found on PATH.
type BinaryNotFoundError struct {
	ProviderName string
	Command      string
}

func (e *BinaryNotFoundError) Error() string {
	return fmt.Sprintf("provider %q: binary %q not found on PATH", e.ProviderName, e.Command)
}

// ProviderRegistry resolves Provider instances at runtime by merging
// builtin catalog entries with config overrides.
type ProviderRegistry struct {
	mu      sync.RWMutex
	entries map[string]*Provider
}

// NewProviderRegistry creates a new ProviderRegistry from config.
// It merges builtin catalog entries with config overrides.
// CODEMINT_ACP_CMD env override is NOT applied here - it's applied
// at resolution time for the System Assistant only (Story 3.1 compatibility).
func NewProviderRegistry(cfg *config.Config) (*ProviderRegistry, error) {
	r := &ProviderRegistry{
		entries: make(map[string]*Provider),
	}

	// Step 1: Load all builtin providers.
	for name := range builtinProviders {
		provider, _ := LookupBuiltinProvider(name)
		r.entries[name] = provider
	}

	// Step 2: Apply config overrides.
	if cfg != nil {
		for _, pc := range cfg.Providers {
			if pc.Disabled {
				// Remove from entries if disabled.
				delete(r.entries, pc.Name)
				continue
			}

			existing, exists := r.entries[pc.Name]
			if exists {
				// Merge override into existing provider.
				override := configToProviderOverride(&pc)
				existing.Merge(override)
			} else {
				// Create new provider from config.
				r.entries[pc.Name] = configToProvider(&pc)
			}
		}
	}

	return r, nil
}

// configToProviderOverride creates a Provider with only the fields that should override.
func configToProviderOverride(pc *config.ProviderConfig) *Provider {
	return &Provider{
		Command:     pc.Command,
		Args:        pc.Args,
		Env:         pc.Env,
		VersionArgs: nil, // Config doesn't override version args
		ModelFlag:   pc.ModelFlag,
	}
}

// configToProvider creates a full Provider from config (for custom providers).
func configToProvider(pc *config.ProviderConfig) *Provider {
	modelFlag := pc.ModelFlag
	if modelFlag == "" {
		modelFlag = "--model" // Default for custom providers
	}
	return &Provider{
		Name:        pc.Name,
		DisplayName: pc.Name, // Custom providers use name as display name
		Command:     pc.Command,
		Args:        pc.Args,
		Env:         pc.Env,
		Capabilities: ProviderCaps{
			Streaming:    true,
			ToolCalls:    true,
			Planning:     true,
			ContextReset: true,
		},
		SystemPromptStrategy: PromptStrategyStdin, // Default for custom providers
		VersionArgs:          []string{"--version"},
		ModelFlag:            modelFlag,
	}
}

// Resolve returns a clone of the provider with the given name.
// Returns ErrProviderNotFound if the provider doesn't exist.
func (r *ProviderRegistry) Resolve(name string) (*Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, exists := r.entries[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, name)
	}

	return provider.Clone(), nil
}

// MustExist checks that the provider's binary exists on PATH.
// Returns a BinaryNotFoundError if the binary cannot be found.
func (r *ProviderRegistry) MustExist(name string) error {
	r.mu.RLock()
	provider, exists := r.entries[name]
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("%w: %s", ErrProviderNotFound, name)
	}

	if _, err := exec.LookPath(provider.Command); err != nil {
		return &BinaryNotFoundError{
			ProviderName: name,
			Command:      provider.Command,
		}
	}

	return nil
}

// List returns all registered (non-disabled) providers sorted by name.
func (r *ProviderRegistry) List() []*Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]*Provider, 0, len(r.entries))
	for _, p := range r.entries {
		providers = append(providers, p.Clone())
	}

	// Sort by name for consistent output.
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Name < providers[j].Name
	})

	return providers
}

// Has checks if a provider is registered (and not disabled).
func (r *ProviderRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.entries[name]
	return exists
}

// Names returns all registered provider names sorted alphabetically.
func (r *ProviderRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.entries))
	for name := range r.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
