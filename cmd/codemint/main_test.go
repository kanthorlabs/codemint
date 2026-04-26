package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"codemint.kanthorlabs.com/internal/db"
	"codemint.kanthorlabs.com/internal/orchestrator"
	"codemint.kanthorlabs.com/internal/registry"
	"codemint.kanthorlabs.com/internal/repl"
	"codemint.kanthorlabs.com/internal/repository/sqlite"
	"codemint.kanthorlabs.com/internal/ui"
)

// TestInvalidModeFlag verifies that invalid mode returns an error.
func TestInvalidModeFlag(t *testing.T) {
	_, err := parseClientMode("invalid")
	if err == nil {
		t.Error("expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "invalid mode") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestValidModeFlags verifies that valid modes are accepted.
func TestValidModeFlags(t *testing.T) {
	tests := []struct {
		mode     string
		expected registry.ClientMode
	}{
		{"cli", registry.ClientModeCLI},
		{"daemon", registry.ClientModeDaemon},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			got, err := parseClientMode(tt.mode)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestREPLExitCommand verifies that /exit triggers graceful shutdown.
func TestREPLExitCommand(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()

	// Open database.
	dbConn, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer dbConn.Close()

	// Run migrations.
	if err := db.RunMigrations(dbConn); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Create repos and seed.
	agentRepo := sqlite.NewAgentRepo(dbConn)
	if err := agentRepo.EnsureSystemAgents(ctx); err != nil {
		t.Fatalf("failed to seed agents: %v", err)
	}

	// Create command registry.
	cmdRegistry := registry.NewCommandRegistry()
	if err := repl.RegisterCoreCommands(cmdRegistry); err != nil {
		t.Fatalf("failed to register commands: %v", err)
	}

	// Create UI mediator with buffer.
	var outBuf bytes.Buffer
	mediator := ui.NewUIMediator(&outBuf)

	// Create dispatcher.
	dispatcher := orchestrator.NewDispatcher(cmdRegistry, mediator, nil)

	// Create active session.
	activeSession := &orchestrator.ActiveSession{
		ClientMode: registry.ClientModeCLI,
		IsGlobal:   true,
	}

	// Create dispatcher wrapper.
	wrapper := &dispatcherWrapper{
		dispatcher: dispatcher,
		session:    activeSession,
	}

	// Simulate input: /exit command.
	input := strings.NewReader("/exit\n")
	var errBuf bytes.Buffer

	err = repl.Loop(ctx, wrapper, input, &errBuf)
	if err == nil {
		t.Fatal("expected error from /exit command")
	}
	if err != registry.ErrShutdownGracefully {
		t.Errorf("expected ErrShutdownGracefully, got: %v", err)
	}

	// Verify output contains goodbye message.
	output := outBuf.String()
	if !strings.Contains(output, "Goodbye") {
		t.Errorf("expected goodbye message in output, got: %s", output)
	}
}

// TestREPLHelpCommand verifies that /help displays command list.
func TestREPLHelpCommand(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()

	// Open database.
	dbConn, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer dbConn.Close()

	// Run migrations.
	if err := db.RunMigrations(dbConn); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Create repos and seed.
	agentRepo := sqlite.NewAgentRepo(dbConn)
	if err := agentRepo.EnsureSystemAgents(ctx); err != nil {
		t.Fatalf("failed to seed agents: %v", err)
	}

	// Create command registry.
	cmdRegistry := registry.NewCommandRegistry()
	if err := repl.RegisterCoreCommands(cmdRegistry); err != nil {
		t.Fatalf("failed to register commands: %v", err)
	}

	// Create UI mediator with buffer.
	var outBuf bytes.Buffer
	mediator := ui.NewUIMediator(&outBuf)

	// Create dispatcher.
	dispatcher := orchestrator.NewDispatcher(cmdRegistry, mediator, nil)

	// Create active session.
	activeSession := &orchestrator.ActiveSession{
		ClientMode: registry.ClientModeCLI,
		IsGlobal:   true,
	}

	// Create dispatcher wrapper.
	wrapper := &dispatcherWrapper{
		dispatcher: dispatcher,
		session:    activeSession,
	}

	// Simulate input: /help then EOF (no /exit).
	input := strings.NewReader("/help\n")
	var errBuf bytes.Buffer

	err = repl.Loop(ctx, wrapper, input, &errBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify output contains help text.
	output := outBuf.String()
	if !strings.Contains(output, "Available commands") {
		t.Errorf("expected help text in output, got: %s", output)
	}
	if !strings.Contains(output, "/help") {
		t.Errorf("expected /help in command list, got: %s", output)
	}
	if !strings.Contains(output, "/exit") {
		t.Errorf("expected /exit in command list, got: %s", output)
	}
}

// TestREPLContextCancellation verifies that context cancellation exits the loop.
// Note: This test demonstrates that the REPL checks for context cancellation
// at the start of each iteration, not during blocking reads.
func TestREPLContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Open database.
	dbConn, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer dbConn.Close()

	// Run migrations.
	if err := db.RunMigrations(dbConn); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Create repos and seed.
	ctx := context.Background()
	agentRepo := sqlite.NewAgentRepo(dbConn)
	if err := agentRepo.EnsureSystemAgents(ctx); err != nil {
		t.Fatalf("failed to seed agents: %v", err)
	}

	// Create command registry.
	cmdRegistry := registry.NewCommandRegistry()
	if err := repl.RegisterCoreCommands(cmdRegistry); err != nil {
		t.Fatalf("failed to register commands: %v", err)
	}

	// Create UI mediator.
	var outBuf bytes.Buffer
	mediator := ui.NewUIMediator(&outBuf)

	// Create dispatcher.
	dispatcher := orchestrator.NewDispatcher(cmdRegistry, mediator, nil)

	// Create active session.
	activeSession := &orchestrator.ActiveSession{
		ClientMode: registry.ClientModeCLI,
		IsGlobal:   true,
	}

	// Create dispatcher wrapper.
	wrapper := &dispatcherWrapper{
		dispatcher: dispatcher,
		session:    activeSession,
	}

	// Create a pre-cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Run the loop with a reader that has no data.
	// The loop should detect context cancellation before trying to read.
	var errBuf bytes.Buffer
	err = repl.Loop(ctx, wrapper, strings.NewReader(""), &errBuf)
	
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// TestDatabaseBusyTimeout verifies that the database is configured with busy timeout.
func TestDatabaseBusyTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	dbConn, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer dbConn.Close()

	// Query the busy_timeout pragma.
	var busyTimeout int
	err = dbConn.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout)
	if err != nil {
		t.Fatalf("failed to query busy_timeout: %v", err)
	}

	// Should be 5000ms as configured.
	if busyTimeout != 5000 {
		t.Errorf("expected busy_timeout=5000, got %d", busyTimeout)
	}
}

// TestDatabaseJournalMode verifies that the database uses WAL mode.
func TestDatabaseJournalMode(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	dbConn, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer dbConn.Close()

	// Query the journal_mode pragma.
	var journalMode string
	err = dbConn.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}

	// Should be WAL as configured.
	if strings.ToLower(journalMode) != "wal" {
		t.Errorf("expected journal_mode=wal, got %s", journalMode)
	}
}

// TestDatabaseForeignKeys verifies that foreign key constraints are enabled.
func TestDatabaseForeignKeys(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	dbConn, err := db.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer dbConn.Close()

	// Query the foreign_keys pragma.
	var foreignKeys int
	err = dbConn.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys)
	if err != nil {
		t.Fatalf("failed to query foreign_keys: %v", err)
	}

	// Should be 1 (enabled) as configured.
	if foreignKeys != 1 {
		t.Errorf("expected foreign_keys=1, got %d", foreignKeys)
	}
}
