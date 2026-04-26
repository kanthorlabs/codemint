// Package ui provides UI adapters for different client modes.
package ui

import (
	"fmt"
	"io"

	"codemint.kanthorlabs.com/internal/registry"
)

// AdapterConfig holds configuration for creating UI adapters.
type AdapterConfig struct {
	// Writer is the output destination for TUI rendering (typically os.Stdout).
	Writer io.Writer
	// LogPath is the file path for CUI daemon log (typically xdg.StateDir()+"/cui.log").
	// If empty, CUIAdapter will use its default path.
	LogPath string
	// VerbosityGetter returns the current verbosity level from the active session.
	VerbosityGetter VerbosityGetter
}

// AdapterSet holds the adapters created for a given client mode.
// Only one of TUI or CUI will be non-nil based on the mode.
type AdapterSet struct {
	TUI *TUIAdapter
	CUI *CUIAdapter
}

// BuildAdapters creates the appropriate adapter(s) based on the client mode.
//
//   - ClientModeCLI: Creates only TUIAdapter for high-bandwidth terminal streaming.
//   - ClientModeDaemon: Creates only CUIAdapter for low-bandwidth pulse notifications.
//
// This centralizes the "which adapters do we register?" decision that was
// previously scattered across main.go.
func BuildAdapters(mode registry.ClientMode, cfg AdapterConfig) (AdapterSet, error) {
	var set AdapterSet

	switch mode {
	case registry.ClientModeCLI:
		// CLI mode: TUI adapter for streaming output.
		set.TUI = NewTUIAdapter(TUIAdapterConfig{
			Writer:          cfg.Writer,
			VerbosityGetter: cfg.VerbosityGetter,
		})

	case registry.ClientModeDaemon:
		// Daemon mode: CUI adapter for low-bandwidth notifications.
		set.CUI = NewCUIAdapter(CUIAdapterConfig{
			// CUIAdapter manages its own log file internally.
			// The LogPath in AdapterConfig is available for future customization.
		})

	default:
		return AdapterSet{}, fmt.Errorf("ui: unsupported client mode: %s", mode)
	}

	return set, nil
}

// RegisterAll registers all non-nil adapters with the given mediator.
func (s *AdapterSet) RegisterAll(mediator *UIMediator) {
	if s.TUI != nil {
		mediator.RegisterAdapter(s.TUI)
	}
	if s.CUI != nil {
		mediator.RegisterAdapter(s.CUI)
	}
}

// Close releases resources held by all adapters in the set.
// It calls Stop() on TUIAdapter and Close() on CUIAdapter if they are non-nil.
func (s *AdapterSet) Close() error {
	if s.TUI != nil {
		s.TUI.Stop()
	}
	if s.CUI != nil {
		if err := s.CUI.Close(); err != nil {
			return fmt.Errorf("close CUI adapter: %w", err)
		}
	}
	return nil
}
