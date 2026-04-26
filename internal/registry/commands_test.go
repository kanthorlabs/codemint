package registry

import "testing"

// TestCommand_SupportsMode_EmptyModes verifies that commands with no SupportedModes
// are available in all modes.
func TestCommand_SupportsMode_EmptyModes(t *testing.T) {
	cmd := Command{
		Name:           "universal",
		Description:    "Available everywhere",
		SupportedModes: nil, // Empty - available in all modes
	}

	modes := []ClientMode{ClientModeCLI, ClientModeDaemon, ClientModeHybrid}
	for _, mode := range modes {
		if !cmd.SupportsMode(mode) {
			t.Errorf("Command with empty SupportedModes should support %s", mode)
		}
	}
}

// TestCommand_SupportsMode_CLIOnly verifies that CLI-only commands work correctly.
func TestCommand_SupportsMode_CLIOnly(t *testing.T) {
	cmd := Command{
		Name:           "exit",
		Description:    "Exit the application",
		SupportedModes: []ClientMode{ClientModeCLI},
	}

	if !cmd.SupportsMode(ClientModeCLI) {
		t.Error("CLI-only command should support CLI mode")
	}
	if cmd.SupportsMode(ClientModeDaemon) {
		t.Error("CLI-only command should NOT support Daemon mode")
	}
	// Hybrid mode inherits from CLI, so CLI-only commands should work.
	if !cmd.SupportsMode(ClientModeHybrid) {
		t.Error("CLI-only command should support Hybrid mode (inherits from CLI)")
	}
}

// TestCommand_SupportsMode_DaemonOnly verifies that Daemon-only commands work correctly.
func TestCommand_SupportsMode_DaemonOnly(t *testing.T) {
	cmd := Command{
		Name:           "approve",
		Description:    "Approve a pending prompt",
		SupportedModes: []ClientMode{ClientModeDaemon},
	}

	if cmd.SupportsMode(ClientModeCLI) {
		t.Error("Daemon-only command should NOT support CLI mode")
	}
	if !cmd.SupportsMode(ClientModeDaemon) {
		t.Error("Daemon-only command should support Daemon mode")
	}
	// Hybrid mode inherits from Daemon, so Daemon-only commands should work.
	if !cmd.SupportsMode(ClientModeHybrid) {
		t.Error("Daemon-only command should support Hybrid mode (inherits from Daemon)")
	}
}

// TestCommand_SupportsMode_BothModes verifies that commands supporting both modes work correctly.
func TestCommand_SupportsMode_BothModes(t *testing.T) {
	cmd := Command{
		Name:           "help",
		Description:    "Show available commands",
		SupportedModes: []ClientMode{ClientModeCLI, ClientModeDaemon},
	}

	if !cmd.SupportsMode(ClientModeCLI) {
		t.Error("Dual-mode command should support CLI mode")
	}
	if !cmd.SupportsMode(ClientModeDaemon) {
		t.Error("Dual-mode command should support Daemon mode")
	}
	if !cmd.SupportsMode(ClientModeHybrid) {
		t.Error("Dual-mode command should support Hybrid mode")
	}
}

// TestCommand_SupportsMode_Hybrid_InheritsCLI verifies that hybrid mode inherits CLI availability.
func TestCommand_SupportsMode_Hybrid_InheritsCLI(t *testing.T) {
	// Command only available in CLI mode.
	cmd := Command{
		Name:           "clear",
		Description:    "Clear the screen",
		SupportedModes: []ClientMode{ClientModeCLI},
	}

	// In hybrid mode, this should be available because TUI adapter supports it.
	if !cmd.SupportsMode(ClientModeHybrid) {
		t.Error("Hybrid mode should inherit availability from CLI mode")
	}
}

// TestCommand_SupportsMode_Hybrid_InheritsDaemon verifies that hybrid mode inherits Daemon availability.
func TestCommand_SupportsMode_Hybrid_InheritsDaemon(t *testing.T) {
	// Command only available in Daemon mode.
	cmd := Command{
		Name:           "status",
		Description:    "Show daemon status",
		SupportedModes: []ClientMode{ClientModeDaemon},
	}

	// In hybrid mode, this should be available because CUI adapter supports it.
	if !cmd.SupportsMode(ClientModeHybrid) {
		t.Error("Hybrid mode should inherit availability from Daemon mode")
	}
}
