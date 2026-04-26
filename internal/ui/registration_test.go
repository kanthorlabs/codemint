package ui

import (
	"bytes"
	"testing"

	"codemint.kanthorlabs.com/internal/registry"
)

func TestBuildAdapters_CLI(t *testing.T) {
	cfg := AdapterConfig{
		Writer:          &bytes.Buffer{},
		VerbosityGetter: func() VerbosityLevel { return VerbosityTask },
	}

	set, err := BuildAdapters(registry.ClientModeCLI, cfg)
	if err != nil {
		t.Fatalf("BuildAdapters(CLI) error = %v", err)
	}

	// CLI mode should only have TUI adapter.
	if set.TUI == nil {
		t.Error("BuildAdapters(CLI) TUI = nil, want non-nil")
	}
	if set.CUI != nil {
		t.Error("BuildAdapters(CLI) CUI = non-nil, want nil")
	}

	// Clean up.
	set.Close()
}

func TestBuildAdapters_Daemon(t *testing.T) {
	cfg := AdapterConfig{
		Writer:          &bytes.Buffer{},
		VerbosityGetter: func() VerbosityLevel { return VerbosityTask },
	}

	set, err := BuildAdapters(registry.ClientModeDaemon, cfg)
	if err != nil {
		t.Fatalf("BuildAdapters(Daemon) error = %v", err)
	}

	// Daemon mode should only have CUI adapter.
	if set.CUI == nil {
		t.Error("BuildAdapters(Daemon) CUI = nil, want non-nil")
	}
	if set.TUI != nil {
		t.Error("BuildAdapters(Daemon) TUI = non-nil, want nil")
	}

	// Clean up.
	set.Close()
}

// TestBuildAdapters_Hybrid verifies that hybrid mode creates both TUI and CUI adapters.
func TestBuildAdapters_Hybrid(t *testing.T) {
	cfg := AdapterConfig{
		Writer:          &bytes.Buffer{},
		VerbosityGetter: func() VerbosityLevel { return VerbosityTask },
	}

	set, err := BuildAdapters(registry.ClientModeHybrid, cfg)
	if err != nil {
		t.Fatalf("BuildAdapters(Hybrid) error = %v", err)
	}

	// Hybrid mode should have BOTH adapters.
	if set.TUI == nil {
		t.Error("BuildAdapters(Hybrid) TUI = nil, want non-nil")
	}
	if set.CUI == nil {
		t.Error("BuildAdapters(Hybrid) CUI = nil, want non-nil")
	}

	// Clean up.
	set.Close()
}

// TestBuildAdapters_Hybrid_VerbosityDefaults verifies default verbosity levels in hybrid mode.
func TestBuildAdapters_Hybrid_VerbosityDefaults(t *testing.T) {
	cfg := AdapterConfig{
		Writer:          &bytes.Buffer{},
		VerbosityGetter: nil, // No dynamic getter
	}

	set, err := BuildAdapters(registry.ClientModeHybrid, cfg)
	if err != nil {
		t.Fatalf("BuildAdapters(Hybrid) error = %v", err)
	}
	defer set.Close()

	// TUI should default to Level 0 (Task).
	if set.TUI.GetVerbosity() != VerbosityTask {
		t.Errorf("TUI verbosity = %d, want %d (Task)", set.TUI.GetVerbosity(), VerbosityTask)
	}
	// CUI doesn't have verbosity concept - it filters by event type instead.
}

func TestBuildAdapters_InvalidMode(t *testing.T) {
	cfg := AdapterConfig{}

	_, err := BuildAdapters("invalid", cfg)
	if err == nil {
		t.Error("BuildAdapters(invalid) error = nil, want error")
	}
}

func TestAdapterSet_RegisterAll(t *testing.T) {
	mediator := NewUIMediator(&bytes.Buffer{})

	// Test CLI mode registration.
	tuiSet := AdapterSet{
		TUI: NewTUIAdapter(TUIAdapterConfig{Writer: &bytes.Buffer{}}),
	}
	tuiSet.RegisterAll(mediator)

	adapters := mediator.Adapters()
	if len(adapters) != 1 {
		t.Errorf("RegisterAll(TUI) adapter count = %d, want 1", len(adapters))
	}

	// Clean up TUI.
	tuiSet.Close()
}

func TestAdapterSet_RegisterAll_Daemon(t *testing.T) {
	mediator := NewUIMediator(&bytes.Buffer{})

	// Test Daemon mode registration.
	cuiSet := AdapterSet{
		CUI: NewCUIAdapter(CUIAdapterConfig{}),
	}
	cuiSet.RegisterAll(mediator)

	adapters := mediator.Adapters()
	if len(adapters) != 1 {
		t.Errorf("RegisterAll(CUI) adapter count = %d, want 1", len(adapters))
	}

	// Clean up CUI.
	cuiSet.Close()
}

// TestAdapterSet_RegisterAll_Hybrid verifies that hybrid mode registers both adapters.
func TestAdapterSet_RegisterAll_Hybrid(t *testing.T) {
	mediator := NewUIMediator(&bytes.Buffer{})

	// Test Hybrid mode registration - both adapters.
	hybridSet := AdapterSet{
		TUI: NewTUIAdapter(TUIAdapterConfig{Writer: &bytes.Buffer{}}),
		CUI: NewCUIAdapter(CUIAdapterConfig{}),
	}
	hybridSet.RegisterAll(mediator)

	adapters := mediator.Adapters()
	if len(adapters) != 2 {
		t.Errorf("RegisterAll(Hybrid) adapter count = %d, want 2", len(adapters))
	}

	// Clean up both adapters.
	hybridSet.Close()
}

func TestAdapterSet_Close(t *testing.T) {
	// Test that Close() doesn't panic on nil adapters.
	set := AdapterSet{}
	if err := set.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}

	// Test that Close() properly closes TUI adapter.
	set = AdapterSet{
		TUI: NewTUIAdapter(TUIAdapterConfig{Writer: &bytes.Buffer{}}),
	}
	if err := set.Close(); err != nil {
		t.Errorf("Close() with TUI error = %v, want nil", err)
	}

	// Test that Close() properly closes CUI adapter.
	set = AdapterSet{
		CUI: NewCUIAdapter(CUIAdapterConfig{}),
	}
	if err := set.Close(); err != nil {
		t.Errorf("Close() with CUI error = %v, want nil", err)
	}
}
